# Installation Guide

## Automated (Recommended)

```bash
git clone https://github.com/291-Group/LAN-Orangutan.git
cd LAN-Orangutan
sudo ./install.sh
```

## Manual Installation

### 1. Install Dependencies

```bash
sudo apt update
sudo apt install -y python3 php-cli php-json nmap
```

### 2. Create Directories

```bash
sudo mkdir -p /opt/lan-orangutan /etc/lan-orangutan /var/lib/lan-orangutan
```

### 3. Copy Files

```bash
sudo cp -r scanner web cli scripts /opt/lan-orangutan/
sudo cp config.example.ini /etc/lan-orangutan/config.ini
```

### 4. Set Permissions

```bash
sudo chmod +x /opt/lan-orangutan/cli/orangutan
sudo chmod +x /opt/lan-orangutan/scripts/start-server.sh
sudo ln -sf /opt/lan-orangutan/cli/orangutan /usr/local/bin/orangutan
```

### 5. Install Service

```bash
sudo cp systemd/lan-orangutan.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable lan-orangutan
sudo systemctl start lan-orangutan
```

### 6. Configure Firewall

```bash
sudo ufw allow 291/tcp
```

## Verify

```bash
sudo systemctl status lan-orangutan
curl http://localhost:291/
```
