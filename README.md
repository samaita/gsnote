# gsnote

A low-friction Telegram bot for capturing habits, voice notes, and notes in markdown files compatible with Obsidian.

## How it works

Send commands to your Telegram bot to log habits, capture voice notes, sync your vault, and schedule supported commands.

Run `/help` in Telegram to see the latest supported commands and usage. The command list changes over time, so the bot's built-in help is the source of truth.

One example command is:

```text
/habit <name> [value] [note]
```

| Argument | Required | Description |
|----------|----------|-------------|
| `name`   | Yes      | Single word, habit identifier |
| `value`  | No       | Number (integer or decimal), for count-based habits |
| `note`   | No       | Free text appended after the value |

### Examples

```
/habit work
/habit work deep focus session
/habit pushup 20
/habit pushup 20 after dinner
/habit run 2.5 morning jog
```

### Output format

Each entry is appended to `<HABITS_ROOT>/<habit>.md`:

```markdown
# Habit: pushup

## 2026-04

- 2026-04-16 20:30 | 20 | after dinner
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/install.sh | bash
```

The script will:
- Prompt for your Telegram bot token, vault folders, and Telegram ID
- Download the latest release binary to `~/.local/bin/gsnote`
- Write config to `~/.config/gsnote/.env`
- Optionally set up a systemd user service

