---
name: lan-orangutan
description: Install, run and operate LAN Orangutan, a self-hosted network scanner that discovers and tracks the devices on a local network. Use when the user wants to find what is on their network, install or set up LAN Orangutan, scan, list, rename, label, group or locate devices, compare snapshots, set up change notifications (webhooks), or script against its CLI or REST API.
---

# LAN Orangutan

LAN Orangutan discovers the devices on a local network with nmap and the machine's own ARP and neighbour tables, and keeps an inventory of them: addresses, MAC, manufacturer, type, and whatever names, groups, locations and notes the user adds. It is one binary with a web dashboard (port 291 by default), a CLI that can do everything the dashboard can, and a REST API.

Project and full documentation: https://github.com/291-Group/LAN-Orangutan

## Ground rules

- Scanning a network is only appropriate on networks the user owns or administers. If it is unclear whose network it is, ask.
- Ask before anything that removes or replaces data: `device delete`, `groups delete`, `groups reset`, `locations delete`, `locations prune`, `snapshot delete`, `webhook delete`, `restore`, and `token create` (which replaces the existing token).
- Never put LAN Orangutan on a public interface or forward port 291 from the internet. Never suggest `allow_insecure` unless a reverse proxy or firewall in front of it already handles authentication.
- Treat the API token and webhook URLs as secrets. Do not echo them back into the conversation, write them into shared files, or put them in command history when an environment variable will do.
- The user sets the dashboard password themselves in the browser. Do not choose one for them unless they ask for an unattended install, and then use `ORANGUTAN_PASSWORD_FILE` rather than a plain variable.

## Installing

nmap is required on every platform except Docker, where it is bundled.

| Platform | Install |
|---|---|
| macOS | `brew install nmap` then `brew install 291-group/tap/lan-orangutan` |
| Windows | Install nmap from nmap.org, then `scoop bucket add 291group https://github.com/291-Group/scoop-bucket` and `scoop install lan-orangutan` |
| Debian, Ubuntu, Raspberry Pi OS | `sudo apt install nmap`, then the `.deb` for the architecture from the Releases page. Runs as a root service |
| Fedora, RHEL | `.rpm` from the Releases page |
| Alpine | `.apk` from the Releases page |
| Docker | Linux only, with host networking: `curl -O https://raw.githubusercontent.com/291-Group/LAN-Orangutan/main/docker-compose.yml` then `docker compose up -d` |
| Anything else | The binary for the platform from https://github.com/291-Group/LAN-Orangutan/releases |

Do not install the Docker image on Docker Desktop for macOS or Windows. The container runs inside a virtual machine there, so it cannot see the real network and a scan reports devices that do not exist. Use the native binary on those systems. On Linux, the container needs `network_mode: host` plus the `NET_RAW` and `NET_ADMIN` capabilities, which the compose file already sets; on bridge networking it finds nothing.

### Running it

```bash
sudo orangutan serve          # Linux and macOS; Windows: run orangutan.exe serve as Administrator
```

Then open `http://localhost:291`, or `http://<that-machine>:291` from another computer. The first visit asks for a password, and nothing is reachable until one is set. That is the whole setup.

Root or Administrator is what lets nmap read MAC addresses and manufacturers. Without it, discovery still works but those two columns stay empty.

Useful variations:

- `orangutan serve --bind 127.0.0.1` keeps it to this machine, and then no password is needed.
- `orangutan serve --port 8080` uses another port.
- `ORANGUTAN_NETWORKS=192.168.10.0/24,10.0.5.0/24` adds networks reached through a router or VLAN, which are never detected on their own. Those results have no MAC or manufacturer, because ARP does not cross a router.

## Using the CLI

The CLI reads and writes the same data folder as the server. A running server sees device, group, location, snapshot and webhook changes straight away, and settings changes by its next scan. That only holds when both use the same folder, which means the same user and the same data directory:

