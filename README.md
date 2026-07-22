# 🦧 LAN Orangutan 

<p align="left">
  <img src="https://291grp.com/assets/badges/stars.svg?v=1" alt="Stars" height="20">
  <img src="https://291grp.com/assets/badges/forks.svg?v=1" alt="Forks" height="20">
  <img src="https://291grp.com/assets/badges/release.svg?v=1" alt="Release" height="20">
  <img src="https://291grp.com/assets/badges/build.svg?v=1" alt="Build" height="20">
  <img src="https://291grp.com/assets/badges/license.svg?v=1" alt="License" height="20">&nbsp;
  <a href="https://github.com/avelino/awesome-go#utilities"><img src="https://awesome.re/mentioned-badge-flat.svg" alt="Mentioned in Awesome" height="20"></a>
</p>


<a href="https://trendshift.io/repositories/17888?utm_source=trendshift-badge&amp;utm_medium=badge&amp;utm_campaign=badge-trendshift-17888" target="_blank" rel="noopener noreferrer"><img src="https://trendshift.io/api/badge/trendshift/repositories/17888/daily?language=Go" alt="291-Group%2FLAN-Orangutan | Trendshift" width="250" height="55"/></a>&nbsp;&nbsp;<a href="https://www.producthunt.com/products/lan-orangutan?embed=true&amp;utm_source=badge-featured&amp;utm_medium=badge&amp;utm_campaign=badge-lan-orangutan" target="_blank" rel="noopener noreferrer"><img alt="LAN Orangutan - Try LAN Orangutan, our all new lightweight network scanner. | Product Hunt" width="250" height="54" src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1203757&amp;theme=dark&amp;t=1784746217551"></a>

**Self-hosted network discovery for homelabbers.**

Scan your networks, discover devices, label and track them - all from a clean web UI or CLI.

