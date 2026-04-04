#!/usr/bin/env bash

set -euo pipefail

lease_file="${APPFACTORY_DEVICE_POOL_LEASE_FILE:-}"
serial="${APPFACTORY_DEVICE_POOL_SERIAL:-${APPFACTORY_DEVICE_SERIAL:-}}"
state_root="${APPFACTORY_DEVICE_POOL_STATE_ROOT:-workspace/appfactory/device-pool}"
owner_id="${APPFACTORY_DEVICE_POOL_OWNER:-}"
force_release="${APPFACTORY_DEVICE_POOL_FORCE_RELEASE:-0}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$lease_file" "$serial" "$state_root" "$owner_id" "$force_release" <<'PY'
import json
import re
import sys
from pathlib import Path


def sanitize_filename(serial: str) -> str:
    return re.sub(r"[^A-Za-z0-9._-]", "_", serial)


lease_file_arg = sys.argv[1].strip()
serial = sys.argv[2].strip()
state_root = Path(sys.argv[3]).expanduser()
owner_id = sys.argv[4].strip()
force_release = sys.argv[5].strip() == "1"

if lease_file_arg:
    lease_path = Path(lease_file_arg).expanduser()
elif serial:
    lease_path = state_root / "leases" / f"{sanitize_filename(serial)}.json"
else:
    print("device_pool_status=release_skipped")
    raise SystemExit(0)

if not lease_path.exists():
    print("device_pool_status=lease_not_found")
    print(f"device_pool_lease_file={lease_path.as_posix()}")
    raise SystemExit(0)

try:
    payload = json.loads(lease_path.read_text(encoding="utf-8"))
except Exception:
    payload = {}

lease_owner_id = str(payload.get("owner_id") or "")
lease_serial = str(payload.get("serial") or serial)
if not force_release and owner_id and lease_owner_id and owner_id != lease_owner_id:
    print("device_pool_status=release_conflict")
    print(f"device_pool_lease_file={lease_path.as_posix()}")
    print(f"device_pool_serial={lease_serial}")
    print(f"device_pool_owner_id={lease_owner_id}")
    raise SystemExit(1)

lease_path.unlink()
print("device_pool_status=released")
print(f"device_pool_lease_file={lease_path.as_posix()}")
print(f"device_pool_serial={lease_serial}")
if lease_owner_id:
    print(f"device_pool_owner_id={lease_owner_id}")
PY