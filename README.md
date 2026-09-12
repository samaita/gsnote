# gsnote

A voice-only Telegram note bot. Send a voice message, get your words back as a
markdown note saved next to the original audio. Nothing else.

## Description & Purpose

gsnote turns Telegram voice messages into durable, plain-text notes on your own
disk. You record a thought on your phone; the bot downloads the audio, saves it,
transcribes it with ElevenLabs Scribe, and writes a markdown note with the
verbatim transcript. No app to open, no cloud notebook, no lock-in — the files
are yours.

There is exactly one input: a Telegram voice message. There is exactly one
command: `/help`. Everything else is voice.

Every capture lands in one folder (`GSNOTE_ROOT`):

```text
voice message -> raw audio saved -> ElevenLabs Scribe transcription -> transcript note
```

```text
00001-20260909153200.ogg   the original audio, saved before anything else runs
00001-20260909.md          frontmatter (id, date, source, audio) + verbatim transcript
_counter.txt               the sequential ID counter
```

The audio is written to disk **before** transcription runs, so a failed
transcription never loses the recording — the audio stays put for retry.

## Requirements

- **Go 1.25.7+** — to build from source (`go.mod` declares `go 1.25.7`).
- **Telegram bot token** — create one with [@BotFather](https://t.me/BotFather).
- **Telegram user ID** — get yours from [@userinfobot](https://t.me/userinfobot);
  only whitelisted IDs are served.
- **ElevenLabs API key** (`xi-...`) — for speech-to-text via
  [Scribe](https://elevenlabs.io/). Without it the bot still runs but voice
  capture is disabled and only `/help` works.
- **A folder for your notes** — the single `GSNOTE_ROOT` holding audio, notes,
  and the counter.

No other dependencies: no ffmpeg, no whisper.cpp, no LLM keys. Telegram OGG/Opus
voice notes are uploaded to ElevenLabs as-is.

## Running Dev

Config is read from `~/.config/gsnote/.env` first, then a local `.env` in the
working directory. Copy the example and fill it in:

```bash
cp .env.example .env
```

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | Bot token from [@BotFather](https://t.me/BotFather) |
| `WHITELIST_TELEGRAM_ID` | Yes | Your Telegram ID from [@userinfobot](https://t.me/userinfobot), comma-separated for multiple |
| `GSNOTE_ROOT` | Yes | Single folder for audio, notes, and the counter |
| `ELEVEN_API_KEY` | For voice | ElevenLabs API key (`xi-...`) |
| `ELEVEN_MODEL` | No | Default `scribe_v1` |
| `ELEVEN_LANGUAGE` | No | ISO-639-1 like `id` or `en`. Empty = auto-detect |

Then:

```bash
make dev     # go run ./cmd/bot
make build   # go build -o gsnote ./cmd/bot
make test    # go test ./...
```

`make dev` runs the bot in the foreground and logs to stderr. Send a voice
message to your bot from a whitelisted account to test a capture end to end.

Before committing, run the same checks CI expects:

```bash
bash -n install.sh uninstall.sh
go test ./...
go build ./...
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/install.sh | bash
```

The script will:

- Prompt for your Telegram bot token, Telegram ID, notes folder, and ElevenLabs API key
- Download the latest release binary to `~/.local/bin/gsnote`
- Write config to `~/.config/gsnote/.env`
- Optionally set up a systemd user service

## Upgrade

Run the same install script — it detects the installed version and upgrades only if a newer release is available:

```bash
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/install.sh | bash
```

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/uninstall.sh | bash
```

## Commands

One command exists: `/help`. Everything else is a voice message.

## License

MIT
