#!/usr/bin/env bash

set -euo pipefail

mode="${APPFACTORY_DEVICE_REGRESSION_MODE:-${1:-fast}}"
regression_root="${APPFACTORY_DEVICE_REGRESSION_ROOT:-workspace/appfactory/device-regression}"
run_alerts="${APPFACTORY_DEVICE_REGRESSION_RUN_ALERTS:-1}"
fail_on_alert="${APPFACTORY_DEVICE_REGRESSION_FAIL_ON_ALERT:-1}"
keep_temp="${APPFACTORY_DEVICE_REGRESSION_KEEP_TEMP:-0}"
verify_command_override="${APPFACTORY_DEVICE_REGRESSION_VERIFY_COMMAND:-}"
device_pool_enabled="${APPFACTORY_DEVICE_POOL_ENABLED:-0}"
device_pool_config="${APPFACTORY_DEVICE_POOL_CONFIG:-config/appfactory-device-pool.json}"
device_pool_state_root="${APPFACTORY_DEVICE_POOL_STATE_ROOT:-workspace/appfactory/device-pool}"
device_pool_status_json="${APPFACTORY_DEVICE_POOL_STATUS_JSON_OUTPUT:-${device_pool_state_root}/status.json}"
device_pool_status_markdown="${APPFACTORY_DEVICE_POOL_STATUS_MARKDOWN_OUTPUT:-${device_pool_state_root}/status.md}"
device_pool_require_tags="${APPFACTORY_DEVICE_POOL_REQUIRE_TAGS:-}"
device_pool_prefer_tags="${APPFACTORY_DEVICE_POOL_PREFER_TAGS:-}"
device_pool_lease_ttl_seconds="${APPFACTORY_DEVICE_POOL_LEASE_TTL_SECONDS:-21600}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

case "$mode" in
  full)
    default_verify_command="make verify-appfactory-public-job-device"
    ;;
  fast)
    default_verify_command="make verify-appfactory-public-job-device-fast"
    ;;
  *)
    echo "unsupported regression mode: $mode" >&2
    exit 1
    ;;
esac

verify_command="${verify_command_override:-$default_verify_command}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-${mode}"
run_dir="$regression_root/runs/$run_id"
artifacts_dir="$run_dir/artifacts"
mkdir -p "$artifacts_dir"

verify_log="$run_dir/verify.log"
alert_log="$run_dir/alert.log"
result_json="$run_dir/regression-result.json"
latest_json="$regression_root/latest.json"
latest_log="$regression_root/latest.log"
index_json="$regression_root/index.json"
latest_markdown="$regression_root/latest.md"

device_pool_serial="${APPFACTORY_DEVICE_SERIAL:-}"
device_pool_label=""
device_pool_tags=""
device_pool_adb_server_socket="${ADB_SERVER_SOCKET:-}"
device_pool_docker_args="${APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS:-}"
device_pool_lease_file=""
device_pool_owner_id=""
device_pool_claimed_at=""
device_pool_expires_at=""
device_pool_claim_status="disabled"
device_pool_released=0

extract_last_value_from_file() {
  local key="$1"
  local file_path="$2"
  local value
  value="$(grep -E "^${key}=" "$file_path" | tail -n1 | sed "s/^${key}=//")"
  printf '%s' "$value"
}

release_device_pool_lease() {
  if [[ "$device_pool_enabled" != "1" || "$device_pool_released" == "1" || -z "$device_pool_lease_file" ]]; then
    return
  fi
  APPFACTORY_DEVICE_POOL_LEASE_FILE="$device_pool_lease_file" \
  APPFACTORY_DEVICE_POOL_SERIAL="$device_pool_serial" \
  APPFACTORY_DEVICE_POOL_STATE_ROOT="$device_pool_state_root" \
  APPFACTORY_DEVICE_POOL_OWNER="$device_pool_owner_id" \
    bash scripts/release-appfactory-device.sh >/dev/null || true
  device_pool_released=1
}

