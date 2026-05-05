# gsnote

A low-friction Telegram bot for capturing habits, tasks, ideas, and notes in markdown files compatible with Obsidian.

## How it works

Send commands to your Telegram bot to log habits, manage tasks, capture ideas, save notes, sync your vault, and schedule supported commands.

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
- Prompt for your Telegram bot token, habits folder, and Telegram ID
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
TASKS_ROOT=/home/youruser/vault/Tasks
NOTES_ROOT=/home/youruser/vault/Notes
CRON_ROOT=/home/youruser/vault/CRON
WHITELIST_TELEGRAM_ID=your_telegram_id
TIMEZONE=Asia/Jakarta
```

- `TELEGRAM_BOT_TOKEN` — obtain from [@BotFather](https://t.me/BotFather)
- `GSNOTE_GITHUB_TOKEN` — GitHub personal access token used by `/sync` for HTTPS remotes
- `GSNOTE_GIT_AUTHOR_NAME` — commit author name used by `/sync`
- `GSNOTE_GIT_AUTHOR_EMAIL` — commit author email used by `/sync`
- `SYNC_ROOT` — root of the git repository to sync (used by `/sync`)
- `HABITS_ROOT` — subdirectory where habit markdown files will be written (created automatically)
- `TASKS_ROOT` — subdirectory where task markdown files will be written (created automatically)
- `NOTES_ROOT` — subdirectory where note markdown files will be written (created automatically)
- `CRON_ROOT` — directory where scheduled command definitions are stored
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
| `/task ...` | Manage daily tasks |
| `/idea <type> <title>` | Capture an idea |
| `/note <link> <desc>` | Save a link with your take |
| `/cron ...` | Schedule `/task view` or `/sync` |
| `/sync` | Sync `SYNC_ROOT` to origin main using embedded go-git flow |
| `/help` | Show usage |

### Cron schedules

`/cron` currently supports:

- `/task view`
- `/sync`

Accepted schedule formats:

- `HH:MM` for a daily recurring run, for example `/cron 06:00 /task view`
- 5-field cron syntax, for example `/cron */5 * * * * /sync`

Manage entries with:

- `/cron view`
- `/cron edit N <spec> <command>`
- `/cron delete N`

## Requirements

- Go 1.21+
- A Telegram bot token
- GitHub credentials for `/sync`:
- `GSNOTE_GITHUB_TOKEN` with access to the remote repository
- Git author identity via `GSNOTE_GIT_AUTHOR_NAME` and `GSNOTE_GIT_AUTHOR_EMAIL`
