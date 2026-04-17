#!/usr/bin/env bash
set -e

REPO="samaita/gsnote"
CONFIG_DIR="$HOME/.config/gsnote"
CONFIG_FILE="$CONFIG_DIR/.env"
BINARY_DIR="$HOME/.local/bin"

mkdir -p "$CONFIG_DIR"
mkdir -p "$BINARY_DIR"

if [ ! -f "$CONFIG_FILE" ]; then
    read -rp "Telegram bot token: " BOT_TOKEN
    read -rp "Habits folder [$HOME/gsnote]: " HABITS_ROOT_INPUT
    HABITS_ROOT="${HABITS_ROOT_INPUT:-$HOME/gsnote}"
    read -rp "Whitelist Telegram ID (from @userinfobot): " WHITELIST_ID

    mkdir -p "$HABITS_ROOT"

    quote() { local v="$1"; [[ "$v" == \"*\" ]] && echo "$v" || echo "\"$v\""; }

    cat > "$CONFIG_FILE" <<EOF
TELEGRAM_BOT_TOKEN=$(quote "$BOT_TOKEN")
HABITS_ROOT=$(quote "$HABITS_ROOT")
WHITELIST_TELEGRAM_ID=$(quote "$WHITELIST_ID")
EOF
    echo ""
    echo "Config saved to: $CONFIG_FILE"
    echo "Habits folder:   $HABITS_ROOT"
    echo ""
    echo "You can reconfigure anytime by editing: $CONFIG_FILE"
else
    echo "Config already exists: $CONFIG_FILE"
fi

OS=$(uname -s)
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="x86_64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
ARCHIVE="gsnote_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$LATEST/$ARCHIVE"

echo "Downloading gsnote $LATEST..."
curl -fsSL "$URL" -o "/tmp/$ARCHIVE"
tar -xzf "/tmp/$ARCHIVE" -C /tmp gsnote
mv /tmp/gsnote "$BINARY_DIR/gsnote"
chmod +x "$BINARY_DIR/gsnote"
rm "/tmp/$ARCHIVE"
echo "Installed: $BINARY_DIR/gsnote"

read -rp "Set up systemd user service? [y/N] " SETUP_SYSTEMD
if [[ "$SETUP_SYSTEMD" =~ ^[Yy]$ ]]; then
    SYSTEMD_DIR="$HOME/.config/systemd/user"
    SERVICE_FILE="$SYSTEMD_DIR/gsnote.service"
    mkdir -p "$SYSTEMD_DIR"
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=gsnote Telegram bot
After=network.target

[Service]
ExecStart=$BINARY_DIR/gsnote
EnvironmentFile=$CONFIG_FILE
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload
    systemctl --user enable --now gsnote
    echo ""
    echo "Systemd service installed and started: $SERVICE_FILE"
    echo ""
    echo "  Status: systemctl --user status gsnote"
    echo "  Logs:   journalctl --user -u gsnote -f"
fi
