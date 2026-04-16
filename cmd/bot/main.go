package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"github.com/axonigma/gsnote/internal/handler"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from environment")
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
