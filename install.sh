#!/bin/bash
#
# Installs LAN Orangutan from a release archive.
#
# This installs the single Go binary. Earlier versions installed a Python and
# PHP application into /opt/lan-orangutan; if that is present it is removed,
# because the two cannot both own the service and the `orangutan` command.
set -e

BIN_PATH="/usr/local/bin/orangutan"
CONFIG_DIR="/etc/lan-orangutan"
DATA_DIR="/var/lib/lan-orangutan"
LEGACY_DIR="/opt/lan-orangutan"
SERVICE_NAME="lan-orangutan"
DEFAULT_PORT=291

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; BLUE='\033[0;34m'; NC='\033[0m'

echo ""
echo -e "${YELLOW}LAN Orangutan Installer${NC}"
echo "================================"
echo ""

[[ $EUID -ne 0 ]] && { echo -e "${RED}✗${NC} Run as root (sudo)"; exit 1; }

# Check OS
# shellcheck disable=SC1091
source /etc/os-release 2>/dev/null || { echo -e "${RED}✗${NC} Cannot detect OS"; exit 1; }
case "$ID" in
    ubuntu|debian|raspbian) echo -e "${GREEN}✓${NC} OS: $PRETTY_NAME" ;;
    *) echo -e "${RED}✗${NC} Unsupported OS: $ID (need Ubuntu/Debian/Raspbian)"; exit 1 ;;
esac

# Dependencies. The binary is self-contained, so nmap is the only requirement.
echo -e "${BLUE}→${NC} Checking dependencies..."
if ! command -v nmap &>/dev/null; then
    echo -e "${YELLOW}!${NC} Installing nmap..."
    apt-get update -qq && apt-get install -y nmap
fi
echo -e "${GREEN}✓${NC} nmap installed"

# Locate the binary shipped alongside this script.
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)  BIN_NAME="orangutan-linux-amd64" ;;
    aarch64|arm64) BIN_NAME="orangutan-linux-arm64" ;;
    armv7l|armhf)  BIN_NAME="orangutan-linux-arm" ;;
    *) echo -e "${RED}✗${NC} Unsupported architecture: $ARCH"; exit 1 ;;
esac

SOURCE_BIN=""
for candidate in "$SOURCE_DIR/$BIN_NAME" "$SOURCE_DIR/orangutan"; do
    [[ -f "$candidate" ]] && { SOURCE_BIN="$candidate"; break; }
done

NEW_VERSION=""
if [[ -n "$SOURCE_BIN" ]]; then
    NEW_VERSION="$("$SOURCE_BIN" version 2>/dev/null | head -1 | sed 's/^LAN Orangutan //')"
fi

if [[ -z "$SOURCE_BIN" ]]; then
    echo -e "${RED}✗${NC} Could not find the orangutan binary next to this script."
    echo "   Expected $BIN_NAME or orangutan in $SOURCE_DIR"
    echo "   Download the release for your platform from:"
    echo "   https://github.com/291-Group/LAN-Orangutan/releases"
    exit 1
fi

# Select port
PORT=$DEFAULT_PORT
if ss -tuln | grep -q ":$PORT "; then
    echo -e "${YELLOW}!${NC} Port $PORT in use"
    echo "Select alternative: 1) 2910  2) 8090  3) Custom"
    read -r -p "Choice [1-3]: " choice
    case "$choice" in
        1) PORT=2910 ;;
        2) PORT=8090 ;;
        3)
            read -r -p "Port: " PORT
            if [[ ! "$PORT" =~ ^[0-9]+$ ]] || [[ "$PORT" -lt 1 ]] || [[ "$PORT" -gt 65535 ]]; then
                echo -e "${YELLOW}!${NC} Invalid port '$PORT', using 2910"
                PORT=2910
            fi
            ;;
        *) PORT=2910 ;;
    esac
fi
echo -e "${GREEN}✓${NC} Using port: $PORT"

# Remove a previous Python/PHP installation. Its files are never read by the
# binary, and leaving them behind means two things claiming the same service.
if [[ -d "$LEGACY_DIR" ]]; then
    echo -e "${BLUE}→${NC} Removing the previous installation in $LEGACY_DIR..."
    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    rm -rf "$LEGACY_DIR"
    echo -e "${GREEN}✓${NC} Old installation removed (your config and data are kept)"
fi

if [[ -x "$BIN_PATH" ]]; then
    OLD_VERSION="$("$BIN_PATH" version 2>/dev/null | head -1 | sed 's/^LAN Orangutan //')"
    echo -e "${BLUE}→${NC} Upgrading ${OLD_VERSION:-unknown} to ${NEW_VERSION:-unknown}..."
else
    echo -e "${BLUE}→${NC} Installing ${NEW_VERSION:-unknown}..."
fi
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
install -m 755 "$SOURCE_BIN" "$BIN_PATH"
echo -e "${GREEN}✓${NC} Installed $BIN_PATH ($("$BIN_PATH" version 2>/dev/null | head -1 | sed 's/^LAN Orangutan //'))"

# Create config, but never overwrite one the user already has.
if [[ -f "$CONFIG_DIR/config.ini" ]]; then
    echo -e "${GREEN}✓${NC} Keeping your existing config"
else
    cat > "$CONFIG_DIR/config.ini" << EOF
[server]
port = $PORT
# Reachable from your network. The first time you open the dashboard you will
# be asked to create a password; nothing is reachable until you do.
# Set bind_address to 127.0.0.1 instead to keep it to this machine only.
bind_address = 0.0.0.0
enable_api = true

# Optional: set a password here to skip the first-run setup screen.
# password =
session_hours = 168
allow_insecure = false

[scanning]
scan_interval = 300
min_scan_interval = 30
enable_port_scan = false

[storage]
max_devices = 1000
retention_days = 90

[tailscale]
enable = true
auto_detect = true

[ui]
theme = auto
EOF
    echo -e "${GREEN}✓${NC} Config created"
fi

# Firewall
if command -v ufw &>/dev/null && ufw status | grep -q "active"; then
    read -r -p "Configure ufw for port $PORT? [y/N]: " fw
    [[ "$fw" =~ ^[Yy]$ ]] && { ufw allow "$PORT/tcp"; echo -e "${GREEN}✓${NC} Firewall configured"; }
fi

# Install service
cp "$SOURCE_DIR/systemd/lan-orangutan.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
sleep 2

if systemctl is-active --quiet "$SERVICE_NAME"; then
    echo -e "${GREEN}✓${NC} Service started"
else
    echo -e "${RED}✗${NC} Service failed. Check: journalctl -u $SERVICE_NAME"
    exit 1
fi

IP=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K\S+' || hostname -I | awk '{print $1}')

echo ""
echo "========================================"
echo -e "${GREEN}LAN Orangutan ${NEW_VERSION:-} installed!${NC}"
echo "========================================"
echo ""
echo -e "Web UI: ${BLUE}http://$IP:$PORT${NC}"
echo ""
echo "You will be asked to create a password the first time you open it."
echo ""
echo "CLI: orangutan scan all"
echo "     orangutan list"
echo "     orangutan --help"
echo ""
