#!/usr/bin/env bash

set -euo pipefail

platform_root="${APPFACTORY_PLATFORM_REGRESSION_ROOT:-workspace/appfactory/platform-regression}"
run_product_flow="${APPFACTORY_PLATFORM_REGRESSION_RUN_PRODUCT_FLOW:-1}"
run_jobs_regression="${APPFACTORY_PLATFORM_REGRESSION_RUN_JOBS_REGRESSION:-1}"
run_builder_runtime_real="${APPFACTORY_PLATFORM_REGRESSION_RUN_BUILDER_RUNTIME_REAL:-1}"
run_device_regression="${APPFACTORY_PLATFORM_REGRESSION_RUN_DEVICE_REGRESSION:-0}"
device_regression_mode="${APPFACTORY_PLATFORM_REGRESSION_DEVICE_MODE:-fast}"
fail_fast="${APPFACTORY_PLATFORM_REGRESSION_FAIL_FAST:-1}"
auto_device_from_pool="${APPFACTORY_PLATFORM_REGRESSION_AUTO_DEVICE_FROM_POOL:-1}"
device_pool_config="${APPFACTORY_DEVICE_POOL_CONFIG:-config/appfactory-device-pool.json}"
run_alerts="${APPFACTORY_PLATFORM_REGRESSION_RUN_ALERTS:-1}"
fail_on_alert="${APPFACTORY_PLATFORM_REGRESSION_FAIL_ON_ALERT:-1}"

run_device_regression_explicit="0"
if [[ -n "${APPFACTORY_PLATFORM_REGRESSION_RUN_DEVICE_REGRESSION+x}" ]]; then
  run_device_regression_explicit="1"
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

run_id="$(date -u +%Y%m%dT%H%M%SZ)-platform"
run_dir="$platform_root/runs/$run_id"
mkdir -p "$run_dir"

product_flow_log="$run_dir/product-flow.log"
jobs_regression_log="$run_dir/jobs-regression.log"
builder_runtime_log="$run_dir/builder-runtime-real.log"
builder_summary_log="$run_dir/builder-runtime-summary.log"
device_log="$run_dir/device-regression.log"
alert_log="$run_dir/platform-alerts.log"
result_json="$run_dir/platform-regression-result.json"

echo "platform_regression_run_id=$run_id"
echo "platform_regression_root=$platform_root"
echo "run_product_flow=$run_product_flow"
echo "run_jobs_regression=$run_jobs_regression"
echo "run_builder_runtime_real=$run_builder_runtime_real"
echo "run_device_regression=$run_device_regression"
echo "run_alerts=$run_alerts"

device_regression_use_pool="0"
device_regression_auto_enabled_from_pool="0"

if [[ "$run_device_regression_explicit" != "1" && "$auto_device_from_pool" == "1" && -f "$device_pool_config" ]]; then
  if python3 - "$device_pool_config" <<'PY'
import json
import sys
from pathlib import Path

config_path = Path(sys.argv[1]).expanduser()
payload = json.loads(config_path.read_text(encoding='utf-8'))
devices = payload.get('devices') if isinstance(payload, dict) else []
enabled = [item for item in devices if isinstance(item, dict) and item.get('enabled', True) is not False and str(item.get('serial') or '').strip()]
raise SystemExit(0 if enabled else 1)
PY
  then
    run_device_regression="1"
    device_regression_use_pool="1"
    device_regression_auto_enabled_from_pool="1"
  fi
fi

echo "device_regression_use_pool=$device_regression_use_pool"
echo "device_regression_auto_enabled_from_pool=$device_regression_auto_enabled_from_pool"

stage_status_product_flow="skipped"
stage_status_jobs_regression="skipped"
stage_status_builder_runtime_real="skipped"
stage_status_builder_runtime_summary="skipped"
stage_status_device_regression="skipped"
stage_status_alerts="skipped"
stage_exit_product_flow=""
stage_exit_jobs_regression=""
stage_exit_builder_runtime_real=""
stage_exit_builder_runtime_summary=""
stage_exit_device_regression=""
stage_exit_alerts=""

