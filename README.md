# gsnote

A voice-only Telegram note bot. Send a voice message, get your words back as a
markdown note saved next to the original audio. Nothing else.

## Description & Purpose

gsnote turns Telegram voice messages into durable, plain-text notes on your own
disk. You record a thought on your phone; the bot downloads the audio, saves it,
transcribes it locally with `whisper-cli`, and writes a markdown note with the
verbatim transcript. No app to open, no cloud notebook, no lock-in — the files
are yours.

There is exactly one input: a Telegram voice message. There is exactly one
command: `/help`. Everything else is voice.

Every capture lands in one folder (`GSNOTE_ROOT`):

```text
voice message -> raw audio saved -> local whisper-cli transcription -> transcript note
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
- **whisper.cpp** — install it so `whisper-cli` is available on `PATH`.
- **ffmpeg** — converts Telegram OGG/Opus audio to the 16 kHz mono WAV input
  used during transcription.
- **A whisper.cpp GGML model** — download the model size you want and configure
  its path with `TRANSCRIBER_MODEL`.
- **A folder for your notes** — the single `GSNOTE_ROOT` holding audio, notes,
  and the counter.

Transcription runs on the same machine as gsnote. The original Telegram audio
is retained; the converted WAV is temporary and removed after `whisper-cli`
finishes. No speech-to-text API key or LLM key is needed.

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
| `TRANSCRIBER_BINARY` | No | `whisper-cli` executable name or absolute path; default `whisper-cli` |
| `TRANSCRIBER_MODEL` | Yes | Path to a downloaded whisper.cpp GGML model |
| `TRANSCRIBER_THREADS` | No | Positive worker thread count; empty lets `whisper-cli` choose |
| `TRANSCRIBER_LANGUAGE` | No | ISO-639-1 code such as `id` or `en`; empty enables auto-detection |

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

- Prompt for your Telegram bot token, Telegram ID, notes folder, and local Whisper settings
- Download the latest release binary to `~/.local/bin/gsnote`
- Write config to `~/.config/gsnote/.env`
- Optionally set up a systemd user service

Install `whisper-cli` and `ffmpeg`, and download a model before running the
installer. The installer validates both executables; gsnote validates the model
path when it starts.

## Docker Compose

Docker Compose pulls the published `ghcr.io/samaita/gsnote:latest` image. The
notes directory uses a **bind mount**, not a Docker volume, so audio and markdown
files remain directly in a directory on the machine. You do not need to clone
the repository or build the image on the deployment machine.

Create a deployment directory and download only the Compose configuration:

```bash
mkdir -p gsnote-deploy
cd gsnote-deploy
curl -fsSLO https://raw.githubusercontent.com/samaita/gsnote/main/compose.yaml
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/.env.example -o .env
mkdir -p /home/user/samaita/workspace/obsidian-sync/Voice models
# Download or copy a GGML model to models/ggml-small-q5_1.bin.
```

Edit `.env`. In particular, `GSNOTE_ROOT` must be an absolute path on the host:

```dotenv
TELEGRAM_BOT_TOKEN=
WHITELIST_TELEGRAM_ID=
GSNOTE_ROOT=/home/user/samaita/workspace/obsidian-sync/Voice
TRANSCRIBER_BINARY=whisper-cli
TRANSCRIBER_MODEL=models/ggml-small-q5_1.bin
TRANSCRIBER_THREADS=2
TRANSCRIBER_LANGUAGE=id
GSNOTE_UID=1000
GSNOTE_GID=1000
```

On Linux, get the ownership values with `id -u` and `id -g`. Create
`GSNOTE_ROOT` before starting; Compose intentionally refuses to silently create
it as a root-owned directory.

Start and inspect the service:

```bash
docker-compose pull
docker-compose up -d
docker-compose logs -f gsnote
```

If GHCR reports `denied`, authenticate before pulling (the package owner must
also grant the account read access if the package is private):

```bash
echo "$GITHUB_TOKEN" | docker login ghcr.io -u YOUR_GITHUB_USERNAME --password-stdin
```

Compose uses the host value of `GSNOTE_ROOT` as the bind-mount source and sets
the application's container-side `GSNOTE_ROOT` to `/data`. Likewise, the host
model file is mounted read-only at `/model/model.bin`. No named Docker volumes
are used.

### Publishing the container image

The `Publish container image` GitHub Actions workflow builds `linux/amd64` and
`linux/arm64` images and pushes them to `ghcr.io/samaita/gsnote`. A push to
`main` publishes `latest`; a Git tag such as `v0.2.0` also publishes `0.2.0` and
`0.2`. The workflow can also be started manually from the Actions tab.

The workflow authenticates with GitHub's built-in `GITHUB_TOKEN`; no registry
secret is required. After the first successful run, open the package settings
at <https://github.com/samaita/gsnote/pkgs/container/gsnote> and change its
visibility to public if deployment machines should pull without logging in.

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
