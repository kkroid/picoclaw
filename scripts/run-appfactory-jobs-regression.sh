#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script_started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

api_base="${APPFACTORY_JOBS_API_BASE:-${APPFACTORY_PRODUCT_FLOW_API_BASE:-http://127.0.0.1:18800}}"
curl_connect_timeout_seconds="${APPFACTORY_JOBS_CURL_CONNECT_TIMEOUT_SECONDS:-3}"
curl_max_time_seconds="${APPFACTORY_JOBS_CURL_MAX_TIME_SECONDS:-30}"
requirement_text=""
requirement_file="${APPFACTORY_JOBS_REQUIREMENT_FILE:-$repo_root/examples/appfactory/bookkeeping/requirement.md}"
title_hint="${APPFACTORY_JOBS_TITLE_HINT:-/jobs regression bookkeeping demo}"
job_id="${APPFACTORY_JOBS_JOB_ID:-}"
prd_id="${APPFACTORY_JOBS_PRD_ID:-}"
template_id="${APPFACTORY_JOBS_TEMPLATE_ID:-}"
goal_summary="${APPFACTORY_JOBS_GOAL_SUMMARY:-validate the current /jobs compile/create/start real-build path}"
human_notes_json="${APPFACTORY_JOBS_HUMAN_NOTES_JSON:-}"
builder_image="${APPFACTORY_JOBS_BUILDER_IMAGE:-picoclaw/appfactory-builder:local}"
timeout_seconds="${APPFACTORY_JOBS_TIMEOUT_SECONDS:-2400}"
poll_interval="${APPFACTORY_JOBS_POLL_INTERVAL:-2}"
output_root="${APPFACTORY_JOBS_REGRESSION_ROOT:-$repo_root/workspace/appfactory/jobs-ui-regression}"
appfactory_root_candidates="${APPFACTORY_JOBS_APPFACTORY_ROOT_CANDIDATES:-$repo_root/workspace/appfactory:$HOME/.picoclaw/workspace/appfactory}"

run_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
job_id="${job_id:-job-jobs-ui-regression-$run_timestamp}"
prd_id="${prd_id:-prd-jobs-ui-regression-$run_timestamp}"
run_dir="$output_root/runs/$run_timestamp"
mkdir -p "$run_dir"

stage_status_compile="pending"
stage_status_create="pending"
stage_status_register_builder="pending"
stage_status_start="pending"
stage_status_run="pending"
terminal_job_status=""
failure_bucket="none"
failure_signature=""
failure_domain=""
failure_category=""
failure_summary=""
latest_run_id=""

usage() {
  cat <<'EOF'
Usage:
  scripts/run-appfactory-jobs-regression.sh \
    [--api-base http://127.0.0.1:18800] \
    [--requirement-file examples/appfactory/bookkeeping/requirement.md] \
    [--job-id job-jobs-ui-regression-001] \
    [--prd-id prd-jobs-ui-regression-001] \
    [--template-id flutter-finance-lite]

Options:
  --api-base          Backend API base URL.
  --requirement       Inline requirement text.
  --requirement-file  Requirement file path.
  --title             Title passed to /api/v1/prds:compile.
  --job-id            Optional fixed job id.
  --prd-id            Optional fixed PRD id.
  --template-id       Optional fixed template id override.
  --goal-summary      Optional job goal summary.
  --human-notes-json  Optional JSON payload forwarded as create-job human_notes.
  --builder-image     Builder image used for compile + builder registration.
  --timeout-seconds   Max seconds to wait for terminal job state.
  --poll-interval     Poll interval in seconds.
  --output-root       Root directory for archived regression snapshots.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-base)
      api_base="$2"
      shift 2
      ;;
    --requirement)
      requirement_text="$2"
      requirement_file=""
      shift 2
      ;;
    --requirement-file)
      requirement_file="$2"
      requirement_text=""
      shift 2
      ;;
    --title)
      title_hint="$2"
      shift 2
      ;;
    --job-id)
      job_id="$2"
      shift 2
      ;;
    --prd-id)
      prd_id="$2"
      shift 2
      ;;
    --template-id)
      template_id="$2"
      shift 2
      ;;
    --goal-summary)
      goal_summary="$2"
      shift 2
      ;;
    --human-notes-json)
      human_notes_json="$2"
      shift 2
      ;;
    --builder-image)
      builder_image="$2"
      shift 2
      ;;
    --timeout-seconds)
      timeout_seconds="$2"
      shift 2
      ;;
    --poll-interval)
      poll_interval="$2"
      shift 2
      ;;
    --output-root)
      output_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -n "$requirement_text" && -n "$requirement_file" ]]; then
  echo "--requirement and --requirement-file are mutually exclusive" >&2
  exit 1
