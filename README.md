# 🦧 LAN Orangutan 

<p align="left">
  <img src="https://291grp.com/assets/badges/stars.svg?v=1" alt="Stars" height="20">
  <img src="https://291grp.com/assets/badges/forks.svg?v=1" alt="Forks" height="20">
  <img src="https://291grp.com/assets/badges/release.svg?v=1" alt="Release" height="20">
  <img src="https://291grp.com/assets/badges/build.svg?v=1" alt="Build" height="20">
  <img src="https://291grp.com/assets/badges/license.svg?v=1" alt="License" height="20">&nbsp;
  <a href="https://github.com/avelino/awesome-go#utilities"><img src="https://291grp.com/assets/awards/mentioned-in-awesome.svg" alt="Mentioned in Awesome" height="20"></a>&nbsp;&nbsp;<a href="https://devhunt.org/tool/lan-orangutan"><img src="https://291grp.com/assets/awards/devhunt.svg" alt="DevHunt - 2nd Product of the Week" height="20"></a>
</p>

<p align="left">
  <img alt="Go" src="https://img.shields.io/badge/go-1.27%2B-00ADD8.svg">
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-linux%20%7C%20macos%20%7C%20windows%20%7C%20pi-lightgrey.svg">
  <a href="https://hub.docker.com/r/291group/lan-orangutan"><img alt="Docker" src="https://img.shields.io/docker/v/291group/lan-orangutan?label=docker&color=2496ED"></a>
</p>


<a href="https://trendshift.io/repositories/17888?utm_source=trendshift-badge&amp;utm_medium=badge&amp;utm_campaign=badge-trendshift-17888" target="_blank" rel="noopener noreferrer"><img src="https://trendshift.io/api/badge/trendshift/repositories/17888/daily?language=Go" alt="291-Group%2FLAN-Orangutan | Trendshift" width="250" height="55"/></a>

**Self-hosted network discovery for homelabbers with Tailscale support.**

Scan your networks, discover devices, label and track them - all from a clean web UI or CLI.

