#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script_started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

api_base="${APPFACTORY_JOBS_API_BASE:-${APPFACTORY_PRODUCT_FLOW_API_BASE:-http://127.0.0.1:18800}}"
curl_connect_timeout_seconds="${APPFACTORY_JOBS_CURL_CONNECT_TIMEOUT_SECONDS:-3}"
curl_max_time_seconds="${APPFACTORY_JOBS_CURL_MAX_TIME_SECONDS:-30}"
requirement_text=""
requirement_file="${APPFACTORY_JOBS_REQUIREMENT_FILE:-$repo_root/examples/appfactory/generic/weight-tracker/requirement.md}"
title_hint="${APPFACTORY_JOBS_TITLE_HINT:-/jobs regression generic weight-tracker manual parity}"
job_id="${APPFACTORY_JOBS_JOB_ID:-}"
prd_id="${APPFACTORY_JOBS_PRD_ID:-}"
template_id="${APPFACTORY_JOBS_TEMPLATE_ID:-}"
goal_summary="${APPFACTORY_JOBS_GOAL_SUMMARY:-validate the current /jobs full-chain manual-equivalent path}"
human_notes_json="${APPFACTORY_JOBS_HUMAN_NOTES_JSON:-}"
builder_image="${APPFACTORY_JOBS_BUILDER_IMAGE:-oneappfactory/builder:local}"
timeout_seconds="${APPFACTORY_JOBS_TIMEOUT_SECONDS:-5400}"
poll_interval="${APPFACTORY_JOBS_POLL_INTERVAL:-2}"
output_root="${APPFACTORY_JOBS_REGRESSION_ROOT:-$repo_root/workspace/appfactory/jobs-ui-regression}"
appfactory_root_candidates="${APPFACTORY_JOBS_APPFACTORY_ROOT_CANDIDATES:-$repo_root/workspace/appfactory:$HOME/.appfactory/workspace/appfactory}"
require_manual_equivalent="${APPFACTORY_JOBS_REQUIRE_MANUAL_EQUIVALENCE:-1}"
completion_probe_enabled="${APPFACTORY_JOBS_COMPLETION_PROBE_ENABLED:-1}"
completion_probe_timeout_seconds="${APPFACTORY_JOBS_COMPLETION_PROBE_TIMEOUT_SECONDS:-20}"
completion_probe_max_tokens="${APPFACTORY_JOBS_COMPLETION_PROBE_MAX_TOKENS:-4}"
completion_probe_prompt="${APPFACTORY_JOBS_COMPLETION_PROBE_PROMPT:-reply with ok}"
patch_wait_failfast_enabled="${APPFACTORY_JOBS_PATCH_WAIT_FAILFAST_ENABLED:-1}"
patch_wait_failfast_seconds="${APPFACTORY_JOBS_PATCH_WAIT_FAILFAST_SECONDS:-180}"
probe_file_path="${APPFACTORY_JOBS_PROBE_FILE_PATH:-lib/oneappfactory_executor_probe.dart}"
auto_resume_enabled="${APPFACTORY_JOBS_AUTO_RESUME_ENABLED:-1}"
auto_resume_max_attempts="${APPFACTORY_JOBS_AUTO_RESUME_MAX_ATTEMPTS:-2}"
auto_resume_target_failure_signature="${APPFACTORY_JOBS_AUTO_RESUME_FAILURE_SIGNATURE:-builder_runtime_model_request_failed}"
auto_resume_note="${APPFACTORY_JOBS_AUTO_RESUME_NOTE:-auto resume after transient builder runtime failure}"

stage_status_preflight="pending"
stage_status_compile="pending"
stage_status_create="pending"
stage_status_register_builder="pending"
stage_status_start="pending"
stage_status_run="pending"
terminal_job_status="unknown"
failure_bucket="none"
failure_signature=""
failure_domain=""
failure_category=""
failure_summary=""
latest_run_id=""
auto_resume_attempts_used="0"
auto_resume_triggered="false"
auto_resume_last_http_status=""
auto_resume_last_job_status=""
auto_resume_last_resume_mode=""
auto_resume_last_failure_signature=""
auto_resume_last_reason=""
preflight_builder_runtime_enabled=""
preflight_builder_runtime_default_model=""
preflight_gateway_start_allowed=""
preflight_gateway_start_reason=""
preflight_config_path=""
preflight_uses_user_home_config=""
preflight_config_load_error=""
preflight_completion_probe_status=""
preflight_completion_probe_provider=""
preflight_completion_probe_model_id=""
preflight_completion_probe_api_base=""
preflight_completion_probe_error=""
preflight_completion_probe_latency_seconds=""

usage() {
  cat <<'EOF'
Usage:
  scripts/run-appfactory-jobs-regression.sh \
    [--api-base http://127.0.0.1:18800] \
    [--requirement-file examples/appfactory/generic/weight-tracker/requirement.md] \
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
  --allow-non-manual-equivalent
                     Do not fail completed runs that are only probe-only / non-manual-equivalent.
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
    --allow-non-manual-equivalent)
      require_manual_equivalent="0"
      shift
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

run_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
job_id="${job_id:-job-jobs-ui-regression-$run_timestamp}"
prd_id="${prd_id:-prd-jobs-ui-regression-$run_timestamp}"
run_dir="$output_root/runs/$run_timestamp"
mkdir -p "$run_dir"

console_log_path="$run_dir/console.log"
latest_console_log_path="$output_root/latest.log"
: > "$console_log_path"
: > "$latest_console_log_path"
exec > >(tee -a "$console_log_path" | tee "$latest_console_log_path") 2>&1

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
  local summary

  probe_status="$(curl -sS -o /dev/null -w '%{http_code}' \
    --connect-timeout "$curl_connect_timeout_seconds" \
    --max-time "$curl_max_time_seconds" \
    "$api_base$probe_path" || true)"

  if [[ "$probe_status" == 2* ]]; then
    return 0
  fi

  summary="appfactory api is unavailable: $api_base$probe_path (status=${probe_status:-000})"
  failure_signature="api_unavailable"
  failure_domain="environment"
  failure_category="api_unavailable"
  fail_with_stage "compile" "environment" "$summary"
}

