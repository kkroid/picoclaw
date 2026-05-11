#!/usr/bin/env bash

set -euo pipefail

template_ref="${1:-flutter-finance-lite}"
builder_image="${ONEAPPFACTORY_BUILDER_IMAGE:-oneappfactory/builder:local}"
cache_root="${APPFACTORY_BUILDER_CACHE_ROOT:-$PWD/workspace/appfactory/builder-cache/template-verify}"
include_apk="${APPFACTORY_TEMPLATE_VERIFY_INCLUDE_APK:-1}"
android_verbose="${APPFACTORY_TEMPLATE_VERIFY_ANDROID_VERBOSE:-0}"
android_gradle_log_level="${APPFACTORY_TEMPLATE_VERIFY_ANDROID_GRADLE_LOG_LEVEL:-info}"
persist_workdir="${APPFACTORY_TEMPLATE_VERIFY_PERSIST_WORKDIR:-1}"
template_source_mode="${APPFACTORY_TEMPLATE_VERIFY_SOURCE:-image}"
template_root_in_image="${APPFACTORY_TEMPLATE_ROOT_IN_IMAGE:-/opt/appfactory/templates}"

resolve_host_template_dir() {
  local ref="$1"
  if [ -d "$ref" ]; then
    printf '%s\n' "$ref"
    return
  fi
  if [ -d "$PWD/$ref" ]; then
    printf '%s\n' "$PWD/$ref"
    return
  fi
  if [ -d "$PWD/examples/appfactory/templates/$ref" ]; then
    printf '%s\n' "$PWD/examples/appfactory/templates/$ref"
    return
  fi
  return 1
}

resolve_image_template_dir() {
  local ref="$1"
  if [ -z "$ref" ]; then
    return 1
  fi
  if [[ "$ref" = /* ]]; then
    printf '%s\n' "$ref"
    return
  fi
  ref="${ref#./}"
  ref="${ref#examples/appfactory/templates/}"
  printf '%s\n' "$template_root_in_image/$ref"
}

case "$template_source_mode" in
  image|host)
    ;;
  *)
    echo "unsupported template verify source: $template_source_mode" >&2
    echo "expected one of: image, host" >&2
    exit 1
    ;;
esac

template_dir=""
container_template_dir=""

if [ "$template_source_mode" = "host" ]; then
  if ! template_dir="$(resolve_host_template_dir "$template_ref")"; then
    echo "template directory not found on host: $template_ref" >&2
    exit 1
  fi
else
  container_template_dir="$(resolve_image_template_dir "$template_ref")"
fi

case "$android_gradle_log_level" in
  quiet|warn|lifecycle|info|debug)
    ;;
  *)
    echo "unsupported android gradle log level: $android_gradle_log_level" >&2
    echo "expected one of: quiet, warn, lifecycle, info, debug" >&2
    exit 1
    ;;
esac

logs_dir="$cache_root/logs"
template_key="$(printf '%s' "$template_ref" | tr '/ ' '__')"
work_root="$cache_root/workdirs/$template_key"
mkdir -p "$cache_root/pub-cache" "$cache_root/gradle" "$cache_root/android" "$logs_dir"

if [ "$persist_workdir" = "1" ]; then
  mkdir -p "$work_root"
  work_mount_args=(-v "$work_root:/workspace/work")
else
  work_mount_args=()
fi

if [ "$include_apk" = "1" ]; then
  echo "template_verify_mode=full"
  if [ "$android_verbose" = "1" ]; then
    echo "android_verbose=1"
    echo "android_gradle_log_level=$android_gradle_log_level"
    echo "android_log_file=$logs_dir/template-verify-android.log"
    apk_commands=$(cat <<EOF
log_file=/workspace/logs/template-verify-android.log
rm -f "\$log_file"
flutter build apk --release --no-pub --config-only -v 2>&1 | tee "\$log_file"
cd android
./gradlew app:assembleRelease --console=plain --stacktrace --warning-mode=all --$android_gradle_log_level 2>&1 | tee -a "\$log_file"
EOF
)
  else
    apk_commands='flutter build apk --release --no-pub'
  fi
else
  apk_commands=''
  echo "template_verify_mode=fast"
fi

echo "template_dir=$template_dir"
echo "template_ref=$template_ref"
echo "template_source_mode=$template_source_mode"
if [ -n "$container_template_dir" ]; then
  echo "container_template_dir=$container_template_dir"
fi
echo "builder_image=$builder_image"
echo "builder_cache_root=$cache_root"
echo "persist_workdir=$persist_workdir"

if [ "$persist_workdir" = "1" ]; then
  echo "template_work_root=$work_root"
fi

container_script=$(cat <<EOF
set -euo pipefail
workdir=/tmp/app
src_dir=/workspace/src
if [ "$template_source_mode" = "image" ]; then
  src_dir="$container_template_dir"
fi
if [ ! -d "\$src_dir" ]; then
  echo "template directory not found: \$src_dir" >&2
  exit 1
fi
for rel_path in pubspec.yaml lib/main.dart test/widget_test.dart android/app/build.gradle.kts; do
  if [ ! -s "\$src_dir/\$rel_path" ]; then
    echo "required template file missing or empty: \$src_dir/\$rel_path" >&2
    exit 1
  fi
done
if [ "$persist_workdir" = "1" ]; then
  workdir=/workspace/work
  mkdir -p "\$workdir"
else
  rm -rf "\$workdir"
  mkdir -p "\$workdir"
fi
rsync -a --delete \
  --exclude build/ \
  --exclude .dart_tool/ \
  --exclude android/.gradle/ \
  --exclude android/app/.cxx/ \
  --exclude android/local.properties \
  "\$src_dir/" "\$workdir/"
cd "\$workdir"
if ! grep -q '^org.gradle.caching=' android/gradle.properties; then
  printf '\norg.gradle.caching=true\n' >> android/gradle.properties
fi
flutter pub get
flutter analyze
flutter test
$apk_commands
EOF
)

if ! docker image inspect "$builder_image" >/dev/null 2>&1; then
  echo "builder image not found: $builder_image" >&2
  echo "run 'make build-appfactory-builder' first or set ONEAPPFACTORY_BUILDER_IMAGE to an existing image" >&2
  exit 1
fi

docker_args=(
  --rm
  -v "$cache_root/pub-cache:/opt/pub-cache"
  -v "$cache_root/gradle:/root/.gradle"
  -v "$cache_root/android:/root/.android"
  -v "$logs_dir:/workspace/logs"
)

if [ "$template_source_mode" = "host" ]; then
  docker_args+=( -v "$template_dir:/workspace/src:ro" )
fi

if [ "${#work_mount_args[@]}" -gt 0 ]; then
  docker_args+=( "${work_mount_args[@]}" )
fi

docker run "${docker_args[@]}" \
  "$builder_image" \
  exec bash -lc "$container_script"