> Get your bot token from [@BotFather](https://t.me/BotFather) and your Telegram ID from [@userinfobot](https://t.me/userinfobot).

## Upgrade

Run the same install script — it detects the installed version and upgrades only if a newer release is available:

```bash
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/install.sh | bash
```

Existing config at `~/.config/gsnote/.env` is never overwritten.

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/samaita/gsnote/main/uninstall.sh | bash
```

The script will:
- Stop and disable the systemd service (if active)
- Remove the binary at `~/.local/bin/gsnote`
- Prompt before removing config at `~/.config/gsnote/`

## Setup (manual)

### 1. Clone and build

```bash
git clone https://github.com/samaita/gsnote.git
cd gsnote
make build
```

### 2. Configure environment

Create `~/.config/gsnote/.env`:

```env
TELEGRAM_BOT_TOKEN=your_bot_token_here
GSNOTE_GITHUB_TOKEN=your_github_token_here
GSNOTE_GIT_AUTHOR_NAME=your_name
GSNOTE_GIT_AUTHOR_EMAIL=your_email@example.com
SYNC_ROOT=/home/youruser/vault
HABITS_ROOT=/home/youruser/vault/Habits
VOICES_ROOT=/home/youruser/vault/Voices
STT_BIN=whisper-cli
STT_MODEL=/home/youruser/models/ggml-medium.bin
STT_LANGUAGE=auto
FFMPEG_BIN=ffmpeg
LLM_API_KEY=your_llm_api_key
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=gpt-4o-mini
WHITELIST_TELEGRAM_ID=your_telegram_id
TIMEZONE=Asia/Jakarta
```

- `TELEGRAM_BOT_TOKEN` — obtain from [@BotFather](https://t.me/BotFather)
- `GSNOTE_GITHUB_TOKEN` — GitHub personal access token used by `/sync` for HTTPS remotes
- `GSNOTE_GIT_AUTHOR_NAME` — commit author name used by `/sync`
- `GSNOTE_GIT_AUTHOR_EMAIL` — commit author email used by `/sync`
- `SYNC_ROOT` — root of the git repository to sync (used by `/sync`)
- `HABITS_ROOT` — subdirectory where habit markdown files will be written (created automatically)
- `VOICES_ROOT` — subdirectory where voice captures are stored (defaults to `<SYNC_ROOT>/Voices`)
- `STT_BIN` — path to the local `whisper-cli` binary from [whisper.cpp](https://github.com/ggerganov/whisper.cpp) (defaults to `whisper-cli` on `PATH`)
- `STT_MODEL` — path to a local ggml whisper model file, e.g. `ggml-small.bin` or `ggml-medium.bin` for Indonesian/English speech (required for voice notes; downloads from `https://huggingface.co/ggerganov/whisper.cpp`)
- `STT_LANGUAGE` — whisper language code or `auto` (defaults to `auto`)
- `FFMPEG_BIN` — path to `ffmpeg`, used to convert Telegram's OGG voice notes to 16kHz WAV before transcription (defaults to `ffmpeg` on `PATH`)
- `LLM_API_KEY` — API key for the LLM used to process transcripts; required to enable voice notes (OpenAI-compatible)
- `LLM_BASE_URL` — base URL for the LLM API (defaults to `https://api.openai.com/v1`)
- `LLM_MODEL` — LLM model name (defaults to `gpt-4o-mini`)
- `WHITELIST_TELEGRAM_ID` — your numeric Telegram ID from [@userinfobot](https://t.me/userinfobot)
- `TIMEZONE` — IANA timezone name used for timestamps (e.g. `Asia/Jakarta`, `UTC`, `America/New_York`)

### 3. Run

```bash
./gsnote
```

## Run as a systemd user service

The install script sets this up automatically. To do it manually:

```bash
systemctl --user enable --now gsnote
systemctl --user status gsnote
journalctl --user -u gsnote -f
```

## Bot commands

Run `/help` in Telegram for the current command list and usage details.

| Command | Description |
|---------|-------------|
| `/habit <name> [value] [note]` | Log a habit entry |
| `/voice` | Voice notes — send a Telegram voice message to capture; `/voice list`, `/voice delete <id>` |
| `/sync` | Sync `SYNC_ROOT` to origin main using embedded go-git flow |
| `/help` | Show usage |

### Voice notes

Voice notes let you capture a thought by sending a Telegram voice message. Transcription runs fully **locally** with [whisper.cpp](https://github.com/ggerganov/whisper.cpp); audio never leaves your machine. Voice capture requires `STT_MODEL` and `LLM_API_KEY` to be set.

Local STT setup:

```bash
# whisper-cli from whisper.cpp (build from source)
git clone https://github.com/ggerganov/whisper.cpp && cd whisper.cpp
make
# add ./build/bin (or ./bin) to PATH, or set STT_BIN to the binary path

# download a model with good Indonesian/English support
wget -O ~/models/ggml-medium.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin

# ffmpeg for converting OGG voice notes to WAV
brew install ffmpeg
```

Set `STT_MODEL=/home/youruser/models/ggml-medium.bin` in `~/.config/gsnote/.env`.

The flow is:

```text
Telegram voice → download audio → ffmpeg → whisper-cli (local) → LLM summary + classification → saved in VOICES_ROOT
```

The original audio, a raw transcript, and a processed markdown note are saved in `VOICES_ROOT` (default `<SYNC_ROOT>/Voices`) with a shared sequential voice ID:

```text
Voices/00001-20260823-xxxx.ogg
Voices/00001.md
Voices/00001-20260823.md
```

The bot replies with the generated voice ID and a title:

```text
Saved 00001

Address Quality sebagai API Audit
```

Manage captures with:

- `/voice list` — list recent voice captures
- `/voice delete 00001` — delete the audio, transcript, and markdown for a capture
- `/voice help` — show voice usage

If STT or LLM processing fails, the original audio is kept so it can be retried later. Duplicate Telegram deliveries do not create duplicate captures.

## Feature update checklist

When adding or changing a feature, update every setup surface that users may copy or run:

- `.env.example` for new or changed environment variables
- `README.md` manual setup examples and command documentation
- `install.sh` prompts, generated `.env` output, created folders, and existing-config migration
- `gsnote.service.example` when service execution, environment loading, working directory, or systemd settings change

Run `bash -n install.sh uninstall.sh`, `go test ./...`, and `go build ./...` before committing the update.

## Requirements

- Go 1.21+
- A Telegram bot token
- GitHub credentials for `/sync`:
- `GSNOTE_GITHUB_TOKEN` with access to the remote repository
- Git author identity via `GSNOTE_GIT_AUTHOR_NAME` and `GSNOTE_GIT_AUTHOR_EMAIL`

For voice notes (optional, only if you use them):

- `ffmpeg` on `PATH` (or `FFMPEG_BIN`) to convert Telegram's OGG voice notes to WAV
- A local whisper.cpp `whisper-cli` binary and a ggml model — see [Voice notes](#voice-notes) for setup
- An OpenAI-compatible `LLM_API_KEY`
