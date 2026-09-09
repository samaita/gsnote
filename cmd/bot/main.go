package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"github.com/axonigma/gsnote/internal/handler"
	"github.com/axonigma/gsnote/internal/jobs"
	"github.com/axonigma/gsnote/internal/transcription"
	"github.com/axonigma/gsnote/internal/voice"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	log.Printf("gsnote version=%s", version)

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("get home dir: %v", err)
	}

	configDir := filepath.Join(home, ".config", "gsnote")
	if configDir == home {
		log.Fatal("invalid config dir: equals home")
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("create config dir: %v", err)
	}

	xdgConfig := filepath.Join(configDir, ".env")
	localConfig := ".env"

	loadedConfig := ""
	if err := godotenv.Load(xdgConfig); err != nil {
		if err2 := godotenv.Load(localConfig); err2 != nil {
			log.Printf("missing config: ~/.config/gsnote/.env")
		} else {
			loadedConfig = localConfig
		}
	} else {
		loadedConfig = xdgConfig
	}
	log.Printf("config file=%s", loadedConfig)

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	root := os.Getenv("DATA_DIR")
	if root == "" {
		root = os.Getenv("GSNOTE_ROOT")
	}
	if root == "" {
		root = "/data"
	}

	if err := os.MkdirAll(root, 0755); err != nil {
		log.Fatalf("create gsnote root: %v", err)
	}

	whitelistTelegramIDMap := make(map[int64]bool)
	whitelistTelegramIDStr := os.Getenv("WHITELIST_TELEGRAM_ID")
	if whitelistTelegramIDStr != "" {
		for _, part := range strings.Split(whitelistTelegramIDStr, ",") {
			res, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil {
				whitelistTelegramIDMap[res] = true
			}
		}
	}

	log.Printf("config data_dir=%s", root)

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	log.Printf("authorized as @%s\n", bot.Self.UserName)

	h := handler.New(bot, whitelistTelegramIDMap)
	vp, err := voice.NewAsyncProcessor(bot, root)
	if err != nil {
		log.Fatalf("init voice pipeline: %v", err)
	}
	h.StartVoiceProcessor(vp)
	threads, _ := strconv.Atoi(os.Getenv("TRANSCRIBER_THREADS"))
	worker := &jobs.Worker{Repo: vp.Repository(), Transcriber: transcription.Whisper{Binary: os.Getenv("TRANSCRIBER_BINARY"), Model: os.Getenv("TRANSCRIBER_MODEL"), Language: os.Getenv("TRANSCRIBER_LANGUAGE"), Threads: threads}, Notifier: vp}
	stop := make(chan struct{})
	go worker.Run(stop)
	defer close(stop)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		h.Handle(update)
	}
}
