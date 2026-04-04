#!/usr/bin/env bash

set -euo pipefail

regression_root="${APPFACTORY_DEVICE_REGRESSION_ROOT:-${1:-workspace/appfactory/device-regression}}"
index_output_path="${APPFACTORY_DEVICE_REGRESSION_INDEX_OUTPUT:-${2:-${regression_root}/index.json}}"
latest_markdown_path="${APPFACTORY_DEVICE_REGRESSION_LATEST_MARKDOWN:-${3:-${regression_root}/latest.md}}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$regression_root" "$index_output_path" "$latest_markdown_path" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

regression_root = Path(sys.argv[1]).expanduser()
index_output_path = Path(sys.argv[2]).expanduser()
latest_markdown_path = Path(sys.argv[3]).expanduser()
generated_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

items = []
for result_path in sorted(regression_root.glob("runs/*/regression-result.json"), reverse=True):
    try:
        payload = json.loads(result_path.read_text(encoding="utf-8"))
    except Exception:
        continue
    payload["result_path"] = result_path.as_posix()
    items.append(payload)

index_payload = {
    "generated_at": generated_at,
    "regression_root": regression_root.as_posix(),
    "run_count": len(items),
    "latest_run_id": items[0].get("run_id") if items else None,
    "items": items,
}
index_output_path.parent.mkdir(parents=True, exist_ok=True)
index_output_path.write_text(json.dumps(index_payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

lines = ["# AppFactory 设备回归最新状态", "", f"- 生成时间：{generated_at}", f"- 回归根目录：{regression_root.as_posix()}", f"- 已归档回归轮次：{len(items)}"]

if not items:
    lines.extend(["", "当前还没有设备回归归档结果。"])
else:
    latest = items[0]
    lines.extend([
        "",
        "## 最新回归",
        "",
        f"- run_id：{latest.get('run_id') or '-'}",
        f"- mode：{latest.get('mode') or '-'}",
        f"- status：{latest.get('status') or '-'}",
        f"- alert_status：{latest.get('alert_status') or '-'}",
        f"- verify_exit_code：{latest.get('verify_exit_code')}",
        f"- alert_exit_code：{latest.get('alert_exit_code')}",
        f"- result_path：{latest.get('result_path') or '-'}",
    ])
    device = latest.get("device") or {}
    if device:
        lines.extend([
            "",
            "## 最新设备",
            "",
            f"- serial：{device.get('serial') or '-'}",
            f"- label：{device.get('label') or '-'}",
            f"- claim_status：{device.get('claim_status') or '-'}",
            f"- owner_id：{device.get('owner_id') or '-'}",
        ])
    artifacts = latest.get("artifacts") or {}
    lines.extend([
        "",
        "## 最新产物",
        "",
        f"- apk：{artifacts.get('apk') or '-'}",
        f"- device_logcat：{artifacts.get('device_logcat') or '-'}",
        f"- device_failure_summary_json：{artifacts.get('device_failure_summary_json') or '-'}",
    ])
    lines.extend([
        "",
        "## 最近 10 次回归",
        "",
        "| run_id | mode | device | status | alert_status | verify_exit_code | alert_exit_code |",
        "|---|---|---|---|---|---:|---:|",
    ])
    for item in items[:10]:
        device = item.get("device") or {}
        device_label = device.get("label") or device.get("serial") or "-"
        lines.append(
            "| {run_id} | {mode} | {device_label} | {status} | {alert_status} | {verify_exit_code} | {alert_exit_code} |".format(
                run_id=item.get("run_id") or "-",
                mode=item.get("mode") or "-",
                device_label=device_label,
                status=item.get("status") or "-",
                alert_status=item.get("alert_status") or "-",
                verify_exit_code=item.get("verify_exit_code") if item.get("verify_exit_code") is not None else "-",
                alert_exit_code=item.get("alert_exit_code") if item.get("alert_exit_code") is not None else "-",
            )
        )

latest_markdown_path.parent.mkdir(parents=True, exist_ok=True)
latest_markdown_path.write_text("\n".join(lines) + "\n", encoding="utf-8")

print(index_output_path.as_posix())
print(latest_markdown_path.as_posix())
PY