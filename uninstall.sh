#!/bin/bash
set -e

INSTALL_DIR="/opt/lan-orangutan"
CONFIG_DIR="/etc/lan-orangutan"
DATA_DIR="/var/lib/lan-orangutan"
SERVICE_NAME="lan-orangutan"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; NC='\033[0m'

[[ $EUID -ne 0 ]] && { echo -e "${RED}✗${NC} Run as root (sudo)"; exit 1; }

echo ""
echo -e "${YELLOW}LAN Orangutan Uninstaller${NC}"
echo "================================"
echo ""

# Read configured port before removing config (for firewall cleanup)
CONFIGURED_PORT=""
if [[ -f "$CONFIG_DIR/config.ini" ]]; then
    CONFIGURED_PORT=$(grep "^port" "$CONFIG_DIR/config.ini" 2>/dev/null | cut -d= -f2 | tr -d ' ' || true)
fi

INSTALLED_VERSION=""
if [[ -x /usr/local/bin/orangutan ]]; then
    INSTALLED_VERSION="$(/usr/local/bin/orangutan version 2>/dev/null | head -1 | sed 's/^LAN Orangutan //')"
fi
if [[ -n "$INSTALLED_VERSION" ]]; then
    echo "Installed version: $INSTALLED_VERSION"
    echo ""
fi

read -r -p "Uninstall LAN Orangutan? [y/N]: " confirm
[[ ! "$confirm" =~ ^[Yy]$ ]] && { echo "Aborted."; exit 0; }

# Stop service
systemctl stop "$SERVICE_NAME" 2>/dev/null || true
systemctl disable "$SERVICE_NAME" 2>/dev/null || true
rm -f /etc/systemd/system/lan-orangutan.service
systemctl daemon-reload
echo -e "${GREEN}✓${NC} Service removed"

# Remove files
rm -f /usr/local/bin/orangutan
rm -rf "$INSTALL_DIR"
rm -rf "$CONFIG_DIR"
echo -e "${GREEN}✓${NC} Files removed"

# Data
read -r -p "Also remove your device list and password ($DATA_DIR)? [y/N]: " data
[[ "$data" =~ ^[Yy]$ ]] && { rm -rf "$DATA_DIR"; echo -e "${GREEN}✓${NC} Data removed"; }

# Firewall - clean up configured port and common alternatives
if command -v ufw &>/dev/null; then
    ufw delete allow 291/tcp 2>/dev/null || true
    ufw delete allow 2910/tcp 2>/dev/null || true
    ufw delete allow 8090/tcp 2>/dev/null || true
    # Also remove custom configured port if different from defaults
    if [[ -n "$CONFIGURED_PORT" ]] && [[ "$CONFIGURED_PORT" != "291" ]] && [[ "$CONFIGURED_PORT" != "2910" ]] && [[ "$CONFIGURED_PORT" != "8090" ]]; then
        ufw delete allow "$CONFIGURED_PORT/tcp" 2>/dev/null || true
    fi
fi

echo ""
echo -e "${GREEN}LAN Orangutan ${INSTALLED_VERSION:-} uninstalled${NC}"
echo ""
