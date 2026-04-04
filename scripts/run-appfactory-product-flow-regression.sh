#!/usr/bin/env bash

set -euo pipefail

product_flow_root="${APPFACTORY_PRODUCT_FLOW_ROOT:-workspace/appfactory/product-e2e}"
run_alerts="${APPFACTORY_PRODUCT_FLOW_REGRESSION_RUN_ALERTS:-1}"
fail_on_alert="${APPFACTORY_PRODUCT_FLOW_REGRESSION_FAIL_ON_ALERT:-1}"
verify_command_override="${APPFACTORY_PRODUCT_FLOW_REGRESSION_VERIFY_COMMAND:-}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

verify_command="${verify_command_override:-make verify-appfactory-product-flow}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-product-flow"
run_dir="$product_flow_root/regression-runs/$run_id"
mkdir -p "$run_dir"

verify_log="$run_dir/verify.log"
summary_log="$run_dir/summary.log"
alert_log="$run_dir/alert.log"
result_json="$run_dir/regression-result.json"

echo "product_flow_regression_run_id=$run_id"
echo "product_flow_root=$product_flow_root"
echo "verify_command=$verify_command"

set +e
bash -lc "$verify_command" 2>&1 | tee "$verify_log"
verify_exit_code=${PIPESTATUS[0]}
set -e

set +e
APPFACTORY_PRODUCT_FLOW_ROOT="$product_flow_root" \
  bash scripts/update-appfactory-product-flow-summary.sh 2>&1 | tee "$summary_log"
summary_exit_code=${PIPESTATUS[0]}
set -e

alert_exit_code=""
alert_status="skipped"
if [[ "$run_alerts" == "1" ]]; then
  set +e
  APPFACTORY_PRODUCT_FLOW_ROOT="$product_flow_root" \
    bash scripts/check-appfactory-product-flow-alerts.sh 2>&1 | tee "$alert_log"
  alert_exit_code=${PIPESTATUS[0]}
  set -e
  alert_status="$(grep -E '^product_flow_alert_status=' "$alert_log" | tail -n1 | sed 's/^product_flow_alert_status=//')"
else
  : > "$alert_log"
fi

python3 - "$result_json" "$run_id" "$product_flow_root" "$verify_command" "$verify_exit_code" "$summary_exit_code" "$alert_exit_code" "$alert_status" "$run_dir" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

result_path = Path(sys.argv[1])
run_id, product_flow_root, verify_command, verify_exit_code, summary_exit_code, alert_exit_code, alert_status, run_dir = sys.argv[2:]

payload = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "run_id": run_id,
    "product_flow_root": product_flow_root,
    "verify_command": verify_command,
    "verify_status": "passed" if verify_exit_code == "0" else "failed",
    "verify_exit_code": int(verify_exit_code),
    "summary_status": "passed" if summary_exit_code == "0" else "failed",
    "summary_exit_code": int(summary_exit_code),
    "alert_status": alert_status,
    "alert_exit_code": int(alert_exit_code) if alert_exit_code else None,
    "run_dir": run_dir,
}
result_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(payload, ensure_ascii=False, indent=2))
PY

if [[ "$verify_exit_code" != "0" ]]; then
  exit "$verify_exit_code"
fi
if [[ "$summary_exit_code" != "0" ]]; then
  exit "$summary_exit_code"
fi
if [[ "$run_alerts" == "1" && "$fail_on_alert" == "1" && -n "$alert_exit_code" && "$alert_exit_code" != "0" ]]; then
  exit "$alert_exit_code"
fi