execute_stage() {
  local log_file="$1"
  shift
  set +e
  bash -lc "$*" 2>&1 | tee "$log_file"
  local exit_code=${PIPESTATUS[0]}
  set -e
  return "$exit_code"
}

run_stage_with_status() {
  local __status_var="$1"
  local __exit_var="$2"
  local log_file="$3"
  shift 3

  local exit_code
  if execute_stage "$log_file" "$@"; then
    exit_code=0
    printf -v "$__status_var" '%s' "passed"
  else
    exit_code=$?
    printf -v "$__status_var" '%s' "failed"
  fi
  printf -v "$__exit_var" '%s' "$exit_code"
}

if [[ "$run_product_flow" == "1" ]]; then
  run_stage_with_status stage_status_product_flow stage_exit_product_flow "$product_flow_log" "APPFACTORY_PRODUCT_FLOW_REGRESSION_FAIL_ON_ALERT=0 make run-appfactory-product-flow-regression"
else
  : > "$product_flow_log"
fi

if [[ "$run_jobs_regression" == "1" ]]; then
  run_stage_with_status stage_status_jobs_regression stage_exit_jobs_regression "$jobs_regression_log" "make run-appfactory-jobs-regression"
else
  : > "$jobs_regression_log"
fi

if [[ "$fail_fast" == "1" && ( "$stage_exit_product_flow" != "" && "$stage_exit_product_flow" != "0" || "$stage_exit_jobs_regression" != "" && "$stage_exit_jobs_regression" != "0" ) ]]; then
  run_builder_runtime_real="0"
  run_device_regression="0"
fi

if [[ "$run_builder_runtime_real" == "1" ]]; then
  run_stage_with_status stage_status_builder_runtime_real stage_exit_builder_runtime_real "$builder_runtime_log" "make validate-builder-runtime-ollama-real"
  run_stage_with_status stage_status_builder_runtime_summary stage_exit_builder_runtime_summary "$builder_summary_log" "make summarize-builder-runtime-validation"
  if [[ "$fail_fast" == "1" && ( "$stage_exit_builder_runtime_real" != "0" || "$stage_exit_builder_runtime_summary" != "0" ) ]]; then
    run_device_regression="0"
  fi
else
  : > "$builder_runtime_log"
  : > "$builder_summary_log"
fi

if [[ "$run_device_regression" == "1" ]]; then
  device_regression_command="APPFACTORY_DEVICE_REGRESSION_MODE=$device_regression_mode bash scripts/run-appfactory-device-regression.sh"
  if [[ "$device_regression_use_pool" == "1" ]]; then
    device_regression_command="APPFACTORY_DEVICE_POOL_ENABLED=1 APPFACTORY_DEVICE_REGRESSION_MODE=$device_regression_mode bash scripts/run-appfactory-device-regression.sh"
  fi
  run_stage_with_status stage_status_device_regression stage_exit_device_regression "$device_log" "$device_regression_command"
else
  : > "$device_log"
fi