refresh_device_pool_status() {
  if [[ ! -f "$device_pool_config" ]]; then
    return
  fi
  APPFACTORY_DEVICE_POOL_CONFIG="$device_pool_config" \
  APPFACTORY_DEVICE_POOL_STATE_ROOT="$device_pool_state_root" \
  APPFACTORY_DEVICE_POOL_STATUS_JSON_OUTPUT="$device_pool_status_json" \
  APPFACTORY_DEVICE_POOL_STATUS_MARKDOWN_OUTPUT="$device_pool_status_markdown" \
  APPFACTORY_DEVICE_REGRESSION_ROOT="$regression_root" \
  APPFACTORY_DEVICE_POOL_LEASE_TTL_SECONDS="$device_pool_lease_ttl_seconds" \
    bash scripts/update-appfactory-device-pool-status.sh >/dev/null || true
}

cleanup() {
  release_device_pool_lease
  refresh_device_pool_status
}

trap cleanup EXIT

if [[ "$device_pool_enabled" == "1" && -z "$device_pool_require_tags" && -z "${APPFACTORY_DEVICE_SERIAL:-}" ]]; then
  device_pool_require_tags="$mode"
fi

echo "regression_run_id=$run_id"
echo "regression_mode=$mode"
echo "regression_root=$regression_root"
echo "verify_command=$verify_command"

if [[ "$device_pool_enabled" == "1" ]]; then
  claim_log="$run_dir/device-pool-claim.log"
  device_pool_owner_id="device-regression:$run_id"
  echo "device_pool_enabled=1"
  echo "device_pool_config=$device_pool_config"
  set +e
  APPFACTORY_DEVICE_POOL_CONFIG="$device_pool_config" \
  APPFACTORY_DEVICE_POOL_STATE_ROOT="$device_pool_state_root" \
  APPFACTORY_DEVICE_POOL_REQUESTED_SERIAL="${APPFACTORY_DEVICE_SERIAL:-}" \
  APPFACTORY_DEVICE_POOL_REQUIRE_TAGS="$device_pool_require_tags" \
  APPFACTORY_DEVICE_POOL_PREFER_TAGS="$device_pool_prefer_tags" \
  APPFACTORY_DEVICE_POOL_LEASE_TTL_SECONDS="$device_pool_lease_ttl_seconds" \
  APPFACTORY_DEVICE_POOL_OWNER="$device_pool_owner_id" \
    bash scripts/claim-appfactory-device.sh 2>&1 | tee "$claim_log"
  claim_exit_code=${PIPESTATUS[0]}
  set -e
  if [[ "$claim_exit_code" != "0" ]]; then
    exit "$claim_exit_code"
  fi
  device_pool_claim_status="$(extract_last_value_from_file device_pool_status "$claim_log")"
  device_pool_serial="$(extract_last_value_from_file device_pool_serial "$claim_log")"
  device_pool_label="$(extract_last_value_from_file device_pool_label "$claim_log")"
  device_pool_tags="$(extract_last_value_from_file device_pool_tags "$claim_log")"
  claimed_adb_server_socket="$(extract_last_value_from_file device_pool_adb_server_socket "$claim_log")"
  claimed_docker_args="$(extract_last_value_from_file device_pool_docker_args "$claim_log")"
  device_pool_lease_file="$(extract_last_value_from_file device_pool_lease_file "$claim_log")"
  device_pool_owner_id="$(extract_last_value_from_file device_pool_owner_id "$claim_log")"
  device_pool_claimed_at="$(extract_last_value_from_file device_pool_claimed_at "$claim_log")"
  device_pool_expires_at="$(extract_last_value_from_file device_pool_expires_at "$claim_log")"
  if [[ -n "$claimed_adb_server_socket" ]]; then
    device_pool_adb_server_socket="$claimed_adb_server_socket"
  fi
  if [[ -n "$claimed_docker_args" ]]; then
    device_pool_docker_args="$claimed_docker_args"
  fi
  export APPFACTORY_DEVICE_SERIAL="$device_pool_serial"
  export APPFACTORY_DEVICE_POOL_LABEL="$device_pool_label"
  if [[ -n "$device_pool_adb_server_socket" ]]; then
    export ADB_SERVER_SOCKET="$device_pool_adb_server_socket"
  fi
  if [[ -n "$device_pool_docker_args" ]]; then
    if [[ -n "${APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS:-}" ]]; then
      export APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS="${APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS} $device_pool_docker_args"
    else
      export APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS="$device_pool_docker_args"
    fi
  fi
