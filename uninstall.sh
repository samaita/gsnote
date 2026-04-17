#!/usr/bin/env bash
set -e

BINARY="$HOME/.local/bin/gsnote"
CONFIG_DIR="$HOME/.config/gsnote"
SERVICE_FILE="$HOME/.config/systemd/user/gsnote.service"

if systemctl --user is-active --quiet gsnote 2>/dev/null; then
    systemctl --user stop gsnote
    echo "Stopped systemd service."
fi

if systemctl --user is-enabled --quiet gsnote 2>/dev/null; then
    systemctl --user disable gsnote
    echo "Disabled systemd service."
fi

if [ -f "$SERVICE_FILE" ]; then
    rm "$SERVICE_FILE"
    systemctl --user daemon-reload
    echo "Removed: $SERVICE_FILE"
fi

if [ -f "$BINARY" ]; then
    rm "$BINARY"
    echo "Removed: $BINARY"
fi

read -rp "Remove config directory $CONFIG_DIR? [y/N] " REMOVE_CONFIG
if [[ "$REMOVE_CONFIG" =~ ^[Yy]$ ]]; then
    rm -rf "$CONFIG_DIR"
    echo "Removed: $CONFIG_DIR"
else
    echo "Config kept: $CONFIG_DIR"
fi

echo "gsnote uninstalled."
