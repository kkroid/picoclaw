#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_root="${APPFACTORY_BUILDER_RUNTIME_AUTO_REPAIR_ROOT:-$repo_root/workspace/appfactory/builder-runtime-auto-repair}"
go_bin="${GO_BIN:-go}"
test_name="${APPFACTORY_BUILDER_RUNTIME_AUTO_REPAIR_TEST:-TestRunnerExecuteRunAutoRepairsFlutterAnalyzeFailure}"
test_package="${APPFACTORY_BUILDER_RUNTIME_AUTO_REPAIR_PACKAGE:-./pkg/appfactory/adapter}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)"
run_dir="$output_root/runs/$run_id"
raw_log="$run_dir/go-test.jsonl"
result_json="$run_dir/regression-result.json"

mkdir -p "$run_dir"

if ! command -v "$go_bin" >/dev/null 2>&1; then
  echo "$go_bin is required" >&2
  exit 1
fi

echo "builder_runtime_auto_repair_run_id=$run_id"
echo "builder_runtime_auto_repair_root=$output_root"
echo "go_test=${test_package}:${test_name}"

set +e
"$go_bin" test -json -run "^${test_name}$" "$test_package" | tee "$raw_log"
test_exit_code=${PIPESTATUS[0]}
set -e

RESULT_JSON="$result_json" \
OUTPUT_ROOT="$output_root" \
RUN_ID="$run_id" \
RAW_LOG="$raw_log" \
TEST_NAME="$test_name" \
TEST_PACKAGE="$test_package" \
TEST_EXIT_CODE="$test_exit_code" \
python3 - <<'PY'
import json
import os
from pathlib import Path
from datetime import datetime, timezone

raw_log = Path(os.environ["RAW_LOG"])
run_id = os.environ["RUN_ID"]
output_root = Path(os.environ["OUTPUT_ROOT"])
result_path = Path(os.environ["RESULT_JSON"])
test_name = os.environ["TEST_NAME"]
test_package = os.environ["TEST_PACKAGE"]
test_exit_code = int(os.environ["TEST_EXIT_CODE"])

task_type = ""
attempts = ""
recovered_check = ""
last_output = ""

for line in raw_log.read_text(encoding="utf-8").splitlines():
    line = line.strip()
    if not line:
        continue
    try:
        payload = json.loads(line)
    except json.JSONDecodeError:
        continue
    output = str(payload.get("Output") or "").strip()
    if not output:
        continue
    last_output = output
    if "auto_repair_task_type=" in output:
        task_type = output.split("auto_repair_task_type=", 1)[1].strip()
    if "auto_repair_attempts=" in output:
        attempts = output.split("auto_repair_attempts=", 1)[1].strip()
    if "auto_repair_recovered_check=" in output:
        recovered_check = output.split("auto_repair_recovered_check=", 1)[1].strip()

result = {
    "schema_version": "0.1.0",
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "run_id": run_id,
    "test_name": test_name,
    "test_package": test_package,
    "status": "passed" if test_exit_code == 0 else "failed",
    "test_exit_code": test_exit_code,
    "auto_repair_observed": bool(task_type),
    "auto_repair_task_type": task_type or None,
    "auto_repair_attempts": int(attempts) if attempts else None,
    "auto_repair_recovered_check": recovered_check or None,
    "raw_log": str(raw_log),
    "last_output": last_output or None,
    "run_dir": str(result_path.parent),
}

result_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
output_root.mkdir(parents=True, exist_ok=True)
(output_root / "latest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
(output_root / "latest.md").write_text(
    "# Builder Runtime Auto Repair Latest Run\n\n"
    f"- run_id: {result['run_id']}\n"
    f"- test_name: {result['test_name']}\n"
    f"- status: {result['status']}\n"
    f"- test_exit_code: {result['test_exit_code']}\n"
    f"- auto_repair_observed: {'true' if result['auto_repair_observed'] else 'false'}\n"
    f"- auto_repair_task_type: {result['auto_repair_task_type'] or 'none'}\n"
    f"- auto_repair_attempts: {result['auto_repair_attempts'] if result['auto_repair_attempts'] is not None else 'none'}\n"
    f"- auto_repair_recovered_check: {result['auto_repair_recovered_check'] or 'none'}\n"
    f"- raw_log: {result['raw_log']}\n",
    encoding="utf-8",
)
PY

echo "builder_runtime_auto_repair_result_json=$result_json"
echo "builder_runtime_auto_repair_latest_json=$output_root/latest.json"
echo "builder_runtime_auto_repair_latest_markdown=$output_root/latest.md"

if [[ "$test_exit_code" != "0" ]]; then
  exit "$test_exit_code"
fi