fi

if [[ -z "$requirement_text" ]]; then
  if [[ ! -f "$requirement_file" ]]; then
    echo "requirement file not found: $requirement_file" >&2
    exit 1
  fi
  requirement_text="$(cat "$requirement_file")"
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

ensure_api_reachable() {
  local probe_path="/api/v1/notifications"
  local probe_status

  probe_status="$(curl -sS -o /dev/null -w '%{http_code}' \
    --connect-timeout "$curl_connect_timeout_seconds" \
    --max-time "$curl_max_time_seconds" \
    "$api_base$probe_path" || true)"

  if [[ "$probe_status" == 2* ]]; then
    return 0
  fi

  echo "appfactory api is unavailable: $api_base$probe_path (status=${probe_status:-000})" >&2
  exit 1
}

request_json() {
  local method="$1"
  local path="$2"
  local payload="${3:-}"
  local response
  local status
  local body

  if [[ "$method" == "GET" ]]; then
    response="$(curl -sS \
      --connect-timeout "$curl_connect_timeout_seconds" \
      --max-time "$curl_max_time_seconds" \
      "$api_base$path" -w $'\n%{http_code}')"
  else
    response="$(curl -sS -X "$method" "$api_base$path" \
      --connect-timeout "$curl_connect_timeout_seconds" \
      --max-time "$curl_max_time_seconds" \
      -H 'Content-Type: application/json' \
      -d "$payload" \
      -w $'\n%{http_code}')"
  fi

  status="${response##*$'\n'}"
  body="${response%$'\n'*}"
  if [[ "$status" != 2* ]]; then
    printf '%s\n' "$body" >&2
    return 1
  fi
  printf '%s' "$body"
}

json_get() {
  local key_path="$1"
  python3 -c '
import json
import sys

key_path = sys.argv[1]
value = json.load(sys.stdin)
for part in key_path.split("."):
    if isinstance(value, dict):
        value = value.get(part)
    else:
        value = None
        break
if value is None:
    sys.exit(1)
if isinstance(value, bool):
    sys.stdout.write("true" if value else "false")
elif isinstance(value, (dict, list)):
    sys.stdout.write(json.dumps(value, ensure_ascii=False))
else:
    sys.stdout.write(str(value))
' "$key_path"
}

json_last_run_id() {
  python3 -c '
import json
import sys

items = json.load(sys.stdin).get("items") or []
run_id = ""
for item in items:
    if isinstance(item, dict) and item.get("run_id"):
        run_id = item["run_id"]
sys.stdout.write(run_id)
'
}