ensure_manual_equivalent_preflight() {
  local config_response
  local gateway_status_response
  local summary

  if [[ "$require_manual_equivalent" != "1" ]]; then
    stage_status_preflight="skipped"
    return 0
  fi

  echo "== preflight runtime mode =="
  if gateway_status_response="$(request_json GET "/api/gateway/status" 2>/dev/null || true)" && [[ -n "$gateway_status_response" ]]; then
    record_step "gateway-status-response.json" "$gateway_status_response"
    preflight_config_path="$(printf '%s' "$gateway_status_response" | json_get "config_path" 2>/dev/null || true)"
    preflight_uses_user_home_config="$(printf '%s' "$gateway_status_response" | json_get "uses_user_home_config" 2>/dev/null || true)"
    preflight_config_load_error="$(printf '%s' "$gateway_status_response" | json_get "config_load_error" 2>/dev/null || true)"
    preflight_gateway_start_allowed="$(printf '%s' "$gateway_status_response" | json_get "gateway_start_allowed" 2>/dev/null || true)"
    preflight_gateway_start_reason="$(printf '%s' "$gateway_status_response" | json_get "gateway_start_reason" 2>/dev/null || true)"
  fi

  if [[ -z "$preflight_config_path" ]]; then
    failure_signature="launcher_observability_missing"
    failure_domain="environment"
    failure_category="runtime_preflight:launcher_observability_missing"
    fail_with_stage "preflight" "environment" "builder runtime preflight failed: /api/gateway/status did not expose config_path; current launcher is stale or missing config-drift telemetry"
  fi

  if ! config_response="$(request_json GET "/api/config")"; then
    if [[ "$preflight_config_load_error" == *"unsupported config version"* ]]; then
      failure_signature="unsupported_config_version"
      failure_domain="environment"
      failure_category="runtime_preflight:unsupported_config_version"
      summary="builder runtime preflight failed: launcher could not load config: $preflight_config_load_error; config_path=$preflight_config_path"
      if [[ "$preflight_uses_user_home_config" == "true" ]]; then
        summary+="; uses_user_home_config=true"
      fi
      fail_with_stage "preflight" "environment" "$summary"
    fi

    failure_signature="runtime_preflight_config_unavailable"
    failure_domain="environment"
    failure_category="runtime_preflight:config_unavailable"
    summary="failed to fetch /api/config for runtime preflight; config_path=$preflight_config_path"
    if [[ -n "$preflight_config_load_error" ]]; then
      summary+="; config_load_error=$preflight_config_load_error"
    fi
    if [[ "$preflight_uses_user_home_config" == "true" ]]; then
      summary+="; uses_user_home_config=true"
    fi
    fail_with_stage "preflight" "environment" "$summary"
  fi
  record_step "config-response.json" "$config_response"

  preflight_builder_runtime_enabled="$(printf '%s' "$config_response" | json_get "appfactory.builder_runtime.enabled" 2>/dev/null || true)"
  preflight_builder_runtime_default_model="$(printf '%s' "$config_response" | json_get_model_primary "appfactory.builder_runtime.default_model" 2>/dev/null || true)"

  if [[ "$preflight_builder_runtime_enabled" != "true" ]]; then
    failure_signature="builder_runtime_disabled"
    failure_domain="environment"
    failure_category="runtime_preflight:thin_fallback_disabled"
    summary="builder runtime preflight failed: /api/config reports appfactory.builder_runtime.enabled=false; current /jobs path is thin fallback, so strict manual-equivalent regression is blocked before compile"
    if [[ -n "$preflight_gateway_start_reason" ]]; then
      summary+="; gateway_status=$preflight_gateway_start_reason"
    fi
    fail_with_stage "preflight" "environment" "$summary"
  fi

  if [[ -z "$preflight_builder_runtime_default_model" ]]; then
    failure_signature="builder_runtime_default_model_missing"
    failure_domain="environment"
    failure_category="runtime_preflight:default_model_missing"
    summary="builder runtime preflight failed: /api/config reports appfactory.builder_runtime.default_model.primary is empty; current /jobs path cannot be treated as real builder-runtime readiness"
    if [[ -n "$preflight_gateway_start_reason" ]]; then
      summary+="; gateway_status=$preflight_gateway_start_reason"
    fi
    fail_with_stage "preflight" "environment" "$summary"
  fi

  if [[ "$preflight_gateway_start_allowed" == "false" ]]; then
    failure_signature="gateway_start_blocked"
    failure_domain="environment"
    failure_category="runtime_preflight:gateway_start_blocked"
    summary="builder runtime preflight failed: /api/gateway/status reports gateway_start_allowed=false; default_model=$preflight_builder_runtime_default_model"
    if [[ -n "$preflight_gateway_start_reason" ]]; then
      summary+="; gateway_status=$preflight_gateway_start_reason"
    fi
    fail_with_stage "preflight" "environment" "$summary"
  fi

  ensure_completion_probe_healthy

  stage_status_preflight="passed"
  echo "preflight_config_path=$preflight_config_path"
  if [[ -n "$preflight_uses_user_home_config" ]]; then
    echo "preflight_uses_user_home_config=$preflight_uses_user_home_config"
  fi
  if [[ -n "$preflight_config_load_error" ]]; then
    echo "preflight_config_load_error=$preflight_config_load_error"
  fi
  echo "preflight_builder_runtime_enabled=$preflight_builder_runtime_enabled"
  echo "preflight_builder_runtime_default_model=$preflight_builder_runtime_default_model"
  if [[ -n "$preflight_gateway_start_allowed" ]]; then
    echo "preflight_gateway_start_allowed=$preflight_gateway_start_allowed"
  fi
  if [[ -n "$preflight_gateway_start_reason" ]]; then
    echo "preflight_gateway_start_reason=$preflight_gateway_start_reason"
  fi
  if [[ -n "$preflight_completion_probe_status" ]]; then
    echo "preflight_completion_probe_status=$preflight_completion_probe_status"
  fi
  if [[ -n "$preflight_completion_probe_provider" ]]; then
    echo "preflight_completion_probe_provider=$preflight_completion_probe_provider"
  fi
  if [[ -n "$preflight_completion_probe_model_id" ]]; then
    echo "preflight_completion_probe_model_id=$preflight_completion_probe_model_id"
  fi
  if [[ -n "$preflight_completion_probe_api_base" ]]; then
    echo "preflight_completion_probe_api_base=$preflight_completion_probe_api_base"
  fi
  if [[ -n "$preflight_completion_probe_latency_seconds" ]]; then
    echo "preflight_completion_probe_latency_seconds=$preflight_completion_probe_latency_seconds"
  fi
  if [[ -n "$preflight_completion_probe_error" ]]; then
    echo "preflight_completion_probe_error=$preflight_completion_probe_error"
  fi
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

request_json_capture() {
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
  REQUEST_JSON_CAPTURE_STATUS="$status"
  REQUEST_JSON_CAPTURE_BODY="$body"
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

json_get_model_primary() {
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
if isinstance(value, str):
  sys.stdout.write(value)
elif isinstance(value, dict):
  primary = value.get("primary")
  if primary is None:
    sys.exit(1)
  sys.stdout.write(str(primary))
else:
  sys.exit(1)
' "$key_path"
}

resolve_completion_probe_target() {
  local config_path="$1"
  local model_name="$2"
  python3 - "$config_path" "$model_name" <<'PY'
import ipaddress
import json
import sys
from urllib.parse import urlparse

config_path = sys.argv[1]
model_name = sys.argv[2]

openai_compatible_protocols = {
  "openai",
  "litellm",
  "openrouter",
  "groq",
  "zhipu",
  "gemini",
  "nvidia",
  "ollama",
  "moonshot",
  "shengsuanyun",
  "deepseek",
  "cerebras",
  "vivgrid",
  "volcengine",
  "vllm",
  "qwen",
  "qwen-intl",
  "qwen-international",
  "dashscope-intl",
  "qwen-us",
  "dashscope-us",
  "mistral",
  "avian",
  "longcat",
  "modelscope",
  "novita",
  "coding-plan",
  "alibaba-coding",
  "qwen-coding",
  "mimo",
  "anthropic",
}

default_api_bases = {
  "openai": "https://api.openai.com/v1",
  "openrouter": "https://openrouter.ai/api/v1",
  "litellm": "http://localhost:4000/v1",
  "novita": "https://api.novita.ai/openai",
  "groq": "https://api.groq.com/openai/v1",
  "zhipu": "https://open.bigmodel.cn/api/paas/v4",
  "gemini": "https://generativelanguage.googleapis.com/v1beta",
  "nvidia": "https://integrate.api.nvidia.com/v1",
  "ollama": "http://localhost:11434/v1",
  "moonshot": "https://api.moonshot.cn/v1",
  "shengsuanyun": "https://router.shengsuanyun.com/api/v1",
  "deepseek": "https://api.deepseek.com/v1",
  "cerebras": "https://api.cerebras.ai/v1",
  "vivgrid": "https://api.vivgrid.com/v1",
  "volcengine": "https://ark.cn-beijing.volces.com/api/v3",
  "qwen": "https://dashscope.aliyuncs.com/compatible-mode/v1",
  "qwen-intl": "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
  "qwen-international": "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
  "dashscope-intl": "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
  "qwen-us": "https://dashscope-us.aliyuncs.com/compatible-mode/v1",
  "dashscope-us": "https://dashscope-us.aliyuncs.com/compatible-mode/v1",
  "coding-plan": "https://coding-intl.dashscope.aliyuncs.com/v1",
  "alibaba-coding": "https://coding-intl.dashscope.aliyuncs.com/v1",
  "qwen-coding": "https://coding-intl.dashscope.aliyuncs.com/v1",
  "vllm": "http://localhost:8000/v1",
  "mistral": "https://api.mistral.ai/v1",
  "avian": "https://api.avian.io/v1",
  "longcat": "https://api.longcat.chat/openai",
  "modelscope": "https://api-inference.modelscope.cn/v1",
  "mimo": "https://api.xiaomimimo.com/v1",
  "anthropic": "https://api.anthropic.com/v1",
}


def emit(payload):
  sys.stdout.write(json.dumps(payload, ensure_ascii=False))


def is_local_or_private_endpoint(api_base: str) -> bool:
  host = (urlparse(api_base).hostname or "").strip().lower()
  if not host:
    return False
  if host == "localhost" or host.endswith(".local"):
    return True
  try:
    ip = ipaddress.ip_address(host)
  except ValueError:
    return False
  return ip.is_private or ip.is_loopback or ip.is_link_local


try:
  with open(config_path, encoding="utf-8") as handle:
    config = json.load(handle)
except Exception as exc:
  emit({
    "status": "config_unreadable",
    "skip_reason": f"config_unreadable:{exc}",
  })
  raise SystemExit(0)

entry = None
for item in config.get("model_list") or []:
  if not isinstance(item, dict):
    continue
  if str(item.get("model_name") or "").strip() == model_name:
    entry = item
    break

if entry is None:
  emit({
    "status": "model_not_found",
    "skip_reason": "model_not_found_in_config",
  })
  raise SystemExit(0)

raw_model = str(entry.get("model") or "").strip()
if "/" in raw_model:
  protocol, model_id = raw_model.split("/", 1)
else:
  protocol, model_id = "openai", raw_model
protocol = protocol.strip()
model_id = model_id.strip()
api_base = str(entry.get("api_base") or default_api_bases.get(protocol, "")).strip()
auth_method = str(entry.get("auth_method") or "").strip().lower()
local_endpoint = is_local_or_private_endpoint(api_base)

if protocol not in openai_compatible_protocols:
  emit({
    "status": "unsupported_protocol",
    "skip_reason": f"unsupported_protocol:{protocol or 'unknown'}",
    "protocol": protocol,
    "model_id": model_id,
    "api_base": api_base,
    "local_endpoint": local_endpoint,
  })
  raise SystemExit(0)

if not api_base:
  emit({
    "status": "missing_api_base",
    "skip_reason": "missing_api_base",
    "protocol": protocol,
    "model_id": model_id,
    "local_endpoint": local_endpoint,
  })
  raise SystemExit(0)

if auth_method not in {"", "local"}:
  emit({
    "status": "requires_credentials",
    "skip_reason": f"requires_credentials:{auth_method}",
    "protocol": protocol,
    "model_id": model_id,
    "api_base": api_base,
    "local_endpoint": local_endpoint,
  })
  raise SystemExit(0)

if not local_endpoint:
  emit({
    "status": "requires_credentials",
    "skip_reason": "requires_credentials:non_local_endpoint",
    "protocol": protocol,
    "model_id": model_id,
    "api_base": api_base,
    "local_endpoint": local_endpoint,
  })
  raise SystemExit(0)

emit({
  "status": "supported",
  "protocol": protocol,
  "model_id": model_id,
  "api_base": api_base,
  "local_endpoint": local_endpoint,
})
PY
}

ensure_completion_probe_healthy() {
  local probe_target_json=""
  local probe_resolution_status=""
  local probe_skip_reason=""
  local probe_payload=""
  local probe_url=""
  local probe_response_summary=""
  local probe_http_status=""
  local probe_start_epoch=""
  local probe_end_epoch=""
  local probe_body_path=""
  local probe_stderr_path=""

  if [[ "$require_manual_equivalent" != "1" ]]; then
  preflight_completion_probe_status="skipped"
  preflight_completion_probe_error="manual_equivalence_not_required"
  return 0
  fi

  if [[ "$completion_probe_enabled" != "1" ]]; then
  preflight_completion_probe_status="disabled"
  return 0
  fi

  probe_target_json="$(resolve_completion_probe_target "$preflight_config_path" "$preflight_builder_runtime_default_model" 2>/dev/null || true)"
  if [[ -z "$probe_target_json" ]]; then
  preflight_completion_probe_status="skipped"
  preflight_completion_probe_error="completion_probe_resolution_empty"
  return 0
  fi

  probe_resolution_status="$(printf '%s' "$probe_target_json" | json_get "status" 2>/dev/null || true)"
  probe_skip_reason="$(printf '%s' "$probe_target_json" | json_get "skip_reason" 2>/dev/null || true)"
  preflight_completion_probe_provider="$(printf '%s' "$probe_target_json" | json_get "protocol" 2>/dev/null || true)"
  preflight_completion_probe_model_id="$(printf '%s' "$probe_target_json" | json_get "model_id" 2>/dev/null || true)"
  preflight_completion_probe_api_base="$(printf '%s' "$probe_target_json" | json_get "api_base" 2>/dev/null || true)"
  record_step "completion-probe-target.json" "$probe_target_json"

  if [[ "$probe_resolution_status" != "supported" ]]; then
  preflight_completion_probe_status="skipped"
  preflight_completion_probe_error="${probe_skip_reason:-completion_probe_not_supported}"
  return 0
  fi

  echo "== preflight completion probe =="
  probe_payload="$(COMPLETION_PROBE_MODEL_ID="$preflight_completion_probe_model_id" COMPLETION_PROBE_MAX_TOKENS="$completion_probe_max_tokens" COMPLETION_PROBE_PROMPT="$completion_probe_prompt" python3 - <<'PY'
import json
import os

payload = {
  "model": os.environ["COMPLETION_PROBE_MODEL_ID"],
  "messages": [
    {"role": "user", "content": os.environ["COMPLETION_PROBE_PROMPT"]},
  ],
  "max_tokens": int(os.environ["COMPLETION_PROBE_MAX_TOKENS"]),
  "temperature": 0,
}
print(json.dumps(payload, ensure_ascii=False))
PY
)"
  probe_url="${preflight_completion_probe_api_base%/}/chat/completions"
  probe_body_path="$(mktemp)"
  probe_stderr_path="$(mktemp)"
  probe_start_epoch="$(date +%s)"
  if probe_http_status="$(curl -sS -o "$probe_body_path" -w '%{http_code}' \
  --connect-timeout "$curl_connect_timeout_seconds" \
  --max-time "$completion_probe_timeout_seconds" \
  -H 'Content-Type: application/json' \
  -d "$probe_payload" \
  "$probe_url" 2>"$probe_stderr_path")"; then
  :
  else
  probe_http_status="000"
  fi
  probe_end_epoch="$(date +%s)"
  preflight_completion_probe_latency_seconds="$((probe_end_epoch - probe_start_epoch))"

  if [[ "$probe_http_status" != 2* ]]; then
  preflight_completion_probe_status="failed"
  preflight_completion_probe_error="$(tr '\n' ' ' < "$probe_stderr_path" | sed 's/[[:space:]]\+/ /g; s/^ //; s/ $//')"
  if [[ -z "$preflight_completion_probe_error" ]]; then
    preflight_completion_probe_error="http_status=$probe_http_status"
  else
    preflight_completion_probe_error="http_status=$probe_http_status; $preflight_completion_probe_error"
  fi
  record_step "completion-probe-result.json" "$(COMPLETION_PROBE_STATUS="$preflight_completion_probe_status" COMPLETION_PROBE_PROVIDER="$preflight_completion_probe_provider" COMPLETION_PROBE_MODEL_ID="$preflight_completion_probe_model_id" COMPLETION_PROBE_API_BASE="$preflight_completion_probe_api_base" COMPLETION_PROBE_ERROR="$preflight_completion_probe_error" COMPLETION_PROBE_LATENCY_SECONDS="$preflight_completion_probe_latency_seconds" python3 - <<'PY'
import json
import os

print(json.dumps({
  "status": os.environ["COMPLETION_PROBE_STATUS"],
  "provider": os.environ["COMPLETION_PROBE_PROVIDER"],
  "model_id": os.environ["COMPLETION_PROBE_MODEL_ID"],
  "api_base": os.environ["COMPLETION_PROBE_API_BASE"],
  "error": os.environ["COMPLETION_PROBE_ERROR"],
  "latency_seconds": int(os.environ["COMPLETION_PROBE_LATENCY_SECONDS"] or "0"),
}, ensure_ascii=False))
PY
)"
  rm -f "$probe_body_path" "$probe_stderr_path"
  failure_signature="model_completion_unhealthy"
  failure_domain="environment"
  failure_category="runtime_preflight:model_completion_unhealthy"
  fail_with_stage "preflight" "environment" "builder runtime preflight failed: default model completion probe did not return a healthy response; model_name=$preflight_builder_runtime_default_model; provider=$preflight_completion_probe_provider; model_id=$preflight_completion_probe_model_id; api_base=$preflight_completion_probe_api_base; error=$preflight_completion_probe_error; latency_seconds=$preflight_completion_probe_latency_seconds"
  fi

  if ! probe_response_summary="$(PROBE_BODY_PATH="$probe_body_path" PROBE_PROVIDER="$preflight_completion_probe_provider" PROBE_MODEL_ID="$preflight_completion_probe_model_id" PROBE_API_BASE="$preflight_completion_probe_api_base" python3 - <<'PY'
import json
import os
from pathlib import Path

body = Path(os.environ["PROBE_BODY_PATH"]).read_text(encoding="utf-8")
try:
  payload = json.loads(body)
except Exception as exc:
  print(json.dumps({"status": "invalid_json", "error": str(exc)}, ensure_ascii=False))
  raise SystemExit(1)

choices = payload.get("choices") or []
if not isinstance(choices, list) or not choices:
  print(json.dumps({"status": "missing_choices", "error": "choices array is empty"}, ensure_ascii=False))
  raise SystemExit(1)

first_choice = choices[0] if isinstance(choices[0], dict) else {}
message = first_choice.get("message") if isinstance(first_choice, dict) else {}
content = ""
if isinstance(message, dict):
  content = str(message.get("content") or "").strip()

print(json.dumps({
  "status": "passed",
  "provider": os.environ["PROBE_PROVIDER"],
  "model_id": os.environ["PROBE_MODEL_ID"],
  "api_base": os.environ["PROBE_API_BASE"],
  "response_model": str(payload.get("model") or "").strip(),
  "choices": len(choices),
  "response_preview": " ".join(content.split())[:80],
}, ensure_ascii=False))
PY
)"; then
  preflight_completion_probe_status="failed"
  preflight_completion_probe_error="$(printf '%s' "$probe_response_summary" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g; s/^ //; s/ $//')"
  if [[ -z "$preflight_completion_probe_error" ]]; then
    preflight_completion_probe_error="invalid completion probe response"
  fi
  record_step "completion-probe-result.json" "$(COMPLETION_PROBE_STATUS="$preflight_completion_probe_status" COMPLETION_PROBE_PROVIDER="$preflight_completion_probe_provider" COMPLETION_PROBE_MODEL_ID="$preflight_completion_probe_model_id" COMPLETION_PROBE_API_BASE="$preflight_completion_probe_api_base" COMPLETION_PROBE_ERROR="$preflight_completion_probe_error" COMPLETION_PROBE_LATENCY_SECONDS="$preflight_completion_probe_latency_seconds" python3 - <<'PY'
