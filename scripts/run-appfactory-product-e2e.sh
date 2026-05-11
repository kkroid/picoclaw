#!/usr/bin/env bash

set -euo pipefail

# 这条脚本负责固定的产品级体验链，不承担通用编排平台职责。

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script_started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
script_start_epoch="$(date +%s)"

api_base="${APPFACTORY_PRODUCT_FLOW_API_BASE:-http://127.0.0.1:18800}"
curl_connect_timeout_seconds="${APPFACTORY_PRODUCT_FLOW_CURL_CONNECT_TIMEOUT_SECONDS:-3}"
curl_max_time_seconds="${APPFACTORY_PRODUCT_FLOW_CURL_MAX_TIME_SECONDS:-30}"
requirement_text=""
requirement_file="${APPFACTORY_PRODUCT_FLOW_REQUIREMENT_FILE:-$repo_root/examples/appfactory/bookkeeping/requirement.md}"
job_id=""
prd_id=""
template_id="${APPFACTORY_PRODUCT_FLOW_TEMPLATE_ID:-flutter-finance-lite}"
builder_id="${APPFACTORY_PRODUCT_FLOW_BUILDER_ID:-local-builder-01}"
display_name="${APPFACTORY_PRODUCT_FLOW_DISPLAY_NAME:-Local Builder 01}"
builder_image="${APPFACTORY_PRODUCT_FLOW_BUILDER_IMAGE:-oneappfactory/builder:local}"
timeout_seconds="${APPFACTORY_PRODUCT_FLOW_TIMEOUT_SECONDS:-2400}"
poll_interval="${APPFACTORY_PRODUCT_FLOW_POLL_INTERVAL:-2}"
reviewer_id="${APPFACTORY_PRODUCT_FLOW_REVIEWER_ID:-product-e2e-reviewer}"
delivery_status="${APPFACTORY_PRODUCT_FLOW_DELIVERY_STATUS:-approved_for_signing}"
delivery_summary="${APPFACTORY_PRODUCT_FLOW_DELIVERY_SUMMARY:-}"
delivery_next_action="${APPFACTORY_PRODUCT_FLOW_DELIVERY_NEXT_ACTION:-}"
delivery_release_channel="${APPFACTORY_PRODUCT_FLOW_RELEASE_CHANNEL:-}"
delivery_rollout_percent="${APPFACTORY_PRODUCT_FLOW_ROLLOUT_PERCENT:-0}"
follow_up_status="${APPFACTORY_PRODUCT_FLOW_FOLLOW_UP_STATUS:-}"
follow_up_summary="${APPFACTORY_PRODUCT_FLOW_FOLLOW_UP_SUMMARY:-}"
follow_up_owner_id="${APPFACTORY_PRODUCT_FLOW_FOLLOW_UP_OWNER_ID:-$reviewer_id}"
device_verification_status="${APPFACTORY_PRODUCT_FLOW_DEVICE_VERIFICATION_STATUS:-}"
device_verification_summary="${APPFACTORY_PRODUCT_FLOW_DEVICE_VERIFICATION_SUMMARY:-}"
output_root="${APPFACTORY_PRODUCT_FLOW_OUTPUT_ROOT:-$repo_root/workspace/appfactory/product-e2e}"

workspace_root="$(REPO_ROOT="$repo_root" python3 - <<'PY'
import json
import os
from pathlib import Path

default_workspace = os.path.expanduser("~/.appfactory/workspace")
config_path = Path(os.environ["REPO_ROOT"]) / "config" / "config.json"
workspace = default_workspace
if config_path.is_file():
    try:
        data = json.loads(config_path.read_text(encoding="utf-8"))
        workspace = data.get("agents", {}).get("defaults", {}).get("workspace", workspace) or workspace
    except Exception:
        pass