write_result_json() {
python3 - "$result_json" "$run_id" "$platform_root" "$run_dir" "$run_product_flow" "$run_jobs_regression" "$run_builder_runtime_real" "$run_device_regression" "$device_regression_mode" "$device_regression_use_pool" "$device_regression_auto_enabled_from_pool" "$device_pool_config" "$stage_status_product_flow" "$stage_exit_product_flow" "$stage_status_jobs_regression" "$stage_exit_jobs_regression" "$stage_status_builder_runtime_real" "$stage_exit_builder_runtime_real" "$stage_status_builder_runtime_summary" "$stage_exit_builder_runtime_summary" "$stage_status_device_regression" "$stage_exit_device_regression" "$stage_status_alerts" "$stage_exit_alerts" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

(
    result_path,
    run_id,
    platform_root,
    run_dir,
    run_product_flow,
    run_jobs_regression,
    run_builder_runtime_real,
    run_device_regression,
    device_regression_mode,
    device_regression_use_pool,
    device_regression_auto_enabled_from_pool,
    device_pool_config,
    stage_status_product_flow,
    stage_exit_product_flow,
    stage_status_jobs_regression,
    stage_exit_jobs_regression,
    stage_status_builder_runtime_real,
    stage_exit_builder_runtime_real,
    stage_status_builder_runtime_summary,
    stage_exit_builder_runtime_summary,
    stage_status_device_regression,
    stage_exit_device_regression,
    stage_status_alerts,
    stage_exit_alerts,
) = sys.argv[1:]

payload = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "run_id": run_id,
    "platform_root": platform_root,
    "run_dir": run_dir,
    "stages": {
        "product_flow": {
            "enabled": run_product_flow == "1",
            "status": stage_status_product_flow,
            "exit_code": int(stage_exit_product_flow) if stage_exit_product_flow else None,
        },
        "jobs_regression": {
          "enabled": run_jobs_regression == "1",
          "status": stage_status_jobs_regression,
          "exit_code": int(stage_exit_jobs_regression) if stage_exit_jobs_regression else None,
        },
        "builder_runtime_real": {
            "enabled": run_builder_runtime_real == "1",
            "status": stage_status_builder_runtime_real,
            "exit_code": int(stage_exit_builder_runtime_real) if stage_exit_builder_runtime_real else None,
        },
        "builder_runtime_summary": {
            "enabled": run_builder_runtime_real == "1",
            "status": stage_status_builder_runtime_summary,
            "exit_code": int(stage_exit_builder_runtime_summary) if stage_exit_builder_runtime_summary else None,
        },
        "device_regression": {
            "enabled": run_device_regression == "1",
            "mode": device_regression_mode,
          "use_pool": device_regression_use_pool == "1",
          "auto_enabled_from_pool": device_regression_auto_enabled_from_pool == "1",
          "device_pool_config": device_pool_config,
            "status": stage_status_device_regression,
            "exit_code": int(stage_exit_device_regression) if stage_exit_device_regression else None,
        },
        "alerts": {
          "enabled": True,
          "status": stage_status_alerts,
          "exit_code": int(stage_exit_alerts) if stage_exit_alerts else None,
        },
    },
}
Path(result_path).write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(payload, ensure_ascii=False, indent=2))
PY
}

write_result_json

APPFACTORY_PLATFORM_REGRESSION_ROOT="$platform_root" bash scripts/update-appfactory-platform-regression-summary.sh >/dev/null || true

if [[ "$run_alerts" == "1" ]]; then
  run_stage_with_status stage_status_alerts stage_exit_alerts "$alert_log" "bash scripts/check-appfactory-platform-regression-alerts.sh"
else
  : > "$alert_log"
fi

write_result_json

APPFACTORY_PLATFORM_REGRESSION_ROOT="$platform_root" bash scripts/update-appfactory-platform-regression-summary.sh >/dev/null || true

if [[ "$stage_exit_product_flow" != "" && "$stage_exit_product_flow" != "0" ]]; then
  exit "$stage_exit_product_flow"
fi
if [[ "$stage_exit_jobs_regression" != "" && "$stage_exit_jobs_regression" != "0" ]]; then
  exit "$stage_exit_jobs_regression"
fi
if [[ "$stage_exit_builder_runtime_real" != "" && "$stage_exit_builder_runtime_real" != "0" ]]; then
  exit "$stage_exit_builder_runtime_real"
fi
if [[ "$stage_exit_builder_runtime_summary" != "" && "$stage_exit_builder_runtime_summary" != "0" ]]; then
  exit "$stage_exit_builder_runtime_summary"
fi
if [[ "$stage_exit_device_regression" != "" && "$stage_exit_device_regression" != "0" ]]; then
  exit "$stage_exit_device_regression"
fi
if [[ "$run_alerts" == "1" && "$fail_on_alert" == "1" && "$stage_exit_alerts" != "" && "$stage_exit_alerts" != "0" ]]; then
  exit "$stage_exit_alerts"
fi