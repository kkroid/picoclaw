#!/usr/bin/env bash

set -euo pipefail

builder_image="${APPFACTORY_BUILDER_IMAGE:-picoclaw/appfactory-builder:local}"
go_bin="${GO_BIN:-go}"
test_package="./web/backend/api"
test_name="${APPFACTORY_PUBLIC_JOB_TEST_NAME:-TestStartPublicJobDefaultExecutorWritesFlutterFallbackProbe}"
job_id="${APPFACTORY_PUBLIC_JOB_JOB_ID:-job-public-default-landing}"
keep_generated_workspace="${KEEP_GENERATED_WORKSPACE:-0}"
device_verification_enabled="${APPFACTORY_DEVICE_VERIFICATION_ENABLED:-0}"
device_serial="${APPFACTORY_DEVICE_SERIAL:-}"
android_app_id="${APPFACTORY_ANDROID_APP_ID:-}"
capture_screenshot="${APPFACTORY_DEVICE_CAPTURE_SCREENSHOT:-0}"
summarize_device_metrics="${APPFACTORY_DEVICE_SUMMARIZE_METRICS:-1}"
adb_timeout_seconds="${APPFACTORY_DEVICE_ADB_TIMEOUT_SECONDS:-60}"
device_docker_args="${APPFACTORY_PUBLIC_JOB_DEVICE_DOCKER_ARGS:-}"
adb_server_socket="${ADB_SERVER_SOCKET:-}"
entrypoint_name="${VERIFY_APPFACTORY_ENTRYPOINT:-make verify-appfactory-public-job}"
temp_dir_record="$(mktemp -t picoclaw-public-job-dir-XXXXXX.txt)"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if ! command -v "$go_bin" >/dev/null 2>&1; then
  echo "$go_bin is required" >&2
  exit 1
fi

if ! docker image inspect "$builder_image" >/dev/null 2>&1; then
  echo "builder image not found: $builder_image" >&2
  echo "run 'make build-appfactory-builder' first or set APPFACTORY_BUILDER_IMAGE to an existing image" >&2
  exit 1
fi

temp_log="$(mktemp -t picoclaw-public-job-verify-XXXXXX.log)"
temp_dir=""
preserved_temp_dir_announced=0

ensure_host_adb_server() {
  local socket="${adb_server_socket:-tcp:host.docker.internal:5037}"
  case "$socket" in
    tcp:127.0.0.1:*|tcp:localhost:*|tcp:host.docker.internal:*)
      if ! command -v adb >/dev/null 2>&1; then
        echo "adb is required when device verification is enabled: $socket" >&2
        exit 1
      fi
      adb start-server >/dev/null
      ;;
  esac
}

host_adb_socket() {
  case "${adb_server_socket:-}" in
    tcp:host.docker.internal:*)
      printf 'tcp:127.0.0.1:%s' "${adb_server_socket##*:}"
      ;;
    *)
      printf '%s' "${adb_server_socket:-}"
      ;;
  esac
}

ensure_host_adb_device_ready() {
  local socket
  local state=""
  local state_exit=0
  if [[ -z "$device_serial" ]]; then
    return
  fi
  socket="$(host_adb_socket)"
  set +e
  if [[ -n "$socket" ]]; then
    state="$(timeout "${adb_timeout_seconds}s" env ADB_SERVER_SOCKET="$socket" adb -s "$device_serial" get-state 2>/dev/null | tr -d '\r\n')"
    state_exit=$?
  else
    state="$(timeout "${adb_timeout_seconds}s" adb -s "$device_serial" get-state 2>/dev/null | tr -d '\r\n')"
    state_exit=$?
  fi
  set -e
  if [[ "$state_exit" != "0" || "$state" != "device" ]]; then
    echo "adb device is not ready: serial=$device_serial state=${state:-unavailable} socket=${socket:-default} exit_code=$state_exit" >&2
    exit 1
  fi
}

announce_preserved_temp_dir() {
  if [[ "$keep_generated_workspace" != "1" || -z "$temp_dir" || "$preserved_temp_dir_announced" == "1" ]]; then
    return
  fi
  echo "preserved_temp_dir=$temp_dir"
  preserved_temp_dir_announced=1
}

cleanup() {
  announce_preserved_temp_dir
  rm -f "$temp_log"
  rm -f "$temp_dir_record"
  if [[ -z "$temp_dir" || "$keep_generated_workspace" == "1" ]]; then
    return
  fi
  case "$temp_dir" in
    /tmp/picoclaw-api-test-*)
      rm -rf "$temp_dir" 2>/dev/null || \
        docker run --rm -v /tmp:/tmp "$builder_image" exec bash -lc 'rm -rf -- "$1"' _ "$temp_dir" >/dev/null
      ;;
  esac
}

trap cleanup EXIT

