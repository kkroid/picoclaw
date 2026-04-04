#!/usr/bin/env bash

set -euo pipefail

config_path="${APPFACTORY_DEVICE_POOL_CONFIG:-config/appfactory-device-pool.json}"
state_root="${APPFACTORY_DEVICE_POOL_STATE_ROOT:-workspace/appfactory/device-pool}"
regression_root="${APPFACTORY_DEVICE_REGRESSION_ROOT:-workspace/appfactory/device-regression}"
status_json_output="${APPFACTORY_DEVICE_POOL_STATUS_JSON_OUTPUT:-${state_root}/status.json}"
status_markdown_output="${APPFACTORY_DEVICE_POOL_STATUS_MARKDOWN_OUTPUT:-${state_root}/status.md}"
lease_ttl_seconds="${APPFACTORY_DEVICE_POOL_LEASE_TTL_SECONDS:-21600}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$config_path" "$state_root" "$regression_root" "$status_json_output" "$status_markdown_output" "$lease_ttl_seconds" <<'PY'
import json
import re
import sys
import time
from datetime import datetime, timezone
from pathlib import Path


def iso_now(ts: float) -> str:
    return datetime.fromtimestamp(ts, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def sanitize_filename(serial: str) -> str:
    return re.sub(r"[^A-Za-z0-9._-]", "_", serial)


def load_json(path: Path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return None


config_path = Path(sys.argv[1]).expanduser()
state_root = Path(sys.argv[2]).expanduser()
regression_root = Path(sys.argv[3]).expanduser()
status_json_output = Path(sys.argv[4]).expanduser()
status_markdown_output = Path(sys.argv[5]).expanduser()
lease_ttl_seconds = int(sys.argv[6])
generated_at = iso_now(time.time())

if not config_path.is_file():
    print(f"missing device pool config: {config_path.as_posix()}", file=sys.stderr)
    raise SystemExit(1)

config = load_json(config_path)
devices = config.get("devices") if isinstance(config, dict) else None
if not isinstance(devices, list):
    print(f"invalid device pool config: {config_path.as_posix()}", file=sys.stderr)
    raise SystemExit(1)

leases_dir = state_root / "leases"
now_ts = time.time()

latest_by_serial = {}
for result_path in sorted(regression_root.glob("runs/*/regression-result.json"), reverse=True):
    payload = load_json(result_path)
    if not isinstance(payload, dict):
        continue
    device = payload.get("device")
    if not isinstance(device, dict):
        continue
    serial = str(device.get("serial") or "").strip()
    if not serial or serial in latest_by_serial:
        continue
    latest_by_serial[serial] = {
        "run_id": payload.get("run_id"),
        "mode": payload.get("mode"),
        "status": payload.get("status"),
        "alert_status": payload.get("alert_status"),
        "result_path": result_path.as_posix(),
        "generated_at": payload.get("generated_at"),
    }

items = []
leased_count = 0
available_count = 0

for raw_device in devices:
    if not isinstance(raw_device, dict):
        continue
    serial = str(raw_device.get("serial") or "").strip()
    if not serial:
        continue
    tags = [str(tag).strip() for tag in raw_device.get("tags", []) if str(tag).strip()]
    lease_path = leases_dir / f"{sanitize_filename(serial)}.json"
    lease = load_json(lease_path) if lease_path.exists() else None
    lease_status = "available"
    if isinstance(lease, dict):
        expires_at = lease.get("expires_at")
        updated_at = lease.get("updated_at")
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
        lease_status = "stale" if stale else "leased"

    status = "disabled" if raw_device.get("enabled", True) is False else lease_status
    if status == "leased":
        leased_count += 1
    elif status == "available":
        available_count += 1

    items.append({
        "serial": serial,
        "label": str(raw_device.get("label") or serial),
        "tags": tags,
        "status": status,
        "adb_server_socket": str(raw_device.get("adb_server_socket") or ""),
        "docker_args": str(raw_device.get("docker_args") or ""),
        "lease": lease if isinstance(lease, dict) else None,
        "last_regression": latest_by_serial.get(serial),
    })

payload = {
    "generated_at": generated_at,
    "config_path": config_path.as_posix(),
    "state_root": state_root.as_posix(),
    "regression_root": regression_root.as_posix(),
    "device_count": len(items),
    "leased_count": leased_count,
    "available_count": available_count,
    "items": items,
}

status_json_output.parent.mkdir(parents=True, exist_ok=True)
status_json_output.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

lines = [
    "# AppFactory 固定设备池状态",
    "",
    f"- 生成时间：{generated_at}",
    f"- 配置文件：{config_path.as_posix()}",
    f"- 状态目录：{state_root.as_posix()}",
    f"- 回归归档目录：{regression_root.as_posix()}",
    f"- 设备总数：{len(items)}",
    f"- 可用设备数：{available_count}",
    f"- 已租约设备数：{leased_count}",
    "",
    "| label | serial | status | tags | last_run_id | last_status | last_alert |",
    "|---|---|---|---|---|---|---|",
]

for item in items:
    last_regression = item.get("last_regression") or {}
    lines.append(
        "| {label} | {serial} | {status} | {tags} | {run_id} | {run_status} | {alert_status} |".format(
            label=item.get("label") or "-",
            serial=item.get("serial") or "-",
            status=item.get("status") or "-",
            tags=",".join(item.get("tags") or []) or "-",
            run_id=last_regression.get("run_id") or "-",
            run_status=last_regression.get("status") or "-",
            alert_status=last_regression.get("alert_status") or "-",
        )
    )

status_markdown_output.parent.mkdir(parents=True, exist_ok=True)
status_markdown_output.write_text("\n".join(lines) + "\n", encoding="utf-8")

print(status_json_output.as_posix())
print(status_markdown_output.as_posix())
PY