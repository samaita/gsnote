package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"github.com/axonigma/gsnote/internal/handler"
	"github.com/axonigma/gsnote/internal/jobs"
	"github.com/axonigma/gsnote/internal/transcription"
	"github.com/axonigma/gsnote/internal/voice"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	worker := false
	if len(args) > 0 && args[0] == "worker" {
		worker = true
		args = args[1:]
	}
	fs := flag.NewFlagSet("gsnote", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	show := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *show {
		fmt.Println(version)
		return nil
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unknown arguments: %s", strings.Join(fs.Args(), " "))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config := filepath.Join(home, ".config", "gsnote", ".env")
	if _, err = os.Stat(config); os.IsNotExist(err) {
		config = ".env"
	}
	if err = godotenv.Load(config); err != nil && config != ".env" {
		if localErr := godotenv.Load(".env"); localErr != nil {
			return fmt.Errorf("load config %s or .env: %w", config, err)
		}
	} else if err != nil {
		return fmt.Errorf("load config %s: %w", config, err)
	}
	root := os.Getenv("GSNOTE_ROOT")
	if root == "" {
		return fmt.Errorf("GSNOTE_ROOT is required")
	}
	if err = os.MkdirAll(root, 0755); err != nil {
		return err
	}
	repo, err := jobs.Open(filepath.Join(root, "gsnote.db"))
	if err != nil {
		return err
	}
	defer repo.Close()
	if worker {
		return runWorker(repo, root, ctx, nil)
	}
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return err
	}
	allowed := map[int64]bool{}
	for _, v := range strings.Split(os.Getenv("WHITELIST_TELEGRAM_ID"), ",") {
		if id, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64); e == nil {
			allowed[id] = true
		}
	}
	h := handler.New(bot, allowed)
	vp, err := voice.NewAsyncProcessor(bot, root, repo)
	if err != nil {
		return err
	}
	h.StartVoiceProcessor(vp)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	wctx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	workerStarted := make(chan error, 1)
	workerDone := make(chan error, 1)
	go func() { workerDone <- runWorker(repo, root, wctx, workerStarted) }()
	if err := <-workerStarted; err != nil {
		return fmt.Errorf("start transcription worker: %w", err)
	}
	updates := bot.GetUpdatesChan(u)
	for {
		select {
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			<-workerDone
			return nil
		case err := <-workerDone:
			if err != nil {
				return fmt.Errorf("transcription worker stopped: %w", err)
			}
			return fmt.Errorf("transcription worker stopped unexpectedly")
		case update, ok := <-updates:
			if !ok {
				cancelWorker()
				<-workerDone
				return nil
			}
			h.Handle(update)
		}
	}
}
func loadTranscriber() (transcription.Whisper, error) {
	binary := os.Getenv("TRANSCRIBER_BINARY")
	if binary == "" {
		binary = "whisper-cli"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return transcription.Whisper{}, err
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return transcription.Whisper{}, err
	}
	model := os.Getenv("TRANSCRIBER_MODEL")
	if model == "" {
		return transcription.Whisper{}, fmt.Errorf("TRANSCRIBER_MODEL is required")
	}
	if _, err := os.Stat(model); err != nil {
		return transcription.Whisper{}, err
	}
	threads := 0
	if raw := os.Getenv("TRANSCRIBER_THREADS"); raw != "" {
		var err error
		threads, err = strconv.Atoi(raw)
		if err != nil || threads < 1 {
			return transcription.Whisper{}, fmt.Errorf("TRANSCRIBER_THREADS must be positive")
		}
	}
	language := os.Getenv("TRANSCRIBER_LANGUAGE")
	if language == "" {
		language = "en"
	}
	return transcription.Whisper{Binary: binary, Model: model, Threads: threads, Language: language}, nil
}
func parseMaxAttempts(raw string) (int, error) {
	if raw == "" {
		return jobs.DefaultMaxAttempts, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("TRANSCRIBE_MAX_ATTEMPTS must be a positive integer")
	}
	return n, nil
}

func runWorker(repo *jobs.Repository, root string, ctx context.Context, started chan<- error) error {
	trans, err := loadTranscriber()
	if err != nil {
		if started != nil {
			started <- err
		}
		return err
	}
	var notify func(int64, int, string) error
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		bot, e := tgbotapi.NewBotAPI(token)
		if e != nil {
			log.Printf("worker notifier disabled: %v", e)
		} else {
			notify = func(chat int64, msg int, text string) error {
				m := tgbotapi.NewMessage(chat, text)
				if msg > 0 {
					m.ReplyToMessageID = msg
				}
				_, e := bot.Send(m)
				return e
			}
		}
	}
	maxAttempts, err := parseMaxAttempts(os.Getenv("TRANSCRIBE_MAX_ATTEMPTS"))
	if err != nil {
		return err
	}
	w := &voice.Worker{Repo: repo, Root: root, Transcriber: trans, Notify: notify, MaxAttempts: maxAttempts}
	if started != nil {
		started <- nil
	}
	return w.Run(ctx)
}
