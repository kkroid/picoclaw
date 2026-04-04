#!/usr/bin/env bash

set -euo pipefail

status_root="${APPFACTORY_INTERNAL_TRIAL_STATUS_ROOT:-workspace/appfactory/internal-trial}"
status_json_path="${APPFACTORY_INTERNAL_TRIAL_STATUS_JSON_OUTPUT:-${status_root}/status.json}"
status_markdown_path="${APPFACTORY_INTERNAL_TRIAL_STATUS_MARKDOWN_OUTPUT:-${status_root}/status.md}"
notifications_json_path="${APPFACTORY_INTERNAL_TRIAL_STATUS_NOTIFICATIONS_JSON:-workspace/appfactory/notifications/index.json}"
critical_notification_types="${APPFACTORY_INTERNAL_TRIAL_STATUS_CRITICAL_NOTIFICATION_TYPES:-prd_approval_requested,template_approval_requested,execution_interrupted,execution_failed,execution_recovery_failed}"
informational_notification_types="${APPFACTORY_INTERNAL_TRIAL_STATUS_INFORMATIONAL_NOTIFICATION_TYPES:-delivery_ready_for_signing,execution_handoff,orchestrator_watch_started}"
notifications_max_age_hours="${APPFACTORY_INTERNAL_TRIAL_STATUS_NOTIFICATIONS_MAX_AGE_HOURS:-24}"
refresh_notifications="${APPFACTORY_INTERNAL_TRIAL_STATUS_REFRESH_NOTIFICATIONS:-0}"
refresh_freshness="${APPFACTORY_INTERNAL_TRIAL_STATUS_REFRESH_FRESHNESS:-1}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

freshness_output_file="$(mktemp -t appfactory-internal-trial-freshness-XXXXXX.txt)"
notifications_refresh_output_file="$(mktemp -t appfactory-internal-trial-notifications-XXXXXX.txt)"
cleanup() {
  rm -f "$freshness_output_file"
    rm -f "$notifications_refresh_output_file"
}
trap cleanup EXIT

freshness_exit_code=0
notifications_refresh_exit_code=0
if [[ "$refresh_freshness" == "1" ]]; then
  set +e
  bash scripts/check-appfactory-internal-trial-freshness.sh > "$freshness_output_file"
  freshness_exit_code=$?
  set -e
fi

if [[ "$refresh_notifications" == "1" ]]; then
    set +e
    bash scripts/refresh-appfactory-notifications.sh > "$notifications_refresh_output_file"
    notifications_refresh_exit_code=$?
    set -e
fi

python3 - "$status_json_path" "$status_markdown_path" "$notifications_json_path" "$critical_notification_types" "$informational_notification_types" "$notifications_max_age_hours" "$freshness_output_file" "$freshness_exit_code" "$notifications_refresh_output_file" "$notifications_refresh_exit_code" "$refresh_notifications" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def parse_key_value_file(path: Path):
    data = {}
    if not path.is_file():
        return data
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        if "=" not in raw_line:
            continue
        key, value = raw_line.split("=", 1)
        data[key.strip()] = value.strip()
    return data


status_json_path = Path(sys.argv[1]).expanduser()
status_markdown_path = Path(sys.argv[2]).expanduser()
notifications_json_path = Path(sys.argv[3]).expanduser()
critical_types = {item.strip() for item in sys.argv[4].split(",") if item.strip()}
informational_types = {item.strip() for item in sys.argv[5].split(",") if item.strip()}
notifications_max_age_hours = float(sys.argv[6])
freshness_output_path = Path(sys.argv[7]).expanduser()
freshness_exit_code = int(sys.argv[8])
notifications_refresh_output_path = Path(sys.argv[9]).expanduser()
notifications_refresh_exit_code = int(sys.argv[10])
refresh_notifications = sys.argv[11] == "1"


def parse_datetime(value):
    if not value:
        return None
    normalized = str(value).strip()
    if not normalized:
        return None
    if normalized.endswith("Z"):
        normalized = normalized[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(normalized)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        return parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)

freshness_data = parse_key_value_file(freshness_output_path)
notifications_refresh_data = parse_key_value_file(notifications_refresh_output_path)
notifications_available = notifications_json_path.is_file()
notifications_payload = {}
notifications_items = []
if notifications_available:
    notifications_payload = json.loads(notifications_json_path.read_text(encoding="utf-8"))
    notifications_items = notifications_payload.get("items") or []

