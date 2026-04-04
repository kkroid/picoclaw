#!/usr/bin/env bash

set -euo pipefail

metrics_root="${APPFACTORY_DEVICE_METRICS_ROOT:-${1:-workspace/appfactory}}"
output_path="${APPFACTORY_DEVICE_METRICS_OUTPUT:-${2:-}}"
json_output_path="${APPFACTORY_DEVICE_METRICS_JSON_OUTPUT:-${3:-}}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

python3 - "$metrics_root" "$output_path" "$json_output_path" <<'PY'
import json
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path


def markdown_escape(text: str) -> str:
    return text.replace("|", "\\|")


metrics_root = Path(sys.argv[1]).expanduser()
output_path = Path(sys.argv[2]).expanduser() if sys.argv[2] else None
json_output_path = Path(sys.argv[3]).expanduser() if len(sys.argv) > 3 and sys.argv[3] else None
generated_at = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')

stats = defaultdict(lambda: {
    "count": 0,
    "runs": set(),
    "jobs": set(),
    "latest_metrics": "",
})
metrics_files = sorted(metrics_root.glob("jobs/*/runs/*/metrics.json"))

for metrics_path in metrics_files:
    try:
        payload = json.loads(metrics_path.read_text(encoding="utf-8"))
    except Exception:
        continue
    job_id = str(payload.get("job_id") or metrics_path.parts[-4])
    run_id = metrics_path.parts[-2]
    for item in payload.get("device_failure_categories") or []:
        category = str(item.get("category") or "").strip()
        failure_domain = str(item.get("failure_domain") or "").strip()
        count = int(item.get("count") or 0)
        if not category or count <= 0:
            continue
        key = (failure_domain or "unknown", category)
        stats[key]["count"] += count
        stats[key]["runs"].add(run_id)
        stats[key]["jobs"].add(job_id)
        stats[key]["latest_metrics"] = metrics_path.as_posix()

lines = []
lines.append("# AppFactory 设备失败类别汇总")
lines.append("")
lines.append(f"- 生成时间：{generated_at}")
lines.append(f"- 扫描根目录：{metrics_root.as_posix()}")
lines.append(f"- metrics 文件数：{len(metrics_files)}")


def write_json_summary(categories):
    if not json_output_path:
        return
    payload = {
        "generated_at": generated_at,
        "metrics_root": metrics_root.as_posix(),
        "metrics_file_count": len(metrics_files),
        "device_failure_category_count": len(categories),
        "categories": categories,
    }
    json_output_path.parent.mkdir(parents=True, exist_ok=True)
    json_output_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

if not stats:
    lines.append("- 设备失败统计：当前未发现包含 `device_failure_categories` 的 metrics.json")
    content = "\n".join(lines) + "\n"
    write_json_summary([])
    if output_path:
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(content, encoding="utf-8")
        print(output_path.as_posix())
        if json_output_path:
            print(json_output_path.as_posix())
    else:
        print(content, end="")
    sys.exit(0)

lines.append(f"- 设备失败类别数：{len(stats)}")
lines.append("")
lines.append("| failure_domain | failure_category | total_count | run_count | job_count | latest_metrics |")
lines.append("|---|---|---:|---:|---:|---|")

categories = []

for (failure_domain, category), item in sorted(
    stats.items(),
    key=lambda entry: (-entry[1]["count"], entry[0][0], entry[0][1]),
):
    categories.append({
        "failure_domain": failure_domain,
        "failure_category": category,
        "total_count": item["count"],
        "run_count": len(item["runs"]),
        "job_count": len(item["jobs"]),
        "latest_metrics": item["latest_metrics"],
    })
    lines.append(
        "| {domain} | {category} | {count} | {run_count} | {job_count} | {latest} |".format(
            domain=markdown_escape(failure_domain),
            category=markdown_escape(category),
            count=item["count"],
            run_count=len(item["runs"]),
            job_count=len(item["jobs"]),
            latest=markdown_escape(item["latest_metrics"]),
        )
    )

content = "\n".join(lines) + "\n"
write_json_summary(categories)
if output_path:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(content, encoding="utf-8")
    print(output_path.as_posix())
    if json_output_path:
        print(json_output_path.as_posix())
else:
    print(content, end="")
PY