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

	elevenAPIKey := os.Getenv("ELEVEN_API_KEY")
	elevenModel := os.Getenv("ELEVEN_MODEL")
	elevenLanguage := os.Getenv("ELEVEN_LANGUAGE")

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

	log.Printf("config gsnote_root=%s eleven_model=%s eleven_language=%s", root, elevenModel, elevenLanguage)

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	log.Printf("authorized as @%s\n", bot.Self.UserName)

	h := handler.New(bot, whitelistTelegramIDMap)

	if elevenAPIKey != "" {
		vp := voice.NewProcessor(bot, elevenAPIKey, elevenModel, elevenLanguage, root)
		h.StartVoiceProcessor(vp)
	} else {
		log.Printf("ELEVEN_API_KEY not set: voice capture disabled, /help only")
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		h.Handle(update)
	}
}