import json
import os

print(json.dumps({
  "status": os.environ["COMPLETION_PROBE_STATUS"],
  "provider": os.environ["COMPLETION_PROBE_PROVIDER"],
  "model_id": os.environ["COMPLETION_PROBE_MODEL_ID"],
  "api_base": os.environ["COMPLETION_PROBE_API_BASE"],
  "error": os.environ["COMPLETION_PROBE_ERROR"],
  "latency_seconds": int(os.environ["COMPLETION_PROBE_LATENCY_SECONDS"] or "0"),
}, ensure_ascii=False))
PY
)"
  rm -f "$probe_body_path" "$probe_stderr_path"
  failure_signature="model_completion_unhealthy"
  failure_domain="environment"
  failure_category="runtime_preflight:model_completion_unhealthy"
  fail_with_stage "preflight" "environment" "builder runtime preflight failed: default model completion probe returned an invalid response; model_name=$preflight_builder_runtime_default_model; provider=$preflight_completion_probe_provider; model_id=$preflight_completion_probe_model_id; api_base=$preflight_completion_probe_api_base; error=$preflight_completion_probe_error; latency_seconds=$preflight_completion_probe_latency_seconds"
  fi

  preflight_completion_probe_status="passed"
  preflight_completion_probe_error=""
  record_step "completion-probe-result.json" "$probe_response_summary"
  rm -f "$probe_body_path" "$probe_stderr_path"
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
import re

def compact(text: str, limit: int = 160) -> str:
  value = " ".join(str(text or "").split())
  if len(value) <= limit:
    return value
  return value[: limit - 3] + "..."

def extract_focus_path(text: str) -> str:
  value = str(text or "")
  patterns = [
    r"task file ([A-Za-z0-9_./-]+\.[A-Za-z0-9_]+)",
    r"((?:lib|android|test)/[A-Za-z0-9_./-]+\.[A-Za-z0-9_]+)",
  ]
  for pattern in patterns:
    match = re.search(pattern, value)
    if match:
      return match.group(1)
  return ""

def friendly_failure(summary: str) -> str:
  value = " ".join(str(summary or "").split())
  lower = value.lower()
  if "did not expose a create entry on the collection root" in lower:
    return "collection page is missing a root-level create action"
  if "workspace patch apply failed" in lower:
    return "generated patch could not be applied to the workspace"
  if "patch generation failed" in lower:
    return "patch generation failed before apply"
  return compact(value, 120)

job = json.loads(os.environ["JOB_PAYLOAD"])
events_payload = os.environ.get("EVENTS_PAYLOAD", "").strip()
events = json.loads(events_payload) if events_payload else {}
items = events.get("items") or []
latest = items[-1] if items else {}

status = str(job.get("status") or "unknown").strip() or "unknown"
phase = str(job.get("phase") or "").strip()
status_label = status if not phase else f"{status}/{phase}"
parts = [f"[poll +{os.environ['ELAPSED_SECONDS']}s]", status_label]
budgets = job.get("budgets") or {}
elapsed_iterations = budgets.get("elapsed_iterations")
if elapsed_iterations not in (None, ""):
  parts.append(f"iter={elapsed_iterations}")
consumed_tokens = budgets.get("consumed_tokens")
if consumed_tokens not in (None, ""):
  parts.append(f"tokens={consumed_tokens}")

failure_context = job.get("failure_context") or {}
failure_summary = str(failure_context.get("last_error_summary") or "").strip()
if status in {"failed", "cancelled"} and failure_summary:
  focus_path = extract_focus_path(failure_summary)
  if focus_path:
    parts.append(f"focus={focus_path}")
  parts.append(f"failure={friendly_failure(failure_summary)}")
  print(" | ".join(parts))
  raise SystemExit(0)

