#!/usr/bin/env bash

set -euo pipefail

# 这个脚本只服务当前的受控拉通测试，不承担长期平台调度职责。

api_base=""
requirement_text=""
requirement_file=""
job_id=""
prd_id=""
template_id="flutter-finance-lite"
builder_id="local-builder-01"
display_name="Local Builder 01"
builder_image="picoclaw/appfactory-builder:local"
timeout_seconds=300
poll_interval=2
output_dir=""

usage() {
  cat <<'EOF'
Usage:
  scripts/run-appfactory-controlled-e2e.sh \
    --api-base http://127.0.0.1:18800 \
    --requirement "..." \
    [--job-id job-e2e-001] \
    [--prd-id prd-e2e-001] \
    [--template-id flutter-finance-lite] \
    [--builder-id local-builder-01] \
    [--display-name "Local Builder 01"] \
    [--builder-image picoclaw/appfactory-builder:local] \
    [--timeout-seconds 300] \
    [--poll-interval 2] \
    [--output-dir /tmp/appfactory-e2e]

Options:
  --api-base         Backend API base URL.
  --requirement      Inline requirement text.
  --requirement-file Path to requirement file.
  --job-id           Optional fixed job id.
  --prd-id           Optional fixed prd id.
  --template-id      Template id, default flutter-finance-lite.
  --builder-id       Builder id used for registration.
  --display-name     Builder display name.
  --builder-image    Builder image label stored in worker profile.
  --timeout-seconds  Max seconds to wait for terminal job state.
  --poll-interval    Poll interval in seconds.
  --output-dir       Directory to store response snapshots.
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
      shift 2
      ;;
    --requirement-file)
      requirement_file="$2"
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
    --builder-id)
      builder_id="$2"
      shift 2
      ;;
    --display-name)
      display_name="$2"
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
    --output-dir)
      output_dir="$2"
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

if [[ -z "$api_base" ]]; then
  echo "--api-base is required" >&2
  exit 1
fi

if [[ -n "$requirement_text" && -n "$requirement_file" ]]; then
  echo "--requirement and --requirement-file are mutually exclusive" >&2
  exit 1
fi

if [[ -z "$requirement_text" && -z "$requirement_file" ]]; then
  echo "either --requirement or --requirement-file is required" >&2
  exit 1
fi

if [[ -n "$requirement_file" ]]; then
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

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
job_id="${job_id:-job-controlled-e2e-$timestamp}"
prd_id="${prd_id:-prd-controlled-e2e-$timestamp}"
output_dir="${output_dir:-$(mktemp -d -t appfactory-controlled-e2e-XXXXXX)}"

mkdir -p "$output_dir"

post_json() {
  local path="$1"
  local payload="$2"
  local response
  local status
  local body

  response="$(curl -sS -X POST "$api_base$path" \
    -H 'Content-Type: application/json' \
    -d "$payload" \
    -w $'\n%{http_code}')"
  status="${response##*$'\n'}"
  body="${response%$'\n'*}"

  if [[ "$status" != 2* ]]; then
    echo "request failed: POST $path (status=$status)" >&2
    echo "$body" >&2
    exit 1
  fi

  printf '%s' "$body"
}

get_json() {
  local path="$1"
  local response
  local status
  local body

  response="$(curl -sS "$api_base$path" -w $'\n%{http_code}')"
  status="${response##*$'\n'}"
  body="${response%$'\n'*}"

  if [[ "$status" != 2* ]]; then
    echo "request failed: GET $path (status=$status)" >&2
    echo "$body" >&2
    exit 1
  fi

  printf '%s' "$body"
}

json_get() {
  local key_path="$1"
  python3 -c '
import json
import sys

key_path = sys.argv[1]
data = json.load(sys.stdin)
value = data
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

json_has_artifact() {
  local artifact_id="$1"
  python3 -c '
import json
import sys

artifact_id = sys.argv[1]
data = json.load(sys.stdin)
items = data.get("items") or []
for item in items:
    if isinstance(item, dict) and item.get("artifact_id") == artifact_id:
        sys.stdout.write("true")
        break
else:
    sys.stdout.write("false")
' "$artifact_id"
}

write_snapshot() {
  local file_name="$1"
  local body="$2"
  printf '%s\n' "$body" > "$output_dir/$file_name"
}

echo "== compile requirement =="
compile_payload="$(REQUIREMENT_TEXT="$requirement_text" JOB_ID="$job_id" PRD_ID="$prd_id" BUILDER_IMAGE="$builder_image" python3 - <<'PY'
import json
import os

print(json.dumps({
    "requirement_text": os.environ["REQUIREMENT_TEXT"],
    "job_id": os.environ["JOB_ID"],
    "prd_id": os.environ["PRD_ID"],
  "executor_image": os.environ.get("BUILDER_IMAGE", ""),
}, ensure_ascii=False))
PY
)"
compile_response="$(post_json "/api/v1/prds:compile" "$compile_payload")"
write_snapshot "compile-response.json" "$compile_response"