print(os.path.expanduser(workspace))
PY
)"
appfactory_root="$workspace_root/appfactory"
shared_pub_cache="$appfactory_root/.runtime/pub-cache"
shared_gradle_user_home="$appfactory_root/.runtime/gradle-user-home"
cache_mode_before="cold"
pub_cache_preexisting="false"
gradle_user_home_preexisting="false"
gradle_wrapper_dists_preexisting="false"
if [[ -d "$shared_pub_cache" ]]; then
  pub_cache_preexisting="true"
fi
if [[ -d "$shared_gradle_user_home" ]]; then
  gradle_user_home_preexisting="true"
  cache_mode_before="warm"
fi
if [[ -d "$shared_gradle_user_home/wrapper/dists" ]]; then
  gradle_wrapper_dists_preexisting="true"
  cache_mode_before="warm"
fi

usage() {
  cat <<'EOF'
Usage:
  scripts/run-appfactory-product-e2e.sh \
    [--api-base http://127.0.0.1:18800] \
    [--requirement "..."] \
    [--requirement-file examples/appfactory/bookkeeping/requirement.md] \
    [--job-id job-product-e2e-001] \
    [--prd-id prd-product-e2e-001] \
    [--template-id flutter-finance-lite] \
    [--reviewer-id product-e2e-reviewer]

Options:
  --api-base                    Backend API base URL.
  --requirement                 Inline requirement text.
  --requirement-file            Requirement file path.
  --job-id                      Optional fixed job id.
  --prd-id                      Optional fixed PRD id.
  --template-id                 Template id.
  --builder-id                  Builder id used for registration.
  --display-name                Builder display name.
  --builder-image               Builder image label stored in worker profile.
  --timeout-seconds             Max seconds to wait for terminal job state. Default: 2400.
  --poll-interval               Poll interval in seconds.
  --reviewer-id                 Reviewer id used for delivery record.
  --delivery-status             Delivery status, default approved_for_signing.
  --delivery-summary            Optional delivery summary override.
  --delivery-next-action        Optional delivery next action override.
  --release-channel             Optional release channel.
  --rollout-percent             Optional rollout percent.
  --follow-up-status            Optional release follow-up status.
  --follow-up-summary           Optional release follow-up summary.
  --follow-up-owner-id          Optional follow-up owner id.
  --device-verification-status  Optional device verification status.
  --device-verification-summary Optional device verification summary.
  --output-root                 Output root for archived snapshots.
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
    --reviewer-id)
      reviewer_id="$2"
      shift 2
      ;;
    --delivery-status)
      delivery_status="$2"
      shift 2
      ;;
    --delivery-summary)
      delivery_summary="$2"
      shift 2
      ;;
    --delivery-next-action)
      delivery_next_action="$2"
      shift 2
      ;;
    --release-channel)
      delivery_release_channel="$2"
      shift 2
      ;;
    --rollout-percent)
      delivery_rollout_percent="$2"
      shift 2
      ;;
    --follow-up-status)
      follow_up_status="$2"
      shift 2
      ;;
    --follow-up-summary)
      follow_up_summary="$2"
      shift 2
      ;;
    --follow-up-owner-id)
      follow_up_owner_id="$2"
      shift 2
      ;;
    --device-verification-status)
      device_verification_status="$2"
      shift 2
      ;;
    --device-verification-summary)
      device_verification_summary="$2"
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
  echo "hint: product-flow backend default is http://127.0.0.1:18800" >&2
  echo "hint: if 18800 returns 404, that launcher is stale and does not expose current AppFactory routes" >&2
  echo "hint: start or restart oneappfactory-launcher, or point APPFACTORY_PRODUCT_FLOW_API_BASE to the live launcher endpoint" >&2
  exit 1
}

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
job_id="${job_id:-job-product-e2e-$timestamp}"
prd_id="${prd_id:-prd-product-e2e-$timestamp}"
run_dir="$output_root/runs/$timestamp"
mkdir -p "$run_dir"

ensure_api_reachable

