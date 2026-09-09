# gsnote

A voice-only Telegram note bot. Send a voice message, get your words back as a markdown note next to the original audio. Nothing else.

## How it works

There is exactly one input: a Telegram voice message.

```text
voice message -> raw audio saved -> local whisper.cpp transcription -> transcript note
```

Every capture lands in one folder (`GSNOTE_ROOT`):

```text
00001-20260909153200.ogg   the original audio, saved before anything else runs
00001-20260909.md          frontmatter (id, date, source, audio) + verbatim transcript
_counter.txt               the sequential ID counter
```

If transcription fails, the audio is still on disk — nothing is lost.

## Commands

One command exists: `/help`. Everything else is a voice message.

## Configuration

Config is read from `~/.config/gsnote/.env` (or a local `.env`):

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | Bot token from [@BotFather](https://t.me/BotFather) |
| `WHITELIST_TELEGRAM_ID` | Yes | Your Telegram ID from [@userinfobot](https://t.me/userinfobot), comma-separated for multiple |
| `GSNOTE_ROOT` | Yes | Single folder for audio, notes, and the counter |
| `TRANSCRIBER_BINARY` | No | whisper.cpp executable, default `whisper-cli` |
| `TRANSCRIBER_MODEL` | Yes | whisper.cpp model path, default `/models/ggml-small-q5_1.bin` |
| `TRANSCRIBER_THREADS` | No | CPU thread count |
| `TRANSCRIBER_LANGUAGE` | No | ISO-639-1 like `id` or `en` |

Install `whisper.cpp` and provide `models/ggml-small-q5_1.bin`. No API key or paid STT service is used.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/install.sh | bash
```

The script will:
- Prompt for your Telegram bot token, Telegram ID, and notes folder
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

## Development

```bash
make dev     # go run ./cmd/bot
make build   # go build -o gsnote ./cmd/bot
make test    # go test ./...
```

Run `bash -n install.sh uninstall.sh`, `go test ./...`, and `go build ./...` before committing changes.

## License

MIT
