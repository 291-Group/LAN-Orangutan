#!/usr/bin/env python3
"""
LAN Orangutan Utilities
Network detection, Tailscale integration, and helper functions
"""

import json
import subprocess
import socket
import os
import sys
from datetime import datetime, timedelta

CONFIG_FILE = "/etc/lan-orangutan/config.ini"
DEVICES_FILE = "/var/lib/lan-orangutan/devices.json"


def get_network_interfaces():
    interfaces = []
    try:
        result = subprocess.run(["ip", "-j", "addr", "show"],
                                capture_output=True, text=True, timeout=10)
        if result.returncode == 0:
            data = json.loads(result.stdout)
            for iface in data:
                name = iface.get("ifname", "")
                if name == "lo":
                    continue
                state = iface.get("operstate", "unknown")
                for addr_info in iface.get("addr_info", []):
                    if addr_info.get("family") == "inet":
                        interfaces.append({
                            "name": name,
                            "ip": addr_info.get("local", ""),
                            "netmask": addr_info.get("prefixlen", 24),
                            "broadcast": addr_info.get("broadcast", ""),
                            "state": state,
                            "is_tailscale": name.startswith("tailscale"),
                            "is_wireless": name.startswith("wlan") or name.startswith("wlp")
                        })
    except Exception as e:
        print(f"Error getting interfaces: {e}", file=sys.stderr)
    return interfaces


def detect_networks():
    networks = []
    interfaces = get_network_interfaces()

    for iface in interfaces:
        if iface["state"] not in ("UP", "up"):
            continue

        ip = iface["ip"]
        prefix = iface["netmask"]
        ip_parts = [int(x) for x in ip.split('.')]
        mask = (0xFFFFFFFF << (32 - prefix)) & 0xFFFFFFFF
        mask_parts = [(mask >> 24) & 0xFF, (mask >> 16) & 0xFF,
                      (mask >> 8) & 0xFF, mask & 0xFF]
        net_parts = [ip_parts[i] & mask_parts[i] for i in range(4)]
        network = f"{net_parts[0]}.{net_parts[1]}.{net_parts[2]}.{net_parts[3]}/{prefix}"

        if iface["is_tailscale"]:
            friendly_name = "Tailscale"
        elif iface["is_wireless"]:
            friendly_name = f"WiFi ({iface['name']})"
        else:
            friendly_name = f"LAN ({iface['name']})"

        networks.append({
            "cidr": network, "interface": iface["name"],
            "friendly_name": friendly_name, "ip": ip,
            "is_tailscale": iface["is_tailscale"]
        })
    return networks


def get_tailscale_status():
    result = {"installed": False, "running": False, "ip": "", "hostname": "",
              "peers": 0, "exit_node": None, "advertised_routes": []}
    try:
        proc = subprocess.run(["tailscale", "status", "--json"],
                              capture_output=True, text=True, timeout=10)
        result["installed"] = True
        if proc.returncode == 0:
            data = json.loads(proc.stdout)
            result["running"] = data.get("BackendState") == "Running"
            self_info = data.get("Self", {})
            result["ip"] = self_info.get("TailscaleIPs", [""])[0] if self_info.get("TailscaleIPs") else ""
            result["hostname"] = self_info.get("HostName", "")
            result["advertised_routes"] = self_info.get("AllowedIPs", [])
            peers = data.get("Peer", {})
            result["peers"] = len(peers) if peers else 0
            exit_node_status = data.get("ExitNodeStatus")
            if exit_node_status:
                result["exit_node"] = exit_node_status.get("ID", "")
    except FileNotFoundError:
        pass
    except Exception as e:
        result["error"] = str(e)
    return result


def get_gateway():
    try:
        result = subprocess.run(["ip", "route", "show", "default"],
                                capture_output=True, text=True, timeout=5)
        if result.returncode == 0:
            parts = result.stdout.split()
            if len(parts) >= 3 and parts[0] == "default":
                return parts[2]
    except:
        pass
    return ""