- Run the CLI as the same user as the server. A server started with `sudo` or installed as a package service runs as root, so use `sudo orangutan ...` for the CLI too.
- In Docker, run the CLI inside the container: `docker exec lan-orangutan orangutan list`.
- `orangutan config` prints the `data_dir` in use. `ORANGUTAN_DATA_DIR` overrides it.
- `orangutan config` and `orangutan status` show the port and bind address from the config file. A server started with `--port` or `--bind` differs; `GET /api/status` reports what the running server actually uses.
- A "permission denied" on the device list means the data belongs to another user, usually root.

For anything a program will read, ask for JSON. Most commands take `--format json`, and `list` also takes `--format csv`.

### Seeing the network

```bash
sudo orangutan scan all                  # Scan every detected network
sudo orangutan scan 192.168.1.0/24       # Scan one network
orangutan list --format json             # Every device
orangutan list --online                  # Only what is up now
orangutan list --type Printer            # Also --group, --location, --offline
orangutan device show 192.168.1.20       # Everything stored for one device
orangutan networks                       # Networks that would be scanned
orangutan status --format json           # Tools found, storage, networks, config
```

`scan` exits with 0 when every network was scanned, 1 when nothing could be scanned, and 2 when nmap failed everywhere but the ARP table still gave a partial list.

In the CLI's JSON, `hostname` is what to show: the user's own name when one is set (`custom_hostname`), otherwise the scanned one (`scanned_hostname`). The API differs, see below. `online` is a boolean; a device counts as offline after an hour unseen.

### Organising devices

```bash
orangutan device set 192.168.1.20 --hostname "Living Room TV" --group Family --location "Living Room"
orangutan device set 192.168.1.20 --hostname ""        # Back to the scanned name
orangutan device set 192.168.1.30 --type Printer       # Or --type auto to detect again
orangutan device set 192.168.1.30 --label NAS --notes "Rack, shelf 2" --expected
orangutan device expect-all                            # Mark everything present now as expected
orangutan groups add Office
orangutan groups rename Office Work
orangutan locations add Garage
orangutan locations rename "Living Room" Lounge
orangutan groups                                       # List groups with device counts
orangutan locations                                    # List locations with device counts
orangutan device set 192.168.1.20 --group "" --location "" --expected=false   # Clear them
```

Only the flags passed are changed, and `""` clears a text field. Setting a location that does not exist yet adds it to the list, and it stays there after its last device leaves; `locations prune` removes empty ones (ask first). Types are Phone, Computer, Printer, TV, Media Player, Router, Server, IoT, Game Console, or auto. Labels are up to 64 characters and notes up to 1000. A custom hostname or type survives every later scan.

Marking devices as expected is what makes "unexpected device" and "missing device" notifications meaningful, so after a first scan on a quiet network, offer `device expect-all`.

### Tracking change

```bash
orangutan snapshot save "Before new router"
orangutan snapshot list                  # Newest is 1
orangutan snapshot compare 1             # Snapshot 1 against the network now
orangutan snapshot compare 2 1           # Between two snapshots
orangutan report                         # Self-contained HTML report of the whole network
orangutan backup                         # Devices, settings, webhooks and snapshots in one file
orangutan export devices.csv             # Or devices.json
```

### Notifications

LAN Orangutan can send a message when a new device joins (`new`), a device not marked expected comes online (`unexpected`), an expected device goes missing (`missing`), or a device changes address (`moved`). Telegram, Discord, Slack, ntfy and Home Assistant work by giving their webhook URL, and anything else that accepts JSON works too.

```bash
orangutan webhook add https://ntfy.sh/my-topic --name Phone --events missing,unexpected
orangutan webhook test 1                 # Sends a test and shows the reply
orangutan webhook                        # Numbered list with the last result of each
orangutan webhook set 1 --events new,missing
orangutan webhook disable 1              # Or enable
```

Always run `webhook test` after adding one, and report what came back. The `webhook` commands print each full URL, and a URL often carries a bot token or private topic, so summarize that output (name, host, events, last result) instead of pasting it.

### Settings

