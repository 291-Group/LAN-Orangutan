# 🦧 LAN Orangutan

*Swing through your network and see what's hanging around*

A lightweight, self-hosted network discovery tool with persistent device labeling, multi-network support, and Tailscale integration.

**By 291 Group** - Canadian defense technology specialists in SIGINT, EW, and network intelligence.

---

## Features

- 🔍 **Network Discovery** - Automatically scan and discover devices across multiple networks
- 🏷️ **Persistent Labeling** - Tag devices with friendly names and notes that survive reboots
- 🌐 **Multi-Network Support** - Scan cable, fiber, wireless, and Tailscale networks simultaneously
- 🦎 **Tailscale Integration** - Auto-detects Tailscale mesh networks and displays peer information
- 🎨 **Clean Web Interface** - Responsive dashboard with light/dark mode
- ⚙️ **Easy Configuration** - Web-based settings with no manual config editing required
- 🔧 **CLI Tools** - Command-line interface for automation and scripting
- 🐧 **Systemd Service** - Runs as a background service with auto-start on boot
- 🔒 **Self-Hosted** - No cloud, no telemetry, your data stays yours

---

## Screenshots

*Coming soon*

---

## Quick Start
```bash
# One-command installation
curl -sSL https://raw.githubusercontent.com/291-Group/LAN-Orangutan/main/install.sh | sudo bash

# Access the web interface
http://your-ip:291
```

---

## Installation

**Requirements:**
- Debian/Ubuntu/Raspbian-based Linux
- Root access (for installation)
- Python 3.x (auto-installed if missing)
- PHP (auto-installed if missing)
- nmap (auto-installed if missing)

**Install:**
```bash
git clone https://github.com/291-Group/LAN-Orangutan.git
cd LAN-Orangutan
sudo ./install.sh
```

The installer will:
- Check and install dependencies
- Configure the service
- Handle firewall setup (with permission)
- Start LAN Orangutan on port 291

---

## Usage

**Web Interface:**
```
http://your-device-ip:291
```

**CLI Commands:**
```bash
# Scan networks
orangutan scan

# List discovered devices
orangutan list

# Export to CSV
orangutan export devices.csv

# Service management
sudo systemctl start lan-orangutan
sudo systemctl stop lan-orangutan
sudo systemctl restart lan-orangutan
sudo systemctl status lan-orangutan
```

---

## Configuration

Access Settings via the web interface, or edit:
```
/etc/lan-orangutan/config.ini
```

**Key settings:**
- Port number (default: 291)
- Networks to scan
- Scan interval
- Firewall configuration

---

## Uninstall
```bash
sudo ./uninstall.sh
```

Removes all files, services, and configurations cleanly.

---

## Development Status

🚧 **Currently in private development** - Testing and refinement in progress.

This repository will be made public once testing is complete.

---

## About 291 Group

291 Group develops advanced technology solutions for signals intelligence, electronic warfare, and custom engineering. Our flagship product is the 291 EW Platform, a comprehensive SIGINT collection and analysis system.

LAN Orangutan started as an internal tool and is being released as open source to benefit the broader community.

**Website:** [291group.com](https://291group.com)

---

## Contributing

Contributions welcome once the project goes public! Please open an issue before starting major work.

---

## License

MIT License - See [LICENSE](LICENSE) file for details.

---

## Why an Orangutan?

Because network tools are boring, and orangutans are not. 🦧

Plus, they're excellent at swinging through complex environments - just like this tool swings through your network infrastructure.

---

**Built with ❤️ by 291 Group**