if isinstance(latest, dict) and latest:
  stage = str(latest.get("stage") or "").strip()
  event_type = str(latest.get("type") or "").strip()
  summary = compact(latest.get("summary") or "")
  latest_label = "/".join(part for part in [stage, event_type] if part)
  if latest_label and summary:
    parts.append(f"latest={latest_label}: {summary}")
  elif latest_label:
    parts.append(f"latest={latest_label}")
  elif summary:
    parts.append(f"latest={summary}")

print(" | ".join(parts))
PY
}

detect_patch_generation_stall() {
  local events_payload="$1"
  local threshold_seconds="$2"
  EVENTS_PAYLOAD="$events_payload" THRESHOLD_SECONDS="$threshold_seconds" python3 - <<'PY'
import json
import os
import re
import sys


def parse_wait_seconds(summary: str) -> int:
  match = re.search(r"after (?:(\d+)m)?(?:(\d+)s)?", summary)
  if not match:
    return 0
  minutes = int(match.group(1) or 0)
  seconds = int(match.group(2) or 0)
  return minutes * 60 + seconds


def extract_focus_path(item: dict, summary: str) -> str:
  target_paths = item.get("target_paths") or []
  for path in target_paths:
    normalized = str(path or "").strip()
    if normalized:
      return normalized
  match = re.search(r"((?:lib|android|test)/[A-Za-z0-9_./-]+\.[A-Za-z0-9_]+)", summary)
  if match:
    return match.group(1)
  return ""


try:
  events = json.loads(os.environ.get("EVENTS_PAYLOAD") or "{}")
except Exception:
  raise SystemExit(1)

items = [item for item in (events.get("items") or []) if isinstance(item, dict)]
if not items:
  raise SystemExit(1)

latest = items[-1]
if str(latest.get("type") or "").strip() != "run_patch_generation_waiting":
  raise SystemExit(1)

summary = str(latest.get("summary") or "").strip()
wait_seconds = parse_wait_seconds(summary)
threshold_seconds = int(os.environ.get("THRESHOLD_SECONDS") or "0")
if wait_seconds < threshold_seconds:
  raise SystemExit(1)

payload = {
  "wait_seconds": wait_seconds,
  "threshold_seconds": threshold_seconds,
  "focus_path": extract_focus_path(latest, summary),
  "checkpoint_key": str(latest.get("checkpoint_key") or "").strip(),
  "round_id": str(latest.get("round_id") or "").strip(),
  "summary": summary,
}
sys.stdout.write(json.dumps(payload, ensure_ascii=False))
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

record_auto_resume_attempt() {
  local attempt="$1"
  local terminal_status_before="$2"
  local failure_signature_value="$3"
  local resume_mode="$4"
  local confirm_value="$5"
  local http_status="$6"
  local response_body="$7"
  local reason="$8"

  RUN_DIR="$run_dir" \
  AUTO_RESUME_ATTEMPT="$attempt" \
  AUTO_RESUME_TERMINAL_STATUS_BEFORE="$terminal_status_before" \
  AUTO_RESUME_FAILURE_SIGNATURE="$failure_signature_value" \
  AUTO_RESUME_RESUME_MODE="$resume_mode" \
  AUTO_RESUME_CONFIRM="$confirm_value" \
  AUTO_RESUME_HTTP_STATUS="$http_status" \
  AUTO_RESUME_RESPONSE_BODY="$response_body" \
  AUTO_RESUME_REASON="$reason" \
  python3 - <<'PY'
import json
import os
from pathlib import Path

run_dir = Path(os.environ["RUN_DIR"])
attempt = os.environ["AUTO_RESUME_ATTEMPT"]
path = run_dir / f"job-resume-attempt-{attempt}.json"
response_body = str(os.environ.get("AUTO_RESUME_RESPONSE_BODY") or "").strip()
response_status = ""
if response_body:
    try:
        payload = json.loads(response_body)
        if isinstance(payload, dict):
            response_status = str(payload.get("status") or "").strip()
    except Exception:
        response_status = ""

path.write_text(json.dumps({
    "attempt": int(os.environ["AUTO_RESUME_ATTEMPT"]),
    "terminal_status_before": str(os.environ.get("AUTO_RESUME_TERMINAL_STATUS_BEFORE") or "").strip(),
    "failure_signature": str(os.environ.get("AUTO_RESUME_FAILURE_SIGNATURE") or "").strip(),
    "resume_mode": str(os.environ.get("AUTO_RESUME_RESUME_MODE") or "").strip(),
    "confirm": str(os.environ.get("AUTO_RESUME_CONFIRM") or "").strip().lower() == "true",
    "http_status": str(os.environ.get("AUTO_RESUME_HTTP_STATUS") or "").strip(),
    "response_status": response_status,
    "reason": str(os.environ.get("AUTO_RESUME_REASON") or "").strip(),
}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY
}

maybe_auto_resume_job() {
  local job_payload="$1"
  local current_job_id="$2"
  local job_status
  local resumable_failure_signature
  local resume_allowed
  local recommended_resume_mode
  local requires_human_confirmation
  local requires_preserved_workspace
  local suggested_action
  local resume_mode
  local confirm_flag="false"
  local resume_payload
  local reason

  auto_resume_last_reason=""
  if [[ "$auto_resume_enabled" != "1" ]]; then
    auto_resume_last_reason="auto_resume_disabled"
    return 1
  fi
  if [[ "$auto_resume_attempts_used" -ge "$auto_resume_max_attempts" ]]; then
    auto_resume_last_reason="auto_resume_attempt_limit_reached"
    return 1
  fi

  job_status="$(printf '%s' "$job_payload" | json_get "status" 2>/dev/null || true)"
  if [[ "$job_status" != "failed" ]]; then
    auto_resume_last_reason="job_not_failed"
    return 1
  fi

  resumable_failure_signature="$(printf '%s' "$job_payload" | json_get "failure_context.failure_signature" 2>/dev/null || true)"
  auto_resume_last_failure_signature="$resumable_failure_signature"
  if [[ "$resumable_failure_signature" != "$auto_resume_target_failure_signature" ]]; then
    auto_resume_last_reason="failure_signature_not_targeted"
    return 1
  fi

  resume_allowed="$(printf '%s' "$job_payload" | json_get "resume_context.resume_allowed" 2>/dev/null || true)"
  if [[ "$resume_allowed" != "true" ]]; then
    auto_resume_last_reason="resume_not_allowed"
    return 1
  fi

  recommended_resume_mode="$(printf '%s' "$job_payload" | json_get "resume_context.recommended_resume_mode" 2>/dev/null || true)"
  requires_human_confirmation="$(printf '%s' "$job_payload" | json_get "resume_context.requires_human_confirmation" 2>/dev/null || true)"
  requires_preserved_workspace="$(printf '%s' "$job_payload" | json_get "resume_context.requires_preserved_workspace" 2>/dev/null || true)"
  suggested_action="$(printf '%s' "$job_payload" | json_get "resume_context.suggested_action" 2>/dev/null || true)"

  resume_mode="$recommended_resume_mode"
  if [[ -z "$resume_mode" ]]; then
    if [[ "$requires_preserved_workspace" == "true" ]]; then
      resume_mode="resume_from_failure"
    else
      resume_mode="retry_failed_run"
    fi
  fi
  if [[ "$requires_preserved_workspace" == "true" ]]; then
    resume_mode="resume_from_failure"
  fi
  if [[ "$requires_human_confirmation" == "true" ]]; then
    confirm_flag="true"
  fi

  auto_resume_attempts_used=$((auto_resume_attempts_used + 1))
  auto_resume_triggered="true"
  auto_resume_last_resume_mode="$resume_mode"

  reason="trigger failure_signature=$resumable_failure_signature resume_mode=$resume_mode"
  if [[ -n "$suggested_action" ]]; then
    reason+=" suggested_action=$suggested_action"
  fi
  auto_resume_last_reason="$reason"

  resume_payload="$(RESUME_MODE="$resume_mode" CONFIRM_FLAG="$confirm_flag" RESUME_NOTE="$auto_resume_note" python3 - <<'PY'
import json
import os

payload = {
    "resume_mode": os.environ["RESUME_MODE"],
    "confirm": os.environ.get("CONFIRM_FLAG", "false").strip().lower() == "true",
}
note = str(os.environ.get("RESUME_NOTE") or "").strip()
if note:
    payload["note"] = note
print(json.dumps(payload, ensure_ascii=False))
PY
)"

  record_step "job-resume-request-${auto_resume_attempts_used}.json" "$resume_payload"
  request_json_capture POST "/api/v1/jobs/$current_job_id:resume" "$resume_payload"
  auto_resume_last_http_status="$REQUEST_JSON_CAPTURE_STATUS"
  record_step "job-resume-response-${auto_resume_attempts_used}.json" "$REQUEST_JSON_CAPTURE_BODY"
  record_auto_resume_attempt "$auto_resume_attempts_used" "$job_status" "$resumable_failure_signature" "$resume_mode" "$confirm_flag" "$REQUEST_JSON_CAPTURE_STATUS" "$REQUEST_JSON_CAPTURE_BODY" "$reason"

  if [[ "$REQUEST_JSON_CAPTURE_STATUS" != 2* ]]; then
    auto_resume_last_reason="resume_request_failed http_status=$REQUEST_JSON_CAPTURE_STATUS"
    return 2
  fi

  auto_resume_last_job_status="$(printf '%s' "$REQUEST_JSON_CAPTURE_BODY" | json_get "status" 2>/dev/null || true)"
  if [[ -z "$auto_resume_last_job_status" ]]; then
    auto_resume_last_job_status="unknown"
  fi
  echo "[auto-resume] attempt=$auto_resume_attempts_used mode=$resume_mode confirm=$confirm_flag http_status=$REQUEST_JSON_CAPTURE_STATUS job_status=$auto_resume_last_job_status"
  return 0
}

write_result() {
  RESULT_JSON="$run_dir/jobs-regression-result.json" \
  OUTPUT_ROOT="$output_root" \
  RUN_DIR="$run_dir" \
  CONSOLE_LOG_PATH="$console_log_path" \
  LATEST_CONSOLE_LOG_PATH="$latest_console_log_path" \
  APPFACTORY_ROOT_CANDIDATES="$appfactory_root_candidates" \
  REQUIRE_MANUAL_EQUIVALENT="$require_manual_equivalent" \
  PROBE_FILE_PATH="$probe_file_path" \
  API_BASE="$api_base" \
  JOB_ID="$job_id" \
  PRD_ID="$prd_id" \
  TEMPLATE_ID="$template_id" \
  RUN_ID="$latest_run_id" \
  STAGE_STATUS_PREFLIGHT="$stage_status_preflight" \
  STAGE_STATUS_COMPILE="$stage_status_compile" \
  STAGE_STATUS_CREATE="$stage_status_create" \
  STAGE_STATUS_REGISTER_BUILDER="$stage_status_register_builder" \
  STAGE_STATUS_START="$stage_status_start" \
  STAGE_STATUS_RUN="$stage_status_run" \
  PREFLIGHT_BUILDER_RUNTIME_ENABLED="$preflight_builder_runtime_enabled" \
  PREFLIGHT_BUILDER_RUNTIME_DEFAULT_MODEL="$preflight_builder_runtime_default_model" \
  PREFLIGHT_GATEWAY_START_ALLOWED="$preflight_gateway_start_allowed" \
  PREFLIGHT_GATEWAY_START_REASON="$preflight_gateway_start_reason" \
  PREFLIGHT_CONFIG_PATH="$preflight_config_path" \
  PREFLIGHT_USES_USER_HOME_CONFIG="$preflight_uses_user_home_config" \
  PREFLIGHT_CONFIG_LOAD_ERROR="$preflight_config_load_error" \
  PREFLIGHT_COMPLETION_PROBE_STATUS="$preflight_completion_probe_status" \
  PREFLIGHT_COMPLETION_PROBE_PROVIDER="$preflight_completion_probe_provider" \
  PREFLIGHT_COMPLETION_PROBE_MODEL_ID="$preflight_completion_probe_model_id" \
  PREFLIGHT_COMPLETION_PROBE_API_BASE="$preflight_completion_probe_api_base" \
  PREFLIGHT_COMPLETION_PROBE_ERROR="$preflight_completion_probe_error" \
  PREFLIGHT_COMPLETION_PROBE_LATENCY_SECONDS="$preflight_completion_probe_latency_seconds" \
  TERMINAL_JOB_STATUS="$terminal_job_status" \
  FAILURE_BUCKET="$failure_bucket" \
  FAILURE_SIGNATURE="$failure_signature" \
  FAILURE_DOMAIN="$failure_domain" \
  FAILURE_CATEGORY="$failure_category" \
  FAILURE_SUMMARY="$failure_summary" \
  AUTO_RESUME_ENABLED="$auto_resume_enabled" \
  AUTO_RESUME_MAX_ATTEMPTS="$auto_resume_max_attempts" \
  AUTO_RESUME_TARGET_FAILURE_SIGNATURE="$auto_resume_target_failure_signature" \
  AUTO_RESUME_ATTEMPTS_USED="$auto_resume_attempts_used" \
  AUTO_RESUME_TRIGGERED="$auto_resume_triggered" \
  AUTO_RESUME_LAST_HTTP_STATUS="$auto_resume_last_http_status" \
  AUTO_RESUME_LAST_JOB_STATUS="$auto_resume_last_job_status" \
  AUTO_RESUME_LAST_RESUME_MODE="$auto_resume_last_resume_mode" \
  AUTO_RESUME_LAST_FAILURE_SIGNATURE="$auto_resume_last_failure_signature" \
  AUTO_RESUME_LAST_REASON="$auto_resume_last_reason" \
  SCRIPT_STARTED_AT="$script_started_at" \
  SCRIPT_FINISHED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  python3 - <<'PY'
import json
import os
import re
from pathlib import Path
from datetime import datetime, timezone

run_dir = Path(os.environ["RUN_DIR"])


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

def load_json(name: str):
    path = run_dir / name
    if not path.is_file():
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


def build_auto_repair_summary_from_events(events):
  if not isinstance(events, dict):
    return {
      "observed": False,
      "task_types": [],
      "recovered_checks": [],
      "round_ids": [],
    }
  task_types = []
  round_ids = []
  for item in events.get("items") or []:
    if not isinstance(item, dict):
      continue
    if str(item.get("type") or "").strip() != "run_heartbeat":
      continue
    summary = str(item.get("summary") or "").strip()
    if "builder-runtime auto repair:" not in summary:
      continue
    match = re.search(r"\btask_type=([a-z_]+)\b", summary)
    task_type = match.group(1).strip() if match else ""
    if task_type in {"analyze_repair", "test_repair", "closure_repair"}:
      task_types.append(task_type)
    round_id = str(item.get("round_id") or "").strip()
    if round_id:
      round_ids.append(round_id)
  return {
    "observed": bool(task_types),
    "task_types": sorted(set(task_types)),
    "recovered_checks": [],
    "round_ids": sorted(set(round_ids)),
  }


def build_auto_repair_summary(builder_output, events=None):
  empty_summary = {
    "observed": False,
    "task_types": [],
    "recovered_checks": [],
    "round_ids": [],
  }
  if not isinstance(builder_output, dict):
    return build_auto_repair_summary_from_events(events)
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
  summary = {
      "observed": bool(task_types or recovered_checks),
      "task_types": sorted(set(task_types)),
      "recovered_checks": sorted(set(recovered_checks)),
      "round_ids": sorted(set(round_ids)),
  }
  if summary["observed"]:
    return summary
  fallback = build_auto_repair_summary_from_events(events)
  if fallback["observed"]:
    return fallback
  return empty_summary


def build_auto_resume_summary(run_dir: Path):
    attempts = []
    for path in sorted(run_dir.glob("job-resume-attempt-*.json")):
        try:
            payload = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            continue
        if isinstance(payload, dict):
            attempts.append(payload)
    return {
        "enabled": is_truthy_env("AUTO_RESUME_ENABLED", default=True),
        "max_attempts": int(str(os.environ.get("AUTO_RESUME_MAX_ATTEMPTS") or "0") or "0"),
        "target_failure_signature": str(os.environ.get("AUTO_RESUME_TARGET_FAILURE_SIGNATURE") or "").strip(),
        "triggered": is_truthy_env("AUTO_RESUME_TRIGGERED", default=False),
        "attempts_used": int(str(os.environ.get("AUTO_RESUME_ATTEMPTS_USED") or "0") or "0"),
        "last_http_status": str(os.environ.get("AUTO_RESUME_LAST_HTTP_STATUS") or "").strip(),
        "last_job_status": str(os.environ.get("AUTO_RESUME_LAST_JOB_STATUS") or "").strip(),
        "last_resume_mode": str(os.environ.get("AUTO_RESUME_LAST_RESUME_MODE") or "").strip(),
        "last_failure_signature": str(os.environ.get("AUTO_RESUME_LAST_FAILURE_SIGNATURE") or "").strip(),
        "last_reason": str(os.environ.get("AUTO_RESUME_LAST_REASON") or "").strip(),
        "attempts": attempts,
    }


def is_truthy_env(name: str, default: bool = False):
    raw = str(os.environ.get(name, "1" if default else "")).strip().lower()
    return raw not in {"", "0", "false", "no", "off"}


def is_concrete_path(path):
    value = str(path or "").strip()
    if not value:
        return False
    return "*" not in value


def unique_concrete_paths(paths):
    return sorted({str(path).strip() for path in paths if is_concrete_path(path)})


def extract_paths_from_file_facts(item, allowed_states):
    values = []
    for fact in item.get("file_facts") or []:
        if not isinstance(fact, dict):
            continue
        path = str(fact.get("path") or "").strip()
        state = str(fact.get("state") or "").strip()
        if path and state in allowed_states and is_concrete_path(path):
            values.append(path)
    return values


def build_manual_equivalence_summary(events, job_status):
    require_manual_equivalent = is_truthy_env("REQUIRE_MANUAL_EQUIVALENT", default=True)
    probe_file = str(os.environ.get("PROBE_FILE_PATH") or "lib/oneappfactory_executor_probe.dart").strip() or "lib/oneappfactory_executor_probe.dart"
    normalized_job_status = str(job_status or "").strip()

    generated_paths = []
    applied_paths = []
    probe_output_detected = False
    thin_prepare_detected = False

    for item in events.get("items") or []:
        if not isinstance(item, dict):
            continue
        stage = str(item.get("stage") or "").strip()
        if stage == "thin-prepare":
            thin_prepare_detected = True

        summary = str(item.get("summary") or "").strip()
        if probe_file in summary:
            probe_output_detected = True

        event_type = str(item.get("type") or "").strip()
        if event_type == "run_patch_generated":
            generated_paths.extend(extract_paths_from_file_facts(item, {"generated"}))
        elif event_type == "run_patch_applied":
            applied_paths.extend(path for path in (item.get("affected_paths") or []) if is_concrete_path(path))
            applied_paths.extend(extract_paths_from_file_facts(item, {"applied", "modified", "finalized"}))

    generated_paths = unique_concrete_paths(generated_paths)
    applied_paths = unique_concrete_paths(applied_paths)
    non_probe_generated_paths = [path for path in generated_paths if path != probe_file]
    non_probe_applied_paths = [path for path in applied_paths if path != probe_file]
    probe_only = (
        (probe_output_detected or probe_file in generated_paths or probe_file in applied_paths)
        and not non_probe_generated_paths
        and not non_probe_applied_paths
    )

    passed = normalized_job_status == "completed"
    failure_signature = ""
    failure_category = ""
    reason = ""
    if require_manual_equivalent and normalized_job_status == "completed" and probe_only:
        passed = False
        failure_signature = "probe_only_completed_run"
        failure_category = "manual_parity:probe_only_completed_run"
        reason = (
            f"job 已 completed，但事件里只生成或应用了 {probe_file}；"
            "这属于 probe-only thin fallback 结果，不能再当成网页手测等价通过。"
        )

    return {
        "required": require_manual_equivalent,
        "passed": passed,
        "probe_only": probe_only,
        "thin_prepare_detected": thin_prepare_detected,
        "probe_file_path": probe_file,
        "probe_output_detected": probe_output_detected,
        "generated_paths": generated_paths,
        "applied_paths": applied_paths,
        "non_probe_generated_paths": non_probe_generated_paths,
        "non_probe_applied_paths": non_probe_applied_paths,
        "failure_signature": failure_signature,
        "failure_category": failure_category,
        "reason": reason,
    }


def extract_focus_path(text: str):
    value = str(text or "")
    patterns = [
        r"task file ([A-Za-z0-9_./-]+\.[A-Za-z0-9_]+)",
        r"((?:lib|android|test)/[A-Za-z0-9_./-]+\.[A-Za-z0-9_]+)",
    ]
    for pattern in patterns:
        match = re.search(pattern, value)
        if match:
            return match.group(1)
    return ""


def compact_path_list(paths):
    values = []
    for item in paths or []:
        normalized = str(item or "").strip()
        if normalized:
            values.append(normalized)
    if not values:
        return ""
    if len(values) == 1:
        return values[0]
    preview = ", ".join(values[:2])
    if len(values) > 2:
        return f"{preview}, +{len(values) - 2} more"
    return preview


def friendly_failure_explanation(signature: str, summary: str):
  value = " ".join(str(summary or "").split())
  lower = value.lower()
  if signature == "probe_only_completed_run":
    return "The job completed, but it only generated the executor probe file. This is not equivalent to a real manual /jobs build."
  if signature == "gateway_start_blocked":
    return "Gateway preflight is blocked before compile/start. Fix the default builder model or its credentials first, otherwise any /jobs regression result will be polluted by environment drift."
  if signature == "model_completion_unhealthy":
    return "The launcher and config are reachable, but the default builder model could not return a minimal chat completion in preflight. Treat this as model throughput noise instead of a new code frontier."
  if signature == "model_patch_generation_stalled":
    return "Builder patch generation stayed in the waiting state beyond the configured threshold. Treat this as model throughput noise and inspect the current frontier file before assuming a new code regression."
  if "did not expose a create entry on the collection root" in lower:
    return (
      "The collection page was generated, but it still has no create entry at the root. "
      "This topology has mutation without an overview/home page, so the list page must expose the create action."
    )
  if signature == "workspace_patch_apply_failed" or "workspace patch apply failed" in lower:
    return "The generated patch reached apply time, but the workspace rejected it. Check the affected file and path constraints first."
  if signature == "builder_runtime_patch_parse_failed":
    return "The model returned patch content that could not be normalized into a valid workspace patch."
  if signature == "builder_runtime_model_request_failed":
    return "The builder runtime reached a real repair frontier, but the model backend timed out before returning a patch. Treat this as model throughput noise, not a fresh repository regression."
  if signature == "builder_runtime_task_output_invalid":
    return "Builder runtime generated files that failed task-level semantic validation, so the run stopped before validation checks."
  if signature.startswith("environment_check_failed:"):
    return "The validation command failed because the builder environment or dependencies are not ready yet."
  if signature.startswith("profile_check_failed:"):
    return "The generated workspace is still missing required profile structure, so the run can be resumed after the missing files are completed."
  if signature.startswith("device_check_failed:"):
    return "The workspace build reached device verification, but the target device or adb workflow failed."
  return compact(value, 160)


def summarize_event(item):
    if not isinstance(item, dict):
        return ""
    stage = str(item.get("stage") or "").strip()
    event_type = str(item.get("type") or "").strip()
    checkpoint = str(item.get("checkpoint_key") or "").strip()
    summary = compact(item.get("summary") or "", 160)
    target_paths = item.get("target_paths") or []
    focus_path = extract_focus_path(summary)
    if not focus_path and target_paths:
        focus_path = compact_path_list(target_paths)
    parts = []
    label = "/".join(part for part in [stage, event_type] if part)
    if label:
        parts.append(label)
    if checkpoint:
        parts.append(checkpoint)
    if focus_path:
        parts.append(f"focus={focus_path}")
    if summary:
        parts.append(summary)
    return " | ".join(parts)


def event_attempt_value(item):
    if not isinstance(item, dict):
        return 0
    raw = item.get("attempt")
    try:
        return int(raw)
    except (TypeError, ValueError):
        return 0


def build_readable_summary(job_final, events, result):
    items = [item for item in (events.get("items") or []) if isinstance(item, dict)]
    frontier = None
    frontier_index = len(items)
    manual_equivalence = result.get("manual_equivalence") or {}
    probe_file = str(manual_equivalence.get("probe_file_path") or "").strip()
    latest_attempt = 0
    for item in items:
        event_type = str(item.get("type") or "").strip()
        if not event_type.startswith("run_"):
            continue
        latest_attempt = max(latest_attempt, event_attempt_value(item))
    if result.get("failure_bucket") == "manual_parity" and manual_equivalence.get("probe_only") and probe_file:
        for index in range(len(items) - 1, -1, -1):
            item = items[index]
            attempt = event_attempt_value(item)
            if latest_attempt > 0 and attempt > 0 and attempt < latest_attempt:
                continue
            summary = str(item.get("summary") or "")
            affected_paths = [str(path or "").strip() for path in (item.get("affected_paths") or [])]
            file_fact_paths = [
                str(fact.get("path") or "").strip()
                for fact in (item.get("file_facts") or [])
                if isinstance(fact, dict)
            ]
            if probe_file in summary or probe_file in affected_paths or probe_file in file_fact_paths:
                frontier = item
                frontier_index = index
                break
    for index in range(len(items) - 1, -1, -1):
        event_type = str(items[index].get("type") or "").strip()
        if not event_type.startswith("run_"):
            continue
        if event_type in {"run_created", "run_failed"}:
            continue
        attempt = event_attempt_value(items[index])
        if latest_attempt > 0 and attempt > 0 and attempt < latest_attempt:
            continue
        frontier = items[index]
        frontier_index = index
        break
    if frontier is None:
        for index in range(len(items) - 1, -1, -1):
            event_type = str(items[index].get("type") or "").strip()
            if event_type.startswith("run_") or event_type == "execution_finished":
                attempt = event_attempt_value(items[index])
                if latest_attempt > 0 and attempt > 0 and attempt < latest_attempt:
                    continue
                frontier = items[index]
                frontier_index = index
                break
    terminal_event = items[-1] if items else {}
    if frontier is None:
        frontier = terminal_event
    last_success = {}
    for index in range(frontier_index - 1, -1, -1):
        event_type = str(items[index].get("type") or "").strip()
        if event_type in {"run_patch_applied", "run_patch_generated", "run_completed"}:
            last_success = items[index]
            break

    focus_path = extract_focus_path(result.get("failure_summary") or "")
    if not focus_path and isinstance(frontier, dict):
        focus_path = extract_focus_path(frontier.get("summary") or "")
    if not focus_path and isinstance(frontier, dict):
        focus_path = compact_path_list(frontier.get("target_paths") or [])
    if not focus_path and manual_equivalence.get("probe_only"):
        focus_path = probe_file

    frontier_label = summarize_event(frontier)
    headline = f"{str(result.get('job_status') or 'unknown').upper()} after {result.get('total_runtime_seconds', 0)}s"
    if result.get("script_status") != "passed" and result.get("failure_bucket") == "manual_parity":
        headline = f"MANUAL PARITY FAILED after {result.get('total_runtime_seconds', 0)}s (job completed)"
    return {
        "headline": headline,
        "frontier": frontier_label,
        "frontier_stage": str((frontier or {}).get("stage") or "").strip(),
        "frontier_checkpoint": str((frontier or {}).get("checkpoint_key") or "").strip(),
        "focus_path": focus_path,
        "failure": compact(result.get("failure_summary") or "", 220),
        "explanation": friendly_failure_explanation(result.get("failure_signature") or "", result.get("failure_summary") or ""),
        "last_success": summarize_event(last_success),
        "terminal_event": summarize_event(terminal_event),
    }


def build_latest_markdown(result):
    summary = result.get("readable_summary") or {}
    key_paths = result.get("key_paths") or {}
    manual_equivalence = result.get("manual_equivalence") or {}
    runtime_preflight = result.get("runtime_preflight") or {}
    auto_repair = result.get("auto_repair") or {}
    auto_resume = result.get("auto_resume") or {}
    lines = [
        "# AppFactory /jobs Regression Latest Run",
        "",
        "## Readable Summary",
        "",
        f"- result: {summary.get('headline') or result.get('job_status') or 'unknown'}",
        f"- script_status: {result.get('script_status') or 'unknown'}",
        f"- job_status: {result.get('job_status') or 'unknown'}",
        f"- failure_bucket: {result.get('failure_bucket') or 'unknown'}",
        f"- failure_signature: {result.get('failure_signature') or 'none'}",
        f"- api_base: {result.get('api_base') or 'unknown'}",
        f"- manual_equivalent: {'true' if manual_equivalence.get('passed') else 'false'}",
        f"- runtime_mode: {runtime_preflight.get('mode') or 'unknown'}",
        f"- auto_resume_enabled: {'true' if auto_resume.get('enabled') else 'false'}",
        f"- auto_resume_triggered: {'true' if auto_resume.get('triggered') else 'false'}",
        f"- auto_resume_attempts_used: {auto_resume.get('attempts_used', 0)}",
    ]
    if auto_resume.get("target_failure_signature"):
        lines.append(f"- auto_resume_target_failure_signature: {auto_resume['target_failure_signature']}")
    if auto_resume.get("last_resume_mode"):
        lines.append(f"- auto_resume_last_resume_mode: {auto_resume['last_resume_mode']}")
    if auto_resume.get("last_http_status"):
        lines.append(f"- auto_resume_last_http_status: {auto_resume['last_http_status']}")
    if auto_resume.get("last_job_status"):
        lines.append(f"- auto_resume_last_job_status: {auto_resume['last_job_status']}")
    if auto_resume.get("last_reason"):
        lines.append(f"- auto_resume_last_reason: {auto_resume['last_reason']}")
    if runtime_preflight.get("config_path"):
        lines.append(f"- config_path: {runtime_preflight['config_path']}")
    if runtime_preflight.get("uses_user_home_config") is not None:
        lines.append(f"- uses_user_home_config: {'true' if runtime_preflight['uses_user_home_config'] else 'false'}")
    if runtime_preflight.get("config_load_error"):
        lines.append(f"- config_load_error: {runtime_preflight['config_load_error']}")
    if runtime_preflight.get("default_model"):
        lines.append(f"- builder_runtime_default_model: {runtime_preflight['default_model']}")
    completion_probe = runtime_preflight.get("completion_probe") or {}
    if completion_probe.get("status"):
      lines.append(f"- completion_probe_status: {completion_probe['status']}")
    if completion_probe.get("provider"):
      lines.append(f"- completion_probe_provider: {completion_probe['provider']}")
    if completion_probe.get("model_id"):
      lines.append(f"- completion_probe_model_id: {completion_probe['model_id']}")
    if completion_probe.get("api_base"):
      lines.append(f"- completion_probe_api_base: {completion_probe['api_base']}")
    if completion_probe.get("latency_seconds") is not None:
      lines.append(f"- completion_probe_latency_seconds: {completion_probe['latency_seconds']}")
    if completion_probe.get("error"):
      lines.append(f"- completion_probe_error: {completion_probe['error']}")
    if runtime_preflight.get("gateway_start_allowed") is not None:
        lines.append(f"- gateway_start_allowed: {'true' if runtime_preflight['gateway_start_allowed'] else 'false'}")
    if runtime_preflight.get("gateway_start_reason"):
        lines.append(f"- gateway_start_reason: {runtime_preflight['gateway_start_reason']}")
    if manual_equivalence.get("probe_only"):
        lines.append("- probe_only: true")
    if manual_equivalence.get("reason"):
        lines.append(f"- manual_equivalence_reason: {manual_equivalence['reason']}")
    if manual_equivalence.get("non_probe_generated_paths"):
        lines.append(f"- non_probe_generated_paths: {compact_path_list(manual_equivalence['non_probe_generated_paths'])}")
    if manual_equivalence.get("non_probe_applied_paths"):
        lines.append(f"- non_probe_applied_paths: {compact_path_list(manual_equivalence['non_probe_applied_paths'])}")
    if summary.get("frontier"):
        lines.append(f"- frontier: {summary['frontier']}")
    if summary.get("focus_path"):
        lines.append(f"- focus_path: {summary['focus_path']}")
    if summary.get("failure"):
        lines.append(f"- failure: {summary['failure']}")
    if summary.get("explanation"):
        lines.append(f"- explanation: {summary['explanation']}")
    if summary.get("last_success"):
        lines.append(f"- last_success: {summary['last_success']}")
    if result.get("workspace_path"):
        lines.append(f"- workspace: {result['workspace_path']}")
    if key_paths.get("summary_log"):
        lines.append(f"- builder_log: {key_paths['summary_log']}")
    if key_paths.get("event_log"):
        lines.append(f"- event_log: {key_paths['event_log']}")
    if key_paths.get("console_log"):
        lines.append(f"- console_log: {key_paths['console_log']}")
    if key_paths.get("latest_console_log"):
        lines.append(f"- latest_console_log: {key_paths['latest_console_log']}")
    lines.extend([
        f"- run_dir: {result['run_dir']}",
        "",
        "## Metadata",
        "",
        f"- job_id: {result['job_id']}",
        f"- prd_id: {result['prd_id']}",
        f"- template_id: {result['template_id'] or 'auto'}",
        f"- run_id: {result['run_id'] or 'unknown'}",
        f"- total_runtime_seconds: {result['total_runtime_seconds']}",
        f"- auto_repair_observed: {'true' if auto_repair.get('observed') else 'false'}",
        f"- auto_resume_triggered: {'true' if auto_resume.get('triggered') else 'false'}",
        f"- auto_resume_attempts_used: {auto_resume.get('attempts_used', 0)}",
        "",
    ])
    return "\n".join(lines)

job_final = load_json("job-final-response.json") or {}
artifacts = load_json("artifacts-response.json") or load_json("artifacts-final-response.json") or {}
events = load_json("events-response.json") or {}
builder_output = load_builder_output(job_final) or {}

delivery_context = job_final.get("delivery_context") if isinstance(job_final, dict) else None
delivery_status = "not_started"
if isinstance(delivery_context, dict):
    delivery_status = str(delivery_context.get("status") or "not_started")

total_runtime_seconds, timeline = build_timeline(job_final, events)
auto_repair = build_auto_repair_summary(builder_output, events)
auto_resume = build_auto_resume_summary(run_dir)
job_status = os.environ.get("TERMINAL_JOB_STATUS", "")
manual_equivalence = build_manual_equivalence_summary(events, job_status)
raw_failure_signature = os.environ.get("FAILURE_SIGNATURE", "")
raw_failure_domain = os.environ.get("FAILURE_DOMAIN", "")
raw_failure_category = os.environ.get("FAILURE_CATEGORY", "")
raw_failure_summary = os.environ.get("FAILURE_SUMMARY", "")
preflight_enabled_raw = str(os.environ.get("PREFLIGHT_BUILDER_RUNTIME_ENABLED", "")).strip().lower()
preflight_gateway_allowed_raw = str(os.environ.get("PREFLIGHT_GATEWAY_START_ALLOWED", "")).strip().lower()

runtime_preflight = {
  "required": is_truthy_env("REQUIRE_MANUAL_EQUIVALENT", default=True),
  "enabled": preflight_enabled_raw == "true",
  "default_model": str(os.environ.get("PREFLIGHT_BUILDER_RUNTIME_DEFAULT_MODEL", "")).strip(),
  "gateway_start_allowed": None if not preflight_gateway_allowed_raw else preflight_gateway_allowed_raw == "true",
  "gateway_start_reason": str(os.environ.get("PREFLIGHT_GATEWAY_START_REASON", "")).strip(),
  "config_path": str(os.environ.get("PREFLIGHT_CONFIG_PATH", "")).strip(),
  "uses_user_home_config": None if str(os.environ.get("PREFLIGHT_USES_USER_HOME_CONFIG", "")).strip().lower() == "" else str(os.environ.get("PREFLIGHT_USES_USER_HOME_CONFIG", "")).strip().lower() == "true",
  "config_load_error": str(os.environ.get("PREFLIGHT_CONFIG_LOAD_ERROR", "")).strip(),
  "completion_probe": {
    "status": str(os.environ.get("PREFLIGHT_COMPLETION_PROBE_STATUS", "")).strip(),
    "provider": str(os.environ.get("PREFLIGHT_COMPLETION_PROBE_PROVIDER", "")).strip(),
    "model_id": str(os.environ.get("PREFLIGHT_COMPLETION_PROBE_MODEL_ID", "")).strip(),
    "api_base": str(os.environ.get("PREFLIGHT_COMPLETION_PROBE_API_BASE", "")).strip(),
    "error": str(os.environ.get("PREFLIGHT_COMPLETION_PROBE_ERROR", "")).strip(),
    "latency_seconds": None if str(os.environ.get("PREFLIGHT_COMPLETION_PROBE_LATENCY_SECONDS", "")).strip() == "" else int(str(os.environ.get("PREFLIGHT_COMPLETION_PROBE_LATENCY_SECONDS", "")).strip()),
  },
}
if runtime_preflight["completion_probe"].get("status") == "failed":
  runtime_preflight["mode"] = "builder_runtime_completion_unhealthy"
elif runtime_preflight.get("gateway_start_allowed") is False:
  runtime_preflight["mode"] = "builder_runtime_blocked"
elif runtime_preflight["enabled"] and runtime_preflight["default_model"]:
  runtime_preflight["mode"] = "builder_runtime"
else:
  runtime_preflight["mode"] = "thin_fallback"

script_status = "passed"
script_failure_bucket = os.environ.get("FAILURE_BUCKET", "unknown")
script_failure_signature = raw_failure_signature
script_failure_domain = raw_failure_domain
script_failure_category = raw_failure_category
script_failure_summary = raw_failure_summary

if job_status != "completed":
  script_status = "failed"
elif manual_equivalence.get("required") and not manual_equivalence.get("passed"):
  script_status = "failed"
  script_failure_bucket = "manual_parity"
  script_failure_signature = manual_equivalence.get("failure_signature") or "manual_parity_failed"
  script_failure_domain = "validation"
  script_failure_category = manual_equivalence.get("failure_category") or "manual_parity"
  script_failure_summary = manual_equivalence.get("reason") or raw_failure_summary
else:
  script_failure_bucket = "none"
  script_failure_signature = ""
  script_failure_domain = ""
  script_failure_category = ""
  script_failure_summary = ""

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
  "job_status": job_status,
  "script_status": script_status,
    "delivery_status": delivery_status,
  "failure_bucket": script_failure_bucket,
  "failure_signature": script_failure_signature,
  "failure_domain": script_failure_domain,
  "failure_category": script_failure_category,
  "failure_summary": script_failure_summary,
  "job_failure_context": {
    "failure_signature": raw_failure_signature,
    "failure_domain": raw_failure_domain,
    "failure_category": raw_failure_category,
    "failure_summary": raw_failure_summary,
  },
    "workspace_path": str(job_final.get("workspace_path") or ""),
    "total_runtime_seconds": total_runtime_seconds,
    "auto_repair": auto_repair,
  "auto_resume": auto_resume,
  "manual_equivalence": manual_equivalence,
    "runtime_preflight": runtime_preflight,
    "stage_status": {
      "preflight": os.environ.get("STAGE_STATUS_PREFLIGHT", "pending"),
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
      "console_log": str(os.environ.get("CONSOLE_LOG_PATH", "") or ""),
      "latest_console_log": str(os.environ.get("LATEST_CONSOLE_LOG_PATH", "") or ""),
        "workspace": str(job_final.get("workspace_path") or ""),
        "build_report": read_artifact_path(artifacts, "build-report", "build-report"),
        "smoke_test_report": read_artifact_path(artifacts, "smoke-test-report", "smoke-test-report"),
        "debug_apk": read_artifact_path(artifacts, "debug-apk", "app-debug.apk"),
        "device_logcat": read_artifact_path(artifacts, "device-logcat", "device-logcat"),
    },
    "run_dir": os.environ["RUN_DIR"],
    "snapshots": sorted(path.name for path in run_dir.glob("*.json")),
}
result["readable_summary"] = build_readable_summary(job_final, events, result)

output_root = Path(os.environ["OUTPUT_ROOT"])
output_root.mkdir(parents=True, exist_ok=True)
result_path = Path(os.environ["RESULT_JSON"])
result_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
(output_root / "latest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
(output_root / "latest.md").write_text(build_latest_markdown(result), encoding="utf-8")
PY
}

print_readable_result() {
  local result_json="$1"
  local latest_json_path="$2"
  local latest_markdown_path="$3"
  RESULT_JSON="$result_json" LATEST_JSON_PATH="$latest_json_path" LATEST_MARKDOWN_PATH="$latest_markdown_path" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["RESULT_JSON"])
if not path.is_file():
    raise SystemExit(0)
payload = json.loads(path.read_text(encoding="utf-8"))
summary = payload.get("readable_summary") or {}
key_paths = payload.get("key_paths") or {}
manual = payload.get("manual_equivalence") or {}

print("== readable summary ==")
print(f"result={summary.get('headline') or payload.get('job_status') or 'unknown'}")
print(f"script_status={payload.get('script_status') or 'unknown'}")
print(f"job_status={payload.get('job_status') or 'unknown'}")
print(f"manual_equivalent={'true' if manual.get('passed') else 'false'}")
runtime_preflight = payload.get("runtime_preflight") or {}
completion_probe = runtime_preflight.get("completion_probe") or {}
if runtime_preflight.get("mode"):
  print(f"runtime_mode={runtime_preflight['mode']}")
if completion_probe.get("status"):
  print(f"completion_probe_status={completion_probe['status']}")
if completion_probe.get("model_id"):
  print(f"completion_probe_model_id={completion_probe['model_id']}")
if completion_probe.get("api_base"):
  print(f"completion_probe_api_base={completion_probe['api_base']}")
if completion_probe.get("error"):
  print(f"completion_probe_error={completion_probe['error']}")
if manual.get("probe_only"):
  print("probe_only=true")
if manual.get("reason"):
  print(f"manual_equivalence_reason={manual['reason']}")
if summary.get("frontier"):
    print(f"frontier={summary['frontier']}")
if summary.get("focus_path"):
    print(f"focus_path={summary['focus_path']}")
if summary.get("failure"):
    print(f"failure={summary['failure']}")
if summary.get("explanation"):
    print(f"explanation={summary['explanation']}")
if summary.get("last_success"):
    print(f"last_success={summary['last_success']}")
if summary.get("terminal_event"):
    print(f"terminal_event={summary['terminal_event']}")
if payload.get("workspace_path"):
    print(f"workspace={payload['workspace_path']}")
if key_paths.get("summary_log"):
    print(f"builder_log={key_paths['summary_log']}")
if key_paths.get("event_log"):
    print(f"event_log={key_paths['event_log']}")
if key_paths.get("console_log"):
  print(f"console_log={key_paths['console_log']}")
if key_paths.get("latest_console_log"):
  print(f"latest_log={key_paths['latest_console_log']}")
print(f"job_id={payload.get('job_id') or ''}")
print(f"run_id={payload.get('run_id') or 'unknown'}")
print(f"run_dir={payload.get('run_dir') or ''}")
print(f"latest_json={os.environ.get('LATEST_JSON_PATH', '')}")
print(f"latest_markdown={os.environ.get('LATEST_MARKDOWN_PATH', '')}")
PY
}

fail_with_stage() {
  local stage="$1"
  local bucket="$2"
  local summary="$3"
  case "$stage" in
    preflight) stage_status_preflight="failed" ;;
    compile) stage_status_compile="failed" ;;
    create) stage_status_create="failed" ;;
    register_builder) stage_status_register_builder="failed" ;;
    start) stage_status_start="failed" ;;
    run) stage_status_run="failed" ;;
  esac
  failure_bucket="$bucket"
  failure_summary="$summary"
  write_result
  print_readable_result "$run_dir/jobs-regression-result.json" "$output_root/latest.json" "$output_root/latest.md"
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
    builder_runtime_patch_parse_failed|builder_runtime_model_request_failed|workspace_patch_apply_failed)
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