```bash
orangutan settings                       # What is in effect and where it comes from
orangutan settings set scan_interval 600
orangutan settings set networks 192.168.10.0/24,10.0.5.0/24
orangutan settings reset scan_interval   # Remove the override, so the config file's value applies
```

Keys: `continuous_scan`, `scan_interval` (10 to 86400 seconds), `enable_service_detection`, `networks`, `exclude_networks`, `only_configured_networks`, `max_devices`, `retention_days`. Out-of-range values are adjusted and the CLI says so. Server settings such as the port and bind address live in the config file and need a restart.

## Using the REST API

Use the API when the client is on another machine or is a dashboard, script or agent that should not touch the data folder.

1. Get a token: `orangutan token create` on the server (only the token goes to standard output, so `TOKEN=$(orangutan token create)` works), or Settings, API access in the dashboard. It is shown once; only a hash is stored. Creating one replaces the previous token, and `orangutan token revoke` turns token access off immediately.
2. Send it as a bearer token on every request:

```bash
curl -s -H "Authorization: Bearer $ORANGUTAN_TOKEN" http://<host>:291/api/devices
```

Every response is `{"success": true, "data": ...}`. Its JSON is not identical to the CLI's:

- `GET /api/devices` returns an object keyed by IP address, not a list.
- `hostname` is always the scanned name; the user's own name, when set, is in `custom_hostname`. Show `custom_hostname` when present, otherwise `hostname`.
- Empty and false values are left out (`custom_hostname`, `type`, `location`, `expected` and others), so treat a missing field as empty or false.
- Snapshots are addressed by their long `id` from `GET /api/snapshots`, not by the 1, 2, 3 numbering the CLI shows.
- Webhook events are written `device.new`, `device.unexpected`, `device.missing` and `device.moved`.

 A refused request is `{"success": false, "error": "..."}`, and when the problem is with the input it also carries a stable `code` (such as `group.exists` or `network.invalid`) and `args`, so act on the code rather than the message text. Tokens only work on `/api/` paths, not on dashboard pages.

Main endpoints:

| Endpoint | Purpose |
|---|---|
| `GET /api/devices` | All devices |
| `GET /api/device?ip=<ip>` | One device |
| `GET /api/stats` | Totals and per-group counts |
| `GET /api/networks` | Networks that would be scanned |
| `GET /api/status` | The running server's port, bind address and API setting |
| `GET /api/settings` | Effective settings |
| `GET` and `POST /api/scan/interval` | Read or change the scan interval, body `{"seconds": 600}` |
| `POST /api/scan/start?network=<cidr or all>` | Start a scan |
| `GET /api/scan/progress` | Progress of the running scan |
| `POST /api/scan/cancel` | Stop it |
| `POST /api/device` | Change `label`, `custom_hostname`, `group`, `location`, `notes`, `type` or `expected` for an `ip` |
| `GET` and `POST /api/snapshots` | List or save snapshots |
| `GET /api/snapshots/compare?from=<id>&to=<id or current>` | What changed |
| `GET /api/report` | HTML report |
| `GET /api/webhooks`, `POST /api/webhooks`, `POST /api/webhooks/test` | Manage and test notifications |

The README's API section lists every endpoint with its parameters.

## When something looks wrong

| Symptom | Cause and fix |
|---|---|
| No MAC addresses or manufacturers | Not running as root or Administrator, or the network is reached through a router |
| Scan finds nothing in Docker | Bridge networking, or Docker Desktop on macOS or Windows. Use host networking on Linux, or the native binary |
| Scan finds devices that do not exist | Docker Desktop on macOS or Windows. Use the native binary |
| `nmap not found` | Install nmap and make sure it is on PATH |
| A routed subnet or VLAN never appears | Add it with `orangutan settings set networks ...` |
| CLI changes do not show in the dashboard | CLI and server use different users or data folders; compare `orangutan config` |
| Forgotten dashboard password | Delete the `auth` file in the data folder and restart; devices and settings are kept |
| A device keeps the wrong name or type | A custom value is set; `--hostname ""` or `--type auto` clears it |
