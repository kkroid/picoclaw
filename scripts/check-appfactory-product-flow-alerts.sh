#!/usr/bin/env bash

set -euo pipefail

product_flow_root="${APPFACTORY_PRODUCT_FLOW_ROOT:-${1:-workspace/appfactory/product-e2e}}"
summary_json_path="${APPFACTORY_PRODUCT_FLOW_SUMMARY_JSON_OUTPUT:-${2:-${product_flow_root}/reports/product-flow-summary.json}}"
refresh_summary="${APPFACTORY_PRODUCT_FLOW_ALERT_REFRESH_SUMMARY:-1}"
require_latest_success="${APPFACTORY_PRODUCT_FLOW_ALERT_REQUIRE_LATEST_SUCCESS:-1}"
max_latest_script_seconds="${APPFACTORY_PRODUCT_FLOW_ALERT_MAX_LATEST_SCRIPT_SECONDS:-300}"
max_latest_run_seconds="${APPFACTORY_PRODUCT_FLOW_ALERT_MAX_LATEST_RUN_SECONDS:-180}"
max_latest_build_apk_seconds="${APPFACTORY_PRODUCT_FLOW_ALERT_MAX_LATEST_BUILD_APK_SECONDS:-120}"
max_warm_build_apk_p95_seconds="${APPFACTORY_PRODUCT_FLOW_ALERT_MAX_WARM_BUILD_APK_P95_SECONDS:-90}"
min_warm_build_apk_samples_for_p95="${APPFACTORY_PRODUCT_FLOW_ALERT_MIN_WARM_BUILD_APK_SAMPLES_FOR_P95:-10}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

if [[ "$refresh_summary" == "1" ]]; then
  APPFACTORY_PRODUCT_FLOW_ROOT="$product_flow_root" \
  APPFACTORY_PRODUCT_FLOW_SUMMARY_JSON_OUTPUT="$summary_json_path" \
    bash scripts/update-appfactory-product-flow-summary.sh >/dev/null
fi

python3 - "$summary_json_path" "$require_latest_success" "$max_latest_script_seconds" "$max_latest_run_seconds" "$max_latest_build_apk_seconds" "$max_warm_build_apk_p95_seconds" "$min_warm_build_apk_samples_for_p95" <<'PY'
import json
import sys
from pathlib import Path

summary_path = Path(sys.argv[1]).expanduser()
require_latest_success = sys.argv[2] == "1"
max_latest_script_seconds = float(sys.argv[3] or 0)
max_latest_run_seconds = float(sys.argv[4] or 0)
max_latest_build_apk_seconds = float(sys.argv[5] or 0)
max_warm_build_apk_p95_seconds = float(sys.argv[6] or 0)
min_warm_build_apk_samples_for_p95 = int(sys.argv[7] or 0)

if not summary_path.is_file():
    print(f"product flow summary not found: {summary_path}", file=sys.stderr)
    sys.exit(1)

payload = json.loads(summary_path.read_text(encoding="utf-8"))
latest = payload.get("latest_run") or {}
warm_build = ((payload.get("build_apk_duration_seconds") or {}).get("warm") or {})
violations = []

latest_status = str(latest.get("job_status") or "")
latest_script = latest.get("script_duration_seconds")
latest_run = latest.get("run_duration_seconds")
latest_build = latest.get("build_apk_duration_seconds")
warm_build_p95 = warm_build.get("p95_seconds")
warm_build_count = int(warm_build.get("count") or 0)

if require_latest_success and latest_status != "completed":
    violations.append(f"latest run not completed: status={latest_status or 'unknown'}")
if max_latest_script_seconds > 0 and isinstance(latest_script, (int, float)) and latest_script >= max_latest_script_seconds:
    violations.append(f"latest script duration threshold hit: {latest_script} >= {max_latest_script_seconds}")
if max_latest_run_seconds > 0 and isinstance(latest_run, (int, float)) and latest_run >= max_latest_run_seconds:
    violations.append(f"latest run duration threshold hit: {latest_run} >= {max_latest_run_seconds}")
if max_latest_build_apk_seconds > 0 and isinstance(latest_build, (int, float)) and latest_build >= max_latest_build_apk_seconds:
    violations.append(f"latest build apk duration threshold hit: {latest_build} >= {max_latest_build_apk_seconds}")
if max_warm_build_apk_p95_seconds > 0 and warm_build_count >= min_warm_build_apk_samples_for_p95 and isinstance(warm_build_p95, (int, float)) and warm_build_p95 >= max_warm_build_apk_p95_seconds:
    violations.append(f"warm build apk p95 threshold hit: {warm_build_p95} >= {max_warm_build_apk_p95_seconds}")

print(f"summary_json={summary_path.as_posix()}")
print(f"product_flow_run_count={payload.get('run_count', 0)}")
print(f"latest_job_id={latest.get('job_id', '')}")
print(f"latest_job_status={latest_status}")
print(f"latest_cache_mode_before={latest.get('cache_mode_before', '')}")
print(f"latest_build_apk_duration_seconds={latest_build}")
print(f"warm_build_apk_sample_count={warm_build_count}")
print(f"warm_build_apk_p95_seconds={warm_build_p95}")
print(f"warm_build_apk_p95_gate_min_samples={min_warm_build_apk_samples_for_p95}")

if violations:
    print("product_flow_alert_status=triggered")
    for violation in violations:
        print(f"product_flow_alert_violation={violation}")
    sys.exit(1)

print("product_flow_alert_status=ok")
PY