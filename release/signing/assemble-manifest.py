#!/usr/bin/env python3
"""Assemble manifest.json for lab/stable releases (unsigned)."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from datetime import datetime, timezone


def sha256_file(path: str) -> tuple[str, int]:
    h = hashlib.sha256()
    size = 0
    with open(path, "rb") as f:
        while True:
            chunk = f.read(1024 * 1024)
            if not chunk:
                break
            size += len(chunk)
            h.update(chunk)
    return h.hexdigest(), size


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--channel", required=True, choices=["lab", "stable"])
    p.add_argument("--version", required=True)
    p.add_argument("--commit", required=True)
    p.add_argument("--release-base", required=True)
    p.add_argument("--artifact-dir", required=True)
    p.add_argument("--output", required=True)
    args = p.parse_args()

    base = args.release_base.rstrip("/")
    prefix = f"{base}/{args.channel}/{args.version}"
    artifacts = []
    for name in sorted(os.listdir(args.artifact_dir)):
        if not name.startswith("pve-zfs-monitor-linux-"):
            continue
        path = os.path.join(args.artifact_dir, name)
        if not os.path.isfile(path):
            continue
        digest, size = sha256_file(path)
        goarch = name.removeprefix("pve-zfs-monitor-linux-")
        artifacts.append(
            {
                "filename": name,
                "goos": "linux",
                "goarch": goarch,
                "sha256": digest,
                "size": size,
                "url": f"{prefix}/{name}",
            }
        )
    if not artifacts:
        raise SystemExit("no linux artifacts found")

    doc = {
        "schema_version": 1,
        "version": args.version,
        "channel": args.channel,
        "commit": args.commit,
        "published_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "artifacts": artifacts,
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
