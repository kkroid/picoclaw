#!/usr/bin/env bash

set -euo pipefail

platform_root="${APPFACTORY_PLATFORM_REGRESSION_ROOT:-${1:-workspace/appfactory/platform-regression}}"
summary_markdown_path="${APPFACTORY_PLATFORM_REGRESSION_SUMMARY_OUTPUT:-${2:-${platform_root}/reports/platform-regression-summary.md}}"
summary_json_path="${APPFACTORY_PLATFORM_REGRESSION_SUMMARY_JSON_OUTPUT:-${3:-${platform_root}/reports/platform-regression-summary.json}}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$platform_root" "$summary_markdown_path" "$summary_json_path" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def read_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def simplify_run(payload):
    stages = payload.get("stages") or {}
    simplified = {
        "run_id": payload.get("run_id", ""),
        "run_dir": payload.get("run_dir", ""),
        "generated_at": payload.get("generated_at", ""),
        "stages": {},
    }
    for stage_name, stage_payload in stages.items():
        if not isinstance(stage_payload, dict):
            continue
        simplified["stages"][stage_name] = {
            "enabled": bool(stage_payload.get("enabled", False)),
            "status": stage_payload.get("status", "unknown"),
            "exit_code": stage_payload.get("exit_code"),
        }
        if stage_name == "device_regression":
            simplified["stages"][stage_name]["mode"] = stage_payload.get("mode", "")
            simplified["stages"][stage_name]["use_pool"] = bool(stage_payload.get("use_pool", False))
            simplified["stages"][stage_name]["auto_enabled_from_pool"] = bool(stage_payload.get("auto_enabled_from_pool", False))
    return simplified


def summarize_stage(runs, stage_name):
    counts = {"passed": 0, "failed": 0, "skipped": 0, "unknown": 0}
    enabled_runs = 0
    for run in runs:
        stage = (run.get("stages") or {}).get(stage_name) or {}
        if stage.get("enabled"):
            enabled_runs += 1
        status = str(stage.get("status") or "unknown")
        if status not in counts:
            counts[status] = 0
        counts[status] += 1
    return {
        "enabled_runs": enabled_runs,
        "counts": counts,
    }


root = Path(sys.argv[1]).expanduser()
summary_markdown_path = Path(sys.argv[2]).expanduser()
summary_json_path = Path(sys.argv[3]).expanduser()
runs_root = root / "runs"

runs = []
for result_path in sorted(runs_root.glob("*/platform-regression-result.json")):
    payload = read_json(result_path)
    payload["result_path"] = result_path.as_posix()
    runs.append(payload)

latest = runs[-1] if runs else None
latest_with_device = next((item for item in reversed(runs) if ((item.get("stages") or {}).get("device_regression") or {}).get("enabled")), None)

summary = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "platform_root": root.as_posix(),
    "run_count": len(runs),
    "latest_run": simplify_run(latest) if latest else None,
    "latest_run_with_device": simplify_run(latest_with_device) if latest_with_device else None,
    "stage_summary": {
        "product_flow": summarize_stage(runs, "product_flow"),
        "jobs_regression": summarize_stage(runs, "jobs_regression"),
        "builder_runtime_real": summarize_stage(runs, "builder_runtime_real"),
        "builder_runtime_summary": summarize_stage(runs, "builder_runtime_summary"),
        "device_regression": summarize_stage(runs, "device_regression"),
    },
    "all_runs": [simplify_run(item) for item in runs],
}

summary_json_path.parent.mkdir(parents=True, exist_ok=True)
summary_json_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

latest_json_path = root / "latest.json"
latest_markdown_path = root / "latest.md"
if latest is not None:
    latest_json_path.write_text(json.dumps(simplify_run(latest), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
else:
    latest_json_path.write_text(json.dumps({}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

def stage_line(name, payload):
    if not payload:
        return f"- {name}: none"
    parts = [
        f"enabled={payload.get('enabled', False)}",
        f"status={payload.get('status', 'unknown')}",
        f"exit_code={payload.get('exit_code')}",
    ]
    if name == "device_regression":
        parts.append(f"mode={payload.get('mode', '')}")
        parts.append(f"use_pool={payload.get('use_pool', False)}")
        parts.append(f"auto_enabled_from_pool={payload.get('auto_enabled_from_pool', False)}")
    return f"- {name}: " + ", ".join(parts)

latest_lines = ["- none"]
if summary["latest_run"]:
    latest_lines = [
        f"- run_id: {summary['latest_run']['run_id']}",
        f"- generated_at: {summary['latest_run']['generated_at']}",
        f"- run_dir: {summary['latest_run']['run_dir']}",
        stage_line("product_flow", summary["latest_run"]["stages"].get("product_flow")),
        stage_line("jobs_regression", summary["latest_run"]["stages"].get("jobs_regression")),
        stage_line("builder_runtime_real", summary["latest_run"]["stages"].get("builder_runtime_real")),
        stage_line("builder_runtime_summary", summary["latest_run"]["stages"].get("builder_runtime_summary")),
        stage_line("device_regression", summary["latest_run"]["stages"].get("device_regression")),
    ]

def stage_summary_line(name, payload):
    counts = payload.get("counts") or {}
    return f"- {name}: enabled_runs={payload.get('enabled_runs', 0)}, passed={counts.get('passed', 0)}, failed={counts.get('failed', 0)}, skipped={counts.get('skipped', 0)}, unknown={counts.get('unknown', 0)}"

markdown = "\n".join([
    "# AppFactory Platform Regression Summary",
    "",
    f"- generated_at: {summary['generated_at']}",
    f"- platform_root: {summary['platform_root']}",
    f"- run_count: {summary['run_count']}",
    "",
    "## Latest Run",
    *latest_lines,
    "",
    "## Stage Summary",
    stage_summary_line("product_flow", summary["stage_summary"]["product_flow"]),
    stage_summary_line("jobs_regression", summary["stage_summary"]["jobs_regression"]),
    stage_summary_line("builder_runtime_real", summary["stage_summary"]["builder_runtime_real"]),
    stage_summary_line("builder_runtime_summary", summary["stage_summary"]["builder_runtime_summary"]),
    stage_summary_line("device_regression", summary["stage_summary"]["device_regression"]),
    "",
]) + "\n"
summary_markdown_path.parent.mkdir(parents=True, exist_ok=True)
summary_markdown_path.write_text(markdown, encoding="utf-8")
latest_markdown_path.write_text(markdown, encoding="utf-8")
PY