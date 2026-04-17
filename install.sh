#!/usr/bin/env bash
set -e

REPO="samaita/gsnote"
CONFIG_DIR="$HOME/.config/gsnote"
CONFIG_FILE="$CONFIG_DIR/.env"
BINARY_DIR="$HOME/.local/bin"

mkdir -p "$CONFIG_DIR"
mkdir -p "$BINARY_DIR"

if [ ! -f "$CONFIG_FILE" ]; then
    cat > "$CONFIG_FILE" <<'EOF'
TELEGRAM_BOT_TOKEN=your_bot_token_here
HABITS_ROOT=/path/to/your/habits/folder
WHITELIST_TELEGRAM_ID=your_telegram_id_from_userinfobot
EOF
    echo "Created config: $CONFIG_FILE"
    echo "Edit it before starting gsnote."
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
    systemctl --user enable gsnote
    echo ""
    echo "Systemd service installed: $SERVICE_FILE"
    echo ""
    echo "  Start:  systemctl --user start gsnote"
    echo "  Status: systemctl --user status gsnote"
    echo "  Logs:   journalctl --user -u gsnote -f"
fi
