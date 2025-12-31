#!/bin/bash
set -e

INSTALL_DIR="/opt/lan-orangutan"
CONFIG_DIR="/etc/lan-orangutan"
DATA_DIR="/var/lib/lan-orangutan"
SERVICE_NAME="lan-orangutan"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; NC='\033[0m'

[[ $EUID -ne 0 ]] && { echo -e "${RED}✗${NC} Run as root (sudo)"; exit 1; }

echo ""
echo -e "${YELLOW}🦧 LAN Orangutan Uninstaller${NC}"
echo "================================"
echo ""

read -p "Remove LAN Orangutan completely? [y/N]: " confirm
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
read -p "Remove device data ($DATA_DIR)? [y/N]: " data
[[ "$data" =~ ^[Yy]$ ]] && { rm -rf "$DATA_DIR"; echo -e "${GREEN}✓${NC} Data removed"; }

# Firewall
if command -v ufw &>/dev/null; then
    ufw delete allow 291/tcp 2>/dev/null || true
    ufw delete allow 2910/tcp 2>/dev/null || true
fi

echo ""
echo -e "${GREEN}🦧 LAN Orangutan uninstalled${NC}"
echo ""