echo "入口: $entrypoint_name"
echo "builder_image=$builder_image"
echo "go_test=${test_package}:${test_name}"
echo "keep_generated_workspace=$keep_generated_workspace"
echo "device_verification_enabled=$device_verification_enabled"
echo "summarize_device_metrics=$summarize_device_metrics"
if [[ "$device_verification_enabled" == "1" ]]; then
  echo "device_serial=${device_serial:-}"
  if [[ -n "$adb_server_socket" ]]; then
    echo "adb_server_socket=$adb_server_socket"
  else
    echo "adb_server_socket=tcp:host.docker.internal:5037"
  fi
  if [[ -n "${APPFACTORY_DEVICE_POOL_LABEL:-}" ]]; then
    echo "device_pool_label=${APPFACTORY_DEVICE_POOL_LABEL}"
  fi
fi

if [[ "$device_verification_enabled" == "1" ]]; then
  ensure_host_adb_server
  ensure_host_adb_device_ready
fi

echo "[1/3] 生成真实 public-job Flutter 工作区..."
PICOCLAW_KEEP_TEST_TEMPDIR=1 \
PICOCLAW_TEST_TEMPDIR_RECORD_FILE="$temp_dir_record" \
"$go_bin" test "$test_package" -run "^${test_name}$" -count=1 -v -json | tee "$temp_log"

temp_dir="$(tr -d '\n' < "$temp_dir_record")"

if [[ -z "$temp_dir" ]]; then
  echo "failed to locate preserved temp dir from test contract file: $temp_dir_record" >&2
  exit 1
fi

workspace_dir="$temp_dir/workspace/appfactory/jobs/$job_id/workspace"
job_root_dir="$temp_dir/workspace/appfactory/jobs/$job_id"
reports_dir="$job_root_dir/reports"
if [[ ! -d "$workspace_dir" ]]; then
  echo "generated workspace not found: $workspace_dir" >&2
  exit 1
fi

echo "[2/3] 在 builder 容器内执行 Flutter 收口链..."
mkdir -p "$reports_dir"

docker_args=(
  run --rm
  --add-host host.docker.internal:host-gateway
  -v "$job_root_dir:/workspace/job"
)

if [[ "$device_verification_enabled" == "1" ]]; then
  if [[ -n "$device_docker_args" ]]; then
    # shellcheck disable=SC2206
    extra_args=($device_docker_args)
    docker_args+=("${extra_args[@]}")
  fi
  docker_args+=(
    -e "APPFACTORY_DEVICE_VERIFICATION_ENABLED=$device_verification_enabled"
    -e "APPFACTORY_DEVICE_CAPTURE_SCREENSHOT=$capture_screenshot"
    -e "APPFACTORY_DEVICE_ADB_TIMEOUT_SECONDS=$adb_timeout_seconds"
  )
  if [[ -n "$device_serial" ]]; then
    docker_args+=(-e "APPFACTORY_DEVICE_SERIAL=$device_serial")
  fi
  if [[ -n "$android_app_id" ]]; then
    docker_args+=(-e "APPFACTORY_ANDROID_APP_ID=$android_app_id")
  fi
  if [[ -n "$adb_server_socket" ]]; then
    docker_args+=(-e "ADB_SERVER_SOCKET=$adb_server_socket")
  else
    docker_args+=(-e "ADB_SERVER_SOCKET=tcp:host.docker.internal:5037")
  fi
fi