echo "jobs_ui_regression_run_id=$run_timestamp"
echo "jobs_ui_regression_root=$output_root"
echo "jobs_ui_api_base=$api_base"
echo "jobs_ui_console_log=$console_log_path"
echo "jobs_ui_latest_log=$latest_console_log_path"

ensure_api_reachable
ensure_manual_equivalent_preflight

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
  patch_generation_stall=""
  patch_generation_stall_focus_path=""
  patch_generation_stall_wait_seconds=""
  patch_generation_stall_checkpoint_key=""
  patch_generation_stall_round_id=""
  patch_generation_stall_summary=""
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
  if [[ "$patch_wait_failfast_enabled" == "1" ]]; then
    patch_generation_stall="$(detect_patch_generation_stall "$poll_events_response" "$patch_wait_failfast_seconds" 2>/dev/null || true)"
    if [[ -n "$patch_generation_stall" ]]; then
      patch_generation_stall_focus_path="$(printf '%s' "$patch_generation_stall" | json_get "focus_path" 2>/dev/null || true)"
      patch_generation_stall_wait_seconds="$(printf '%s' "$patch_generation_stall" | json_get "wait_seconds" 2>/dev/null || true)"
      patch_generation_stall_checkpoint_key="$(printf '%s' "$patch_generation_stall" | json_get "checkpoint_key" 2>/dev/null || true)"
      patch_generation_stall_round_id="$(printf '%s' "$patch_generation_stall" | json_get "round_id" 2>/dev/null || true)"
      patch_generation_stall_summary="$(printf '%s' "$patch_generation_stall" | json_get "summary" 2>/dev/null || true)"
      record_step "job-final-response.json" "$final_job_response"
      record_step "events-response.json" "$poll_events_response"
      failure_signature="model_patch_generation_stalled"
      failure_domain="environment"
      failure_category="run_guard:model_patch_generation_stalled"
      fail_with_stage "run" "environment" "builder runtime patch generation is still waiting after ${patch_generation_stall_wait_seconds}s; threshold_seconds=$patch_wait_failfast_seconds; focus_path=${patch_generation_stall_focus_path:-unknown}; checkpoint_key=${patch_generation_stall_checkpoint_key:-unknown}; round_id=${patch_generation_stall_round_id:-unknown}; latest_wait_summary=${patch_generation_stall_summary:-unknown}; treat this as model throughput noise instead of waiting for the full builder timeout"
    fi
  fi
  if [[ "$terminal_job_status" == "completed" || "$terminal_job_status" == "failed" || "$terminal_job_status" == "cancelled" ]]; then
    if maybe_auto_resume_job "$final_job_response" "$job_id"; then
      final_job_response="$REQUEST_JSON_CAPTURE_BODY"
      terminal_job_status="$(printf '%s' "$final_job_response" | json_get "status" 2>/dev/null || true)"
      last_progress_line=""
      sleep "$poll_interval"
      elapsed=$((elapsed + poll_interval))
      continue
    fi
    auto_resume_rc=$?
    if [[ "$auto_resume_rc" -eq 2 ]]; then
      fail_with_stage "run" "environment" "job auto resume failed after terminal failure; http_status=${auto_resume_last_http_status:-unknown}; reason=${auto_resume_last_reason:-unknown}; failure_signature=${auto_resume_last_failure_signature:-unknown}"
    fi
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

