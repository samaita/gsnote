package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"github.com/axonigma/gsnote/internal/handler"
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

	root := os.Getenv("GSNOTE_ROOT")
	if root == "" {
		log.Fatal("GSNOTE_ROOT is required")
	}

	if err := os.MkdirAll(root, 0755); err != nil {
		log.Fatalf("create gsnote root: %v", err)
	}

	transcriberBinary := os.Getenv("TRANSCRIBER_BINARY")
	if transcriberBinary == "" {
		transcriberBinary = "whisper-cli"
	}
	if _, err := exec.LookPath(transcriberBinary); err != nil {
		log.Fatalf("find whisper-cli (%s): %v", transcriberBinary, err)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Fatalf("find ffmpeg: %v", err)
	}

	transcriberModel := os.Getenv("TRANSCRIBER_MODEL")
	if transcriberModel == "" {
		log.Fatal("TRANSCRIBER_MODEL is required")
	}
	if _, err := os.Stat(transcriberModel); err != nil {
		log.Fatalf("read TRANSCRIBER_MODEL: %v", err)
	}
	transcriberLanguage := os.Getenv("TRANSCRIBER_LANGUAGE")
	transcriberThreads := 0
	if raw := os.Getenv("TRANSCRIBER_THREADS"); raw != "" {
		transcriberThreads, err = strconv.Atoi(raw)
		if err != nil || transcriberThreads < 1 {
			log.Fatalf("TRANSCRIBER_THREADS must be a positive integer, got %q", raw)
		}
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

	log.Printf("config gsnote_root=%s transcriber_binary=%s transcriber_model=%s transcriber_language=%s transcriber_threads=%d", root, transcriberBinary, transcriberModel, transcriberLanguage, transcriberThreads)

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	log.Printf("authorized as @%s\n", bot.Self.UserName)

	h := handler.New(bot, whitelistTelegramIDMap)

	transcriber := transcription.Whisper{
		Binary:   transcriberBinary,
		Model:    transcriberModel,
		Language: transcriberLanguage,
		Threads:  transcriberThreads,
	}
	vp := voice.NewProcessor(bot, transcriber, root)
	h.StartVoiceProcessor(vp)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		h.Handle(update)
	}
}