timeline_summary() {
  local job_payload="$1"
  local events_payload="$2"
  JOB_PAYLOAD="$job_payload" EVENTS_PAYLOAD="$events_payload" python3 - <<'PY'
import json
import os
from datetime import datetime, timezone

def parse_ts(value: str):
  raw = str(value or "").strip()
  if not raw:
    return None
  if raw.endswith("Z"):
    raw = raw[:-1] + "+00:00"
  try:
    return datetime.fromisoformat(raw)
  except ValueError:
    return None

def compact(text: str, limit: int = 96) -> str:
  value = " ".join(str(text or "").split())
  if len(value) <= limit:
    return value
  return value[: limit - 3] + "..."

def label_for(item: dict) -> str:
  event_type = str(item.get("type") or "").strip()
  stage = str(item.get("stage") or "").strip()
  summary = str(item.get("summary") or "").strip()
  if event_type == "execution_started":
    return "orchestrator dispatch"
  if event_type == "run_completed":
    return compact(summary or "run completed")
  if event_type == "run_heartbeat" and summary.startswith("acceptance check:"):
    parts = [part.strip() for part in summary.split("|")]
    if len(parts) >= 2:
      return f"{parts[0]} | {parts[1]}"
  if stage:
    return compact(f"{stage} | {summary or event_type}")
  return compact(summary or event_type or "unknown")

job = json.loads(os.environ["JOB_PAYLOAD"])
events = json.loads(os.environ["EVENTS_PAYLOAD"])
items = events.get("items") or []
relevant = []
for item in items:
  if not isinstance(item, dict):
    continue
  event_type = str(item.get("type") or "").strip()
  if event_type in {"execution_started", "run_heartbeat", "run_completed"}:
    ts = parse_ts(item.get("at"))
    if ts is not None:
      relevant.append((ts, item))

relevant.sort(key=lambda pair: pair[0])
started_at = parse_ts(job.get("started_at"))
finished_at = parse_ts(job.get("finished_at")) or parse_ts(job.get("updated_at")) or datetime.now(timezone.utc)
if started_at is None and relevant:
  started_at = relevant[0][0]
if started_at is None:
  started_at = finished_at

print(f"total_runtime_seconds={int(max((finished_at - started_at).total_seconds(), 0))}")
for index, (current_ts, item) in enumerate(relevant):
  next_ts = finished_at
  if index + 1 < len(relevant):
    next_ts = relevant[index + 1][0]
  duration = int(max((next_ts - current_ts).total_seconds(), 0))
  stage = str(item.get("stage") or "").strip() or "none"
  event_type = str(item.get("type") or "").strip() or "unknown"
  label = label_for(item)
  print(f"timeline={stage}\t{event_type}\t{duration}\t{label}")
PY
}

format_poll_progress() {
  local job_payload="$1"
  local events_payload="${2:-}"
  local elapsed_seconds="$3"
  JOB_PAYLOAD="$job_payload" EVENTS_PAYLOAD="$events_payload" ELAPSED_SECONDS="$elapsed_seconds" python3 - <<'PY'
import json
import os

def compact(text: str, limit: int = 160) -> str:
  value = " ".join(str(text or "").split())
  if len(value) <= limit:
    return value
  return value[: limit - 3] + "..."

job = json.loads(os.environ["JOB_PAYLOAD"])
events_payload = os.environ.get("EVENTS_PAYLOAD", "").strip()
events = json.loads(events_payload) if events_payload else {}
items = events.get("items") or []
latest = items[-1] if items else {}

parts = [f"[poll +{os.environ['ELAPSED_SECONDS']}s]"]
parts.append(f"status={job.get('status') or 'unknown'}")
phase = str(job.get("phase") or "").strip()
if phase:
  parts.append(f"phase={phase}")
run_id = str(job.get("builder_output_path") or "").split("/")
if len(run_id) >= 2:
  parts.append(f"run_id={run_id[-2]}")
budgets = job.get("budgets") or {}
elapsed_iterations = budgets.get("elapsed_iterations")
if elapsed_iterations not in (None, ""):
  parts.append(f"iterations={elapsed_iterations}")
consumed_tokens = budgets.get("consumed_tokens")
if consumed_tokens not in (None, ""):
  parts.append(f"tokens={consumed_tokens}")

if isinstance(latest, dict) and latest:
  event_parts = []
  at = str(latest.get("at") or "").strip()
  if at:
    event_parts.append(at)
  stage = str(latest.get("stage") or "").strip()
  if stage:
    event_parts.append(f"stage={stage}")
  event_type = str(latest.get("type") or "").strip()
  if event_type:
    event_parts.append(f"type={event_type}")
  summary = compact(latest.get("summary") or "")
  if summary:
    event_parts.append(f"summary={summary}")
  if event_parts:
    parts.append("event{" + " | ".join(event_parts) + "}")

print(" | ".join(parts))
PY
}

write_snapshot() {
  local file_name="$1"
  local body="$2"
  printf '%s\n' "$body" > "$run_dir/$file_name"
}

record_step() {
  local file_name="$1"
  local body="$2"
  write_snapshot "$file_name" "$body"
}