unacknowledged = [item for item in notifications_items if not item.get("acknowledged")]
critical_unacknowledged = [item for item in unacknowledged if str(item.get("type") or "") in critical_types]
informational_unacknowledged = [item for item in unacknowledged if str(item.get("type") or "") in informational_types]
only_informational_unacknowledged = bool(unacknowledged) and len(informational_unacknowledged) == len(unacknowledged)
notifications_generated_at = notifications_payload.get("generated_at") if isinstance(notifications_payload, dict) else None
notifications_generated_at_dt = parse_datetime(notifications_generated_at)
notifications_generated_at_age_hours = None
notification_snapshot_status = "unavailable"
if notifications_available:
    if notifications_generated_at_dt is None:
        notification_snapshot_status = "missing_generated_at"
    else:
        notifications_generated_at_age_hours = round(
            (datetime.now(timezone.utc) - notifications_generated_at_dt).total_seconds() / 3600.0,
            3,
        )
        if notifications_generated_at_age_hours > notifications_max_age_hours:
            notification_snapshot_status = "stale"
        else:
            notification_snapshot_status = "fresh"

oldest_unacknowledged_age_hours = None
if unacknowledged:
    created_times = [parse_datetime(item.get("created_at")) for item in unacknowledged]
    created_times = [item for item in created_times if item is not None]
    if created_times:
        oldest_unacknowledged_age_hours = round(
            (datetime.now(timezone.utc) - min(created_times)).total_seconds() / 3600.0,
            3,
        )

unacknowledged_by_type = {}
for item in unacknowledged:
    key = str(item.get("type") or "unknown")
    unacknowledged_by_type[key] = unacknowledged_by_type.get(key, 0) + 1

overall_status = "ok"
notes = []
if freshness_exit_code != 0:
    overall_status = "alert"
    notes.append("freshness check is stale")
elif critical_unacknowledged:
    overall_status = "alert"
    notes.append("critical unacknowledged notifications present")
elif only_informational_unacknowledged:
    notes.append("only informational keep-latest notifications remain")
elif unacknowledged:
    overall_status = "attention"
    notes.append("unacknowledged notifications present")
elif notification_snapshot_status == "stale":
    overall_status = "attention"
    notes.append("notifications snapshot is stale")
elif notification_snapshot_status == "missing_generated_at":
    overall_status = "attention"
    notes.append("notifications snapshot missing generated_at")
elif refresh_notifications and notifications_refresh_exit_code != 0 and not notifications_available:
    overall_status = "attention"
    notes.append("notifications snapshot refresh failed")
elif not notifications_available:
    notes.append("notifications index not available in current workspace")

payload = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "status_root": status_json_path.parent.as_posix(),
    "overall_status": overall_status,
    "notes": notes,
    "freshness": {
        "status": freshness_data.get("internal_trial_freshness_status", "unknown"),
        "exit_code": freshness_exit_code,
        "product_latest_json": freshness_data.get("product_latest_json"),
        "product_latest_job_id": freshness_data.get("product_latest_job_id"),
        "product_latest_status": freshness_data.get("product_latest_status"),
        "product_latest_age_hours": freshness_data.get("product_latest_age_hours"),
        "platform_latest_json": freshness_data.get("platform_latest_json"),
        "platform_latest_run_id": freshness_data.get("platform_latest_run_id"),
        "platform_latest_age_hours": freshness_data.get("platform_latest_age_hours"),
        "require_device": freshness_data.get("require_device"),
        "require_device_reason": freshness_data.get("require_device_reason"),
        "device_index_json": freshness_data.get("device_index_json"),
        "device_latest_run_id": freshness_data.get("device_latest_run_id"),
        "device_latest_status": freshness_data.get("device_latest_status"),
        "device_latest_age_hours": freshness_data.get("device_latest_age_hours"),
        "violations": [
            value
            for key, value in freshness_data.items()
            if key == "internal_trial_freshness_violation"
        ],
    },
    "notifications": {
        "available": notifications_available,
        "path": notifications_json_path.as_posix(),
        "refresh_requested": refresh_notifications,
        "refresh_exit_code": notifications_refresh_exit_code,
        "refresh_generated_at": notifications_refresh_data.get("notifications_rebuild_generated_at"),
        "generated_at": notifications_generated_at,
        "generated_at_age_hours": notifications_generated_at_age_hours,
        "snapshot_status": notification_snapshot_status,
        "total_count": len(notifications_items),
        "unacknowledged_count": len(unacknowledged),
        "critical_unacknowledged_count": len(critical_unacknowledged),
        "informational_unacknowledged_count": len(informational_unacknowledged),
        "only_informational_unacknowledged": only_informational_unacknowledged,
        "oldest_unacknowledged_age_hours": oldest_unacknowledged_age_hours,
        "unacknowledged_by_type": dict(sorted(unacknowledged_by_type.items())),
        "critical_types": sorted(critical_types),
        "informational_types": sorted(informational_types),
        "latest_unacknowledged": [
            {
                "notification_id": item.get("notification_id"),
                "type": item.get("type"),
                "job_id": item.get("job_id"),
                "suggested_action": item.get("suggested_action"),
                "acknowledged": bool(item.get("acknowledged")),
                "summary": item.get("summary"),
            }
            for item in unacknowledged[:10]
        ],
    },
}