fi

set +e
KEEP_GENERATED_WORKSPACE=1 \
APPFACTORY_DEVICE_SUMMARIZE_METRICS=1 \
VERIFY_APPFACTORY_ENTRYPOINT="run-appfactory-device-regression:$mode" \
  bash -lc "$verify_command" 2>&1 | tee "$verify_log"
verify_exit_code=${PIPESTATUS[0]}
set -e

extract_last_value() {
  local key="$1"
  extract_last_value_from_file "$key" "$verify_log"
}

workspace_path="$(extract_last_value workspace)"
apk_path="$(extract_last_value apk)"
device_logcat_path="$(extract_last_value device_logcat)"
device_screenshot_path="$(extract_last_value device_screenshot)"
device_failure_summary_path="$(extract_last_value device_failure_summary)"
device_failure_summary_json_path="$(extract_last_value device_failure_summary_json)"
preserved_temp_dir="$(extract_last_value preserved_temp_dir)"
verify_device_serial="$(extract_last_value device_serial)"
verify_adb_server_socket="$(extract_last_value adb_server_socket)"

if [[ -n "$verify_device_serial" ]]; then
  device_pool_serial="$verify_device_serial"
fi
if [[ -n "$verify_adb_server_socket" ]]; then
  device_pool_adb_server_socket="$verify_adb_server_socket"
fi

copy_if_exists() {
  local source_path="$1"
  local target_name="$2"
  if [[ -z "$source_path" || ! -f "$source_path" ]]; then
    return
  fi
  cp "$source_path" "$artifacts_dir/$target_name"
}

copy_if_exists "$apk_path" "app-debug.apk"
copy_if_exists "$device_logcat_path" "device-logcat.txt"
copy_if_exists "$device_screenshot_path" "device-screenshot.png"
copy_if_exists "$device_failure_summary_path" "device-failure-summary.md"
copy_if_exists "$device_failure_summary_json_path" "device-failure-summary.json"

alert_exit_code=""
alert_status="skipped"
if [[ "$verify_exit_code" == "0" && "$run_alerts" == "1" ]]; then
  set +e
  bash scripts/check-appfactory-device-alerts.sh 2>&1 | tee "$alert_log"
  alert_exit_code=${PIPESTATUS[0]}
  set -e
  alert_status="$(grep -E '^device_alert_status=' "$alert_log" | tail -n1 | sed 's/^device_alert_status=//')"
else
  : > "$alert_log"
fi

python3 - "$result_json" <<'PY' \
  "$run_id" "$mode" "$verify_command" "$verify_exit_code" "$alert_exit_code" "$alert_status" "$workspace_path" "$apk_path" "$device_logcat_path" "$device_screenshot_path" "$device_failure_summary_path" "$device_failure_summary_json_path" "$preserved_temp_dir" "$run_dir" "$device_pool_enabled" "$device_pool_claim_status" "$device_pool_serial" "$device_pool_label" "$device_pool_tags" "$device_pool_adb_server_socket" "$device_pool_docker_args" "$device_pool_lease_file" "$device_pool_owner_id" "$device_pool_claimed_at" "$device_pool_expires_at"
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

