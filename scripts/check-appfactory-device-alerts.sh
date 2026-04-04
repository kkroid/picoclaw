#!/usr/bin/env bash

set -euo pipefail

metrics_root="${APPFACTORY_DEVICE_METRICS_ROOT:-${1:-workspace/appfactory}}"
summary_json_path="${APPFACTORY_DEVICE_METRICS_JSON_OUTPUT:-${2:-${metrics_root}/reports/device-failure-summary.json}}"
refresh_summary="${APPFACTORY_DEVICE_ALERT_REFRESH_SUMMARY:-1}"
max_total_count="${APPFACTORY_DEVICE_ALERT_MAX_TOTAL_COUNT:-0}"
max_run_count="${APPFACTORY_DEVICE_ALERT_MAX_RUN_COUNT:-0}"
focus_categories_raw="${APPFACTORY_DEVICE_ALERT_FOCUS_CATEGORIES:-}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

if [[ "$refresh_summary" == "1" ]]; then
  APPFACTORY_DEVICE_METRICS_ROOT="$metrics_root" \
  APPFACTORY_DEVICE_METRICS_JSON_OUTPUT="$summary_json_path" \
    bash scripts/update-appfactory-device-summary.sh >/dev/null
fi

python3 - "$summary_json_path" "$max_total_count" "$max_run_count" "$focus_categories_raw" <<'PY'
import json
import sys
from pathlib import Path

summary_path = Path(sys.argv[1]).expanduser()
max_total_count = int(sys.argv[2] or 0)
max_run_count = int(sys.argv[3] or 0)
focus_categories = {item.strip() for item in (sys.argv[4] or "").split(",") if item.strip()}

if not summary_path.is_file():
    print(f"device alert summary not found: {summary_path}", file=sys.stderr)
    sys.exit(1)

payload = json.loads(summary_path.read_text(encoding="utf-8"))
categories = payload.get("categories") or []
violations = []

for item in categories:
    category = str(item.get("failure_category") or "").strip()
    total_count = int(item.get("total_count") or 0)
    run_count = int(item.get("run_count") or 0)
    if max_total_count > 0 and total_count >= max_total_count:
        violations.append(f"total_count threshold hit: {category}={total_count} >= {max_total_count}")
    if max_run_count > 0 and run_count >= max_run_count:
        violations.append(f"run_count threshold hit: {category}={run_count} >= {max_run_count}")
    if focus_categories and category in focus_categories and total_count > 0:
        violations.append(f"focus category present: {category} total_count={total_count}")

print(f"summary_json={summary_path.as_posix()}")
print(f"device_failure_category_count={len(categories)}")
if categories:
    top = categories[0]
    print(
        "top_category={category} total_count={count} run_count={runs}".format(
            category=top.get("failure_category", ""),
            count=top.get("total_count", 0),
            runs=top.get("run_count", 0),
        )
    )

if violations:
    print("device_alert_status=triggered")
    for violation in violations:
        print(f"device_alert_violation={violation}")
    sys.exit(1)

print("device_alert_status=ok")
PY