write_result() {
  RESULT_JSON="$run_dir/jobs-regression-result.json" \
  OUTPUT_ROOT="$output_root" \
  RUN_DIR="$run_dir" \
  APPFACTORY_ROOT_CANDIDATES="$appfactory_root_candidates" \
  API_BASE="$api_base" \
  JOB_ID="$job_id" \
  PRD_ID="$prd_id" \
  TEMPLATE_ID="$template_id" \
  RUN_ID="$latest_run_id" \
  STAGE_STATUS_COMPILE="$stage_status_compile" \
  STAGE_STATUS_CREATE="$stage_status_create" \
  STAGE_STATUS_REGISTER_BUILDER="$stage_status_register_builder" \
  STAGE_STATUS_START="$stage_status_start" \
  STAGE_STATUS_RUN="$stage_status_run" \
  TERMINAL_JOB_STATUS="$terminal_job_status" \
  FAILURE_BUCKET="$failure_bucket" \
  FAILURE_SIGNATURE="$failure_signature" \
  FAILURE_DOMAIN="$failure_domain" \
  FAILURE_CATEGORY="$failure_category" \
  FAILURE_SUMMARY="$failure_summary" \
  SCRIPT_STARTED_AT="$script_started_at" \
  SCRIPT_FINISHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  python3 - <<'PY'
import json
import os
from pathlib import Path
from datetime import datetime, timezone

run_dir = Path(os.environ["RUN_DIR"])

def load_json(name: str):
    path = run_dir / name
    if not path.is_file():
        return None

    def load_builder_output(job_final):
      builder_output_path = str(job_final.get("builder_output_path") or "").strip()
      if not builder_output_path:
        return None
      raw_candidates = str(os.environ.get("APPFACTORY_ROOT_CANDIDATES", "")).split(":")
      for candidate in raw_candidates:
        candidate = candidate.strip()
        if not candidate:
          continue
        path = Path(candidate) / builder_output_path
        if not path.is_file():
          continue
        try:
          return json.loads(path.read_text(encoding="utf-8"))
        except Exception:
          return None
      return None
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return None

def read_artifact_path(artifacts, artifact_id: str, needle: str = ""):
    items = []
    if isinstance(artifacts, dict):
        items = artifacts.get("items") or []
    for item in items:
        if not isinstance(item, dict):
            continue
        if artifact_id and item.get("artifact_id") == artifact_id and item.get("produced"):
            return item.get("path") or ""
        path = str(item.get("path") or "")
        if needle and item.get("produced") and needle in path:
            return path
    return ""

  def parse_ts(value):
    raw = str(value or "").strip()
    if not raw:
      return None
    if raw.endswith("Z"):
      raw = raw[:-1] + "+00:00"
    try:
      return datetime.fromisoformat(raw)
    except ValueError:
      return None

  def compact(text: str, limit: int = 96):
    value = " ".join(str(text or "").split())
    if len(value) <= limit:
      return value
    return value[: limit - 3] + "..."

  def timeline_label(item):
    event_type = str(item.get("type") or "").strip()
    stage = str(item.get("stage") or "").strip()
    summary = str(item.get("summary") or "").strip()
    if event_type == "execution_started":
      return "orchestrator dispatch"
    if event_type == "run_completed":
      return compact(summary or "run completed")
    if event_type == "run_heartbeat" and summary.startswith("acceptance check:"):
      parts = [part.strip() for part in summary.split("|")]
      if len(parts) >= 2:
        return f"{parts[0]} | {parts[1]}"
    if stage:
      return compact(f"{stage} | {summary or event_type}")
    return compact(summary or event_type or "unknown")

  def build_timeline(job_final, events):
    items = []
    for item in (events.get("items") or []):
      if not isinstance(item, dict):
        continue
      event_type = str(item.get("type") or "").strip()
      if event_type not in {"execution_started", "run_heartbeat", "run_completed"}:
        continue
      ts = parse_ts(item.get("at"))
      if ts is None:
        continue
      items.append((ts, item))
    items.sort(key=lambda pair: pair[0])
    started_at = parse_ts(job_final.get("started_at"))
    finished_at = parse_ts(job_final.get("finished_at")) or parse_ts(job_final.get("updated_at")) or datetime.now(timezone.utc)
    if started_at is None and items:
      started_at = items[0][0]
    if started_at is None:
      started_at = finished_at
    total_runtime_seconds = int(max((finished_at - started_at).total_seconds(), 0))
    timeline = []
    for index, (current_ts, item) in enumerate(items):
      next_ts = finished_at
      if index + 1 < len(items):
        next_ts = items[index + 1][0]
      timeline.append({
        "at": current_ts.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "stage": str(item.get("stage") or "").strip() or None,
        "type": str(item.get("type") or "").strip() or None,
        "duration_seconds": int(max((next_ts - current_ts).total_seconds(), 0)),
        "label": timeline_label(item),
      })
    return total_runtime_seconds, timeline

  def build_auto_repair_summary(builder_output):
    if not isinstance(builder_output, dict):
      return {
        "observed": False,
        "task_types": [],
        "recovered_checks": [],
        "round_ids": [],
      }
    task_types = []
    recovered_checks = []
    round_ids = []
    for round_output in builder_output.get("round_outputs") or []:
      if not isinstance(round_output, dict):
        continue
      round_id = str(round_output.get("round_id") or "").strip()
      builder_runtime = round_output.get("builder_runtime_execution") or {}
      task_type = str(builder_runtime.get("task_type") or "").strip()
      if task_type in {"analyze_repair", "test_repair", "closure_repair"}:
        task_types.append(task_type)
        if round_id:
          round_ids.append(round_id)
      for validation in round_output.get("validation_results") or []:
        if not isinstance(validation, dict):
          continue
        summary = str(validation.get("summary") or validation.get("details") or "").strip()
        if "after builder-runtime repair" in summary:
          check_id = str(validation.get("check_id") or "").strip()
          if check_id:
            recovered_checks.append(check_id)
    return {
      "observed": bool(task_types or recovered_checks),
      "task_types": sorted(set(task_types)),
      "recovered_checks": sorted(set(recovered_checks)),
      "round_ids": sorted(set(round_ids)),
    }

job_final = load_json("job-final-response.json") or {}
artifacts = load_json("artifacts-response.json") or load_json("artifacts-final-response.json") or {}
  events = load_json("events-response.json") or {}
  builder_output = load_builder_output(job_final) or {}

delivery_context = job_final.get("delivery_context") if isinstance(job_final, dict) else None
delivery_status = "not_started"
if isinstance(delivery_context, dict):
    delivery_status = str(delivery_context.get("status") or "not_started")

total_runtime_seconds, timeline = build_timeline(job_final, events)
auto_repair = build_auto_repair_summary(builder_output)

result = {
    "schema_version": "0.1.0",
    "generated_at": os.environ["SCRIPT_FINISHED_AT"],
    "script_started_at": os.environ["SCRIPT_STARTED_AT"],
    "script_finished_at": os.environ["SCRIPT_FINISHED_AT"],
    "api_base": os.environ["API_BASE"],
    "job_id": os.environ["JOB_ID"],
    "prd_id": os.environ["PRD_ID"],
    "template_id": os.environ["TEMPLATE_ID"],
    "run_id": os.environ.get("RUN_ID", ""),
    "job_status": os.environ.get("TERMINAL_JOB_STATUS", ""),
    "delivery_status": delivery_status,
    "failure_bucket": os.environ.get("FAILURE_BUCKET", "unknown"),
    "failure_signature": os.environ.get("FAILURE_SIGNATURE", ""),
    "failure_domain": os.environ.get("FAILURE_DOMAIN", ""),
    "failure_category": os.environ.get("FAILURE_CATEGORY", ""),
    "failure_summary": os.environ.get("FAILURE_SUMMARY", ""),
    "total_runtime_seconds": total_runtime_seconds,
    "auto_repair": auto_repair,
    "stage_status": {
        "compile": os.environ.get("STAGE_STATUS_COMPILE", "pending"),
        "create": os.environ.get("STAGE_STATUS_CREATE", "pending"),
        "register_builder": os.environ.get("STAGE_STATUS_REGISTER_BUILDER", "pending"),
        "start": os.environ.get("STAGE_STATUS_START", "pending"),
        "run": os.environ.get("STAGE_STATUS_RUN", "pending"),
    },
    "timeline": timeline,
    "key_paths": {
        "builder_output": str(job_final.get("builder_output_path") or ""),
        "summary_log": str(((job_final.get("logs") or {}) if isinstance(job_final, dict) else {}).get("summary_path") or ""),
        "event_log": str(((job_final.get("logs") or {}) if isinstance(job_final, dict) else {}).get("event_log_path") or ""),
        "build_report": read_artifact_path(artifacts, "build-report", "build-report"),
        "smoke_test_report": read_artifact_path(artifacts, "smoke-test-report", "smoke-test-report"),
        "debug_apk": read_artifact_path(artifacts, "debug-apk", "app-debug.apk"),
        "device_logcat": read_artifact_path(artifacts, "device-logcat", "device-logcat"),
    },
    "run_dir": os.environ["RUN_DIR"],
    "snapshots": sorted(path.name for path in run_dir.glob("*.json")),
}

output_root = Path(os.environ["OUTPUT_ROOT"])
output_root.mkdir(parents=True, exist_ok=True)
result_path = Path(os.environ["RESULT_JSON"])
result_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
(output_root / "latest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
(output_root / "latest.md").write_text(
    "# AppFactory /jobs Regression Latest Run\n\n"
    f"- job_id: {result['job_id']}\n"
    f"- prd_id: {result['prd_id']}\n"
    f"- template_id: {result['template_id'] or 'auto'}\n"
    f"- run_id: {result['run_id'] or 'unknown'}\n"
    f"- job_status: {result['job_status'] or 'unknown'}\n"
    f"- delivery_status: {result['delivery_status']}\n"
    f"- failure_bucket: {result['failure_bucket']}\n"
    f"- failure_signature: {result['failure_signature'] or 'none'}\n"
    f"- total_runtime_seconds: {result['total_runtime_seconds']}\n"
    f"- auto_repair_observed: {'true' if result['auto_repair']['observed'] else 'false'}\n"
    f"- run_dir: {result['run_dir']}\n",
    encoding="utf-8",
)
PY
}

fail_with_stage() {
  local stage="$1"
  local bucket="$2"
  local summary="$3"
  case "$stage" in
    compile) stage_status_compile="failed" ;;
    create) stage_status_create="failed" ;;
    register_builder) stage_status_register_builder="failed" ;;
    start) stage_status_start="failed" ;;
    run) stage_status_run="failed" ;;
  esac
  failure_bucket="$bucket"
  failure_summary="$summary"
  write_result
  echo "$summary" >&2
  exit 1
}

