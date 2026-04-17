#!/usr/bin/env bash
set -e

BINARY="$HOME/.local/bin/gsnote"
CONFIG_DIR="$HOME/.config/gsnote"

if [ "$(id -u)" -eq 0 ]; then
    SERVICE_FILE="/etc/systemd/system/gsnote.service"
    SYSTEMCTL="systemctl"
else
    SERVICE_FILE="$HOME/.config/systemd/user/gsnote.service"
    SYSTEMCTL="systemctl --user"
fi

if $SYSTEMCTL is-active --quiet gsnote 2>/dev/null; then
    $SYSTEMCTL stop gsnote
    echo "Stopped systemd service."
fi

if $SYSTEMCTL is-enabled --quiet gsnote 2>/dev/null; then
    $SYSTEMCTL disable gsnote
    echo "Disabled systemd service."
fi

if [ -f "$SERVICE_FILE" ]; then
    rm "$SERVICE_FILE"
    $SYSTEMCTL daemon-reload
    echo "Removed: $SERVICE_FILE"
fi

if [ -f "$BINARY" ]; then
    rm "$BINARY"
    echo "Removed: $BINARY"
fi

read -rp "Remove config directory $CONFIG_DIR? [y/N] " REMOVE_CONFIG </dev/tty
if [[ "$REMOVE_CONFIG" =~ ^[Yy]$ ]]; then
    rm -rf "$CONFIG_DIR"
    echo "Removed: $CONFIG_DIR"
else
    echo "Config kept: $CONFIG_DIR"
fi

echo "gsnote uninstalled."
