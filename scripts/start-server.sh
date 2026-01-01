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

# Validate port is a number between 1-65535
if [[ ! "$PORT" =~ ^[0-9]+$ ]] || [[ "$PORT" -lt 1 ]] || [[ "$PORT" -gt 65535 ]]; then
    echo "Warning: Invalid port '$PORT', using default 291" >&2
    PORT=291
fi

# Validate bind address is a valid IPv4 address
if [[ ! "$BIND" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
    echo "Warning: Invalid bind address '$BIND', using default 0.0.0.0" >&2
    BIND="0.0.0.0"
fi

cd "$WEB_DIR"
echo "Starting LAN Orangutan on $BIND:$PORT"
exec php -S "$BIND:$PORT" -t "$WEB_DIR"