print_readable_result "$run_dir/jobs-regression-result.json" "$output_root/latest.json" "$output_root/latest.md"

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

if auto_resume_summary="$(RESULT_JSON="$run_dir/jobs-regression-result.json" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["RESULT_JSON"])
if not path.is_file():
    raise SystemExit(0)
payload = json.loads(path.read_text(encoding="utf-8"))
auto = payload.get("auto_resume") or {}
print(f"auto_resume_enabled={'true' if auto.get('enabled') else 'false'}")
print(f"auto_resume_triggered={'true' if auto.get('triggered') else 'false'}")
print(f"auto_resume_attempts_used={auto.get('attempts_used', 0)}")
if auto.get("last_resume_mode"):
    print(f"auto_resume_last_resume_mode={auto['last_resume_mode']}")
if auto.get("last_http_status"):
    print(f"auto_resume_last_http_status={auto['last_http_status']}")
if auto.get("last_job_status"):
    print(f"auto_resume_last_job_status={auto['last_job_status']}")
if auto.get("last_reason"):
    print(f"auto_resume_last_reason={auto['last_reason']}")
PY
)"; then
  if [[ -n "$auto_resume_summary" ]]; then
    echo "== auto resume summary =="
    printf '%s\n' "$auto_resume_summary"
  fi
