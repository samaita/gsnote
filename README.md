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

## Setup

### 1. Clone and build

```bash
git clone https://github.com/axonigma/gsnote.git
cd gsnote
make build
```

### 2. Configure environment

Create a `.env` file:

```env
TELEGRAM_BOT_TOKEN=your_bot_token_here
HABITS_ROOT=/path/to/habits
```

- `TELEGRAM_BOT_TOKEN` — obtain from [@BotFather](https://t.me/BotFather) on Telegram
- `HABITS_ROOT` — directory where habit markdown files will be written (created automatically)

### 3. Run

```bash
./gsnote
```

## Run as a systemd service

Copy and edit the example unit file:

```bash
cp gsnote.service.example /etc/systemd/system/gsnote.service
# edit User, WorkingDirectory, EnvironmentFile, ExecStart
sudo systemctl daemon-reload
sudo systemctl enable --now gsnote
```

## Bot commands

| Command | Description |
|---------|-------------|
| `/habit <name> [value] [note]` | Log a habit entry |
| `/help` | Show usage |

## Requirements

- Go 1.21+
- A Telegram bot token
