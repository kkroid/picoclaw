#!/usr/bin/env bash

set -euo pipefail

builder_image="${APPFACTORY_BUILDER_IMAGE:-picoclaw/appfactory-builder:local}"
go_bin="${GO_BIN:-go}"
test_package="./web/backend/api"
test_name="TestStartPublicJobDefaultExecutorCreatesFlutterLandingFiles"
job_id="job-public-default-landing"
keep_generated_workspace="${KEEP_GENERATED_WORKSPACE:-0}"
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

cleanup() {
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
if [[ ! -d "$workspace_dir" ]]; then
  echo "generated workspace not found: $workspace_dir" >&2
  exit 1
fi

echo "[2/3] 在 builder 容器内执行 Flutter 收口链..."
docker run --rm \
  -v "$workspace_dir:/workspace/app" \
  "$builder_image" \
  exec bash -lc 'cd /workspace/app && flutter pub get && flutter analyze && flutter test && flutter build apk --debug'

apk_path="$workspace_dir/build/app/outputs/flutter-apk/app-debug.apk"
if [[ ! -f "$apk_path" ]]; then
  echo "expected debug apk not found: $apk_path" >&2
  exit 1
fi

echo "[3/3] 验证完成。"
echo "failure_signal=non-zero exit code from go test, flutter command chain, or missing apk artifact"
echo "workspace=$workspace_dir"
echo "apk=$apk_path"
if [[ "$keep_generated_workspace" == "1" ]]; then
  echo "preserved_temp_dir=$temp_dir"
fi