fi

if manual_equivalence_summary="$(RESULT_JSON="$run_dir/jobs-regression-result.json" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["RESULT_JSON"])
if not path.is_file():
    raise SystemExit(0)
payload = json.loads(path.read_text(encoding="utf-8"))
manual = payload.get("manual_equivalence") or {}
print(f"manual_equivalence_required={'true' if manual.get('required') else 'false'}")
print(f"manual_equivalent={'true' if manual.get('passed') else 'false'}")
print(f"probe_only_detected={'true' if manual.get('probe_only') else 'false'}")
if manual.get("non_probe_generated_paths"):
    print("non_probe_generated_paths=" + ",".join(manual["non_probe_generated_paths"]))
if manual.get("non_probe_applied_paths"):
    print("non_probe_applied_paths=" + ",".join(manual["non_probe_applied_paths"]))
if manual.get("reason"):
    print(f"manual_equivalence_reason={manual['reason']}")
PY
)"; then
  if [[ -n "$manual_equivalence_summary" ]]; then
    echo "== manual equivalence summary =="
    printf '%s\n' "$manual_equivalence_summary"
  fi
fi

script_status="$(RESULT_JSON="$run_dir/jobs-regression-result.json" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["RESULT_JSON"])
if not path.is_file():
    print("failed")
    raise SystemExit(0)
payload = json.loads(path.read_text(encoding="utf-8"))
print(payload.get("script_status") or "failed")
PY
)"

if [[ "$script_status" != "passed" ]]; then
  exit 1
fi