By [291 Group](https://291group.com)

<p align="center">
  <img src="docs/LO1.png" width="48%" />
  <img src="docs/LO2.png" width="48%" />
</p>

## Features

- Auto-discover devices using nmap<br>
- Password protected, with a password you set the first time you open it<br>
- Label, group, and add notes to devices<br>
- Multi-network support<br>
- Tailscale integration - tailnet peers discovered automatically<br>
- Live scan progress you can cancel<br>
- Modern web dashboard with light/dark mode<br>
- Full CLI with JSON output<br>
- Cross-platform (Linux, macOS, Windows)<br>
- Single binary, no dependencies<br>

## Quick Start

### Download

Grab the latest release of LAN Orangutan for your platform from [GitHub Releases](https://github.com/291-Group/LAN-Orangutan/releases).

### Docker

```bash
curl -O https://raw.githubusercontent.com/291-Group/LAN-Orangutan/main/docker-compose.yml
docker compose up -d
```

Then open `http://<that-machine>:291` and create a password. Your data is kept in a `data/` folder next to the compose file.

**Docker requires Linux.** The container uses host networking, because on Docker's own private network it would only ever see other containers (`172.17.0.0/16`) rather than the devices on your LAN. Docker Desktop on macOS and Windows runs Linux inside a virtual machine, so host networking attaches to that VM instead of your computer: the dashboard is unreachable and a scan finds nothing but the VM. On macOS and Windows, download the binary and run it directly, as described below.

### Run

```bash
# Linux/macOS - Run with sudo for full device info (MAC addresses, vendors)
sudo ./orangutan serve

# Windows (run as Administrator for full device info)
orangutan.exe serve
```

Open `http://localhost:291` in your browser, or `http://<that-machine>:291` from another computer on your network.

The first time you open it, LAN Orangutan asks you to create a password. Nothing else is reachable until you do. That is the whole setup.

### Upgrading from an earlier version

If you installed LAN Orangutan with `install.sh` before this release, you were running a Python and PHP version of the app from `/opt/lan-orangutan`. Running the new `install.sh` replaces it with the single Go binary at `/usr/local/bin/orangutan` and removes that old directory.

**Your config and your device list are kept.** Nothing in `/etc/lan-orangutan` or `/var/lib/lan-orangutan` is touched.

Two things change that you will notice:

- The dashboard now asks you to create a password the first time you open it after upgrading. Until you do, nothing else is reachable.
- PHP and Python are no longer needed. You can remove them if you installed them only for LAN Orangutan.

Upgrading through a package (`.deb`, `.rpm`, `.apk`) or Docker needs nothing extra, since those already ran the Go binary.

## What works where

Discovery needs to see your real network. How you run LAN Orangutan decides whether it can.

| How you run it | Finds devices | MAC + manufacturer | Notes |
|---|---|---|---|
| Binary, Linux, with `sudo` | Yes | Yes | Everything works |
| Binary, macOS, with `sudo` | Yes | Yes | Everything works |
| Binary, Windows, as Administrator | Yes | Yes | Everything works |
| Binary, any OS, without `sudo` | Yes | No | IP addresses and hostnames only |
| `.deb` / `.rpm` / `.apk` package | Yes | Yes | Runs as a root service |
| **Docker on Linux, host networking** | **Yes** | **Yes** | The supported Docker setup |
| Docker on Linux, bridge networking | No | No | Sees only other containers |
| **Docker on macOS or Windows** | **No** | **No** | **Not supported, see below** |

**Why Docker on macOS and Windows does not work.** Docker Desktop runs Linux inside a virtual machine. The container never touches your real network, and the virtual machine's gateway answers probes on behalf of addresses that do not exist, so a scan appears to succeed while reporting devices that were never there. That is worse than finding nothing, because the result looks convincing. There is no setting that fixes this. Run the binary directly instead; it is a single file and takes one command.

LAN Orangutan detects this situation itself. If it cannot reach a real network it says so at startup and puts a warning on the dashboard, so results are never silently wrong.

## Requirements

- **nmap** must be installed:
  - macOS: `brew install nmap`
  - Ubuntu/Debian: `sudo apt install nmap`
  - Windows: Download from nmap.org

## CLI Usage

```bash
# Scan network (use sudo for MAC addresses and vendor info)
sudo orangutan scan                    # Scan default network
sudo orangutan scan 192.168.1.0/24     # Scan specific network
sudo orangutan scan all                # Scan all detected networks

# Start web server
sudo orangutan serve                   # Default port 291
sudo orangutan serve --port 8080       # Custom port
sudo orangutan serve --bind 127.0.0.1  # This machine only, no password needed
sudo orangutan serve --allow-insecure  # No password at all (see Security)

# List devices
orangutan list                         # List all devices
orangutan list --online                # List online devices only
orangutan list --format json           # JSON output

# Export
orangutan export devices.csv           # Export to CSV

# Check status
orangutan status                       # Show system status
orangutan config                       # Show settings in effect
orangutan networks                     # Show detected networks
orangutan version                      # Show version info
```

### Why sudo?

Running with `sudo` (or as Administrator on Windows) allows nmap to:
- Read the ARP table to get MAC addresses
- Look up device vendors from MAC addresses
- Get more accurate hostname resolution

Without elevated privileges, you'll still see device IPs but MAC addresses and vendors will be missing.

## Web Dashboard

The web dashboard provides:
- Real-time device status (online/offline)
- Device grouping (Server, Desktop, Laptop, Mobile, IoT, etc.)
- Labels and notes for each device
- Search and filter devices
- Export to CSV/JSON
- Auto-refresh option
- Keyboard shortcuts (/ to search, R to refresh, T to toggle theme)

### Scan progress

Scans run in the background, so the dashboard stays responsive and a long scan will not time out. Progress shows which network is being scanned, how many devices have been found, and a time estimate based on how long that network took to scan last time. Scanning a large network takes a few minutes, and you can cancel at any point.

The first scan of a network has no previous timing to estimate from, so it shows elapsed time instead of a percentage.

## Tailscale

Tailscale devices are picked up automatically: if Tailscale is connected, its peers are added to your device list alongside the machines found on your local networks.

Tailscale peers cannot be found by scanning, because Tailscale gives every node its own single-address network, leaving no range to sweep. Instead the peers are read from Tailscale itself, which is faster and needs no elevated privileges.

Only peers that are currently online are listed, and they are shown with their Tailscale hostname and operating system. Peers have no MAC address, so no hardware vendor is looked up for them.

## Security

LAN Orangutan listens on your network by default, because it is normally installed on a server or a Raspberry Pi and opened from another machine. To make that safe, it shows you nothing until a password exists.

**First run.** Opening the dashboard presents a "create a password" screen. Until you finish it, every page and every API endpoint is refused. There is no default password and none is generated for you. What you choose is stored as a bcrypt hash in a file readable only by its owner, never in plain text, and separately from your config file so setup never rewrites a file you maintain by hand.

**Signing in.** After that you sign in with that password and stay signed in for a week by default. Sign out is in the header. Five wrong guesses lock that address out for fifteen minutes.

**Keeping it private instead.** Set `bind_address = 127.0.0.1` and the dashboard is only reachable from the machine it runs on. No password is asked for, because nobody else can reach it.

**Setting the password in advance.** Useful for Docker and automated installs:

```bash
ORANGUTAN_PASSWORD=your-password orangutan serve

# or keep the secret out of the environment
ORANGUTAN_PASSWORD_FILE=/run/secrets/orangutan orangutan serve
```

**Turning authentication off.** If something else already controls access, such as a reverse proxy that handles login, set `allow_insecure = true` (or pass `--allow-insecure`). This disables password protection completely, so only do it when access control genuinely lives elsewhere.

There is no HTTPS built in, so put LAN Orangutan behind a reverse proxy or reach it over Tailscale if you need the connection encrypted. See [SECURITY.md](SECURITY.md) for the full picture, the known limitations, and how to report a vulnerability.

### Scanning a network that is not detected

LAN Orangutan finds networks by reading this machine's own interfaces. A subnet reachable through a router, or a VLAN, has no local interface and so is never offered. List those explicitly:

```bash
ORANGUTAN_NETWORKS=192.168.10.0/24,10.0.5.0/24 orangutan serve
```

or in the config file:

```ini
[scanning]
networks = 192.168.10.0/24, 10.0.5.0/24
```

They appear on the dashboard alongside the detected ones and can be scanned the same way. Results for a routed network have no MAC addresses or manufacturer names, because those come from ARP and only work on the same network segment.

This is not a way to make Docker work on macOS or Windows. There the container runs inside a virtual machine whose NAT answers probes on its own, so a scan reports devices that do not exist.

## Configuration

Config file location:
- Linux: `~/.config/lan-orangutan/config.ini` or `/etc/lan-orangutan/config.ini` (as root)
- macOS: `~/Library/Application Support/lan-orangutan/config.ini`
- Windows: `%APPDATA%\lan-orangutan\config.ini`

See `config.example.ini` for available options, and run `orangutan config` to print the settings actually in effect.

Every setting can also be supplied through the environment, which is usually easier in Docker. These override the config file.

| Variable | Purpose |
|---|---|
| `ORANGUTAN_PORT` | Port to listen on |
| `ORANGUTAN_BIND_ADDRESS` | Address to bind to |
| `ORANGUTAN_PASSWORD` | Set the password directly |
| `ORANGUTAN_PASSWORD_FILE` | Read the password from a file (wins over the above) |
| `ORANGUTAN_SESSION_HOURS` | How long a login lasts |
| `ORANGUTAN_ALLOW_INSECURE` | Skip password protection |
| `ORANGUTAN_DATA_DIR` | Where devices and settings are stored |
| `ORANGUTAN_SCAN_INTERVAL` | Auto-scan interval in seconds |
| `ORANGUTAN_NETWORKS` | Extra networks to scan, comma separated (see below) |
| `ORANGUTAN_THEME` | `light`, `dark` or `auto` |

## Building from Source

```bash
git clone https://github.com/291-Group/LAN-Orangutan.git
cd LAN-Orangutan
make build          # builds to bin/orangutan with version details baked in
```

`go build -o orangutan ./cmd/orangutan` also works. It reports the commit it was built from rather than a release number, since there is no tag to read.

Run the tests with `make test`.
## Philosophy
LAN Orangutan isn't trying to be the most powerful scanner out there, and that's the point. It's built to be the simplest and fastest one to reach for.

There are already excellent, deeply capable tools in this space, and they're the right choice when you need everything they offer. LAN Orangutan is for the other times, when you just want to scan a network now, without the setup and complexity that heavier tools bring. One binary, almost no configuration, running in seconds.

If you don't need the extra features, and the extra friction that comes with them, then this is for you. We'll keep pushing back on anything that makes it harder to use or deploy, because staying simple and fast is the whole reason it exists.

## Feedback & Contributing
Found a bug, hit a problem, or have an idea? Open an [issue](https://github.com/291-Group/LAN-Orangutan/issues) and let us know.

Feature requests are welcome too, with one caveat that follows from the philosophy above: if something would add real complexity or setup, it's probably not a fit for LAN Orangutan. But we'd still rather hear the idea, so don't hold back.

## License

MIT License

---

Built with ❤️ by [291 Group](https://291group.com)
