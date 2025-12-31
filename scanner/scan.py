#!/usr/bin/env python3
"""
LAN Orangutan Network Scanner
Discovers devices on local networks using nmap or arp-scan
"""

import json
import sys
import subprocess
import socket
import re
import time
import os
from datetime import datetime
from pathlib import Path

STATE_FILE = "/var/lib/lan-orangutan/scan_state.json"
DEVICES_FILE = "/var/lib/lan-orangutan/devices.json"
CONFIG_FILE = "/etc/lan-orangutan/config.ini"
MIN_SCAN_INTERVAL = 30

# Common MAC vendor prefixes
MAC_VENDORS = {
    "00:50:56": "VMware", "00:0C:29": "VMware", "08:00:27": "VirtualBox",
    "52:54:00": "QEMU/KVM", "B8:27:EB": "Raspberry Pi", "DC:A6:32": "Raspberry Pi",
    "E4:5F:01": "Raspberry Pi", "28:CD:C1": "Raspberry Pi", "D8:3A:DD": "Raspberry Pi",
    "00:E0:4C": "Realtek", "00:15:5D": "Microsoft Hyper-V",
    "00:23:AE": "Dell", "00:14:22": "Dell", "18:A9:9B": "Dell",
    "3C:D9:2B": "HP", "00:1E:0B": "HP", "00:21:5A": "HP",
    "00:1F:C6": "ASUSTek", "00:1A:92": "ASUSTek", "14:DA:E9": "ASUSTek",
    "00:1C:C0": "Intel", "00:1F:3B": "Intel", "3C:A9:F4": "Intel",
    "5C:B9:01": "Ubiquiti", "00:27:22": "Ubiquiti", "04:18:D6": "Ubiquiti",
    "74:83:C2": "Ubiquiti", "FC:EC:DA": "Ubiquiti",
    "00:18:0A": "Cisco", "00:1B:2A": "Cisco", "64:F6:9D": "Cisco",
    "00:14:BF": "Linksys", "00:1A:70": "Linksys", "C0:C1:C0": "Linksys",
    "00:1F:33": "Netgear", "00:22:3F": "Netgear", "A0:63:91": "Netgear",
    "08:86:3B": "Belkin", "94:10:3E": "Belkin",
    "00:26:5A": "D-Link", "00:1E:58": "D-Link", "1C:7E:E5": "D-Link",
    "00:1D:0F": "TP-Link", "14:CC:20": "TP-Link", "50:C7:BF": "TP-Link",
    "00:25:00": "Apple", "00:26:08": "Apple", "28:CF:DA": "Apple",
    "3C:15:C2": "Apple", "5C:F9:38": "Apple", "78:31:C1": "Apple",
    "18:65:90": "Samsung", "50:01:BB": "Samsung", "94:35:0A": "Samsung",
    "2C:54:91": "Microsoft", "7C:1E:52": "Microsoft",
}


def get_mac_vendor(mac):
    if not mac:
        return "Unknown"
    mac = mac.upper().replace("-", ":")
    prefix = mac[:8]
    return MAC_VENDORS.get(prefix, "Unknown")


def reverse_dns(ip):
    try:
        return socket.gethostbyaddr(ip)[0]
    except (socket.herror, socket.gaierror, socket.timeout):
        return ""


def load_scan_state():
    try:
        if os.path.exists(STATE_FILE):
            with open(STATE_FILE, 'r') as f:
                return json.load(f)
    except:
        pass
    return {"last_scan": {}}


def save_scan_state(state):
    os.makedirs(os.path.dirname(STATE_FILE), exist_ok=True)
    with open(STATE_FILE, 'w') as f:
        json.dump(state, f)


def check_rate_limit(network):
    state = load_scan_state()
    last_scan = state.get("last_scan", {}).get(network, 0)
    elapsed = time.time() - last_scan
    if elapsed < MIN_SCAN_INTERVAL:
        return False, int(MIN_SCAN_INTERVAL - elapsed)
    return True, 0


def update_scan_time(network):
    state = load_scan_state()
    if "last_scan" not in state:
        state["last_scan"] = {}
    state["last_scan"][network] = time.time()
    save_scan_state(state)


