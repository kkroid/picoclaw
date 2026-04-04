#!/usr/bin/env bash

set -euo pipefail

config_path="${APPFACTORY_DEVICE_POOL_CONFIG:-config/appfactory-device-pool.json}"
state_root="${APPFACTORY_DEVICE_POOL_STATE_ROOT:-workspace/appfactory/device-pool}"
requested_serial="${APPFACTORY_DEVICE_POOL_REQUESTED_SERIAL:-${APPFACTORY_DEVICE_SERIAL:-}}"
require_tags="${APPFACTORY_DEVICE_POOL_REQUIRE_TAGS:-}"
prefer_tags="${APPFACTORY_DEVICE_POOL_PREFER_TAGS:-}"
lease_ttl_seconds="${APPFACTORY_DEVICE_POOL_LEASE_TTL_SECONDS:-21600}"
owner_id="${APPFACTORY_DEVICE_POOL_OWNER:-$(hostname)-$$}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$config_path" "$state_root" "$requested_serial" "$require_tags" "$prefer_tags" "$lease_ttl_seconds" "$owner_id" <<'PY'
import json
import os
import re
import socket
import sys
import time
from datetime import datetime, timezone
from pathlib import Path


def iso_now(ts: float) -> str:
    return datetime.fromtimestamp(ts, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_csv(value: str) -> list[str]:
    if not value:
        return []
    return [item.strip() for item in value.split(",") if item.strip()]


def sanitize_filename(serial: str) -> str:
    return re.sub(r"[^A-Za-z0-9._-]", "_", serial)


config_path = Path(sys.argv[1]).expanduser()
state_root = Path(sys.argv[2]).expanduser()
requested_serial = sys.argv[3].strip()
require_tags = parse_csv(sys.argv[4])
prefer_tags = parse_csv(sys.argv[5])
lease_ttl_seconds = int(sys.argv[6])
owner_id = sys.argv[7].strip() or f"{socket.gethostname()}-{os.getpid()}"

if not config_path.is_file():
    print(f"device_pool_status=missing_config")
    print(f"device_pool_config={config_path.as_posix()}")
    raise SystemExit(1)

config = json.loads(config_path.read_text(encoding="utf-8"))
devices = config.get("devices")
if not isinstance(devices, list) or not devices:
    print("device_pool_status=invalid_config")
    print(f"device_pool_config={config_path.as_posix()}")
    raise SystemExit(1)

state_root.mkdir(parents=True, exist_ok=True)
leases_dir = state_root / "leases"
leases_dir.mkdir(parents=True, exist_ok=True)

candidates = []
for index, raw_device in enumerate(devices):
    if not isinstance(raw_device, dict):
        continue
    serial = str(raw_device.get("serial") or "").strip()
    if not serial:
        continue
    if raw_device.get("enabled", True) is False:
        continue
    tags = [str(tag).strip() for tag in raw_device.get("tags", []) if str(tag).strip()]
    tag_set = set(tags)
    if requested_serial and serial != requested_serial:
        continue
    if require_tags and not set(require_tags).issubset(tag_set):
        continue
    score = sum(1 for tag in prefer_tags if tag in tag_set)
    candidates.append(( -score, index, raw_device, tags ))

candidates.sort(key=lambda item: (item[0], item[1], str(item[2].get("label") or item[2].get("serial") or "")))

if not candidates:
    print("device_pool_status=no_matching_device")
    print(f"device_pool_config={config_path.as_posix()}")
    if requested_serial:
        print(f"device_pool_requested_serial={requested_serial}")
    if require_tags:
        print(f"device_pool_required_tags={','.join(require_tags)}")
    raise SystemExit(1)

hostname = socket.gethostname()
now_ts = time.time()

for _score, _index, device, tags in candidates:
    serial = str(device.get("serial")).strip()
    lease_file = leases_dir / f"{sanitize_filename(serial)}.json"
    reclaim_stale = False
    while True:
        now_ts = time.time()
        payload = {
            "serial": serial,
            "label": str(device.get("label") or serial),
            "tags": tags,
            "adb_server_socket": str(device.get("adb_server_socket") or ""),
            "docker_args": str(device.get("docker_args") or ""),
            "owner_id": owner_id,
            "hostname": hostname,
            "pid": os.getpid(),
            "requested_serial": requested_serial or None,
            "required_tags": require_tags,
            "preferred_tags": prefer_tags,
            "claimed_at": iso_now(now_ts),
            "updated_at": iso_now(now_ts),
            "expires_at": iso_now(now_ts + lease_ttl_seconds),
            "lease_ttl_seconds": lease_ttl_seconds,
            "config_path": config_path.as_posix(),
            "lease_file": lease_file.as_posix(),
        }
        try:
            fd = os.open(lease_file, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o644)
        except FileExistsError:
            try:
                existing = json.loads(lease_file.read_text(encoding="utf-8"))
            except Exception:
                existing = {}
            expires_at = existing.get("expires_at")
            updated_at = existing.get("updated_at")
            stale = False
            if isinstance(expires_at, str) and expires_at:
                try:
                    stale = datetime.fromisoformat(expires_at.replace("Z", "+00:00")).timestamp() <= now_ts
                except Exception:
                    stale = False
            if not stale and isinstance(updated_at, str) and updated_at:
                try:
                    stale = datetime.fromisoformat(updated_at.replace("Z", "+00:00")).timestamp() + lease_ttl_seconds <= now_ts
                except Exception:
                    stale = False
            if stale and not reclaim_stale:
                reclaim_stale = True
                try:
                    lease_file.unlink()
                except FileNotFoundError:
                    pass
                continue
            break
        else:
            with os.fdopen(fd, "w", encoding="utf-8") as handle:
                json.dump(payload, handle, ensure_ascii=False, indent=2)
                handle.write("\n")
            print("device_pool_status=acquired")
            print(f"device_pool_config={config_path.as_posix()}")
            print(f"device_pool_state_root={state_root.as_posix()}")
            print(f"device_pool_serial={serial}")
            print(f"device_pool_label={payload['label']}")
            print(f"device_pool_tags={','.join(tags)}")
            print(f"device_pool_adb_server_socket={payload['adb_server_socket']}")
            print(f"device_pool_docker_args={payload['docker_args']}")
            print(f"device_pool_owner_id={owner_id}")
            print(f"device_pool_lease_file={lease_file.as_posix()}")
            print(f"device_pool_claimed_at={payload['claimed_at']}")
            print(f"device_pool_expires_at={payload['expires_at']}")
            raise SystemExit(0)

print("device_pool_status=unavailable")
print(f"device_pool_config={config_path.as_posix()}")
print(f"device_pool_state_root={state_root.as_posix()}")
if requested_serial:
    print(f"device_pool_requested_serial={requested_serial}")
if require_tags:
    print(f"device_pool_required_tags={','.join(require_tags)}")
raise SystemExit(1)
PY