container_script='
set -euo pipefail
cd /workspace/job/workspace
flutter pub get
flutter analyze
flutter test
flutter build apk --debug
if [[ "${APPFACTORY_DEVICE_VERIFICATION_ENABLED:-0}" == "1" ]]; then
  REPORTS_DIR="/workspace/job/reports"
  mkdir -p "$REPORTS_DIR"
  APP_ID="${APPFACTORY_ANDROID_APP_ID:-}"
  if [[ -z "$APP_ID" && -f android/app/build.gradle.kts ]]; then
    APP_ID="$(sed -n "s/.*applicationId *= *\"\([^\"]*\)\".*/\1/p" android/app/build.gradle.kts | head -n1)"
  fi
  if [[ -z "$APP_ID" && -f android/app/build.gradle ]]; then
    APP_ID="$(sed -n "s/.*applicationId[[:space:]]*\"\([^\"]*\)\".*/\1/p" android/app/build.gradle | head -n1)"
  fi
  test -n "$APP_ID"
  install_debug_apk() {
    if [[ -n "${APPFACTORY_DEVICE_SERIAL:-}" ]]; then
      adb -s "$APPFACTORY_DEVICE_SERIAL" install -r build/app/outputs/flutter-apk/app-debug.apk
    else
      adb install -r build/app/outputs/flutter-apk/app-debug.apk
    fi
  }
  if [[ -n "${APPFACTORY_DEVICE_SERIAL:-}" ]]; then
    timeout "${APPFACTORY_DEVICE_ADB_TIMEOUT_SECONDS:-60}s" adb -s "$APPFACTORY_DEVICE_SERIAL" wait-for-device
    install_exit=0
    install_output="$(install_debug_apk 2>&1)" || install_exit=$?
    install_exit="${install_exit:-0}"
    if [[ "$install_exit" -ne 0 && "$install_output" == *"INSTALL_FAILED_UPDATE_INCOMPATIBLE"* ]]; then
      adb -s "$APPFACTORY_DEVICE_SERIAL" uninstall "$APP_ID" >/dev/null 2>&1 || true
      install_exit=0
      install_output="$(install_debug_apk 2>&1)" || install_exit=$?
      install_exit="${install_exit:-0}"
    fi
    if [[ "$install_exit" -ne 0 ]]; then
      printf "%s\n" "$install_output" >&2
      exit "$install_exit"
    fi
    adb -s "$APPFACTORY_DEVICE_SERIAL" shell monkey -p "$APP_ID" -c android.intent.category.LAUNCHER 1
    adb -s "$APPFACTORY_DEVICE_SERIAL" logcat -d > "$REPORTS_DIR/device-logcat.txt"
    if [[ "${APPFACTORY_DEVICE_CAPTURE_SCREENSHOT:-0}" == "1" ]]; then
      adb -s "$APPFACTORY_DEVICE_SERIAL" exec-out screencap -p > "$REPORTS_DIR/device-screenshot.png"
    fi
  else
    timeout "${APPFACTORY_DEVICE_ADB_TIMEOUT_SECONDS:-60}s" adb wait-for-device
    install_exit=0
    install_output="$(install_debug_apk 2>&1)" || install_exit=$?
    install_exit="${install_exit:-0}"
    if [[ "$install_exit" -ne 0 && "$install_output" == *"INSTALL_FAILED_UPDATE_INCOMPATIBLE"* ]]; then
      adb uninstall "$APP_ID" >/dev/null 2>&1 || true
      install_exit=0
      install_output="$(install_debug_apk 2>&1)" || install_exit=$?
      install_exit="${install_exit:-0}"
    fi
    if [[ "$install_exit" -ne 0 ]]; then
      printf "%s\n" "$install_output" >&2
      exit "$install_exit"
    fi
    adb shell monkey -p "$APP_ID" -c android.intent.category.LAUNCHER 1
    adb logcat -d > "$REPORTS_DIR/device-logcat.txt"
    if [[ "${APPFACTORY_DEVICE_CAPTURE_SCREENSHOT:-0}" == "1" ]]; then
      adb exec-out screencap -p > "$REPORTS_DIR/device-screenshot.png"
    fi
  fi
  test -s "$REPORTS_DIR/device-logcat.txt"
fi
'

docker "${docker_args[@]}" "$builder_image" exec bash -lc "$container_script"

apk_path="$workspace_dir/build/app/outputs/flutter-apk/app-debug.apk"
if [[ ! -f "$apk_path" ]]; then
  echo "expected debug apk not found: $apk_path" >&2
  exit 1
fi

echo "[3/3] 验证完成。"
echo "failure_signal=non-zero exit code from go test, flutter command chain, or missing apk artifact"
echo "workspace=$workspace_dir"
echo "apk=$apk_path"
if [[ "$device_verification_enabled" == "1" ]]; then
  logcat_path="$reports_dir/device-logcat.txt"
  if [[ ! -s "$logcat_path" ]]; then
    echo "expected device logcat not found or empty: $logcat_path" >&2
    exit 1
  fi
  if [[ -n "$device_serial" ]]; then
    echo "device_serial=$device_serial"
  fi
  if [[ -n "$adb_server_socket" ]]; then
    echo "adb_server_socket=$adb_server_socket"
  else
    echo "adb_server_socket=tcp:host.docker.internal:5037"
  fi
  echo "device_logcat=$logcat_path"
  if [[ "$capture_screenshot" == "1" ]]; then
    screenshot_path="$reports_dir/device-screenshot.png"
    if [[ ! -s "$screenshot_path" ]]; then
      echo "expected device screenshot not found or empty: $screenshot_path" >&2
      exit 1
    fi
    echo "device_screenshot=$screenshot_path"
  fi
  if [[ "$summarize_device_metrics" == "1" ]]; then
    summary_path="$reports_dir/device-failure-summary.md"
    summary_json_path="$reports_dir/device-failure-summary.json"
    APPFACTORY_DEVICE_METRICS_ROOT="$temp_dir/workspace/appfactory" \
      APPFACTORY_DEVICE_METRICS_OUTPUT="$summary_path" \
    APPFACTORY_DEVICE_METRICS_JSON_OUTPUT="$summary_json_path" \
      bash scripts/update-appfactory-device-summary.sh >/dev/null
    if [[ -s "$summary_path" ]]; then
      echo "device_failure_summary=$summary_path"
    fi
    if [[ -s "$summary_json_path" ]]; then
      echo "device_failure_summary_json=$summary_json_path"
    fi
  fi
fi
announce_preserved_temp_dir