def get_dns_servers():
    servers = []
    try:
        result = subprocess.run(["resolvectl", "status"],
                                capture_output=True, text=True, timeout=5)
        if result.returncode == 0:
            for line in result.stdout.split('\n'):
                if "DNS Servers:" in line:
                    servers.extend(line.split(':')[1].strip().split())
        if not servers:
            with open('/etc/resolv.conf', 'r') as f:
                for line in f:
                    if line.startswith('nameserver'):
                        servers.append(line.split()[1])
    except:
        pass
    return servers


def get_device_count():
    stats = {"total": 0, "online": 0, "new_24h": 0}
    if not os.path.exists(DEVICES_FILE):
        return stats
    try:
        with open(DEVICES_FILE, 'r') as f:
            data = json.load(f)
        devices = data.get("devices", {})
        stats["total"] = len(devices)
        now = datetime.now()
        one_hour_ago = now - timedelta(hours=1)
        one_day_ago = now - timedelta(days=1)

        for device in devices.values():
            last_seen = device.get("last_seen", "")
            first_seen = device.get("first_seen", "")
            if last_seen:
                try:
                    seen = datetime.fromisoformat(last_seen.replace('Z', '+00:00'))
                    if seen > one_hour_ago:
                        stats["online"] += 1
                except:
                    pass
            if first_seen:
                try:
                    first = datetime.fromisoformat(first_seen.replace('Z', '+00:00'))
                    if first > one_day_ago:
                        stats["new_24h"] += 1
                except:
                    pass
    except:
        pass
    return stats


def parse_config():
    config = {
        "server": {"port": 291, "bind_address": "0.0.0.0", "enable_api": True},
        "scanning": {"scan_interval": 300, "min_scan_interval": 30,
                     "enable_port_scan": False, "port_scan_range": "1-1024"},
        "networks": {},
        "storage": {"max_devices": 1000, "retention_days": 90,
                    "data_dir": "/var/lib/lan-orangutan"},
        "tailscale": {"enable": True, "auto_detect": True},
        "ui": {"theme": "auto"}
    }
    if not os.path.exists(CONFIG_FILE):
        return config
    try:
        current_section = None
        with open(CONFIG_FILE, 'r') as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith('#') or line.startswith(';'):
                    continue
                if line.startswith('[') and line.endswith(']'):
                    current_section = line[1:-1]
                    if current_section not in config:
                        config[current_section] = {}
                    continue
                if '=' in line and current_section:
                    key, value = line.split('=', 1)
                    key = key.strip()
                    value = value.strip()
                    if '#' in value:
                        value = value.split('#')[0].strip()
                    if value.lower() in ('true', 'yes', 'on'):
                        value = True
                    elif value.lower() in ('false', 'no', 'off'):
                        value = False
                    elif value.isdigit():
                        value = int(value)
                    config[current_section][key] = value
    except Exception as e:
        print(f"Error parsing config: {e}", file=sys.stderr)
    return config


def check_disk_space(path="/var/lib/lan-orangutan"):
    try:
        stat = os.statvfs(path if os.path.exists(path) else "/")
        free_bytes = stat.f_bavail * stat.f_frsize
        total_bytes = stat.f_blocks * stat.f_frsize
        return {
            "free_mb": free_bytes // (1024 * 1024),
            "total_mb": total_bytes // (1024 * 1024),
            "percent_free": (free_bytes / total_bytes * 100) if total_bytes > 0 else 0,
            "ok": free_bytes > (100 * 1024 * 1024)
        }
    except:
        return {"free_mb": 0, "total_mb": 0, "percent_free": 0, "ok": False}


if __name__ == "__main__":
    print("=== Network Interfaces ===")
    for iface in get_network_interfaces():
        print(json.dumps(iface, indent=2))
    print("\n=== Detected Networks ===")
    for net in detect_networks():
        print(json.dumps(net, indent=2))
    print("\n=== Tailscale Status ===")
    print(json.dumps(get_tailscale_status(), indent=2))