result_path = Path(sys.argv[1])
(
    run_id,
    mode,
    verify_command,
    verify_exit_code,
    alert_exit_code,
    alert_status,
    workspace_path,
    apk_path,
    device_logcat_path,
    device_screenshot_path,
    device_failure_summary_path,
    device_failure_summary_json_path,
    preserved_temp_dir,
    run_dir,
    device_pool_enabled,
    device_pool_claim_status,
    device_pool_serial,
    device_pool_label,
    device_pool_tags,
    device_pool_adb_server_socket,
    device_pool_docker_args,
    device_pool_lease_file,
    device_pool_owner_id,
    device_pool_claimed_at,
    device_pool_expires_at,
) = sys.argv[2:]

payload = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "run_id": run_id,
    "mode": mode,
    "verify_command": verify_command,
    "status": "passed" if verify_exit_code == "0" else "failed",
    "verify_exit_code": int(verify_exit_code or 0),
    "alert_status": alert_status or "skipped",
    "alert_exit_code": int(alert_exit_code) if alert_exit_code not in ("", None) else None,
    "run_dir": run_dir,
    "workspace_path": workspace_path or None,
    "preserved_temp_dir": preserved_temp_dir or None,
    "device": {
      "pool_enabled": device_pool_enabled == "1",
      "claim_status": device_pool_claim_status or ("manual" if device_pool_serial else "disabled"),
      "serial": device_pool_serial or None,
      "label": device_pool_label or None,
      "tags": [tag for tag in device_pool_tags.split(",") if tag],
      "adb_server_socket": device_pool_adb_server_socket or None,
      "docker_args": device_pool_docker_args or None,
      "lease_file": device_pool_lease_file or None,
      "owner_id": device_pool_owner_id or None,
      "claimed_at": device_pool_claimed_at or None,
      "expires_at": device_pool_expires_at or None,
    },
    "artifacts": {
        "apk": apk_path or None,
        "device_logcat": device_logcat_path or None,
        "device_screenshot": device_screenshot_path or None,
        "device_failure_summary": device_failure_summary_path or None,
        "device_failure_summary_json": device_failure_summary_json_path or None,
    },
}
result_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

cp "$result_json" "$latest_json"
cp "$verify_log" "$latest_log"

release_device_pool_lease

APPFACTORY_DEVICE_REGRESSION_ROOT="$regression_root" \
APPFACTORY_DEVICE_REGRESSION_INDEX_OUTPUT="$index_json" \
APPFACTORY_DEVICE_REGRESSION_LATEST_MARKDOWN="$latest_markdown" \
  bash scripts/update-appfactory-device-regression-index.sh >/dev/null

refresh_device_pool_status

if [[ "$keep_temp" != "1" && -n "$preserved_temp_dir" ]]; then
  case "$preserved_temp_dir" in
    /tmp/picoclaw-api-test-*)
      rm -rf "$preserved_temp_dir" 2>/dev/null || true
      ;;
  esac
fi

echo "regression_result_json=$result_json"
echo "regression_index_json=$index_json"
echo "regression_latest_markdown=$latest_markdown"
if [[ -n "$device_pool_serial" ]]; then
  echo "regression_device_serial=$device_pool_serial"
fi
if [[ -f "$device_pool_status_json" ]]; then
  echo "device_pool_status_json=$device_pool_status_json"
fi
if [[ -f "$device_pool_status_markdown" ]]; then
  echo "device_pool_status_markdown=$device_pool_status_markdown"
fi
echo "regression_verify_exit_code=$verify_exit_code"
if [[ -n "$alert_exit_code" ]]; then
  echo "regression_alert_exit_code=$alert_exit_code"
fi

if [[ "$verify_exit_code" != "0" ]]; then
  exit "$verify_exit_code"
fi
if [[ "$run_alerts" == "1" && "$fail_on_alert" == "1" && -n "$alert_exit_code" && "$alert_exit_code" != "0" ]]; then
  exit "$alert_exit_code"
fi