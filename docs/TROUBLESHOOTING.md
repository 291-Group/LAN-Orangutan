# Troubleshooting

## Service won't start

```bash
sudo systemctl status lan-orangutan
sudo journalctl -u lan-orangutan -n 50
```

## Port in use

```bash
sudo ss -tulnp | grep :291
```

Change port in `/etc/lan-orangutan/config.ini`

## No devices found

```bash
# Check nmap
which nmap
sudo nmap -sn 192.168.1.0/24

# Test scanner
sudo python3 /opt/lan-orangutan/scanner/scan.py 192.168.1.0/24
```

## Can't access web UI

```bash
# Check service
sudo systemctl status lan-orangutan

# Check firewall
sudo ufw status
sudo ufw allow 291/tcp

# Test locally
curl http://localhost:291/
```

## Tailscale not detected

```bash
which tailscale
tailscale status
```

## View logs

```bash
sudo journalctl -u lan-orangutan -f
```