post_json() {
  local path="$1"
  local payload="$2"
  local response
  local status
  local body

  response="$(curl -sS -X POST "$api_base$path" \
    --connect-timeout "$curl_connect_timeout_seconds" \
    --max-time "$curl_max_time_seconds" \
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

  response="$(curl -sS \
    --connect-timeout "$curl_connect_timeout_seconds" \
    --max-time "$curl_max_time_seconds" \
    "$api_base$path" -w $'\n%{http_code}')"
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

json_builder_output_run_id() {
  python3 -c '
import json
import re
import sys

value = json.load(sys.stdin).get("builder_output_path") or ""
match = re.search(r"/runs/([^/]+)/", value)
sys.stdout.write(match.group(1) if match else "")
'
}

write_snapshot() {
  local file_name="$1"
  local body="$2"
  printf '%s\n' "$body" > "$run_dir/$file_name"
}

record_step() {
  local step_name="$1"
  local body="$2"
  echo "$step_name" >&2
  write_snapshot "$step_name.json" "$body"
}

reconcile_job_readiness() {
  local attempt=1
  local job_response
  local action
  while [[ "$attempt" -le 6 ]]; do
    job_response="$(get_json "/api/v1/jobs/$job_id")"
    write_snapshot "job-readiness-$attempt.json" "$job_response"
    action="$(printf '%s' "$job_response" | json_get "status_context.suggested_action" 2>/dev/null || true)"
    case "$action" in
      submit_prd_approval)
        echo "== submit PRD approval =="
        record_step "prd-approval-$attempt" "$(post_json "/api/v1/prds/$prd_id:submit-approval" "$(python3 -c 'import json, sys; print(json.dumps({"job_id": sys.argv[1]}, ensure_ascii=False))' "$job_id")")"
        ;;
      submit_template_approval)
        echo "== submit template approval =="
        record_step "template-approval-$attempt" "$(post_json "/api/v1/templates/$template_id:submit-approval" "$(python3 -c 'import json, sys; print(json.dumps({"job_id": sys.argv[1], "prd_id": sys.argv[2]}, ensure_ascii=False))' "$job_id" "$prd_id")")"
        ;;
      compile_prepare_bundle)
        echo "== compile prepare bundle =="
        record_step "compile-prepare-$attempt" "$(post_json "/api/v1/jobs/$job_id:compile-prepare" "{}")"
        ;;
      ""|start)
        printf '%s' "$job_response"
        return 0
        ;;
      *)
        echo "unsupported suggested_action before start: $action" >&2
        echo "$job_response" >&2
        exit 1
        ;;
    esac
    attempt=$((attempt + 1))
  done

  echo "job readiness reconciliation exceeded retry budget" >&2
  exit 1
}

echo "== compile requirement =="
compile_response="$(post_json "/api/v1/prds:compile" "$(REQUIREMENT_TEXT="$requirement_text" JOB_ID="$job_id" PRD_ID="$prd_id" BUILDER_IMAGE="$builder_image" python3 - <<'PY'
import json
import os

print(json.dumps({
    "requirement_text": os.environ["REQUIREMENT_TEXT"],
    "job_id": os.environ["JOB_ID"],
    "prd_id": os.environ["PRD_ID"],
    "executor_image": os.environ.get("BUILDER_IMAGE", ""),
}, ensure_ascii=False))
PY
)")"
record_step "compile-response" "$compile_response"

compile_template_id="$(printf '%s' "$compile_response" | json_get "template_id" 2>/dev/null || true)"
if [[ -n "$compile_template_id" ]]; then
  template_id="$compile_template_id"
fi

echo "== create job =="
job_create_response="$(post_json "/api/v1/jobs" "$(PRD_ID="$prd_id" TEMPLATE_ID="$template_id" python3 - <<'PY'
import json
import os

print(json.dumps({
    "prd_id": os.environ["PRD_ID"],
    "template_id": os.environ["TEMPLATE_ID"],
}, ensure_ascii=False))
PY
)")"
record_step "job-create-response" "$job_create_response"

