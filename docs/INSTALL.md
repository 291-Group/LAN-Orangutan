# Installation Guide

## Quick Install (Recommended)

Download the latest release for your platform from [GitHub Releases](https://github.com/291-Group/LAN-Orangutan/releases).

### Linux

```bash
# Download and extract
wget https://github.com/291-Group/LAN-Orangutan/releases/latest/download/orangutan-linux-amd64.tar.gz
tar xzf orangutan-linux-amd64.tar.gz

# Run (use sudo for MAC addresses and vendor info)
sudo ./orangutan serve
```

### macOS

```bash
# Download and extract
curl -LO https://github.com/291-Group/LAN-Orangutan/releases/latest/download/orangutan-darwin-arm64.tar.gz
tar xzf orangutan-darwin-arm64.tar.gz

# Run (use sudo for MAC addresses and vendor info)
sudo ./orangutan serve
```

### Windows

Download `orangutan-windows-amd64.zip` from [GitHub Releases](https://github.com/291-Group/LAN-Orangutan/releases), extract, and run as Administrator.

## Requirements

- **nmap** must be installed:
  - Linux: `sudo apt install nmap` or `sudo dnf install nmap`
  - macOS: `brew install nmap`
  - Windows: Download from [nmap.org](https://nmap.org/download.html)

## Building from Source

```bash
git clone https://github.com/291-Group/LAN-Orangutan.git
cd LAN-Orangutan
go build -o orangutan ./cmd/orangutan
```

## Running as a Service

### Linux (systemd)

Create `/etc/systemd/system/lan-orangutan.service`:

```ini
[Unit]
Description=LAN Orangutan Network Discovery
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/orangutan serve
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo cp orangutan /usr/local/bin/
sudo systemctl daemon-reload
sudo systemctl enable lan-orangutan
sudo systemctl start lan-orangutan
```

### macOS (launchd)

Create `~/Library/LaunchAgents/com.291group.lan-orangutan.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.291group.lan-orangutan</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/orangutan</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

Then:

```bash
cp orangutan /usr/local/bin/
launchctl load ~/Library/LaunchAgents/com.291group.lan-orangutan.plist
```

## Configuration

Config file locations:
- Linux: `~/.config/lan-orangutan/config.ini` or `/etc/lan-orangutan/config.ini` (as root)
- macOS: `~/Library/Application Support/lan-orangutan/config.ini`
- Windows: `%APPDATA%\lan-orangutan\config.ini`

See `config.example.ini` for available options.

## Firewall

Allow port 291 (or your configured port):

```bash
# Linux (ufw)
sudo ufw allow 291/tcp

# Linux (firewalld)
sudo firewall-cmd --add-port=291/tcp --permanent
sudo firewall-cmd --reload
```

## Verify Installation

```bash
orangutan version
orangutan status
curl http://localhost:291/
```
