#!/usr/bin/env bash

set -euo pipefail

platform_root="${APPFACTORY_PLATFORM_REGRESSION_ROOT:-${1:-workspace/appfactory/platform-regression}}"
summary_json_path="${APPFACTORY_PLATFORM_REGRESSION_SUMMARY_JSON_OUTPUT:-${2:-${platform_root}/reports/platform-regression-summary.json}}"
refresh_summary="${APPFACTORY_PLATFORM_REGRESSION_ALERT_REFRESH_SUMMARY:-1}"
require_latest_product_flow_success="${APPFACTORY_PLATFORM_REGRESSION_ALERT_REQUIRE_LATEST_PRODUCT_FLOW_SUCCESS:-1}"
require_latest_jobs_regression_success="${APPFACTORY_PLATFORM_REGRESSION_ALERT_REQUIRE_LATEST_JOBS_REGRESSION_SUCCESS:-1}"
require_latest_builder_runtime_success="${APPFACTORY_PLATFORM_REGRESSION_ALERT_REQUIRE_LATEST_BUILDER_RUNTIME_SUCCESS:-1}"
require_latest_device_success_when_enabled="${APPFACTORY_PLATFORM_REGRESSION_ALERT_REQUIRE_LATEST_DEVICE_SUCCESS_WHEN_ENABLED:-1}"
max_failed_runs_per_stage="${APPFACTORY_PLATFORM_REGRESSION_ALERT_MAX_FAILED_RUNS_PER_STAGE:-0}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

if [[ "$refresh_summary" == "1" ]]; then
  APPFACTORY_PLATFORM_REGRESSION_ROOT="$platform_root" \
  APPFACTORY_PLATFORM_REGRESSION_SUMMARY_JSON_OUTPUT="$summary_json_path" \
    bash scripts/update-appfactory-platform-regression-summary.sh >/dev/null
fi

python3 - "$summary_json_path" "$require_latest_product_flow_success" "$require_latest_jobs_regression_success" "$require_latest_builder_runtime_success" "$require_latest_device_success_when_enabled" "$max_failed_runs_per_stage" <<'PY'
import json
import sys
from pathlib import Path

summary_path = Path(sys.argv[1]).expanduser()
require_latest_product_flow_success = sys.argv[2] == "1"
require_latest_jobs_regression_success = sys.argv[3] == "1"
require_latest_builder_runtime_success = sys.argv[4] == "1"
require_latest_device_success_when_enabled = sys.argv[5] == "1"
max_failed_runs_per_stage = int(sys.argv[6] or 0)

if not summary_path.is_file():
    print(f"platform regression summary not found: {summary_path}", file=sys.stderr)
    sys.exit(1)

payload = json.loads(summary_path.read_text(encoding="utf-8"))
latest = payload.get("latest_run") or {}
latest_stages = latest.get("stages") or {}
stage_summary = payload.get("stage_summary") or {}
violations = []

product_flow = latest_stages.get("product_flow") or {}
jobs_regression = latest_stages.get("jobs_regression") or {}
builder_runtime_real = latest_stages.get("builder_runtime_real") or {}
builder_runtime_summary = latest_stages.get("builder_runtime_summary") or {}
device_regression = latest_stages.get("device_regression") or {}

if require_latest_product_flow_success and product_flow.get("enabled") and product_flow.get("status") != "passed":
    violations.append(f"latest product_flow not passed: {product_flow.get('status', 'unknown')}")
if require_latest_jobs_regression_success and jobs_regression.get("enabled") and jobs_regression.get("status") != "passed":
    violations.append(f"latest jobs_regression not passed: {jobs_regression.get('status', 'unknown')}")
if require_latest_builder_runtime_success and builder_runtime_real.get("enabled") and builder_runtime_real.get("status") != "passed":
    violations.append(f"latest builder_runtime_real not passed: {builder_runtime_real.get('status', 'unknown')}")
if require_latest_builder_runtime_success and builder_runtime_summary.get("enabled") and builder_runtime_summary.get("status") != "passed":
    violations.append(f"latest builder_runtime_summary not passed: {builder_runtime_summary.get('status', 'unknown')}")
if require_latest_device_success_when_enabled and device_regression.get("enabled") and device_regression.get("status") != "passed":
    violations.append(f"latest device_regression enabled but not passed: {device_regression.get('status', 'unknown')}")

if max_failed_runs_per_stage > 0:
    for stage_name, summary in stage_summary.items():
        failed_count = int((summary.get("counts") or {}).get("failed") or 0)
        if failed_count >= max_failed_runs_per_stage:
            violations.append(f"failed count threshold hit: {stage_name}={failed_count} >= {max_failed_runs_per_stage}")

print(f"summary_json={summary_path.as_posix()}")
print(f"platform_regression_run_count={payload.get('run_count', 0)}")
print(f"latest_run_id={latest.get('run_id', '')}")
print(f"latest_product_flow_status={product_flow.get('status', '')}")
print(f"latest_jobs_regression_status={jobs_regression.get('status', '')}")
print(f"latest_builder_runtime_real_status={builder_runtime_real.get('status', '')}")
print(f"latest_builder_runtime_summary_status={builder_runtime_summary.get('status', '')}")
print(f"latest_device_regression_enabled={device_regression.get('enabled', False)}")
print(f"latest_device_regression_status={device_regression.get('status', '')}")

if violations:
    print("platform_regression_alert_status=triggered")
    for violation in violations:
        print(f"platform_regression_alert_violation={violation}")
    sys.exit(1)

print("platform_regression_alert_status=ok")
PY