echo "== register builder =="
builder_register_response="$(post_json "/internal/v1/builders:register" "$(BUILDER_ID="$builder_id" DISPLAY_NAME="$display_name" BUILDER_IMAGE="$builder_image" python3 - <<'PY'
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
)")"
record_step "builder-register-response" "$builder_register_response"

echo "== reconcile approvals and prepare =="
ready_job_response="$(reconcile_job_readiness)"
write_snapshot "job-ready-response.json" "$ready_job_response"

echo "== start job =="
job_start_response="$(post_json "/api/v1/jobs/$job_id:start" "$(python3 -c 'import json, sys; print(json.dumps({"timeout_seconds": int(sys.argv[1])}, ensure_ascii=False))' "$timeout_seconds")")"
record_step "job-start-response" "$job_start_response"

echo "== poll job status =="
elapsed=0
final_job_response=""
final_status=""
while [[ "$elapsed" -le "$timeout_seconds" ]]; do
  final_job_response="$(get_json "/api/v1/jobs/$job_id")"
  final_status="$(printf '%s' "$final_job_response" | json_get "status" 2>/dev/null || true)"
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

record_step "job-final-response" "$final_job_response"

echo "== fetch artifacts, events, notifications =="
artifacts_response="$(get_json "/api/v1/jobs/$job_id/artifacts")"
events_response="$(get_json "/api/v1/jobs/$job_id/events")"
notifications_response="$(get_json "/api/v1/notifications")"
record_step "artifacts-response" "$artifacts_response"
record_step "events-response" "$events_response"
record_step "notifications-response" "$notifications_response"

if [[ "$final_status" != "completed" ]]; then
  echo "product flow finished in terminal failure state" >&2
  echo "job_id=$job_id" >&2
  echo "status=$final_status" >&2
  echo "snapshots_dir=$run_dir" >&2
  exit 1
fi

run_id="$(printf '%s' "$events_response" | json_last_run_id)"
if [[ -z "$run_id" ]]; then
  run_id="$(printf '%s' "$final_job_response" | json_builder_output_run_id)"
fi
if [[ -z "$run_id" ]]; then
  echo "failed to derive run_id from events or builder_output_path" >&2
  echo "snapshots_dir=$run_dir" >&2
  exit 1
fi

echo "== prepare review =="
review_response="$(post_json "/internal/v1/reviews:prepare" "$(python3 -c 'import json, sys; print(json.dumps({"run_id": sys.argv[1]}, ensure_ascii=False))' "$run_id")")"
record_step "review-prepare-response" "$review_response"

review_bundle_path="$(printf '%s' "$review_response" | json_get "review_bundle_path" 2>/dev/null || true)"
handoff_checklist_path="$(printf '%s' "$review_response" | json_get "handoff_checklist_path" 2>/dev/null || true)"
artifact_manifest_path="$(printf '%s' "$review_response" | json_get "artifact_manifest_path" 2>/dev/null || true)"
metrics_path="$(printf '%s' "$review_response" | json_get "metrics_path" 2>/dev/null || true)"
builder_output_path="$(printf '%s' "$final_job_response" | json_get "builder_output_path" 2>/dev/null || true)"

