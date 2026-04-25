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
    read -rp "GitHub token for /sync HTTPS remote [optional]: " GITHUB_TOKEN </dev/tty
    read -rp "Git author name for /sync commits: " GIT_AUTHOR_NAME </dev/tty
    read -rp "Git author email for /sync commits: " GIT_AUTHOR_EMAIL </dev/tty
    read -rp "Vault (sync) folder [$HOME/vault]: " SYNC_ROOT_INPUT </dev/tty
    SYNC_ROOT="${SYNC_ROOT_INPUT:-$HOME/vault}"
    read -rp "Habits folder [$SYNC_ROOT/Habits]: " HABITS_ROOT_INPUT </dev/tty
    HABITS_ROOT="${HABITS_ROOT_INPUT:-$SYNC_ROOT/Habits}"
    read -rp "Tasks folder [$SYNC_ROOT/Tasks]: " TASKS_ROOT_INPUT </dev/tty
    TASKS_ROOT="${TASKS_ROOT_INPUT:-$SYNC_ROOT/Tasks}"
    read -rp "Notes folder [$SYNC_ROOT/Notes]: " NOTES_ROOT_INPUT </dev/tty
    NOTES_ROOT="${NOTES_ROOT_INPUT:-$SYNC_ROOT/Notes}"
    read -rp "Whitelist Telegram ID (from @userinfobot): " WHITELIST_ID </dev/tty
    read -rp "Timezone [Asia/Jakarta]: " TIMEZONE_INPUT </dev/tty
    TIMEZONE="${TIMEZONE_INPUT:-Asia/Jakarta}"

    mkdir -p "$HABITS_ROOT"
    mkdir -p "$TASKS_ROOT"
    mkdir -p "$NOTES_ROOT"

    quote() { local v="$1"; [[ "$v" == \"*\" ]] && echo "$v" || echo "\"$v\""; }

    cat > "$CONFIG_FILE" <<EOF
TELEGRAM_BOT_TOKEN=$(quote "$BOT_TOKEN")
GSNOTE_GITHUB_TOKEN=$(quote "$GITHUB_TOKEN")
GSNOTE_GIT_AUTHOR_NAME=$(quote "$GIT_AUTHOR_NAME")
GSNOTE_GIT_AUTHOR_EMAIL=$(quote "$GIT_AUTHOR_EMAIL")
SYNC_ROOT=$(quote "$SYNC_ROOT")
HABITS_ROOT=$(quote "$HABITS_ROOT")
TASKS_ROOT=$(quote "$TASKS_ROOT")
IDEAS_ROOT=$(quote "$SYNC_ROOT/Ideas")
NOTES_ROOT=$(quote "$NOTES_ROOT")
WHITELIST_TELEGRAM_ID=$(quote "$WHITELIST_ID")
TIMEZONE=$(quote "$TIMEZONE")
EOF
    echo ""
    echo "Config saved to: $CONFIG_FILE"
    echo "Sync (vault) folder: $SYNC_ROOT"
    echo "Habits folder:       $HABITS_ROOT"
    echo "Tasks folder:        $TASKS_ROOT"
    echo "Notes folder:        $NOTES_ROOT"
    echo "Timezone:            $TIMEZONE"
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
