#!/usr/bin/env bash
set -e

CONFIG_DIR="$HOME/.config/gsnote"
CONFIG_FILE="$CONFIG_DIR/.env"
BINARY_DIR="$HOME/.local/bin"

mkdir -p "$CONFIG_DIR"
mkdir -p "$BINARY_DIR"

if [ ! -f "$CONFIG_FILE" ]; then
    cp .env.example "$CONFIG_FILE"
    echo "Created config: $CONFIG_FILE"
    echo "Edit it before starting gsnote."
else
    echo "Config already exists: $CONFIG_FILE"
fi

go build -o "$BINARY_DIR/gsnote" ./cmd/bot
echo "Installed: $BINARY_DIR/gsnote"
