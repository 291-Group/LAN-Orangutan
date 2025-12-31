#!/bin/bash
set -e

INSTALL_DIR="/opt/lan-orangutan"
CONFIG_FILE="/etc/lan-orangutan/config.ini"
WEB_DIR="$INSTALL_DIR/web"

PORT=291
BIND="0.0.0.0"

if [[ -f "$CONFIG_FILE" ]]; then
    PORT=$(grep "^port" "$CONFIG_FILE" 2>/dev/null | cut -d= -f2 | tr -d ' ' || echo "291")
    BIND=$(grep "^bind_address" "$CONFIG_FILE" 2>/dev/null | cut -d= -f2 | tr -d ' ' || echo "0.0.0.0")
fi

[[ ! "$PORT" =~ ^[0-9]+$ ]] && PORT=291

cd "$WEB_DIR"
echo "Starting LAN Orangutan on $BIND:$PORT"
exec php -S "$BIND:$PORT" -t "$WEB_DIR"
