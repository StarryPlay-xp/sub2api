#!/usr/bin/env python3
"""Prepare a private one-time import plan, without starting a tunnel."""
import argparse
import ipaddress
import json
import os
from pathlib import Path

REMOVED = {"Table", "FwMark", "PostUp", "PostDown"}
FIELDS = {
    "[Interface]": {"PrivateKey", "Address", "DNS", "MTU", "ListenPort"},
    "[Peer]": {"PublicKey", "PresharedKey", "Endpoint", "AllowedIPs", "PersistentKeepalive"},
}


def adapt(text, dns):
    ipaddress.ip_address(dns)
    section = ""
    seen = []
    output = []
    for line in text.splitlines():
        line = line.split("#", 1)[0].strip()
        if not line:
            continue
        if line.startswith("["):
            if line not in FIELDS or line in seen:
                raise ValueError("Expected one Interface and one Peer")
            section = line
            seen.append(line)
            output.append(line)
            continue
        if "=" not in line or section not in FIELDS:
            raise ValueError("Invalid WireGuard configuration syntax")
        key, value = (part.strip() for part in line.split("=", 1))
        if key in REMOVED:
            continue  # Never execute shell hooks or alter system routing.
        if key not in FIELDS[section]:
            raise ValueError("Unsupported WireGuard configuration field")
        output.append(f"{key} = {value}")
    if seen != ["[Interface]", "[Peer]"]:
        raise ValueError("Expected Interface followed by Peer")
    if not any(line.startswith("DNS = ") for line in output):
        output.insert(output.index("[Peer]"), f"DNS = {dns}")
    return "\n".join(output) + "\n"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--account-ids", required=True)
    parser.add_argument("--name", default="wg-b")
    parser.add_argument("--dns", default="1.1.1.1")
    args = parser.parse_args()
    try:
        ids = sorted(int(value) for value in args.account_ids.split(","))
        if not ids or len(ids) > 100 or min(ids) < 1 or len(ids) != len(set(ids)):
            raise ValueError("Invalid account IDs")
        raw = Path(args.source).read_text()
        if len(raw.encode()) > 32768:
            raise ValueError("Configuration is too large")
        plan = {"name": args.name, "account_ids": ids, "config": adapt(raw, args.dns)}
        # Refuse overwrite; never print configuration or keys.
        fd = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as stream:
            json.dump(plan, stream)
        print("Private bootstrap plan prepared; no tunnel or business request was started")
    except (OSError, ValueError):
        raise SystemExit("Could not prepare plan; check input format, account IDs and output file (details withheld)")


if __name__ == "__main__":
    main()
