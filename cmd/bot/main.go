package main

import (
	"log"
	"os"

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

	if err := os.MkdirAll(habitsRoot, 0755); err != nil {
		log.Fatalf("create habits root: %v", err)
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("init bot: %v", err)
	}

	log.Printf("authorized as @%s", bot.Self.UserName)

	h := handler.New(bot, habitsRoot)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		h.Handle(update)
	}
}
