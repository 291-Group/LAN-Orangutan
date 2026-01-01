# Troubleshooting

## No MAC addresses or vendor info

MAC addresses and vendor information require elevated privileges to read the ARP table.

**Solution:** Run with sudo (or as Administrator on Windows):

```bash
# Linux/macOS
sudo orangutan scan
sudo orangutan serve

# Windows - run Command Prompt as Administrator
orangutan.exe scan
```

## Service won't start

```bash
# Check service status
sudo systemctl status lan-orangutan

# View logs
sudo journalctl -u lan-orangutan -n 50

# Check if binary exists and is executable
ls -la /usr/local/bin/orangutan
```

## Port already in use

```bash
# Check what's using the port
sudo ss -tulnp | grep :291

# Use a different port
orangutan serve --port 8080
```

Or change the port in your config file.

## No devices found

1. **Check nmap is installed:**
   ```bash
   which nmap
   nmap --version
   ```

2. **Test nmap directly:**
   ```bash
   sudo nmap -sn 192.168.1.0/24
   ```

3. **Check network detection:**
   ```bash
   orangutan status
   ```

4. **Scan a specific network:**
   ```bash
   sudo orangutan scan 192.168.1.0/24
   ```

## Can't access web UI

1. **Check if server is running:**
   ```bash
   orangutan status
   # or
   curl http://localhost:291/
   ```

2. **Check firewall:**
   ```bash
   # Linux (ufw)
   sudo ufw status
   sudo ufw allow 291/tcp

   # Linux (firewalld)
   sudo firewall-cmd --list-ports
   sudo firewall-cmd --add-port=291/tcp --permanent
   ```

3. **Check if binding to correct address:**
   ```bash
   # Allow external access
   orangutan serve --bind 0.0.0.0
   ```

## Tailscale not detected

1. **Check Tailscale is installed and running:**
   ```bash
   tailscale status
   ```

2. **On macOS, ensure CLI is available:**
   ```bash
   # If using Tailscale app from App Store
   /Applications/Tailscale.app/Contents/MacOS/Tailscale status
   ```

## Rate limiting errors

If you see "Rate limited" errors when scanning, wait 30 seconds between scans. This prevents overloading your network.

## Data not persisting

Check that the data directory exists and is writable:

```bash
# Linux
ls -la ~/.local/share/lan-orangutan/
# or as root
ls -la /var/lib/lan-orangutan/

# macOS
ls -la ~/Library/Application\ Support/lan-orangutan/
```

## View debug output

```bash
# Run with verbose output
orangutan scan -v

# Check version
orangutan version
```

## Still having issues?

1. Check the [GitHub Issues](https://github.com/291-Group/LAN-Orangutan/issues)
2. Open a new issue with:
   - Your OS and version
   - Output of `orangutan version`
   - Output of `orangutan status`
   - Steps to reproduce the problem
