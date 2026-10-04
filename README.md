# gsnote

A voice-only Telegram note bot. A voice message is saved as durable audio in
`GSNOTE_ROOT/Inbox/Voices`, queued in SQLite, and immediately acknowledged with
its VN number. A separate worker later transcribes in English and writes a
verbatim Markdown note beneath `GSNOTE_ROOT/Inbox/Texts`, then replies to the
original Telegram message. Audio is retained for retry.

## Requirements

- Go 1.25.7+ to build from source.
- Telegram bot token and allowed Telegram user IDs.
- Bot process: Telegram network access and writable `GSNOTE_ROOT`.
- Worker: `whisper-cli`, `ffmpeg`, and a whisper.cpp GGML model.

Config is loaded from `~/.config/gsnote/.env`, falling back to `.env` in the
working directory. Start the bot with `make dev`, or build using `make build`.
Run unit tests with `go test ./...`.

| Variable | Required | Description |
|---|---:|---|
| `TELEGRAM_BOT_TOKEN` | bot | Telegram bot token |
| `WHITELIST_TELEGRAM_ID` | bot | Comma-separated allowed Telegram IDs |
| `GSNOTE_ROOT` | yes | Persistent data root; preserve the existing data path, since this new version expects the audio/text subfolders and an SQLite queue below it. |
| `TRANSCRIBER_BINARY` | worker | CLI name/path; default `whisper-cli` |
| `TRANSCRIBER_MODEL` | worker | Local GGML model path |
| `TRANSCRIBER_THREADS` | no | Positive worker thread count |
| `TRANSCRIBER_LANGUAGE` | no | Language code; worker defaults to `en` |
| `TRANSCRIBE_MAX_ATTEMPTS` | worker | Positive integer; default `5` |

## Worker and retry behavior

Run `gsnote` for Telegram polling and `gsnote worker` as a separate long-running process using the same config and SQLite database. The bot only downloads, saves and enqueues audio; it does not need Whisper, so queued captures remain durable while the worker is offline. Worker shutdown on SIGINT/SIGTERM preserves unclaimed queue rows; stale in-progress claims are recovered on restart. Keep both processes supervised (for example, separate systemd services or named screen sessions). Failures retry at 30s, 60s, 120s and exponentially up to one hour; the default terminal limit is five attempts. Set `TRANSCRIBE_MAX_ATTEMPTS` to a positive integer to override it. Only terminal failures trigger failure notification. A successful worker writes a dated Markdown file linked to the retained OGG, marks the database row DONE, and replies to the original Telegram message. `make air` is for bot development only.
