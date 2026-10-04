#!/usr/bin/env bash
set -e

REPO="samaita/gsnote"
CONFIG_DIR="$HOME/.config/gsnote"
CONFIG_FILE="$CONFIG_DIR/.env"
BINARY_DIR="$HOME/.local/bin"
mkdir -p "$CONFIG_DIR" "$BINARY_DIR"

if [ ! -f "$CONFIG_FILE" ]; then
    read -rp "Telegram bot token: " BOT_TOKEN </dev/tty
    read -rp "Data root (Inbox/ is created beneath it) [$HOME/gsnote]: " GSNOTE_ROOT_INPUT </dev/tty
    GSNOTE_ROOT="${GSNOTE_ROOT_INPUT:-$HOME/gsnote}"
    read -rp "whisper-cli binary [whisper-cli]: " TRANSCRIBER_BINARY_INPUT </dev/tty
    TRANSCRIBER_BINARY="${TRANSCRIBER_BINARY_INPUT:-whisper-cli}"
    DEFAULT_MODEL="$HOME/.local/share/gsnote/models/ggml-small-q5_1.bin"
    read -rp "Whisper model path [$DEFAULT_MODEL]: " TRANSCRIBER_MODEL_INPUT </dev/tty
    TRANSCRIBER_MODEL="${TRANSCRIBER_MODEL_INPUT:-$DEFAULT_MODEL}"
    read -rp "Whisper threads [2]: " TRANSCRIBER_THREADS_INPUT </dev/tty
    TRANSCRIBER_THREADS="${TRANSCRIBER_THREADS_INPUT:-2}"
    read -rp "Whisper language [en]: " TRANSCRIBER_LANGUAGE_INPUT </dev/tty
    TRANSCRIBER_LANGUAGE="${TRANSCRIBER_LANGUAGE_INPUT:-en}"
    TRANSCRIBE_MAX_ATTEMPTS="${TRANSCRIBE_MAX_ATTEMPTS:-5}"
    read -rp "Whitelist Telegram ID: " WHITELIST_ID </dev/tty
    mkdir -p "$GSNOTE_ROOT"
    quote() { local v="$1"; [[ "$v" == *'"'* ]] && v="${v//\"/\\\"}"; printf '"%s"' "$v"; }
    umask 077
    cat > "$CONFIG_FILE" <<EOF
TELEGRAM_BOT_TOKEN=$(quote "$BOT_TOKEN")
WHITELIST_TELEGRAM_ID=$(quote "$WHITELIST_ID")
GSNOTE_ROOT=$(quote "$GSNOTE_ROOT")
TRANSCRIBER_BINARY=$(quote "$TRANSCRIBER_BINARY")
TRANSCRIBER_MODEL=$(quote "$TRANSCRIBER_MODEL")
TRANSCRIBER_THREADS=$(quote "$TRANSCRIBER_THREADS")
TRANSCRIBER_LANGUAGE=$(quote "$TRANSCRIBER_LANGUAGE")
TRANSCRIBE_MAX_ATTEMPTS=$(quote "$TRANSCRIBE_MAX_ATTEMPTS")
EOF
    echo "Config saved to: $CONFIG_FILE"
else
    echo "Config already exists: $CONFIG_FILE"
fi

# Read values without sourcing the file (which would execute arbitrary shell text).
read_config_value() {
    local key="$1" line value
    line=$(awk -v key="$key" 'index($0,key"=")==1 {print; found=1} END {if (!found) exit 1}' "$CONFIG_FILE") || return 1
    value=${line#*=}
    if [[ "$value" == '"'*'"' ]]; then value=${value:1:${#value}-2}; value=${value//\\\"/\"}; fi
    printf '%s' "$value"
}

ROOT_VALUE=$(read_config_value GSNOTE_ROOT)
if [ -z "$ROOT_VALUE" ]; then
    echo "Missing or malformed GSNOTE_ROOT in $CONFIG_FILE" >&2
    exit 1
fi
case "$ROOT_VALUE" in *$'\n'*|*$'\r'*) echo "Malformed GSNOTE_ROOT" >&2; exit 1;; esac
if [ ! -d "$ROOT_VALUE" ]; then
    echo "GSNOTE_ROOT must be an existing directory. Set it to your existing notes path before rerunning." >&2
    exit 1
fi

if ! command -v ffmpeg >/dev/null 2>&1; then echo "ffmpeg is required but was not found in PATH." >&2; exit 1; fi
MODEL=$(read_config_value TRANSCRIBER_MODEL)
if [ -z "$MODEL" ] || [ ! -f "$MODEL" ]; then echo "Whisper model path is missing or invalid." >&2; exit 1; fi
MAX_ATTEMPTS_RAW=$(read_config_value TRANSCRIBE_MAX_ATTEMPTS)
if [ -z "$MAX_ATTEMPTS_RAW" ]; then MAX_ATTEMPTS_RAW=5; fi
if ! [[ "$MAX_ATTEMPTS_RAW" =~ ^[1-9][0-9]*$ ]]; then echo "TRANSCRIBE_MAX_ATTEMPTS must be positive." >&2; exit 1; fi

OS=$(uname -s); ARCH=$(uname -m)
case "$ARCH" in x86_64) ARCH=x86_64;; aarch64|arm64) ARCH=arm64;; *) echo "Unsupported architecture: $ARCH"; exit 1;; esac
LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
ARCHIVE="gsnote_${OS}_${ARCH}.tar.gz"; URL="https://github.com/$REPO/releases/download/$LATEST/$ARCHIVE"
INSTALLED_VERSION=""
if [ -x "$BINARY_DIR/gsnote" ]; then INSTALLED_VERSION=$(timeout 3s "$BINARY_DIR/gsnote" -version 2>/dev/null || true); fi
if [ "${INSTALLED_VERSION#v}" != "${LATEST#v}" ]; then
    curl -fsSL "$URL" -o "/tmp/$ARCHIVE"
    tar -xzf "/tmp/$ARCHIVE" -C /tmp gsnote
    mv /tmp/gsnote "$BINARY_DIR/gsnote"; chmod +x "$BINARY_DIR/gsnote"; rm "/tmp/$ARCHIVE"
    echo "Installed gsnote $LATEST"
else echo "gsnote $LATEST is already up to date."; fi

read -rp "Set up systemd service? [y/N] " SETUP_SYSTEMD </dev/tty
if [[ "$SETUP_SYSTEMD" =~ ^[Yy]$ ]]; then
    if [ "$(id -u)" -eq 0 ]; then
        SYSTEMD_DIR=/etc/systemd/system; SERVICE_FILE="$SYSTEMD_DIR/gsnote.service"
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
        systemctl daemon-reload; systemctl enable --now gsnote
    else
        SYSTEMD_DIR="$HOME/.config/systemd/user"; SERVICE_FILE="$SYSTEMD_DIR/gsnote.service"; mkdir -p "$SYSTEMD_DIR"
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
        systemctl --user daemon-reload; systemctl --user enable --now gsnote
    fi
    echo "Installed bot service at $SERVICE_FILE"
fi