echo "== record delivery =="
delivery_response="$(post_json "/internal/v1/deliveries:record" "$(RUN_ID="$run_id" REVIEWER_ID="$reviewer_id" DELIVERY_STATUS="$delivery_status" DELIVERY_SUMMARY="$delivery_summary" DELIVERY_NEXT_ACTION="$delivery_next_action" DELIVERY_RELEASE_CHANNEL="$delivery_release_channel" DELIVERY_ROLLOUT_PERCENT="$delivery_rollout_percent" REVIEW_BUNDLE_PATH="$review_bundle_path" HANDOFF_CHECKLIST_PATH="$handoff_checklist_path" ARTIFACT_MANIFEST_PATH="$artifact_manifest_path" METRICS_PATH="$metrics_path" BUILDER_OUTPUT_PATH="$builder_output_path" DEVICE_VERIFICATION_STATUS="$device_verification_status" DEVICE_VERIFICATION_SUMMARY="$device_verification_summary" python3 - <<'PY'
import json
import os

evidence_paths = [
    os.environ.get("REVIEW_BUNDLE_PATH", ""),
    os.environ.get("HANDOFF_CHECKLIST_PATH", ""),
    os.environ.get("ARTIFACT_MANIFEST_PATH", ""),
    os.environ.get("METRICS_PATH", ""),
    os.environ.get("BUILDER_OUTPUT_PATH", ""),
]
evidence_paths = [item for item in evidence_paths if item]

payload = {
    "run_id": os.environ["RUN_ID"],
    "reviewer_id": os.environ["REVIEWER_ID"],
    "status": os.environ["DELIVERY_STATUS"],
    "summary": os.environ.get("DELIVERY_SUMMARY", ""),
    "next_action": os.environ.get("DELIVERY_NEXT_ACTION", ""),
    "release_channel": os.environ.get("DELIVERY_RELEASE_CHANNEL", ""),
    "rollout_percent": int(os.environ.get("DELIVERY_ROLLOUT_PERCENT", "0") or "0"),
    "evidence_paths": evidence_paths,
}
if os.environ.get("DEVICE_VERIFICATION_STATUS", ""):
    payload["device_verification_status"] = os.environ["DEVICE_VERIFICATION_STATUS"]
    payload["device_verification_summary"] = os.environ.get("DEVICE_VERIFICATION_SUMMARY", "")
print(json.dumps(payload, ensure_ascii=False))
PY
)")"
record_step "delivery-record-response" "$delivery_response"

follow_up_response=""
if [[ -n "$follow_up_status" ]]; then
  echo "== record release follow-up =="
  follow_up_response="$(post_json "/internal/v1/deliveries:follow-up" "$(RUN_ID="$run_id" OWNER_ID="$follow_up_owner_id" FOLLOW_UP_STATUS="$follow_up_status" FOLLOW_UP_SUMMARY="$follow_up_summary" python3 - <<'PY'
import json
import os

print(json.dumps({
    "run_id": os.environ["RUN_ID"],
    "owner_id": os.environ["OWNER_ID"],
    "status": os.environ["FOLLOW_UP_STATUS"],
    "summary": os.environ.get("FOLLOW_UP_SUMMARY", ""),
}, ensure_ascii=False))
PY
)")"
  record_step "delivery-follow-up-response" "$follow_up_response"
fi

echo "== fetch final public surfaces =="
final_public_job_response="$(get_json "/api/v1/jobs/$job_id")"
final_artifacts_response="$(get_json "/api/v1/jobs/$job_id/artifacts")"
final_events_response="$(get_json "/api/v1/jobs/$job_id/events")"
final_notifications_response="$(get_json "/api/v1/notifications")"
record_step "job-public-final-response" "$final_public_job_response"
record_step "artifacts-final-response" "$final_artifacts_response"
record_step "events-final-response" "$final_events_response"
record_step "notifications-final-response" "$final_notifications_response"

result_json="$run_dir/product-flow-result.json"
script_finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
script_end_epoch="$(date +%s)"
RESULT_JSON="$result_json" RUN_DIR="$run_dir" OUTPUT_ROOT="$output_root" JOB_ID="$job_id" PRD_ID="$prd_id" TEMPLATE_ID="$template_id" RUN_ID="$run_id" FINAL_STATUS="$final_status" DELIVERY_STATUS="$delivery_status" FOLLOW_UP_STATUS="$follow_up_status" REVIEWER_ID="$reviewer_id" SCRIPT_STARTED_AT="$script_started_at" SCRIPT_FINISHED_AT="$script_finished_at" SCRIPT_START_EPOCH="$script_start_epoch" SCRIPT_END_EPOCH="$script_end_epoch" WORKSPACE_ROOT="$workspace_root" APPFACTORY_ROOT="$appfactory_root" SHARED_PUB_CACHE="$shared_pub_cache" SHARED_GRADLE_USER_HOME="$shared_gradle_user_home" PUB_CACHE_PREEXISTING="$pub_cache_preexisting" GRADLE_USER_HOME_PREEXISTING="$gradle_user_home_preexisting" GRADLE_WRAPPER_DISTS_PREEXISTING="$gradle_wrapper_dists_preexisting" CACHE_MODE_BEFORE="$cache_mode_before" python3 - <<'PY'
import json
import os
from datetime import datetime, timezone
from pathlib import Path


