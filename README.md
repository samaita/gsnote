# gsnote

A low-friction habit logging bot for Telegram. Logs entries as append-only markdown files compatible with Obsidian.

## How it works

Send a command to your Telegram bot — the entry is appended to a markdown file under your configured habits directory.

```
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
HABITS_ROOT=/home/youruser/gsnote
WHITELIST_TELEGRAM_ID=your_telegram_id
```

- `TELEGRAM_BOT_TOKEN` — obtain from [@BotFather](https://t.me/BotFather)
- `HABITS_ROOT` — directory where habit markdown files will be written (created automatically)
- `WHITELIST_TELEGRAM_ID` — your numeric Telegram ID from [@userinfobot](https://t.me/userinfobot)

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

| Command | Description |
|---------|-------------|
| `/habit <name> [value] [note]` | Log a habit entry |
| `/help` | Show usage |

## Requirements

- Go 1.21+
- A Telegram bot token
