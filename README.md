# 🦧 LAN Orangutan

**Self-hosted network discovery for homelabbers.**

Scan your networks, discover devices, label and track them - all from a clean web UI or CLI.

*By [291 Group](https://291group.com) - Canadian defense technology*

## Features

- 🔍 Auto-discover devices using nmap
- 🏷️ Label and categorize devices
- 🌐 Multi-network support
- 🔗 Tailscale integration
- 💻 Web dashboard with light/dark mode
- ⌨️ Full CLI
- 🍓 Raspberry Pi ready

## Quick Start

```bash
git clone https://github.com/291-Group/LAN-Orangutan.git
cd LAN-Orangutan
sudo ./install.sh
```

Open `http://your-ip:291`

## Requirements

- Ubuntu, Debian, or Raspberry Pi OS
- Python 3.7+
- PHP 7.4+
- nmap (auto-installed)

## CLI Usage

```bash
orangutan scan all           # Scan all networks
orangutan list --online      # List online devices
orangutan export devices.csv # Export to CSV
orangutan help               # Show help
```

## Configuration

Edit `/etc/lan-orangutan/config.ini`

## Uninstall

```bash
sudo /opt/lan-orangutan/uninstall.sh
```

## License

MIT License

---

Built with ❤️ by [291 Group](https://291group.com)