classify_failure_bucket() {
  local category="$1"
  local signature="$2"
  if [[ "$stage_status_compile" == "failed" ]]; then
    echo "compile"
    return
  fi
  if [[ "$stage_status_create" == "failed" ]]; then
    echo "create"
    return
  fi
  if [[ "$stage_status_start" == "failed" ]]; then
    echo "start"
    return
  fi
  case "$signature" in
    builder_runtime_patch_parse_failed|workspace_patch_apply_failed)
      echo "builder_runtime"
      return
      ;;
  esac
  case "$category" in
    environment_check_failed:*|profile_check_failed:*)
      echo "validate"
      return
      ;;
    device_check_failed:*)
      echo "device"
      return
      ;;
  esac
  echo "unknown"
}

ensure_api_reachable

echo "jobs_ui_regression_run_id=$run_timestamp"
echo "jobs_ui_regression_root=$output_root"
echo "jobs_ui_api_base=$api_base"

echo "== compile requirement via /jobs-equivalent payload =="
compile_payload="$(REQUIREMENT_TEXT="$requirement_text" TITLE_HINT="$title_hint" JOB_ID="$job_id" PRD_ID="$prd_id" TEMPLATE_ID="$template_id" BUILDER_IMAGE="$builder_image" python3 - <<'PY'
import json
import os

