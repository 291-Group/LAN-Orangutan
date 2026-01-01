#!/bin/bash
set -e

INSTALL_DIR="/opt/lan-orangutan"
CONFIG_DIR="/etc/lan-orangutan"
DATA_DIR="/var/lib/lan-orangutan"
SERVICE_NAME="lan-orangutan"
DEFAULT_PORT=291

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; BLUE='\033[0;34m'; NC='\033[0m'

echo ""
echo -e "${YELLOW}🦧 LAN Orangutan Installer${NC}"
echo "================================"
echo ""

[[ $EUID -ne 0 ]] && { echo -e "${RED}✗${NC} Run as root (sudo)"; exit 1; }

# Check OS
source /etc/os-release 2>/dev/null || { echo -e "${RED}✗${NC} Cannot detect OS"; exit 1; }
case "$ID" in
    ubuntu|debian|raspbian) echo -e "${GREEN}✓${NC} OS: $PRETTY_NAME" ;;
    *) echo -e "${RED}✗${NC} Unsupported OS: $ID (need Ubuntu/Debian/Raspbian)"; exit 1 ;;
esac

# Check/install dependencies
echo -e "${BLUE}→${NC} Checking dependencies..."

if ! command -v python3 &>/dev/null; then
    echo -e "${YELLOW}!${NC} Installing Python3..."
    apt-get update -qq && apt-get install -y python3
fi
echo -e "${GREEN}✓${NC} Python $(python3 --version | cut -d' ' -f2)"

if ! command -v php &>/dev/null; then
    echo -e "${YELLOW}!${NC} Installing PHP..."
    apt-get update -qq && apt-get install -y php-cli php-json
fi
echo -e "${GREEN}✓${NC} PHP $(php -v | head -n1 | cut -d' ' -f2)"

if ! command -v nmap &>/dev/null; then
    echo -e "${YELLOW}!${NC} Installing nmap..."
    apt-get update -qq && apt-get install -y nmap
fi
echo -e "${GREEN}✓${NC} nmap installed"

# Select port
PORT=$DEFAULT_PORT
if ss -tuln | grep -q ":$PORT "; then
    echo -e "${YELLOW}!${NC} Port $PORT in use"
    echo "Select alternative: 1) 2910  2) 8090  3) Custom"
    read -p "Choice [1-3]: " choice
    case "$choice" in
        1) PORT=2910 ;;
        2) PORT=8090 ;;
        3)
            read -p "Port: " PORT
            # Validate custom port is a number between 1-65535
            if [[ ! "$PORT" =~ ^[0-9]+$ ]] || [[ "$PORT" -lt 1 ]] || [[ "$PORT" -gt 65535 ]]; then
                echo -e "${YELLOW}!${NC} Invalid port '$PORT', using 2910"
                PORT=2910
            fi
            ;;
        *) PORT=2910 ;;
    esac
fi
echo -e "${GREEN}✓${NC} Using port: $PORT"

# Install files
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo -e "${BLUE}→${NC} Installing to $INSTALL_DIR..."

mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR"
cp -r "$SOURCE_DIR/scanner" "$INSTALL_DIR/"
cp -r "$SOURCE_DIR/web" "$INSTALL_DIR/"
cp -r "$SOURCE_DIR/cli" "$INSTALL_DIR/"
cp -r "$SOURCE_DIR/scripts" "$INSTALL_DIR/"

chmod +x "$INSTALL_DIR/cli/orangutan"
chmod +x "$INSTALL_DIR/scripts/start-server.sh"
chmod +x "$INSTALL_DIR/scanner/scan.py"
ln -sf "$INSTALL_DIR/cli/orangutan" /usr/local/bin/orangutan

echo -e "${GREEN}✓${NC} Files installed"

# Create config
cat > "$CONFIG_DIR/config.ini" << EOF
[server]
port = $PORT
bind_address = 0.0.0.0
enable_api = true

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

# Firewall
if command -v ufw &>/dev/null && ufw status | grep -q "active"; then
    read -p "Configure ufw for port $PORT? [y/N]: " fw
    [[ "$fw" =~ ^[Yy]$ ]] && { ufw allow "$PORT/tcp"; echo -e "${GREEN}✓${NC} Firewall configured"; }
fi

# Install service
cp "$SOURCE_DIR/systemd/lan-orangutan.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl start "$SERVICE_NAME"
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
echo -e "${GREEN}🦧 LAN Orangutan installed!${NC}"
echo "========================================"
echo ""
echo -e "Web UI: ${BLUE}http://$IP:$PORT${NC}"
echo ""
echo "CLI: orangutan scan all"
echo "     orangutan list"
echo "     orangutan help"
echo ""
