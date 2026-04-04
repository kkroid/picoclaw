#!/usr/bin/env bash

set -euo pipefail

product_latest_json="${APPFACTORY_INTERNAL_TRIAL_PRODUCT_LATEST_JSON:-workspace/appfactory/product-e2e/latest.json}"
platform_latest_json="${APPFACTORY_INTERNAL_TRIAL_PLATFORM_LATEST_JSON:-workspace/appfactory/platform-regression/latest.json}"
device_index_json="${APPFACTORY_INTERNAL_TRIAL_DEVICE_INDEX_JSON:-workspace/appfactory/device-regression/index.json}"
device_pool_config="${APPFACTORY_INTERNAL_TRIAL_DEVICE_POOL_CONFIG:-config/appfactory-device-pool.json}"
require_device="${APPFACTORY_INTERNAL_TRIAL_REQUIRE_DEVICE:-auto}"
max_product_age_hours="${APPFACTORY_INTERNAL_TRIAL_MAX_PRODUCT_FLOW_AGE_HOURS:-24}"
max_platform_age_hours="${APPFACTORY_INTERNAL_TRIAL_MAX_PLATFORM_AGE_HOURS:-24}"
max_device_age_hours="${APPFACTORY_INTERNAL_TRIAL_MAX_DEVICE_AGE_HOURS:-24}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$product_latest_json" "$platform_latest_json" "$device_index_json" "$device_pool_config" "$require_device" "$max_product_age_hours" "$max_platform_age_hours" "$max_device_age_hours" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def read_json(path_str: str):
    path = Path(path_str).expanduser()
    if not path.is_file():
        raise FileNotFoundError(path)
    return path, json.loads(path.read_text(encoding="utf-8"))


def parse_time(value):
    if not value:
        return None
    text = str(value)
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    return datetime.fromisoformat(text)


def age_hours(dt):
    if dt is None:
        return None
    delta = datetime.now(timezone.utc) - dt.astimezone(timezone.utc)
    return round(delta.total_seconds() / 3600, 3)


def bool_from_string(value: str):
    lowered = value.strip().lower()
    if lowered in {"1", "true", "yes", "on"}:
        return True
    if lowered in {"0", "false", "no", "off"}:
        return False
    raise ValueError(f"unsupported boolean value: {value}")


(
    product_latest_json,
    platform_latest_json,
    device_index_json,
    device_pool_config,
    require_device_raw,
    max_product_age_hours,
    max_platform_age_hours,
    max_device_age_hours,
) = sys.argv[1:]

max_product_age_hours = float(max_product_age_hours)
max_platform_age_hours = float(max_platform_age_hours)
max_device_age_hours = float(max_device_age_hours)

product_path, product = read_json(product_latest_json)
platform_path, platform = read_json(platform_latest_json)

require_device_reason = require_device_raw
if require_device_raw == "auto":
    config_path = Path(device_pool_config).expanduser()
    if config_path.is_file():
        payload = json.loads(config_path.read_text(encoding="utf-8"))
        devices = payload.get("devices") if isinstance(payload, dict) else []
        require_device = any(
            isinstance(item, dict)
            and str(item.get("serial") or "").strip()
            and item.get("enabled", True) is not False
            for item in devices
        )
        require_device_reason = f"auto:{'enabled-pool' if require_device else 'no-enabled-device'}"
    else:
        require_device = False
        require_device_reason = "auto:no-config"
else:
    require_device = bool_from_string(require_device_raw)
    require_device_reason = f"explicit:{str(require_device).lower()}"

violations = []

product_time = parse_time(product.get("script_finished_at") or product.get("run_finished_at") or product.get("script_started_at"))
product_age = age_hours(product_time)
product_status = str(product.get("job_status") or "")

if product_status != "completed":
    violations.append(f"product flow latest not completed: {product_status or 'unknown'}")
if product_age is None:
    violations.append("product flow latest missing finished timestamp")
elif max_product_age_hours > 0 and product_age > max_product_age_hours:
    violations.append(f"product flow latest too old: {product_age}h > {max_product_age_hours}h")

platform_time = parse_time(platform.get("generated_at"))
platform_age = age_hours(platform_time)
platform_stages = platform.get("stages") or {}

if platform_age is None:
    violations.append("platform latest missing generated_at")
elif max_platform_age_hours > 0 and platform_age > max_platform_age_hours:
    violations.append(f"platform latest too old: {platform_age}h > {max_platform_age_hours}h")

for stage_name, stage_payload in platform_stages.items():
    if stage_payload.get("enabled") and stage_payload.get("status") != "passed":
        violations.append(f"platform latest stage not passed: {stage_name}={stage_payload.get('status', 'unknown')}")

device_age = None
device_status = "skipped"
device_run_id = ""

if require_device:
    device_path, device_index = read_json(device_index_json)
    items = device_index.get("items") or []
    latest_device = items[0] if items else None
    if not latest_device:
      violations.append("device regression latest missing")
    else:
        device_time = parse_time(latest_device.get("generated_at"))
        device_age = age_hours(device_time)
        device_status = str(latest_device.get("status") or "")
        device_run_id = str(latest_device.get("run_id") or "")
        if device_status != "passed":
            violations.append(f"device regression latest not passed: {device_status or 'unknown'}")
        if device_age is None:
            violations.append("device regression latest missing generated_at")
        elif max_device_age_hours > 0 and device_age > max_device_age_hours:
            violations.append(f"device regression latest too old: {device_age}h > {max_device_age_hours}h")
else:
    device_path = None

print(f"product_latest_json={product_path.as_posix()}")
print(f"product_latest_job_id={product.get('job_id', '')}")
print(f"product_latest_status={product_status}")
print(f"product_latest_age_hours={product_age}")
print(f"platform_latest_json={platform_path.as_posix()}")
print(f"platform_latest_run_id={platform.get('run_id', '')}")
print(f"platform_latest_age_hours={platform_age}")
print(f"require_device={str(require_device).lower()}")
print(f"require_device_reason={require_device_reason}")
if require_device:
    print(f"device_index_json={device_path.as_posix()}")
    print(f"device_latest_run_id={device_run_id}")
    print(f"device_latest_status={device_status}")
    print(f"device_latest_age_hours={device_age}")

if violations:
    print("internal_trial_freshness_status=stale")
    for violation in violations:
        print(f"internal_trial_freshness_violation={violation}")
    sys.exit(1)

print("internal_trial_freshness_status=ok")
PY