def parse_ts(value: str):
  if not value:
    return None
  try:
    normalized = value
    if value.endswith("Z"):
      body = value[:-1]
      if "." in body:
        head, frac = body.split(".", 1)
        body = head + "." + frac[:6]
      normalized = body + "+00:00"
    return datetime.fromisoformat(normalized)
  except ValueError:
    return None


def seconds_between(start: str, finish: str):
  left = parse_ts(start)
  right = parse_ts(finish)
  if left is None or right is None:
    return None
  return round((right - left).total_seconds(), 3)


run_dir = Path(os.environ["RUN_DIR"])
job_response = json.loads((run_dir / "job-final-response.json").read_text(encoding="utf-8"))
events_response = json.loads((run_dir / "events-final-response.json").read_text(encoding="utf-8"))

builder_output = None
builder_output_path = job_response.get("builder_output_path", "")
if builder_output_path:
  candidate = Path(os.environ["APPFACTORY_ROOT"]) / Path(builder_output_path)
  if candidate.is_file():
    builder_output = json.loads(candidate.read_text(encoding="utf-8"))

metrics = None
metrics_path = ""
review_prepare_path = run_dir / "review-prepare-response.json"
if review_prepare_path.is_file():
  review_prepare = json.loads(review_prepare_path.read_text(encoding="utf-8"))
  metrics_path = review_prepare.get("metrics_path", "")
if metrics_path:
  candidate = Path(os.environ["APPFACTORY_ROOT"]) / Path(metrics_path)
  if candidate.is_file():
    metrics = json.loads(candidate.read_text(encoding="utf-8"))

items = events_response.get("items", [])
build_apk_started_at = ""
run_terminal_at = ""
for item in items:
  summary = item.get("summary", "")
  if not build_apk_started_at and "acceptance check: check-flutter-build-apk" in summary:
    build_apk_started_at = item.get("at", "")
  if item.get("type") in {"run_completed", "run_failed"}:
    run_terminal_at = item.get("at", "")

run_started_at = ""
run_finished_at = ""
if builder_output:
  run_started_at = builder_output.get("started_at", "")
  run_finished_at = builder_output.get("finished_at", "")
if not run_started_at:
  run_started_at = job_response.get("started_at", "")
if not run_finished_at:
  run_finished_at = job_response.get("finished_at", "")
if not run_terminal_at:
  run_terminal_at = run_finished_at

script_duration_seconds = int(os.environ["SCRIPT_END_EPOCH"]) - int(os.environ["SCRIPT_START_EPOCH"])
run_duration_seconds = seconds_between(run_started_at, run_finished_at)
build_apk_duration_seconds = seconds_between(build_apk_started_at, run_terminal_at)

shared_pub_cache = Path(os.environ["SHARED_PUB_CACHE"])
shared_gradle_user_home = Path(os.environ["SHARED_GRADLE_USER_HOME"])
cache_state_before = {
  "mode": os.environ["CACHE_MODE_BEFORE"],
  "pub_cache_preexisting": os.environ["PUB_CACHE_PREEXISTING"] == "true",
  "gradle_user_home_preexisting": os.environ["GRADLE_USER_HOME_PREEXISTING"] == "true",
  "gradle_wrapper_dists_preexisting": os.environ["GRADLE_WRAPPER_DISTS_PREEXISTING"] == "true",
}
cache_state_after = {
  "pub_cache_exists": shared_pub_cache.is_dir(),
  "gradle_user_home_exists": shared_gradle_user_home.is_dir(),
  "gradle_wrapper_dists_exists": (shared_gradle_user_home / "wrapper" / "dists").is_dir(),
}

