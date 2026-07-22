# Security Policy

LAN Orangutan scans your network and stores a record of what it finds, so it is worth being clear about what it does, what it does not do, and how to report a problem.

## Reporting a vulnerability

Please report security issues privately rather than opening a public issue.

- Use [GitHub's private vulnerability reporting](https://github.com/291-Group/LAN-Orangutan/security/advisories/new)

Please include what you found, how to reproduce it, and what an attacker could do with it. We will acknowledge your report within a few days and keep you updated while we work on a fix.

Please do not test against networks or devices you do not own or have permission to scan.

## Supported versions

Fixes are released for the most recent version. Older releases are not patched, unless otherwise stated by 291 Group.

## How access is protected

**The dashboard listens on your network by default.** LAN Orangutan is normally installed on a server or a Raspberry Pi and opened from another machine, so restricting it to the local machine would break the usual setup.

**Nothing is reachable until a password exists.** On first run, every page and every API endpoint refuses the request and sends you to a page that creates a password. There is no default password, and none is generated for you.

**Passwords are stored hashed.** bcrypt, in a file readable only by its owner, kept separately from your config file so that completing setup never rewrites a file you maintain by hand. A password supplied through the config file or the environment may be given as plain text or as a bcrypt hash.

**Sessions** are random 256-bit tokens held in memory, sent as an `HttpOnly` cookie with `SameSite=Lax`, and marked `Secure` when served over HTTPS. Signing out invalidates the session on the server, not just in the browser. Sessions are lost on restart, so everyone signs in again. Setting a new password invalidates all existing sessions.

**Repeated failed logins** are limited to five per address per fifteen minutes, keyed on the address rather than the connection, so reconnecting does not reset the count.

**Mutating API requests** must present a JSON content type or an `X-Requested-With` header, so a form on another site cannot make your browser change your data.

## Known limitations

Worth understanding before you deploy it.

**No HTTPS.** Traffic between your browser and LAN Orangutan is unencrypted, including your password at sign-in. On a home network this is usually accepted. If it is not acceptable for you, put it behind a reverse proxy that terminates TLS, or reach it over Tailscale or a VPN.

**It does not open a port on your router.** LAN Orangutan contains no UPnP or NAT-PMP code and makes no outbound connections at all, so it cannot ask your router to expose it. Behind a normal home router, a machine on your LAN is not reachable from the internet unless you deliberately forward a port.

**Two situations where it would be reachable from the internet.** Running it on a machine that already has a public address, such as a VPS or cloud instance, publishes it the moment it starts. And a machine with a globally routable IPv6 address may be reachable directly, because IPv6 usually has no NAT: whether it is depends on your router's IPv6 firewall. `bind_address = 0.0.0.0` listens on IPv4 only, so IPv6 is not published unless you ask for it with `bind_address = ::`. In either case the setup screen still stands in the way, but do not rely on that alone on a public host: set a password up front with `ORANGUTAN_PASSWORD`, or bind to loopback and reach it over a VPN.

**Whoever opens it first sets the password.** Between the server starting and someone completing setup, anyone who can reach it could claim it. On a home network the window is small and this is the same trade-off Home Assistant and Portainer make, but on an untrusted network you should set `ORANGUTAN_PASSWORD` in advance instead.

**One shared password.** There are no user accounts, roles, or per-user permissions. Anyone with the password has full control.

**`allow_insecure` disables authentication completely.** It exists for people running behind a proxy that already handles access control. Setting it on an otherwise open network leaves your device list readable and writable by anyone.

**Scanning needs elevated privileges to be useful.** Without `sudo` or Administrator, nmap cannot read the ARP table, so you get IP addresses without MAC addresses or vendor names. Running as root is a trade-off you are choosing; it is not required for the app to run.

**Your data is not encrypted at rest.** The device list is plain JSON in the data directory. It records what is on your network, when it was seen, and any labels or notes you add. Protect that directory as you would any other private file.

## What LAN Orangutan does not do

- It does not phone home, send telemetry, or contact any external service
- It does not require an account or an internet connection
- Vendor lookups are done from a list built into the binary, not by querying an online service