def run_nmap(cidr):
    devices = []
    try:
        result = subprocess.run(
            ["nmap", "-sn", "-oX", "-", cidr],
            capture_output=True, text=True, timeout=300
        )
        if result.returncode != 0:
            return devices

        output = result.stdout
        host_pattern = r'<host[^>]*>(.*?)</host>'
        addr_pattern = r'<address addr="([^"]+)" addrtype="([^"]+)"'
        hostname_pattern = r'<hostname name="([^"]+)"'
        status_pattern = r'<status state="([^"]+)"'

        for host_match in re.finditer(host_pattern, output, re.DOTALL):
            host_block = host_match.group(1)
            status_match = re.search(status_pattern, host_block)
            if not status_match or status_match.group(1) != "up":
                continue

            device = {"ip": "", "mac": "", "hostname": "", "vendor": "",
                      "last_seen": datetime.now().isoformat(), "response_time": None}

            for addr_match in re.finditer(addr_pattern, host_block):
                addr, addr_type = addr_match.groups()
                if addr_type == "ipv4":
                    device["ip"] = addr
                elif addr_type == "mac":
                    device["mac"] = addr

            hostname_match = re.search(hostname_pattern, host_block)
            if hostname_match:
                device["hostname"] = hostname_match.group(1)

            if not device["hostname"] and device["ip"]:
                device["hostname"] = reverse_dns(device["ip"])

            if device["mac"]:
                device["vendor"] = get_mac_vendor(device["mac"])

            if device["ip"]:
                devices.append(device)

    except subprocess.TimeoutExpired:
        print("nmap scan timed out", file=sys.stderr)
    except FileNotFoundError:
        print("nmap not found", file=sys.stderr)
    except Exception as e:
        print(f"nmap error: {e}", file=sys.stderr)

    return devices


def run_arp_scan(cidr, interface=None):
    devices = []
    cmd = ["arp-scan"]
    if interface:
        cmd.extend(["--interface", interface])
    cmd.append(cidr)

    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
        if result.returncode != 0:
            return devices

        for line in result.stdout.split('\n'):
            parts = line.split('\t')
            if len(parts) >= 2:
                ip = parts[0].strip()
                mac = parts[1].strip() if len(parts) > 1 else ""
                vendor = parts[2].strip() if len(parts) > 2 else ""

                if not re.match(r'^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$', ip):
                    continue

                device = {
                    "ip": ip, "mac": mac, "hostname": reverse_dns(ip),
                    "vendor": vendor or get_mac_vendor(mac),
                    "last_seen": datetime.now().isoformat(), "response_time": None
                }
                devices.append(device)

    except subprocess.TimeoutExpired:
        print("arp-scan timed out", file=sys.stderr)
    except FileNotFoundError:
        print("arp-scan not found", file=sys.stderr)
    except Exception as e:
        print(f"arp-scan error: {e}", file=sys.stderr)

    return devices


def scan_network(cidr, interface=None):
    allowed, wait_time = check_rate_limit(cidr)
    if not allowed:
        return {"success": False, "error": f"Rate limited. Wait {wait_time} seconds.",
                "devices": [], "network": cidr, "timestamp": datetime.now().isoformat()}

    devices = run_nmap(cidr)
    scanner_used = "nmap"

    if not devices:
        devices = run_arp_scan(cidr, interface)
        scanner_used = "arp-scan"

    update_scan_time(cidr)

    return {"success": True, "devices": devices, "device_count": len(devices),
            "network": cidr, "scanner": scanner_used, "timestamp": datetime.now().isoformat()}


def load_devices():
    try:
        if os.path.exists(DEVICES_FILE):
            with open(DEVICES_FILE, 'r') as f:
                return json.load(f)
    except:
        backup = DEVICES_FILE + ".backup"
        if os.path.exists(backup):
            try:
                with open(backup, 'r') as f:
                    return json.load(f)
            except:
                pass
    return {"devices": {}, "networks": {}}


def save_devices(data):
    os.makedirs(os.path.dirname(DEVICES_FILE), exist_ok=True)
    if os.path.exists(DEVICES_FILE):
        try:
            os.rename(DEVICES_FILE, DEVICES_FILE + ".backup")
        except:
            pass
    with open(DEVICES_FILE, 'w') as f:
        json.dump(data, f, indent=2)


def merge_devices(existing, new_devices, network):
    if "devices" not in existing:
        existing["devices"] = {}
    if "networks" not in existing:
        existing["networks"] = {}

    for device in new_devices:
        ip = device["ip"]
        if ip in existing["devices"]:
            old = existing["devices"][ip]
            old["last_seen"] = device["last_seen"]
            old["mac"] = device["mac"] or old.get("mac", "")
            old["hostname"] = device["hostname"] or old.get("hostname", "")
            old["vendor"] = device["vendor"] or old.get("vendor", "")
        else:
            device["first_seen"] = device["last_seen"]
            device["label"] = ""
            device["notes"] = ""
            device["group"] = ""
            existing["devices"][ip] = device

    existing["networks"][network] = {
        "last_scan": datetime.now().isoformat(),
        "device_count": len(new_devices)
    }
    return existing


def main():
    if len(sys.argv) < 2:
        print(json.dumps({"success": False, "error": "Usage: scan.py <network_cidr> [interface]"}))
        sys.exit(1)

    cidr = sys.argv[1]
    interface = sys.argv[2] if len(sys.argv) > 2 else None

    if not re.match(r'^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}/\d{1,2}$', cidr):
        print(json.dumps({"success": False, "error": f"Invalid CIDR format: {cidr}"}))
        sys.exit(1)

    result = scan_network(cidr, interface)

    if result["success"] and result["devices"]:
        existing = load_devices()
        updated = merge_devices(existing, result["devices"], cidr)
        save_devices(updated)

    print(json.dumps(result, indent=2))
    sys.exit(0 if result["success"] else 1)


if __name__ == "__main__":
    main()