By [291 Group](https://291group.com)

<p align="center">
  <img src="docs/LO1.png" width="48%" />
  <img src="docs/LO2.png" width="48%" />
</p>

## Why the name
"Orangutan" is Malay for "person of the forest" [orang, person; hutan, forest]. LAN Orangutan is the person of your network. It sits quietly in your spanning tree, watches everything that moves, remembers every device it has ever met, tells you what is on your network and what has changed, and bothers no one. Orangutans are also the most solitary of the great apes: no troop, no cloud, no phoning home.

## Features

- Auto-discover devices using nmap, plus devices already in this machine's ARP table that don't answer a scan<br>
- IPv4 and IPv6, with a device seen on both shown once<br>
- Password protected, with a password you set the first time you open it<br>
- Label, group, and add notes to devices, with your own group names and locations<br>
- Rename a device's hostname or set its type yourself, and scans keep your choice<br>
- Save a snapshot of your network and compare it later to see what changed<br>
- Mark the devices you expect, and get told when something new appears or something goes missing<br>
- Notifications by webhook to Telegram, Discord, Slack, ntfy, Home Assistant or anything that takes JSON<br>
- Back up and restore everything, or export a shareable HTML report of your network<br>
- Change scanning and network settings from the Settings page, no config file needed<br>
- Multi-network support, with faster scans of networks reached through a router or VPN, tuned automatically<br>
- Tailscale integration - connect, disconnect, and discover tailnet peers from inside the app<br>
- Live scan progress you can cancel<br>
- Modern web dashboard with light/dark mode<br>
- Full CLI with JSON output, able to do everything the web interface can<br>
- REST API with tokens, for scripts, dashboards and agents<br>
- Available in 16 languages, including Arabic and Hebrew right to left<br>
- Cross-platform (Linux, macOS, Windows)<br>
- Single binary, with nmap the only thing it needs alongside it<br>

## Quick Start

### Download

Grab the latest release of LAN Orangutan for your platform from [GitHub Releases](https://github.com/291-Group/LAN-Orangutan/releases).

### Package managers

```bash
# macOS (Homebrew)
brew install lan-orangutan

# Windows (Scoop)
scoop bucket add 291group https://github.com/291-Group/scoop-bucket
scoop install lan-orangutan
```

Both channels update on every release. Linux packages (`.deb`, `.rpm`, `.apk`) are on the [Releases](https://github.com/291-Group/LAN-Orangutan/releases) page.

### Docker

Pull the image from Docker Hub:

```bash
docker pull 291group/lan-orangutan
```

Or use the compose file for the full setup with host networking and a persistent data folder:

```bash
curl -O https://raw.githubusercontent.com/291-Group/LAN-Orangutan/main/docker-compose.yml
docker compose up -d
```

Then open `http://<that-machine>:291` and create a password. Your data is kept in a `data/` folder next to the compose file.

The image is published to both [Docker Hub](https://hub.docker.com/r/291group/lan-orangutan) (`291group/lan-orangutan`) and [GitHub Container Registry](https://github.com/291-Group/LAN-Orangutan/pkgs/container/lan-orangutan) (`ghcr.io/291-group/lan-orangutan`). The compose file uses GHCR; pull from whichever you prefer.

**Docker requires Linux.** The container uses host networking, because on Docker's own private network it would only ever see other containers (`172.17.0.0/16`) rather than the devices on your LAN. Docker Desktop on macOS and Windows runs Linux inside a virtual machine, so host networking attaches to that VM instead of your computer: the dashboard is unreachable and a scan finds nothing but the VM. On macOS and Windows, download the binary and run it directly, as described below.

### Run

```bash
# Linux/macOS: extract, then run with sudo for full device info (MAC addresses, vendors).
# The file is named for your platform, for example orangutan-linux-amd64.
tar xzf orangutan-linux-amd64.tar.gz
chmod +x orangutan-linux-amd64
sudo ./orangutan-linux-amd64 serve

# Windows (run as Administrator for full device info)
orangutan-windows-amd64.exe serve
```

Open `http://localhost:291` in your browser, or `http://<that-machine>:291` from another computer on your network.

The first time you open it, LAN Orangutan asks you to create a password. Nothing else is reachable until you do. That is the whole setup.

### Upgrading

Upgrading to 4.0 needs nothing extra. Install it the same way you installed your current version, and your devices, groups, locations, labels, settings and password all carry over. From 4.0, version numbers follow semantic versioning.

#### From the Python and PHP version (before 3.1.3)

If you installed LAN Orangutan with `install.sh` before 3.1.3, you were running a Python and PHP version of the app from `/opt/lan-orangutan`. Running the new `install.sh` replaces it with the single Go binary at `/usr/local/bin/orangutan` and removes that old directory.

**Your config and your device list are kept.** Nothing in `/etc/lan-orangutan` or `/var/lib/lan-orangutan` is touched.

Two things change that you will notice:

- The dashboard now asks you to create a password the first time you open it after upgrading. Until you do, nothing else is reachable.
- PHP and Python are no longer needed. You can remove them if you installed them only for LAN Orangutan.

Upgrading through a package (`.deb`, `.rpm`, `.apk`) or Docker needs nothing extra, since those already ran the Go binary.

### Uninstalling

Remove it the same way you installed it. Your device list, settings and password are kept in a separate data folder; delete that too if you want nothing left behind.

```bash
# macOS (Homebrew)
brew uninstall lan-orangutan

# Windows (Scoop)
scoop uninstall lan-orangutan

# Debian, Ubuntu, Raspberry Pi OS (.deb)
sudo apt remove lan-orangutan

# Fedora, RHEL (.rpm)
sudo dnf remove lan-orangutan

# Alpine (.apk)
sudo apk del lan-orangutan

# Installed with install.sh: run the uninstaller from the same download
sudo ./uninstall.sh

# Docker: run this next to your compose file, then delete its data/ folder
docker compose down
```

If you downloaded the binary and ran it directly, delete the `orangutan` file. If you set it up as a service by following [docs/INSTALL.md](docs/INSTALL.md), remove that service first: `sudo systemctl disable --now lan-orangutan` and delete `/etc/systemd/system/lan-orangutan.service` on Linux, or `launchctl unload` the plist and delete `~/Library/LaunchAgents/com.291group.lan-orangutan.plist` on macOS.

The data folder is in one of these places, unless you set `data_dir` yourself:

- Linux: `/var/lib/lan-orangutan` when run as root or as a service, otherwise `~/.local/share/lan-orangutan`
- macOS: `~/Library/Application Support/lan-orangutan`
- Windows: `%APPDATA%\lan-orangutan`

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
orangutan list --offline               # Devices not seen for an hour
orangutan list --format json           # JSON output
orangutan list --format csv            # Same columns as "orangutan export"
orangutan list --type Printer          # Only one device type
orangutan list --group Family          # Only one group

# Edit a device (anything the web edit dialog can change)
orangutan device show 192.168.1.20
orangutan device set 192.168.1.20 --hostname "Living Room TV" --group Family
orangutan device set 192.168.1.20 --hostname ""     # Back to the scanned name
orangutan device set 192.168.1.30 --type Printer --expected
orangutan device set 192.168.1.25 --location "Living Room"
orangutan device set 192.168.1.30 --label NAS --notes "Rack, shelf 2"
orangutan device delete 192.168.1.40
orangutan device expect-all            # Mark every current device as expected
orangutan device expect-all --clear    # Remove the mark from every device

# Groups
orangutan groups                       # List groups and how many devices are in each
orangutan groups add Office
orangutan groups rename Office Work    # Devices in it move with it
orangutan groups delete Work           # Devices are kept, without a group
orangutan groups reset                 # Back to the built-in groups

# Locations
orangutan locations                    # List locations and how many devices are in each
orangutan locations add Garage
orangutan locations rename "Living Room" Lounge
orangutan locations delete Garage      # Devices are kept, without a location
orangutan locations prune              # Delete every location with no devices
orangutan list --location Kitchen      # Only what is in one place

# Snapshots
orangutan snapshot save "Before new router"
orangutan snapshot list                # Newest is 1
orangutan snapshot compare 1           # What changed since snapshot 1
orangutan snapshot compare 2 1         # Between two snapshots
orangutan snapshot delete 3

# Backup, report and API token
orangutan backup                       # Devices, settings and snapshots to one file
orangutan restore backup.json --yes    # Replaces what is there now
orangutan report                       # Shareable HTML report
orangutan token create                 # Shown once; a running server picks it up
TOKEN=$(orangutan token create)        # Only the token goes to stdout
orangutan token revoke                 # Stops working immediately

# Notifications (webhooks)
orangutan webhook                      # List webhooks, numbered, with the last result
orangutan webhook add "https://api.telegram.org/bot<token>/sendMessage?chat_id=<id>" --name Phone
orangutan webhook add https://ntfy.sh/my-topic --events missing,unexpected
orangutan webhook test 1               # Send a test message and show the answer
orangutan webhook set 1 --name Phone --events new,missing
orangutan webhook disable 1            # Or enable; keeps it in the list
orangutan webhook delete 1

# Export
orangutan export devices.csv           # Export to CSV
orangutan export devices.json          # Same JSON as "list --format json"

# Check status
orangutan status                       # Show system status
orangutan config                       # Show settings in effect
orangutan settings                     # The settings the Settings page can change
orangutan settings set scan_interval 600
orangutan settings set networks 192.168.10.0/24,10.0.5.0/24
orangutan settings reset scan_interval # Back to the config file's value
orangutan networks                     # Show detected networks
orangutan version                      # Show version info
orangutan status --format json         # Most listing commands take --format json
```

The `scan all` CLI command also reads the IPv6 neighbor cache and folds dual-stack sightings into the same device entry by MAC address.

### Why sudo?

Running with `sudo` (or as Administrator on Windows) allows nmap to:
- Read the ARP table to get MAC addresses
- Look up device vendors from MAC addresses
- Get more accurate hostname resolution

Without elevated privileges, you'll still see device IPs but MAC addresses and vendors will be missing.

## Web Dashboard

The web dashboard provides:
- Real-time device status (online/offline)
- Device types (Server, Desktop, Laptop, Mobile, IoT and more), plus your own groups and locations
- Labels and notes for each device
- Search and filter devices
- Export to CSV/JSON
- Continuous scanning you can toggle on or off, keeping the view live
- Keyboard shortcuts (/ to search, R to refresh, T to toggle theme)
- A language menu in the header, and a Credits page behind the heart

### Scan progress

Scans run in the background, so the dashboard stays responsive and a long scan will not time out. Progress shows which network is being scanned, how many devices have been found, and a time estimate based on how long that network took to scan last time. Scanning a large network takes a few minutes, and you can cancel at any point.

The first scan of a network has no previous timing to estimate from, so it shows elapsed time instead of a percentage.

## Languages

LAN Orangutan is available in English, Arabic, Bengali, Chinese, French, German, Hebrew, Hindi, Italian, Japanese, Korean, Polish, Portuguese, Russian, Spanish and Ukrainian. Pick one from the language menu in the header. Your browser remembers the choice, so everyone using the same instance can have their own, and English is the default. Arabic and Hebrew use a right to left layout, and the HTML report downloaded from the dashboard comes out in the language you are using. The CLI and notification messages stay in English.

If you speak one of these languages and something reads wrong, or you would like another language added, say so in [Discussion #53](https://github.com/291-Group/LAN-Orangutan/discussions/53).

## Notifications

LAN Orangutan can post to a webhook when something changes on your network.
Add one under **Settings → Notifications**, or with `orangutan webhook add`,
and choose which events it gets:

| Event | When it fires |
|---|---|
| `device.new` | A device is seen for the first time |
| `device.unexpected` | A device you have not marked as expected comes online: a new one, or one back after being offline |
| `device.missing` | A device you marked as expected has not been seen for an hour |
| `device.moved` | A device (matched by its MAC address) shows up at a new IP address |

Each event fires once per change, not on every scan: a device that stays
online, or stays missing, is not reported again. A webhook that gets both
"new" and "unexpected" receives one message for a brand new device, not two.
The very first scan, into an empty device list, sends nothing, and no more
than 20 messages go to one webhook from a single scan. "Missing" is checked on
every scan, including the automatic ones.

Every webhook gets a JSON `POST`:

```json
{
  "event": "device.moved",
  "time": "2026-10-20T14:03:00+01:00",
  "text": "Device changed address: Living Room TV moved from 192.168.1.20 to 192.168.1.35",
  "content": "Device changed address: Living Room TV moved from 192.168.1.20 to 192.168.1.35",
  "device": {"ip": "192.168.1.35", "mac": "aa:bb:cc:dd:ee:ff", "hostname": "Living Room TV",
             "vendor": "Apple", "label": "", "group": "Mobile", "type": "Phone"},
  "previous_ip": "192.168.1.20"
}
```

The same message is in `text` and `content`, so most services show it with no
setup. Messages are always in English, whatever language the interface is in,
because they are read in another app.

| Service | URL to paste |
|---|---|
| Telegram | `https://api.telegram.org/bot<token>/sendMessage?chat_id=<chat id>` (the chat ID is also sent in the body, where Telegram reads it) |
| Discord | The channel's webhook URL (Channel settings → Integrations → Webhooks) |
| Slack | An incoming webhook URL |
| ntfy | `https://ntfy.sh/<topic>`. For ntfy.sh the message is sent as plain text with a title, so it reads cleanly. For a self-hosted ntfy server, add ntfy's templating to the URL: `https://ntfy.example.com/<topic>?tpl=1&m={{.text}}` |
| Home Assistant | A webhook trigger URL, `http://homeassistant.local:8123/api/webhook/<id>`. The fields are available as `trigger.json` |

Delivery happens in the background and never slows a scan down. Each attempt
times out after 10 seconds, and a failure (no answer, or a 5xx or 429) is
retried up to three times in all. The outcome of the last attempt is shown
next to each webhook, and **Send test** (or `orangutan webhook test`) posts a
test message straight away and shows the answer.

Webhook URLs often contain a secret, such as a bot token, so they are shown
only as their host (`https://api.telegram.org/…`) once saved. They are stored
in the state file in the data directory, and are part of a backup.

## Tailscale

Tailscale devices are picked up automatically: if Tailscale is connected, its peers are added to your device list alongside the machines found on your local networks.

Tailscale peers cannot be found by scanning, because Tailscale gives every node its own single-address network, leaving no range to sweep. Instead the peers are read from Tailscale itself, which is faster and needs no elevated privileges.

Only peers that are currently online are listed, and they are shown with their Tailscale hostname and operating system. Peers have no MAC address, so no hardware vendor is looked up for them.

You can also control Tailscale from the app. The dashboard and settings pages show whether Tailscale is connected, with a button to connect or disconnect. If the machine still needs to sign in, the app shows you the sign-in link to open. Disconnecting warns you first, since you might be reaching the dashboard over Tailscale itself. This needs the Tailscale CLI on the machine running LAN Orangutan.

**Tailscale needs a host install, not Docker.** These features rely on the Tailscale CLI being present where LAN Orangutan runs. In the standard Docker setup the container has its own filesystem and cannot see the host's Tailscale, so the card will read "Not Installed" even when the host is on your tailnet. To use Tailscale with LAN Orangutan, run the binary directly on the host rather than in a container.

## API

Every endpoint below is enabled by default (`enable_api = true`) and returns JSON
shaped as `{"success": true, "data": ...}`. A refused request returns
`{"success": false, "error": "..."}`; when the problem is with what was sent,
it also carries a stable `code` (`group.exists`, `url.scheme`, `network.invalid`
and so on) and the values that went into the message as `args`
(`{"name": "Den"}`), so a script can act on the code and the web interface can
show the message in the visitor's language.

### Getting a token

Open **Settings → API access** and create one. It is shown once, because only a
hash of it is stored, so copy it when it appears. Creating another replaces the
previous one, and Revoke turns off token access entirely.

Send it on every request:

```bash
curl -H "Authorization: Bearer lano_your-token-here" \
     http://localhost:291/api/devices
```

A token is only accepted on `/api/` paths. Browser pages still use a normal
login, so a token is no use for sharing a dashboard link.

### Reading

| Endpoint | What it returns |
|---|---|
| `GET /api/devices` | Every device: address, MAC, hostname (and your own name for it, if set), vendor, type, group, location, label, first and last seen, and whether it is `online` |
| `GET /api/device?ip=<ip>` | One device |
| `GET /api/stats` | Totals: how many devices, how many online, counts per group |
| `GET /api/networks` | The networks that would be scanned |
| `GET /api/status` | That the server is running, the current time, and the port, bind address and `enable_api` setting it was started with |
| `GET /api/groups` | The group list and how many devices are in each |
| `GET /api/snapshots` | Saved snapshots |
| `GET /api/snapshots/compare?from=<id>&to=<id\|current>` | What changed between two points in time |
| `GET /api/report` | The whole network as a self-contained HTML file |
| `GET /api/backup` | Everything, as one JSON file |
| `GET /api/scan/progress` | Progress of a scan that is running |
| `GET /api/settings` | Current effective settings |
| `GET /api/webhooks` | Notification webhooks, with URLs masked to their host, and the last delivery result of each |

### Writing

| Endpoint | What it does |
|---|---|
| `POST /api/scan/start?network=<cidr>` | Start a scan of one network, or `all` |
| `POST /api/scan/cancel` | Stop the scan in progress |
| `POST /api/device` | Update a device's `label`, `custom_hostname`, `group`, `location`, `notes`, `type` or `expected`. An empty `custom_hostname` goes back to the scanned name |
| `POST /api/devices/mark-expected` | Mark everything currently present as expected. Body `{"expected": true}`, or `false` to clear the flag from every device |
| `POST /api/groups` | Create, rename or delete a group |
| `POST /api/locations` | Create, rename or delete a location. `GET` lists them with device counts |
| `POST /api/snapshots` | Save a snapshot. `DELETE` removes one |
| `POST /api/backup/restore` | Restore from a backup file |
| `POST /api/token` | Create a token. `DELETE` revokes it |
| `POST /api/webhooks` | Add, update or delete a webhook. Takes `action` (`add`, `update`, `delete`), `id`, and `url`, `name`, `events`, `enabled` as needed |
| `POST /api/webhooks/test` | Send a test message to the webhook with this `id` and return the result |

Settings have their own endpoints: `scan/interval`, `scan/continuous`,
`scan/service-detection`, `scan/networks`, `tailscale/connect` and
`tailscale/disconnect`.

### Example

```bash
# Everything that is online right now
curl -s -H "Authorization: Bearer $ORANGUTAN_TOKEN" \
  http://localhost:291/api/devices | jq '.data[] | select(.online) | .ip'

# Scan, then see what changed since this morning's snapshot
curl -s -X POST -H "Authorization: Bearer $ORANGUTAN_TOKEN" \
  "http://localhost:291/api/scan/start?network=all"
```

## Using it with an AI assistant

[`skills/lan-orangutan/SKILL.md`](skills/lan-orangutan/SKILL.md) teaches an AI assistant to install LAN Orangutan, operate it from the CLI and connect to it through the API, including what to ask you before changing or deleting anything. It follows the open Agent Skills format: add the `lan-orangutan` folder to your assistant's skills, or give it the file's link and ask it to read it.

## Privacy

This program will not transfer any information to other networked systems unless specifically requested by the user or the person installing or operating it.

In practice: LAN Orangutan sends nothing anywhere on its own. There is no telemetry, no update check and no account. The only outbound messages are notifications to webhook addresses you add yourself. Scanning stays on the networks you choose.

## Security

LAN Orangutan listens on your network by default, because it is normally installed on a server or a Raspberry Pi and opened from another machine. To make that safe, it shows you nothing until a password exists.

**First run.** Opening the dashboard presents a "create a password" screen. Until you finish it, every page and every API endpoint is refused. There is no default password and none is generated for you. What you choose is stored as a bcrypt hash in a file readable only by its owner, never in plain text, and separately from your config file so setup never rewrites a file you maintain by hand.

**Signing in.** After that you sign in with that password and stay signed in for a week by default. Sign out is in the header. Five wrong guesses lock that address out for fifteen minutes.

**Forgot your password?** Because LAN Orangutan is self-hosted, you own the machine, so there is always a way back in. The password is a single bcrypt hash in a file named `auth` inside your data directory (`data/auth` next to the Docker Compose file, or the `data_dir` you configured). Delete that file and restart LAN Orangutan; the dashboard returns to the "create a password" screen and you set a new one. Your discovered devices, labels and settings live in separate files and are left untouched. (There is no email reset and no recovery question by design: the app has no account system and sends nothing off the machine.)

**Keeping it private instead.** Set `bind_address = 127.0.0.1` and the dashboard is only reachable from the machine it runs on. No password is asked for, because nobody else can reach it.

**Setting the password in advance.** Useful for Docker and automated installs:

```bash
ORANGUTAN_PASSWORD=your-password orangutan serve

# or keep the secret out of the environment
ORANGUTAN_PASSWORD_FILE=/run/secrets/orangutan orangutan serve
```

**Turning authentication off.** If something else already controls access, such as a reverse proxy that handles login, set `allow_insecure = true` (or pass `--allow-insecure`). This disables password protection completely, including a password you created earlier through the setup page, so only do it when access control genuinely lives elsewhere. On a network bind (`0.0.0.0`) that means anyone who can route to the host can not only read your device list but also delete devices and connect or disconnect Tailscale, all without signing in. Never combine `allow_insecure` with a network bind unless a proxy or firewall in front of it is doing the authentication.

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

Networks reached through a router or a tunnel such as WireGuard are scanned with tighter timing automatically, which makes them much faster to scan. Your local networks are scanned as before. To tune it for your own link, set `nmap_max_rtt_timeout` and `nmap_max_retries` in the config file, or the matching environment variables below.

This is not a way to make Docker work on macOS or Windows. There the container runs inside a virtual machine whose NAT answers probes on its own, so a scan reports devices that do not exist.

## Configuration

Config file location:
- Linux: `~/.config/lan-orangutan/config.ini` or `/etc/lan-orangutan/config.ini` (as root)
- macOS: `~/Library/Application Support/lan-orangutan/config.ini`
- Windows: `%APPDATA%\lan-orangutan\config.ini`

See `config.example.ini` for available options, and run `orangutan config` to print the settings actually in effect. The settings the Settings page can change can also be changed with `orangutan settings set`, which a running server picks up without a restart.

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
| `ORANGUTAN_NETWORKS` | Extra networks to scan, comma separated (see above) |
| `ORANGUTAN_EXCLUDE_NETWORKS` | Detected networks never to scan, comma separated |
| `ORANGUTAN_ONLY_CONFIGURED_NETWORKS` | `true` to scan only the networks you list, ignoring detection |
| `ORANGUTAN_CONTINUOUS_SCAN` | `false` to scan only when asked, instead of on the interval |
| `ORANGUTAN_ENABLE_SERVICE_DETECTION` | `true` to check a few well-known ports, for more precise device types |
| `ORANGUTAN_THEME` | `light`, `dark` or `auto` |
| `ORANGUTAN_NMAP_MAX_RTT_TIMEOUT` | Probe timeout on networks reached through a router or tunnel, e.g. `200ms`. Set automatically if left out |
| `ORANGUTAN_NMAP_MAX_RETRIES` | Retries on those networks, e.g. `1`. Set automatically if left out |

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

## No manual
There is no manual for LAN Orangutan, and there isn't going to be one. This README is the whole of it.

That's a rule we hold ourselves to, not a gap we haven't got around to. If you need a manual to use something, it has already failed at being simple. So when a feature turns out to need explaining, we treat that as a bug in the feature, and we change the tool rather than write the explanation.

Please hold us to that. If you had to stop and work something out, or you found something by accident that the tool should have shown you, open an issue and say so. That isn't a complaint, it's the report we want most, because it points at the exact spot where the tool wasn't obvious enough. Hold each other to it too: if you're helping someone in an issue and find yourself writing a paragraph of instructions, that paragraph is a bug report.

## Contributing
LAN Orangutan was built by 291 Group, but it belongs to the community, and it always will. Nothing would make us prouder than to see the people who use it make it better.

This is the people's tool, and the people's tool has to work for any skill level. Advanced users know what they need and have serious tools that meet it; when that's you, we'll happily point you to them. LAN Orangutan isn't trying to win on feature count or go head to head with those. It wins on getting the job done faster, and sometimes the first hour is the one that matters. It's also for everyone who doesn't have the time, or the background, for a complicated setup. This tool shouldn't make anyone contend with technology. It should make technology help them out.

So if you can make it better, please do. Found a bug, hit a problem, or have an idea? Open an [issue](https://github.com/291-Group/LAN-Orangutan/issues). Want to change something yourself? [CONTRIBUTING.md](CONTRIBUTING.md) has the short version of how. Feature requests are welcome, with one caveat that follows from everything above: if something would add real complexity or setup for the user, it's probably not a fit. We'd still rather hear the idea, so don't hold back.

## Thanks

LAN Orangutan is shaped by the people who use it. Nearly everything in recent releases started as somebody opening an issue or a discussion, and several of them went back and forth with us until the fix was right.

Thank you to [@Bluesguy1984](https://github.com/Bluesguy1984), [@chraso](https://github.com/chraso), [@dao55](https://github.com/dao55), [@dax29](https://github.com/dax29), [@dbm26](https://github.com/dbm26), [@dcchillin46](https://github.com/dcchillin46), [@devra1](https://github.com/devra1), [@Extra-Citron-7630](https://github.com/Extra-Citron-7630), [@forke24x7](https://github.com/forke24x7), [@GrafDogi](https://github.com/GrafDogi), [@JanusHoi](https://github.com/JanusHoi), [@Louai199](https://github.com/Louai199), [@luciancelmare](https://github.com/luciancelmare), [@maurognx](https://github.com/maurognx), [@msanaullahsahar](https://github.com/msanaullahsahar), [@technoplayer](https://github.com/technoplayer), [@tolga-balci](https://github.com/tolga-balci), [@wmgordon3](https://github.com/wmgordon3) and [@y90ziv](https://github.com/y90ziv).

For code contributions, thank you to [@katsugtgz](https://github.com/katsugtgz), [@dajiaohuang](https://github.com/dajiaohuang) and [@AdamMagued](https://github.com/AdamMagued).

## License

MIT License

---

Built with ❤️ by [291 Group](https://291group.com)
