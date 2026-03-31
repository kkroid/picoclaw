#!/usr/bin/env bash

set -euo pipefail

template_dir="${1:-examples/appfactory/templates/flutter-finance-lite}"

if [ ! -d "$template_dir" ]; then
  echo "template directory not found: $template_dir" >&2
  exit 1
fi

docker run --rm \
  -v "$PWD/$template_dir:/workspace/app" \
  "${APPFACTORY_BUILDER_IMAGE:-picoclaw/appfactory-builder:local}" \
  exec bash -lc 'cd /workspace/app && flutter pub get && flutter analyze && flutter test && flutter build apk --debug'