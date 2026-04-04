#!/usr/bin/env bash

set -euo pipefail

api_base="${APPFACTORY_NOTIFICATIONS_API_BASE:-${APPFACTORY_PRODUCT_FLOW_API_BASE:-http://127.0.0.1:18800}}"
snapshot_path="${APPFACTORY_NOTIFICATIONS_SNAPSHOT_PATH:-workspace/appfactory/notifications/index.json}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

response="$(curl -sS -X POST "$api_base/api/v1/notifications:rebuild" -H 'Content-Type: application/json' -d '{}' -w $'\n%{http_code}')"
http_body="${response%$'\n'*}"
http_code="${response##*$'\n'}"

if [[ ! "$http_code" =~ ^2 ]]; then
  echo "notifications_rebuild_http_code=$http_code" >&2
  echo "$http_body" >&2
  exit 1
fi

python3 - "$snapshot_path" "$http_body" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

snapshot_path = Path(sys.argv[1]).expanduser()
response_payload = json.loads(sys.argv[2]) if sys.argv[2].strip() else {}

items = response_payload.get("items") or []
generated_at = response_payload.get("generated_at") or datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
snapshot_payload = {
  "generated_at": generated_at,
  "items": items,
}
snapshot_path.parent.mkdir(parents=True, exist_ok=True)
snapshot_path.write_text(json.dumps(snapshot_payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

unacknowledged_count = sum(1 for item in items if not item.get("acknowledged"))

print(f"notifications_rebuild_snapshot_path={snapshot_path.as_posix()}")
print(f"notifications_rebuild_generated_at={generated_at or ''}")
print(f"notifications_rebuild_total_count={len(items)}")
print(f"notifications_rebuild_unacknowledged_count={unacknowledged_count}")
print(f"notifications_rebuild_snapshot_exists=true")
PY