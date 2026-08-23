package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

	habitsRoot := os.Getenv("HABITS_ROOT")
	if habitsRoot == "" {
		log.Fatal("HABITS_ROOT is required")
	}

	syncRoot := os.Getenv("SYNC_ROOT")
	if syncRoot == "" {
		log.Fatal("SYNC_ROOT is required")
	}

	voicesRoot := os.Getenv("VOICES_ROOT")
	if voicesRoot == "" {
		voicesRoot = filepath.Join(syncRoot, "Voices")
	}

	sttBin := os.Getenv("STT_BIN")
	if sttBin == "" {
		sttBin = "whisper-cli"
	}
	sttModel := os.Getenv("STT_MODEL")
	sttLang := os.Getenv("STT_LANGUAGE")
	if sttLang == "" {
		sttLang = "auto"
	}
	ffmpegBin := os.Getenv("FFMPEG_BIN")
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}

	llmAPIKey := os.Getenv("LLM_API_KEY")
	llmBaseURL := os.Getenv("LLM_BASE_URL")
	if llmBaseURL == "" {
		llmBaseURL = "https://api.openai.com/v1"
	}
	llmModel := os.Getenv("LLM_MODEL")
	if llmModel == "" {
		llmModel = "gpt-4o-mini"
	}

	githubToken := os.Getenv("GSNOTE_GITHUB_TOKEN")
	gitAuthorName := os.Getenv("GSNOTE_GIT_AUTHOR_NAME")
	gitAuthorEmail := os.Getenv("GSNOTE_GIT_AUTHOR_EMAIL")

	tz := os.Getenv("TIMEZONE")
	if tz == "" {
		tz = "Asia/Jakarta"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Fatalf("invalid TIMEZONE %q: %v", tz, err)
	}
	time.Local = loc
	log.Printf("timezone=%s", tz)

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

	if err := os.MkdirAll(voicesRoot, 0755); err != nil {
		log.Fatalf("create voices root: %v", err)
	}

	log.Printf("config habits_root=%s sync_root=%s voices_root=%s whitelist_telegram_id=%s", habitsRoot, syncRoot, voicesRoot, whitelistTelegramIDStr)

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

	h := handler.New(bot, habitsRoot, syncRoot, githubToken, gitAuthorName, gitAuthorEmail, whitelistTelegramIDMap)

	if llmAPIKey != "" && sttModel != "" {
		vp := voice.NewProcessor(bot, sttBin, sttModel, sttLang, llmAPIKey, llmBaseURL, llmModel, voicesRoot, syncRoot)
		h.StartVoiceProcessor(vp)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	for update := range updates {
		h.Handle(update)
	}
}