payload = {
    "requirement_text": os.environ["REQUIREMENT_TEXT"],
    "requirement_source": "jobs-ui",
    "title": os.environ.get("TITLE_HINT", ""),
    "job_id": os.environ["JOB_ID"],
    "prd_id": os.environ["PRD_ID"],
    "real_checks": True,
    "executor_image": os.environ["BUILDER_IMAGE"],
}
if os.environ.get("TEMPLATE_ID", ""):
    payload["template_id"] = os.environ["TEMPLATE_ID"]
print(json.dumps(payload, ensure_ascii=False))
PY
)"
if ! compile_response="$(request_json POST "/api/v1/prds:compile" "$compile_payload")"; then
  fail_with_stage "compile" "compile" "compile request failed"
fi
stage_status_compile="passed"
record_step "compile-response.json" "$compile_response"

compiled_prd_id="$(printf '%s' "$compile_response" | json_get "prd_id" 2>/dev/null || true)"
compiled_template_id="$(printf '%s' "$compile_response" | json_get "template_id" 2>/dev/null || true)"
if [[ -n "$compiled_prd_id" ]]; then
  prd_id="$compiled_prd_id"
fi
if [[ -n "$compiled_template_id" ]]; then
  template_id="$compiled_template_id"
fi
if [[ -z "$template_id" ]]; then
  fail_with_stage "compile" "compile" "compile finished without template_id"
