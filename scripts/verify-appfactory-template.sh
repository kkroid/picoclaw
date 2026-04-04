#!/usr/bin/env bash

set -euo pipefail

template_dir="${1:-examples/appfactory/templates/flutter-finance-lite}"
builder_image="${APPFACTORY_BUILDER_IMAGE:-picoclaw/appfactory-builder:local}"
cache_root="${APPFACTORY_BUILDER_CACHE_ROOT:-$PWD/workspace/appfactory/builder-cache/template-verify}"
include_apk="${APPFACTORY_TEMPLATE_VERIFY_INCLUDE_APK:-1}"
android_verbose="${APPFACTORY_TEMPLATE_VERIFY_ANDROID_VERBOSE:-0}"
android_gradle_log_level="${APPFACTORY_TEMPLATE_VERIFY_ANDROID_GRADLE_LOG_LEVEL:-info}"
persist_workdir="${APPFACTORY_TEMPLATE_VERIFY_PERSIST_WORKDIR:-1}"

if [ ! -d "$template_dir" ]; then
  echo "template directory not found: $template_dir" >&2
  exit 1
fi

for rel_path in pubspec.yaml lib/main.dart test/widget_test.dart android/app/build.gradle.kts; do
  if [ ! -s "$template_dir/$rel_path" ]; then
    echo "required template file missing or empty: $template_dir/$rel_path" >&2
    exit 1
  fi
done

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
template_key="$(printf '%s' "$template_dir" | tr '/ ' '__')"
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
flutter build apk --debug --no-pub --config-only -v 2>&1 | tee "\$log_file"
cd android
./gradlew app:assembleDebug --console=plain --stacktrace --warning-mode=all --$android_gradle_log_level 2>&1 | tee -a "\$log_file"
EOF
)
  else
    apk_commands='flutter build apk --debug --no-pub'
  fi
else
  apk_commands=''
  echo "template_verify_mode=fast"
fi

echo "template_dir=$template_dir"
echo "builder_image=$builder_image"
echo "builder_cache_root=$cache_root"
echo "persist_workdir=$persist_workdir"

if [ "$persist_workdir" = "1" ]; then
  echo "template_work_root=$work_root"
fi

container_script=$(cat <<EOF
set -euo pipefail
workdir=/tmp/app
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
  /workspace/src/ "\$workdir/"
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
  echo "run 'make build-appfactory-builder' first or set APPFACTORY_BUILDER_IMAGE to an existing image" >&2
  exit 1
fi

docker run --rm \
  -v "$PWD/$template_dir:/workspace/src:ro" \
  -v "$cache_root/pub-cache:/opt/pub-cache" \
  -v "$cache_root/gradle:/root/.gradle" \
  -v "$cache_root/android:/root/.android" \
  -v "$logs_dir:/workspace/logs" \
  "${work_mount_args[@]}" \
  "$builder_image" \
  exec bash -lc "$container_script"