result = {
    "schema_version": "0.1.0",
    "job_id": os.environ["JOB_ID"],
    "prd_id": os.environ["PRD_ID"],
    "template_id": os.environ["TEMPLATE_ID"],
    "run_id": os.environ["RUN_ID"],
    "job_status": os.environ["FINAL_STATUS"],
    "delivery_status": os.environ["DELIVERY_STATUS"],
    "follow_up_status": os.environ.get("FOLLOW_UP_STATUS", ""),
    "reviewer_id": os.environ["REVIEWER_ID"],
    "script_started_at": os.environ["SCRIPT_STARTED_AT"],
    "script_finished_at": os.environ["SCRIPT_FINISHED_AT"],
    "script_duration_seconds": script_duration_seconds,
    "run_started_at": run_started_at,
    "run_finished_at": run_finished_at,
    "run_duration_seconds": run_duration_seconds,
    "build_apk_started_at": build_apk_started_at,
    "build_apk_finished_at": run_terminal_at,
    "build_apk_duration_seconds": build_apk_duration_seconds,
    "builder_metrics_duration_seconds": metrics.get("duration_seconds") if metrics else None,
    "cache_state_before": cache_state_before,
    "cache_state_after": cache_state_after,
    "shared_cache_paths": {
      "workspace_root": os.environ["WORKSPACE_ROOT"],
      "appfactory_root": os.environ["APPFACTORY_ROOT"],
      "pub_cache": os.environ["SHARED_PUB_CACHE"],
      "gradle_user_home": os.environ["SHARED_GRADLE_USER_HOME"],
    },
    "run_dir": os.environ["RUN_DIR"],
    "snapshots": sorted([path.name for path in Path(os.environ["RUN_DIR"]).glob("*.json")]),
}
Path(os.environ["RESULT_JSON"]).write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
Path(os.environ["OUTPUT_ROOT"]).mkdir(parents=True, exist_ok=True)
Path(os.environ["OUTPUT_ROOT"], "latest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
Path(os.environ["OUTPUT_ROOT"], "latest.md").write_text(
    "# AppFactory Product Flow Latest Run\n\n"
    f"- job_id: {result['job_id']}\n"
    f"- prd_id: {result['prd_id']}\n"
    f"- template_id: {result['template_id']}\n"
    f"- run_id: {result['run_id']}\n"
    f"- job_status: {result['job_status']}\n"
    f"- delivery_status: {result['delivery_status']}\n"
    f"- follow_up_status: {result['follow_up_status'] or 'none'}\n"
    f"- reviewer_id: {result['reviewer_id']}\n"
  f"- script_duration_seconds: {result['script_duration_seconds']}\n"
  f"- run_duration_seconds: {result['run_duration_seconds']}\n"
  f"- build_apk_duration_seconds: {result['build_apk_duration_seconds']}\n"
  f"- cache_mode_before: {result['cache_state_before']['mode']}\n"
  f"- gradle_wrapper_dists_preexisting: {result['cache_state_before']['gradle_wrapper_dists_preexisting']}\n"
    f"- run_dir: {result['run_dir']}\n",
    encoding="utf-8",
)
PY

echo "== product flow passed =="
echo "job_id=$job_id"
echo "prd_id=$prd_id"
echo "template_id=$template_id"
echo "run_id=$run_id"
echo "status=$final_status"
echo "delivery_status=$delivery_status"
if [[ -n "$follow_up_status" ]]; then
  echo "follow_up_status=$follow_up_status"
fi
echo "run_dir=$run_dir"
echo "latest_json=$output_root/latest.json"
echo "latest_markdown=$output_root/latest.md"