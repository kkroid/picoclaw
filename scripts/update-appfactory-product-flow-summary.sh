#!/usr/bin/env bash

set -euo pipefail

product_flow_root="${APPFACTORY_PRODUCT_FLOW_ROOT:-${1:-workspace/appfactory/product-e2e}}"
summary_markdown_path="${APPFACTORY_PRODUCT_FLOW_SUMMARY_OUTPUT:-${2:-${product_flow_root}/reports/product-flow-summary.md}}"
summary_json_path="${APPFACTORY_PRODUCT_FLOW_SUMMARY_JSON_OUTPUT:-${3:-${product_flow_root}/reports/product-flow-summary.json}}"

APPFACTORY_PRODUCT_FLOW_ROOT="$product_flow_root" \
APPFACTORY_PRODUCT_FLOW_SUMMARY_OUTPUT="$summary_markdown_path" \
APPFACTORY_PRODUCT_FLOW_SUMMARY_JSON_OUTPUT="$summary_json_path" \
  bash scripts/summarize-appfactory-product-flow.sh