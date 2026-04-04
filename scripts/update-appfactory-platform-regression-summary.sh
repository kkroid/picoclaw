#!/usr/bin/env bash

set -euo pipefail

platform_root="${APPFACTORY_PLATFORM_REGRESSION_ROOT:-${1:-workspace/appfactory/platform-regression}}"
summary_markdown_path="${APPFACTORY_PLATFORM_REGRESSION_SUMMARY_OUTPUT:-${2:-${platform_root}/reports/platform-regression-summary.md}}"
summary_json_path="${APPFACTORY_PLATFORM_REGRESSION_SUMMARY_JSON_OUTPUT:-${3:-${platform_root}/reports/platform-regression-summary.json}}"

APPFACTORY_PLATFORM_REGRESSION_ROOT="$platform_root" \
APPFACTORY_PLATFORM_REGRESSION_SUMMARY_OUTPUT="$summary_markdown_path" \
APPFACTORY_PLATFORM_REGRESSION_SUMMARY_JSON_OUTPUT="$summary_json_path" \
  bash scripts/summarize-appfactory-platform-regression.sh