fi

echo "== create job =="
create_payload="$(PRD_ID="$prd_id" TEMPLATE_ID="$template_id" GOAL_SUMMARY="$goal_summary" HUMAN_NOTES_JSON="$human_notes_json" python3 - <<'PY'
import json
import os
import sys

payload = {
    "prd_id": os.environ["PRD_ID"],
    "template_id": os.environ["TEMPLATE_ID"],
}
if os.environ.get("GOAL_SUMMARY", ""):
    payload["goal_summary"] = os.environ["GOAL_SUMMARY"]
human_notes = os.environ.get("HUMAN_NOTES_JSON", "").strip()
if human_notes:
  try:
    payload["human_notes"] = json.loads(human_notes)
  except json.JSONDecodeError as exc:
    print(f"invalid human notes json: {exc}", file=sys.stderr)
    sys.exit(1)
print(json.dumps(payload, ensure_ascii=False))
PY
)"
if ! create_response="$(request_json POST "/api/v1/jobs" "$create_payload")"; then
  fail_with_stage "create" "create" "job creation failed"
fi
stage_status_create="passed"
record_step "job-create-response.json" "$create_response"

echo "== register builder =="
builder_id="jobs-ui-builder-$job_id"
builder_payload="$(BUILDER_ID="$builder_id" JOB_ID="$job_id" BUILDER_IMAGE="$builder_image" python3 - <<'PY'
import json
import os

print(json.dumps({
    "builder_id": os.environ["BUILDER_ID"],
    "display_name": f"Jobs UI Builder {os.environ['JOB_ID']}",
    "capability_tags": ["flutter"],
    "max_parallel_runs": 1,
    "worker_profile": {
        "image": os.environ["BUILDER_IMAGE"],
    },
}, ensure_ascii=False))
PY
)"
if ! builder_response="$(request_json POST "/internal/v1/builders:register" "$builder_payload")"; then
  fail_with_stage "register_builder" "start" "builder registration failed"
fi
stage_status_register_builder="passed"
record_step "builder-register-response.json" "$builder_response"

echo "== start job =="
if ! start_response="$(request_json POST "/api/v1/jobs/$job_id:start" "{}")"; then
  fail_with_stage "start" "start" "job start failed"
fi
stage_status_start="passed"
record_step "job-start-response.json" "$start_response"

echo "== poll job status =="
elapsed=0
poll_events_response=""
last_progress_line=""
next_progress_echo_at=0
while [[ "$elapsed" -le "$timeout_seconds" ]]; do
  if ! final_job_response="$(request_json GET "/api/v1/jobs/$job_id")"; then
    fail_with_stage "run" "unknown" "failed to fetch final job status"
  fi
  poll_events_response="$(request_json GET "/api/v1/jobs/$job_id/events" 2>/dev/null || true)"
  terminal_job_status="$(printf '%s' "$final_job_response" | json_get "status" 2>/dev/null || true)"
  progress_line="$(format_poll_progress "$final_job_response" "$poll_events_response" "$elapsed")"
  if [[ "$progress_line" != "$last_progress_line" || "$elapsed" -ge "$next_progress_echo_at" ]]; then
    echo "$progress_line"
    last_progress_line="$progress_line"
    next_progress_echo_at=$((elapsed + 30))
  fi
  if [[ "$terminal_job_status" == "completed" || "$terminal_job_status" == "failed" || "$terminal_job_status" == "cancelled" ]]; then
    break
  fi
  sleep "$poll_interval"
  elapsed=$((elapsed + poll_interval))
