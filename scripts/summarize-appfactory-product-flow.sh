#!/usr/bin/env bash

set -euo pipefail

product_flow_root="${APPFACTORY_PRODUCT_FLOW_ROOT:-${1:-workspace/appfactory/product-e2e}}"
summary_markdown_path="${APPFACTORY_PRODUCT_FLOW_SUMMARY_OUTPUT:-${2:-${product_flow_root}/reports/product-flow-summary.md}}"
summary_json_path="${APPFACTORY_PRODUCT_FLOW_SUMMARY_JSON_OUTPUT:-${3:-${product_flow_root}/reports/product-flow-summary.json}}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$product_flow_root" "$summary_markdown_path" "$summary_json_path" <<'PY'
import json
import statistics
import sys
from datetime import datetime, timezone
from pathlib import Path


def read_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def summarize_numbers(values):
    cleaned = [float(value) for value in values if isinstance(value, (int, float))]
    if not cleaned:
        return {
            "count": 0,
            "min_seconds": None,
            "max_seconds": None,
            "avg_seconds": None,
            "median_seconds": None,
            "p95_seconds": None,
        }
    ordered = sorted(cleaned)
    p95_index = max(0, min(len(ordered) - 1, int(len(ordered) * 0.95 + 0.999999) - 1))
    return {
        "count": len(ordered),
        "min_seconds": round(ordered[0], 3),
        "max_seconds": round(ordered[-1], 3),
        "avg_seconds": round(sum(ordered) / len(ordered), 3),
        "median_seconds": round(statistics.median(ordered), 3),
        "p95_seconds": round(ordered[p95_index], 3),
    }


def simplify_run(payload):
    return {
        "job_id": payload.get("job_id", ""),
        "run_id": payload.get("run_id", ""),
        "job_status": payload.get("job_status", ""),
        "delivery_status": payload.get("delivery_status", ""),
        "script_started_at": payload.get("script_started_at", ""),
        "script_duration_seconds": payload.get("script_duration_seconds"),
        "run_duration_seconds": payload.get("run_duration_seconds"),
        "build_apk_duration_seconds": payload.get("build_apk_duration_seconds"),
        "cache_mode_before": (payload.get("cache_state_before") or {}).get("mode", "unknown"),
        "run_dir": payload.get("run_dir", ""),
    }


root = Path(sys.argv[1]).expanduser()
summary_markdown_path = Path(sys.argv[2]).expanduser()
summary_json_path = Path(sys.argv[3]).expanduser()
runs_root = root / "runs"

runs = []
for result_path in sorted(runs_root.glob("*/product-flow-result.json")):
    payload = read_json(result_path)
    payload["result_path"] = result_path.as_posix()
    runs.append(payload)

latest = runs[-1] if runs else None
completed_runs = [item for item in runs if str(item.get("job_status", "")).lower() == "completed"]
failed_runs = [item for item in runs if str(item.get("job_status", "")).lower() != "completed"]
warm_runs = [item for item in runs if ((item.get("cache_state_before") or {}).get("mode") == "warm")]
cold_runs = [item for item in runs if ((item.get("cache_state_before") or {}).get("mode") == "cold")]

summary = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "product_flow_root": root.as_posix(),
    "run_count": len(runs),
    "completed_run_count": len(completed_runs),
    "failed_run_count": len(failed_runs),
    "latest_run": simplify_run(latest) if latest else None,
    "latest_warm_run": simplify_run(warm_runs[-1]) if warm_runs else None,
    "latest_cold_run": simplify_run(cold_runs[-1]) if cold_runs else None,
    "all_runs": [simplify_run(item) for item in runs],
    "script_duration_seconds": {
        "overall": summarize_numbers([item.get("script_duration_seconds") for item in runs]),
        "warm": summarize_numbers([item.get("script_duration_seconds") for item in warm_runs]),
        "cold": summarize_numbers([item.get("script_duration_seconds") for item in cold_runs]),
    },
    "run_duration_seconds": {
        "overall": summarize_numbers([item.get("run_duration_seconds") for item in runs]),
        "warm": summarize_numbers([item.get("run_duration_seconds") for item in warm_runs]),
        "cold": summarize_numbers([item.get("run_duration_seconds") for item in cold_runs]),
    },
    "build_apk_duration_seconds": {
        "overall": summarize_numbers([item.get("build_apk_duration_seconds") for item in runs]),
        "warm": summarize_numbers([item.get("build_apk_duration_seconds") for item in warm_runs]),
        "cold": summarize_numbers([item.get("build_apk_duration_seconds") for item in cold_runs]),
    },
}

summary_json_path.parent.mkdir(parents=True, exist_ok=True)
summary_json_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

latest_lines = [
    f"- job_id: {summary['latest_run']['job_id']}",
    f"- run_id: {summary['latest_run']['run_id']}",
    f"- job_status: {summary['latest_run']['job_status']}",
    f"- cache_mode_before: {summary['latest_run']['cache_mode_before']}",
    f"- script_duration_seconds: {summary['latest_run']['script_duration_seconds']}",
    f"- run_duration_seconds: {summary['latest_run']['run_duration_seconds']}",
    f"- build_apk_duration_seconds: {summary['latest_run']['build_apk_duration_seconds']}",
    f"- run_dir: {summary['latest_run']['run_dir']}",
] if summary["latest_run"] else ["- none"]

def stat_line(label, payload):
    return f"- {label}: count={payload['count']}, avg={payload['avg_seconds']}, median={payload['median_seconds']}, p95={payload['p95_seconds']}, min={payload['min_seconds']}, max={payload['max_seconds']}"

markdown = "\n".join([
    "# AppFactory Product Flow Summary",
    "",
    f"- generated_at: {summary['generated_at']}",
    f"- product_flow_root: {summary['product_flow_root']}",
    f"- run_count: {summary['run_count']}",
    f"- completed_run_count: {summary['completed_run_count']}",
    f"- failed_run_count: {summary['failed_run_count']}",
    "",
    "## Latest Run",
    *latest_lines,
    "",
    "## Script Duration",
    stat_line("overall", summary["script_duration_seconds"]["overall"]),
    stat_line("warm", summary["script_duration_seconds"]["warm"]),
    stat_line("cold", summary["script_duration_seconds"]["cold"]),
    "",
    "## Run Duration",
    stat_line("overall", summary["run_duration_seconds"]["overall"]),
    stat_line("warm", summary["run_duration_seconds"]["warm"]),
    stat_line("cold", summary["run_duration_seconds"]["cold"]),
    "",
    "## Build APK Duration",
    stat_line("overall", summary["build_apk_duration_seconds"]["overall"]),
    stat_line("warm", summary["build_apk_duration_seconds"]["warm"]),
    stat_line("cold", summary["build_apk_duration_seconds"]["cold"]),
    "",
]) + "\n"
summary_markdown_path.parent.mkdir(parents=True, exist_ok=True)
summary_markdown_path.write_text(markdown, encoding="utf-8")
PY