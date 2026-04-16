package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"github.com/axonigma/gsnote/internal/handler"
)

func main() {
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

	if err := godotenv.Load(xdgConfig); err != nil {
		if err2 := godotenv.Load(localConfig); err2 != nil {
			log.Printf("Missing config: ~/.config/gsnote/.env")
		}
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	habitsRoot := os.Getenv("HABITS_ROOT")
	if habitsRoot == "" {
		log.Fatal("HABITS_ROOT is required")
	}

	whitelistTelegramIDMap := make(map[int64]bool)
	whitelistTelegramIDStr := os.Getenv("WHITELIST_TELEGRAM_ID")
	if whitelistTelegramIDStr != "" {
		for i := range strings.Split(whitelistTelegramIDStr, ",") {
			res, err := strconv.ParseInt(strings.Split(whitelistTelegramIDStr, ",")[i], 10, 64)
			if err == nil {
				whitelistTelegramIDMap[res] = true
			}
		}
	}

	if err := os.MkdirAll(habitsRoot, 0755); err != nil {
		log.Fatalf("create habits root: %v", err)
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	log.Printf("authorized as @%s\n", bot.Self.UserName)

	var whitelistedTelegramIDs []string
	for i := range whitelistTelegramIDMap {
		whitelistedTelegramIDs = append(whitelistedTelegramIDs, fmt.Sprintf("%d", i))
	}
	log.Printf("allowed for %s\n", strings.Join(whitelistedTelegramIDs, ","))

	h := handler.New(bot, habitsRoot, whitelistTelegramIDMap)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		h.Handle(update)
	}
}
