#!/usr/bin/env bash
set -e

REPO="samaita/gsnote"
CONFIG_DIR="$HOME/.config/gsnote"
CONFIG_FILE="$CONFIG_DIR/.env"
BINARY_DIR="$HOME/.local/bin"

mkdir -p "$CONFIG_DIR"
mkdir -p "$BINARY_DIR"

if [ ! -f "$CONFIG_FILE" ]; then
    read -rp "Telegram bot token: " BOT_TOKEN </dev/tty
    read -rp "Notes folder [$HOME/gsnote]: " GSNOTE_ROOT_INPUT </dev/tty
    GSNOTE_ROOT="${GSNOTE_ROOT_INPUT:-$HOME/gsnote}"
    read -rp "whisper-cli binary [whisper-cli]: " TRANSCRIBER_BINARY_INPUT </dev/tty
    TRANSCRIBER_BINARY="${TRANSCRIBER_BINARY_INPUT:-whisper-cli}"
    DEFAULT_MODEL="$HOME/.local/share/gsnote/models/ggml-small-q5_1.bin"
    read -rp "Whisper model path [$DEFAULT_MODEL]: " TRANSCRIBER_MODEL_INPUT </dev/tty
    TRANSCRIBER_MODEL="${TRANSCRIBER_MODEL_INPUT:-$DEFAULT_MODEL}"
    read -rp "Whisper threads [2]: " TRANSCRIBER_THREADS_INPUT </dev/tty
    TRANSCRIBER_THREADS="${TRANSCRIBER_THREADS_INPUT:-2}"
    read -rp "Whisper language, empty for auto-detect [id]: " TRANSCRIBER_LANGUAGE_INPUT </dev/tty
    TRANSCRIBER_LANGUAGE="${TRANSCRIBER_LANGUAGE_INPUT:-id}"
    read -rp "Whitelist Telegram ID (from @userinfobot): " WHITELIST_ID </dev/tty

    mkdir -p "$GSNOTE_ROOT"

    quote() { local v="$1"; [[ "$v" == \"*\" ]] && echo "$v" || echo "\"$v\""; }

    cat > "$CONFIG_FILE" <<EOF
TELEGRAM_BOT_TOKEN=$(quote "$BOT_TOKEN")
WHITELIST_TELEGRAM_ID=$(quote "$WHITELIST_ID")
GSNOTE_ROOT=$(quote "$GSNOTE_ROOT")
TRANSCRIBER_BINARY=$(quote "$TRANSCRIBER_BINARY")
TRANSCRIBER_MODEL=$(quote "$TRANSCRIBER_MODEL")
TRANSCRIBER_THREADS=$(quote "$TRANSCRIBER_THREADS")
TRANSCRIBER_LANGUAGE=$(quote "$TRANSCRIBER_LANGUAGE")
EOF
    echo ""
    echo "Config saved to: $CONFIG_FILE"
    echo "Notes folder: $GSNOTE_ROOT"
    echo ""
    echo "You can reconfigure anytime by editing: $CONFIG_FILE"
else
    echo "Config already exists: $CONFIG_FILE"
    if ! grep -q '^GSNOTE_ROOT=' "$CONFIG_FILE" || ! grep -q '^TRANSCRIBER_MODEL=' "$CONFIG_FILE"; then
        echo "Legacy config detected: required local transcription settings are missing."
        echo "Edit $CONFIG_FILE manually or remove it and re-run this script."
        exit 1
    fi
fi

if ! command -v ffmpeg >/dev/null 2>&1; then
    echo "ffmpeg is required but was not found in PATH."
    exit 1
fi

TRANSCRIBER_COMMAND=$(sed -n 's/^TRANSCRIBER_BINARY=["'"']\{0,1\}\([^"'"']*\)["'"']\{0,1\}$/\1/p' "$CONFIG_FILE")
TRANSCRIBER_COMMAND="${TRANSCRIBER_COMMAND:-whisper-cli}"
if ! command -v "$TRANSCRIBER_COMMAND" >/dev/null 2>&1 && [ ! -x "$TRANSCRIBER_COMMAND" ]; then
    echo "$TRANSCRIBER_COMMAND is required but was not found or executable."
    exit 1
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

INSTALLED_VERSION=""
if [ -x "$BINARY_DIR/gsnote" ]; then
    INSTALLED_VERSION=$(timeout 3s "$BINARY_DIR/gsnote" -version 2>/dev/null || true)
    if [ -z "$INSTALLED_VERSION" ]; then
        echo "Existing gsnote did not return version (old build). Removing..."
        rm -f "$BINARY_DIR/gsnote"
    fi
fi

INSTALLED_VERSION_NORM="${INSTALLED_VERSION#v}"
LATEST_NORM="${LATEST#v}"

if [ "$INSTALLED_VERSION_NORM" = "$LATEST_NORM" ]; then
    echo "gsnote $LATEST is already up to date."
else
    if [ -n "$INSTALLED_VERSION" ]; then
        echo "Upgrading gsnote $INSTALLED_VERSION -> $LATEST..."
    else
        echo "Downloading gsnote $LATEST..."
    fi
    curl -fsSL "$URL" -o "/tmp/$ARCHIVE"
    tar -xzf "/tmp/$ARCHIVE" -C /tmp gsnote
    mv /tmp/gsnote "$BINARY_DIR/gsnote"
    chmod +x "$BINARY_DIR/gsnote"
    rm "/tmp/$ARCHIVE"
    echo "Installed: $BINARY_DIR/gsnote ($LATEST)"
fi

read -rp "Set up systemd service? [y/N] " SETUP_SYSTEMD </dev/tty
if [[ "$SETUP_SYSTEMD" =~ ^[Yy]$ ]]; then
    if [ "$(id -u)" -eq 0 ]; then
        # Running as root: use system-level service
        SYSTEMD_DIR="/etc/systemd/system"
        SERVICE_FILE="$SYSTEMD_DIR/gsnote.service"
        cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=gsnote Telegram bot
After=network.target

[Service]
ExecStart=$BINARY_DIR/gsnote
EnvironmentFile=$CONFIG_FILE
Environment=HOME=$HOME
Environment=PATH=$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
        systemctl daemon-reload
        systemctl enable --now gsnote
        echo ""
        echo "Systemd service installed and started: $SERVICE_FILE"
        echo ""
        echo "  Status: systemctl status gsnote"
        echo "  Logs:   journalctl -u gsnote -f"
    else
        # Regular user: use user-level service
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
Environment=PATH=$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin
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
fi