done

if [[ "$terminal_job_status" != "completed" && "$terminal_job_status" != "failed" && "$terminal_job_status" != "cancelled" ]]; then
  fail_with_stage "run" "unknown" "job did not reach terminal state within timeout"
fi

record_step "job-final-response.json" "$final_job_response"

if ! artifacts_response="$(request_json GET "/api/v1/jobs/$job_id/artifacts")"; then
  fail_with_stage "run" "unknown" "failed to fetch job artifacts"
fi
if ! events_response="$(request_json GET "/api/v1/jobs/$job_id/events")"; then
  fail_with_stage "run" "unknown" "failed to fetch job events"
fi
record_step "artifacts-response.json" "$artifacts_response"
record_step "events-response.json" "$events_response"

latest_run_id="$(printf '%s' "$events_response" | json_last_run_id)"
failure_signature="$(printf '%s' "$final_job_response" | json_get "failure_context.failure_signature" 2>/dev/null || true)"
failure_domain="$(printf '%s' "$final_job_response" | json_get "failure_context.failure_domain" 2>/dev/null || true)"
failure_category="$(printf '%s' "$final_job_response" | json_get "failure_context.failure_category" 2>/dev/null || true)"
failure_summary="$(printf '%s' "$final_job_response" | json_get "failure_context.last_error_summary" 2>/dev/null || true)"

if [[ "$terminal_job_status" == "completed" ]]; then
  stage_status_run="passed"
  failure_bucket="none"
else
  stage_status_run="failed"
  failure_bucket="$(classify_failure_bucket "$failure_category" "$failure_signature")"
fi

write_result

echo "== timeline summary =="
while IFS= read -r timeline_line; do
  case "$timeline_line" in
    total_runtime_seconds=*)
      echo "$timeline_line"
      ;;
    timeline=*)
      timeline_payload="${timeline_line#timeline=}"
      IFS=$'\t' read -r timeline_stage timeline_type timeline_duration timeline_label <<< "$timeline_payload"
      echo "- stage=${timeline_stage} type=${timeline_type} duration_seconds=${timeline_duration} label=${timeline_label}"
      ;;
  esac
done < <(timeline_summary "$final_job_response" "$events_response")

if auto_repair_summary="$(RESULT_JSON="$run_dir/jobs-regression-result.json" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["RESULT_JSON"])
if not path.is_file():
    raise SystemExit(0)
payload = json.loads(path.read_text(encoding="utf-8"))
auto = payload.get("auto_repair") or {}
print(f"auto_repair_observed={'true' if auto.get('observed') else 'false'}")
if auto.get("task_types"):
    print("auto_repair_task_types=" + ",".join(auto["task_types"]))
if auto.get("recovered_checks"):
    print("auto_repair_recovered_checks=" + ",".join(auto["recovered_checks"]))
PY
)"; then
  if [[ -n "$auto_repair_summary" ]]; then
    echo "== auto repair summary =="
    printf '%s\n' "$auto_repair_summary"
  fi
fi

if [[ "$terminal_job_status" != "completed" ]]; then
  echo "jobs regression finished in terminal failure state" >&2
  echo "job_id=$job_id" >&2
  echo "status=$terminal_job_status" >&2
  echo "failure_bucket=$failure_bucket" >&2
  echo "run_dir=$run_dir" >&2
  exit 1
fi

echo "== /jobs regression passed =="
echo "job_id=$job_id"
echo "prd_id=$prd_id"
echo "template_id=$template_id"
echo "run_id=${latest_run_id:-unknown}"
echo "status=$terminal_job_status"
echo "run_dir=$run_dir"
echo "latest_json=$output_root/latest.json"
echo "latest_markdown=$output_root/latest.md"