status_json_path.parent.mkdir(parents=True, exist_ok=True)
status_json_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

lines = [
    "# AppFactory Internal Trial Status",
    "",
    f"- generated_at: {payload['generated_at']}",
    f"- overall_status: {payload['overall_status']}",
]
if notes:
    lines.extend(["- notes:"] + [f"  - {note}" for note in notes])

freshness = payload["freshness"]
lines.extend(
    [
        "",
        "## Freshness",
        "",
        f"- status: {freshness.get('status') or '-'}",
        f"- product_latest_job_id: {freshness.get('product_latest_job_id') or '-'}",
        f"- product_latest_age_hours: {freshness.get('product_latest_age_hours') or '-'}",
        f"- platform_latest_run_id: {freshness.get('platform_latest_run_id') or '-'}",
        f"- platform_latest_age_hours: {freshness.get('platform_latest_age_hours') or '-'}",
        f"- require_device: {freshness.get('require_device') or '-'}",
        f"- device_latest_run_id: {freshness.get('device_latest_run_id') or '-'}",
        f"- device_latest_age_hours: {freshness.get('device_latest_age_hours') or '-'}",
    ]
)

notifications = payload["notifications"]
lines.extend(
    [
        "",
        "## Notifications",
        "",
        f"- available: {notifications.get('available')}",
        f"- refresh_requested: {notifications.get('refresh_requested')}",
        f"- refresh_exit_code: {notifications.get('refresh_exit_code')}",
        f"- snapshot_status: {notifications.get('snapshot_status') or '-'}",
        f"- generated_at_age_hours: {notifications.get('generated_at_age_hours') if notifications.get('generated_at_age_hours') is not None else '-'}",
        f"- path: {notifications.get('path') or '-'}",
        f"- total_count: {notifications.get('total_count')}",
        f"- unacknowledged_count: {notifications.get('unacknowledged_count')}",
        f"- critical_unacknowledged_count: {notifications.get('critical_unacknowledged_count')}",
        f"- oldest_unacknowledged_age_hours: {notifications.get('oldest_unacknowledged_age_hours') if notifications.get('oldest_unacknowledged_age_hours') is not None else '-'}",
    ]
)

unack_by_type = notifications.get("unacknowledged_by_type") or {}
if unack_by_type:
    lines.extend(["", "## Unacknowledged By Type", "", "| type | count |", "|---|---|"])
    for key, value in unack_by_type.items():
        lines.append(f"| {key} | {value} |")

unack_items = notifications.get("latest_unacknowledged") or []
if unack_items:
    lines.extend(["", "## Latest Unacknowledged", "", "| notification_id | type | job_id | suggested_action |", "|---|---|---|---|"])
    for item in unack_items:
        lines.append(
            f"| {item.get('notification_id') or '-'} | {item.get('type') or '-'} | {item.get('job_id') or '-'} | {item.get('suggested_action') or '-'} |"
        )

status_markdown_path.parent.mkdir(parents=True, exist_ok=True)
status_markdown_path.write_text("\n".join(lines) + "\n", encoding="utf-8")

print(f"internal_trial_status_json={status_json_path.as_posix()}")
print(f"internal_trial_status_markdown={status_markdown_path.as_posix()}")
print(f"internal_trial_status={overall_status}")
print(f"internal_trial_notification_unacknowledged_count={len(unacknowledged)}")
print(f"internal_trial_notification_critical_unacknowledged_count={len(critical_unacknowledged)}")
PY