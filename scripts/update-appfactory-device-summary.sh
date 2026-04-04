#!/usr/bin/env bash

set -euo pipefail

metrics_root="${APPFACTORY_DEVICE_METRICS_ROOT:-${1:-workspace/appfactory}}"
output_path="${APPFACTORY_DEVICE_METRICS_OUTPUT:-${2:-${metrics_root}/reports/device-failure-summary.md}}"
json_output_path="${APPFACTORY_DEVICE_METRICS_JSON_OUTPUT:-${3:-${metrics_root}/reports/device-failure-summary.json}}"

APPFACTORY_DEVICE_METRICS_ROOT="$metrics_root" \
APPFACTORY_DEVICE_METRICS_OUTPUT="$output_path" \
APPFACTORY_DEVICE_METRICS_JSON_OUTPUT="$json_output_path" \
  bash scripts/summarize-appfactory-device-metrics.sh