compile_template_id="$(printf '%s' "$compile_response" | json_get "template_id" || true)"
if [[ -n "$compile_template_id" ]]; then
  template_id="$compile_template_id"
fi

echo "== create job =="
job_create_payload="$(PRD_ID="$prd_id" TEMPLATE_ID="$template_id" python3 - <<'PY'
import json
import os

print(json.dumps({
    "prd_id": os.environ["PRD_ID"],
    "template_id": os.environ["TEMPLATE_ID"],
}, ensure_ascii=False))
PY
)"
job_create_response="$(post_json "/api/v1/jobs" "$job_create_payload")"
write_snapshot "job-create-response.json" "$job_create_response"

echo "== register builder =="
builder_register_payload="$(BUILDER_ID="$builder_id" DISPLAY_NAME="$display_name" BUILDER_IMAGE="$builder_image" python3 - <<'PY'
import json
import os

print(json.dumps({
    "builder_id": os.environ["BUILDER_ID"],
    "display_name": os.environ["DISPLAY_NAME"],
    "capability_tags": ["flutter"],
    "max_parallel_runs": 1,
    "worker_profile": {
        "image": os.environ["BUILDER_IMAGE"],
    },
}, ensure_ascii=False))
PY
)"
builder_register_response="$(post_json "/internal/v1/builders:register" "$builder_register_payload")"
write_snapshot "builder-register-response.json" "$builder_register_response"

echo "== start job =="
job_start_payload="$(python3 -c 'import json, sys; print(json.dumps({"timeout_seconds": int(sys.argv[1])}, ensure_ascii=False))' "$timeout_seconds")"
job_start_response="$(post_json "/api/v1/jobs/$job_id:start" "$job_start_payload")"
write_snapshot "job-start-response.json" "$job_start_response"

echo "== poll job status =="
elapsed=0
final_job_response=""
final_status=""
while [[ "$elapsed" -le "$timeout_seconds" ]]; do
  final_job_response="$(get_json "/api/v1/jobs/$job_id")"
  final_status="$(printf '%s' "$final_job_response" | json_get "status" || true)"
  if [[ "$final_status" == "completed" || "$final_status" == "failed" || "$final_status" == "cancelled" ]]; then
    break
  fi
  sleep "$poll_interval"
  elapsed=$((elapsed + poll_interval))
done

if [[ "$final_status" != "completed" && "$final_status" != "failed" && "$final_status" != "cancelled" ]]; then
  echo "job did not reach terminal state within timeout" >&2
  exit 1
fi

write_snapshot "job-final-response.json" "$final_job_response"

echo "== fetch artifacts, events, notifications =="
artifacts_response="$(get_json "/api/v1/jobs/$job_id/artifacts")"
events_response="$(get_json "/api/v1/jobs/$job_id/events")"
notifications_response="$(get_json "/api/v1/notifications")"

write_snapshot "artifacts-response.json" "$artifacts_response"
write_snapshot "events-response.json" "$events_response"
write_snapshot "notifications-response.json" "$notifications_response"

if [[ "$final_status" != "completed" ]]; then
  echo "controlled e2e finished in terminal failure state" >&2
  echo "job_id=$job_id" >&2
  echo "status=$final_status" >&2
  echo "snapshots_dir=$output_dir" >&2
  exit 1
fi

builder_output_path="$(printf '%s' "$final_job_response" | json_get "builder_output_path" || true)"
if [[ -z "$builder_output_path" ]]; then
  echo "completed job is missing builder_output_path" >&2
  echo "snapshots_dir=$output_dir" >&2
  exit 1
fi

has_review_bundle="$(printf '%s' "$artifacts_response" | json_has_artifact "review-bundle")"
has_handoff_checklist="$(printf '%s' "$artifacts_response" | json_has_artifact "handoff-checklist")"

if [[ "$has_review_bundle" != "true" || "$has_handoff_checklist" != "true" ]]; then
  echo "completed job is missing review/handoff artifacts" >&2
  echo "review_bundle=$has_review_bundle handoff_checklist=$has_handoff_checklist" >&2
  echo "snapshots_dir=$output_dir" >&2
  exit 1
fi

echo "== controlled e2e passed =="
echo "job_id=$job_id"
echo "prd_id=$prd_id"
echo "template_id=$template_id"
echo "status=$final_status"
echo "builder_output_path=$builder_output_path"
echo "snapshots_dir=$output_dir"
echo "next_step=manual_device_validation_if_adb_is_available"