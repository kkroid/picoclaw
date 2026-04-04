#!/usr/bin/env bash

set -euo pipefail

governance_root="${APPFACTORY_NOTIFICATION_GOVERNANCE_ROOT:-workspace/appfactory/notifications/governance}"
governance_json_path="${APPFACTORY_NOTIFICATION_GOVERNANCE_JSON_OUTPUT:-${governance_root}/latest.json}"
governance_markdown_path="${APPFACTORY_NOTIFICATION_GOVERNANCE_MARKDOWN_OUTPUT:-${governance_root}/latest.md}"
notifications_json_path="${APPFACTORY_NOTIFICATION_GOVERNANCE_NOTIFICATIONS_JSON:-workspace/appfactory/notifications/index.json}"
api_base="${APPFACTORY_NOTIFICATION_GOVERNANCE_API_BASE:-${APPFACTORY_NOTIFICATIONS_API_BASE:-${APPFACTORY_PRODUCT_FLOW_API_BASE:-http://127.0.0.1:18800}}}"
refresh_notifications="${APPFACTORY_NOTIFICATION_GOVERNANCE_REFRESH_NOTIFICATIONS:-0}"
apply_auto_ack="${APPFACTORY_NOTIFICATION_GOVERNANCE_APPLY_AUTO_ACK:-0}"
allow_offline_snapshot_ack="${APPFACTORY_NOTIFICATION_GOVERNANCE_ALLOW_OFFLINE_SNAPSHOT_ACK:-0}"
critical_types="${APPFACTORY_NOTIFICATION_GOVERNANCE_CRITICAL_TYPES:-prd_approval_requested,template_approval_requested,execution_interrupted,execution_failed,execution_recovery_failed}"
manual_review_types="${APPFACTORY_NOTIFICATION_GOVERNANCE_MANUAL_REVIEW_TYPES:-builder_failed,execution_cancelled}"
auto_ack_types="${APPFACTORY_NOTIFICATION_GOVERNANCE_AUTO_ACK_TYPES:-delivery_ready_for_signing,execution_handoff,orchestrator_watch_started}"
auto_ack_min_age_hours="${APPFACTORY_NOTIFICATION_GOVERNANCE_AUTO_ACK_MIN_AGE_HOURS:-2}"
keep_latest_per_auto_type="${APPFACTORY_NOTIFICATION_GOVERNANCE_KEEP_LATEST_PER_AUTO_TYPE:-1}"
supersede_failure_types="${APPFACTORY_NOTIFICATION_GOVERNANCE_SUPERSEDE_FAILURE_TYPES:-execution_failed,execution_interrupted,builder_failed,execution_cancelled}"
supersede_success_types="${APPFACTORY_NOTIFICATION_GOVERNANCE_SUPERSEDE_SUCCESS_TYPES:-delivery_ready_for_signing}"
historical_close_job_prefixes="${APPFACTORY_NOTIFICATION_GOVERNANCE_HISTORICAL_CLOSE_JOB_PREFIXES:-job-controlled-e2e}"
historical_close_types="${APPFACTORY_NOTIFICATION_GOVERNANCE_HISTORICAL_CLOSE_TYPES:-execution_failed,execution_interrupted,builder_failed,execution_cancelled}"
historical_close_min_age_hours="${APPFACTORY_NOTIFICATION_GOVERNANCE_HISTORICAL_CLOSE_MIN_AGE_HOURS:-72}"
informational_keep_latest_types="${APPFACTORY_NOTIFICATION_GOVERNANCE_INFORMATIONAL_KEEP_LATEST_TYPES:-delivery_ready_for_signing,execution_handoff,orchestrator_watch_started}"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

refresh_output_file="$(mktemp -t appfactory-notification-governance-refresh-XXXXXX.txt)"
cleanup() {
  rm -f "$refresh_output_file"
}
trap cleanup EXIT

refresh_exit_code=0
if [[ "$refresh_notifications" == "1" ]]; then
  set +e
  APPFACTORY_NOTIFICATIONS_API_BASE="$api_base" \
    APPFACTORY_NOTIFICATIONS_SNAPSHOT_PATH="$notifications_json_path" \
    bash scripts/refresh-appfactory-notifications.sh > "$refresh_output_file"
  refresh_exit_code=$?
  set -e
fi

python3 - "$governance_json_path" "$governance_markdown_path" "$notifications_json_path" "$api_base" "$refresh_notifications" "$refresh_exit_code" "$refresh_output_file" "$apply_auto_ack" "$allow_offline_snapshot_ack" "$critical_types" "$manual_review_types" "$auto_ack_types" "$auto_ack_min_age_hours" "$keep_latest_per_auto_type" "$supersede_failure_types" "$supersede_success_types" "$historical_close_job_prefixes" "$historical_close_types" "$historical_close_min_age_hours" "$informational_keep_latest_types" <<'PY'
import json
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from urllib import error, parse, request


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


def read_snapshot(path: Path):
    if not path.is_file():
        return {}, []
    payload = json.loads(path.read_text(encoding="utf-8"))
    items = payload.get("items") or []
    if not isinstance(items, list):
        items = []
    return payload, items


def build_row(item, age_hours):
    return {
        "notification_id": item.get("notification_id"),
        "type": item.get("type"),
        "job_id": item.get("job_id"),
        "status": item.get("status"),
        "summary": item.get("summary"),
        "suggested_action": item.get("suggested_action"),
        "created_at": item.get("created_at"),
        "age_hours": age_hours,
        "delivery_status": item.get("delivery_status"),
        "failure_category": item.get("failure_category"),
    }


def derive_job_family(job_id):
    normalized = str(job_id or "").strip()
    if not normalized:
        return ""
    for marker in ("-202",):
        index = normalized.find(marker)
        if index > 0:
            return normalized[:index]
    return normalized


def post_json(url, payload):
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = request.Request(url, data=body, headers={"Content-Type": "application/json"}, method="POST")
    with request.urlopen(req, timeout=30) as resp:
        data = resp.read().decode("utf-8")
        return json.loads(data) if data.strip() else {}


def ack_notification(api_base, notification_id):
    quoted = parse.quote(str(notification_id), safe="")
    req = request.Request(
        f"{api_base}/api/v1/notifications/{quoted}:ack",
        data=b"{}",
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with request.urlopen(req, timeout=30) as resp:
        data = resp.read().decode("utf-8")
        return json.loads(data) if data.strip() else {}


def apply_snapshot_ack(path, items, notification_ids):
    acknowledged_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    id_set = {str(item) for item in notification_ids if str(item).strip()}
    updated = []
    applied = []
    for item in items:
        copied = dict(item)
        notification_id = str(copied.get("notification_id") or "")
        if notification_id in id_set and not copied.get("acknowledged"):
            copied["acknowledged"] = True
            copied["acknowledged_at"] = acknowledged_at
            applied.append(
                {
                    "notification_id": notification_id,
                    "acknowledged_at": acknowledged_at,
                    "type": copied.get("type"),
                }
            )
        updated.append(copied)
    path.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "items": updated,
    }
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return payload, applied


def classify(
    items,
    critical_types,
    manual_review_types,
    auto_ack_types,
    auto_ack_min_age_hours,
    keep_latest_per_auto_type,
    supersede_failure_types,
    supersede_success_types,
    historical_close_job_prefixes,
    historical_close_types,
    historical_close_min_age_hours,
    informational_keep_latest_types,
):
    now = datetime.now(timezone.utc)
    unacknowledged = []
    for item in items:
        if item.get("acknowledged"):
            continue
        created_at_dt = parse_datetime(item.get("created_at"))
        age_hours = None
        if created_at_dt is not None:
            age_hours = round((now - created_at_dt).total_seconds() / 3600.0, 3)
        enriched = dict(item)
        enriched["_age_hours"] = age_hours
        enriched["_created_at_dt"] = created_at_dt
        enriched["_job_family"] = derive_job_family(item.get("job_id"))
        unacknowledged.append(enriched)

    success_by_family = defaultdict(list)
    for item in items:
        item_type = str(item.get("type") or "")
        if item_type not in supersede_success_types:
            continue
        if str(item.get("status") or "") != "completed":
            continue
        created_at_dt = parse_datetime(item.get("created_at"))
        if created_at_dt is None:
            continue
        job_family = derive_job_family(item.get("job_id"))
        if not job_family:
            continue
        success_by_family[job_family].append(created_at_dt)

    for timestamps in success_by_family.values():
        timestamps.sort()

    grouped_auto = defaultdict(list)
    for item in unacknowledged:
        item_type = str(item.get("type") or "")
        if item_type in auto_ack_types:
            grouped_auto[item_type].append(item)

    auto_ack_candidates = []
    blocked_keep_latest = []
    informational_keep_latest = []
    blocked_too_fresh = []
    superseded_by_success = []
    historical_close_candidates = []
    consumed_ids = set()
    for item_type, grouped_items in grouped_auto.items():
        ordered = sorted(
            grouped_items,
            key=lambda item: item.get("_created_at_dt") or datetime.min.replace(tzinfo=timezone.utc),
            reverse=True,
        )
        keep = max(0, keep_latest_per_auto_type)
        for index, item in enumerate(ordered):
            notification_id = item.get("notification_id")
            if not notification_id:
                continue
            if index < keep:
                row = build_row(item, item.get("_age_hours"))
                if item_type in informational_keep_latest_types:
                    row["policy_reason"] = "informational_keep_latest"
                    informational_keep_latest.append(row)
                else:
                    blocked_keep_latest.append(row)
                consumed_ids.add(notification_id)
                continue
            age_hours = item.get("_age_hours")
            if age_hours is None or age_hours < auto_ack_min_age_hours:
                blocked_too_fresh.append(build_row(item, age_hours))
                consumed_ids.add(notification_id)
                continue
            auto_ack_candidates.append(build_row(item, age_hours))
            consumed_ids.add(notification_id)

    for item in unacknowledged:
        notification_id = item.get("notification_id")
        if not notification_id or notification_id in consumed_ids:
            continue
        item_type = str(item.get("type") or "")
        created_at_dt = item.get("_created_at_dt")
        job_family = item.get("_job_family")
        if item_type in supersede_failure_types and created_at_dt is not None and job_family in success_by_family:
            if any(success_dt > created_at_dt for success_dt in success_by_family[job_family]):
                row = build_row(item, item.get("_age_hours"))
                row["policy_reason"] = "superseded_by_later_success"
                row["job_family"] = job_family
                superseded_by_success.append(row)
                consumed_ids.add(notification_id)
                continue
        if item_type in historical_close_types and job_family in historical_close_job_prefixes:
            age_hours = item.get("_age_hours")
            if age_hours is not None and age_hours >= historical_close_min_age_hours:
                row = build_row(item, age_hours)
                row["policy_reason"] = "historical_close_window_elapsed"
                row["job_family"] = job_family
                historical_close_candidates.append(row)
                consumed_ids.add(notification_id)
                continue

    critical = []
    manual_review = []
    unknown = []
    for item in unacknowledged:
        notification_id = item.get("notification_id")
        if notification_id in consumed_ids:
            continue
        item_type = str(item.get("type") or "")
        row = build_row(item, item.get("_age_hours"))
        if item_type in critical_types:
            critical.append(row)
            continue
        if not manual_review_types or item_type in manual_review_types:
            manual_review.append(row)
            continue
        unknown.append(row)

    by_type = defaultdict(int)
    for item in unacknowledged:
        by_type[str(item.get("type") or "unknown")] += 1

    return {
        "total_unacknowledged_count": len(unacknowledged),
        "critical_count": len(critical),
        "manual_review_count": len(manual_review),
        "unknown_count": len(unknown),
        "auto_ack_candidate_count": len(auto_ack_candidates),
        "superseded_by_success_count": len(superseded_by_success),
        "historical_close_candidate_count": len(historical_close_candidates),
        "informational_keep_latest_count": len(informational_keep_latest),
        "blocked_keep_latest_count": len(blocked_keep_latest),
        "blocked_too_fresh_count": len(blocked_too_fresh),
        "unacknowledged_by_type": dict(sorted(by_type.items())),
        "critical": critical,
        "manual_review": manual_review,
        "unknown": unknown,
        "auto_ack_candidates": auto_ack_candidates,
        "superseded_by_success": superseded_by_success,
        "historical_close_candidates": historical_close_candidates,
        "informational_keep_latest": informational_keep_latest,
        "blocked_keep_latest": blocked_keep_latest,
        "blocked_too_fresh": blocked_too_fresh,
    }


governance_json_path = Path(sys.argv[1]).expanduser()
governance_markdown_path = Path(sys.argv[2]).expanduser()
notifications_json_path = Path(sys.argv[3]).expanduser()
api_base = sys.argv[4].rstrip("/")
refresh_notifications = sys.argv[5] == "1"
refresh_exit_code = int(sys.argv[6])
refresh_output = parse_key_value_file(Path(sys.argv[7]).expanduser())
apply_auto_ack = sys.argv[8] == "1"
allow_offline_snapshot_ack = sys.argv[9] == "1"
critical_types = {item.strip() for item in sys.argv[10].split(",") if item.strip()}
manual_review_types = {item.strip() for item in sys.argv[11].split(",") if item.strip()}
auto_ack_types = {item.strip() for item in sys.argv[12].split(",") if item.strip()}
auto_ack_min_age_hours = float(sys.argv[13])
keep_latest_per_auto_type = int(sys.argv[14])
supersede_failure_types = {item.strip() for item in sys.argv[15].split(",") if item.strip()}
supersede_success_types = {item.strip() for item in sys.argv[16].split(",") if item.strip()}
historical_close_job_prefixes = {item.strip() for item in sys.argv[17].split(",") if item.strip()}
historical_close_types = {item.strip() for item in sys.argv[18].split(",") if item.strip()}
historical_close_min_age_hours = float(sys.argv[19])
informational_keep_latest_types = {item.strip() for item in sys.argv[20].split(",") if item.strip()}

snapshot_before, items_before = read_snapshot(notifications_json_path)
classification_before = classify(
    items_before,
    critical_types,
    manual_review_types,
    auto_ack_types,
    auto_ack_min_age_hours,
    keep_latest_per_auto_type,
    supersede_failure_types,
    supersede_success_types,
    historical_close_job_prefixes,
    historical_close_types,
    historical_close_min_age_hours,
    informational_keep_latest_types,
)

applied = []
failed = []
refresh_after_apply = {}
apply_mode = "none"
if apply_auto_ack:
    apply_candidates = []
    for row in classification_before["auto_ack_candidates"]:
        if row.get("notification_id"):
            apply_candidates.append(row)
    for row in classification_before["superseded_by_success"]:
        if row.get("notification_id"):
            apply_candidates.append(row)
    for row in classification_before["historical_close_candidates"]:
        if row.get("notification_id"):
            apply_candidates.append(row)
    deduped_candidates = []
    seen_candidate_ids = set()
    for row in apply_candidates:
        notification_id = row.get("notification_id")
        if not notification_id or notification_id in seen_candidate_ids:
            continue
        seen_candidate_ids.add(notification_id)
        deduped_candidates.append(row)
    candidate_ids = [row.get("notification_id") for row in deduped_candidates]
    api_failed = False
    if candidate_ids:
        apply_mode = "api"
        for item in deduped_candidates:
            notification_id = item.get("notification_id")
            if not notification_id:
                continue
            try:
                acked = ack_notification(api_base, notification_id)
            except error.HTTPError as exc:
                api_failed = True
                failed.append({
                    "notification_id": notification_id,
                    "http_status": exc.code,
                    "message": exc.read().decode("utf-8", errors="replace"),
                })
                break
            except Exception as exc:
                api_failed = True
                failed.append({
                    "notification_id": notification_id,
                    "message": str(exc),
                })
                break
            else:
                applied.append({
                    "notification_id": notification_id,
                    "acknowledged_at": acked.get("acknowledged_at"),
                    "type": acked.get("type") or item.get("type"),
                })
        if api_failed and allow_offline_snapshot_ack:
            apply_mode = "offline_snapshot"
            failed = []
            snapshot_after_apply, applied = apply_snapshot_ack(notifications_json_path, items_before, candidate_ids)
            refresh_after_apply = {
                "generated_at": snapshot_after_apply.get("generated_at"),
                "total_count": len(snapshot_after_apply.get("items") or []),
            }
        elif applied:
            try:
                rebuild_payload = post_json(f"{api_base}/api/v1/notifications:rebuild", {})
            except Exception as exc:
                failed.append({
                    "notification_id": "notifications:rebuild",
                    "message": str(exc),
                })
            else:
                generated_at = rebuild_payload.get("generated_at") or datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
                refresh_after_apply = {
                    "generated_at": generated_at,
                    "total_count": len(rebuild_payload.get("items") or []),
                }
                notifications_json_path.parent.mkdir(parents=True, exist_ok=True)
                notifications_json_path.write_text(
                    json.dumps(
                        {
                            "generated_at": generated_at,
                            "items": rebuild_payload.get("items") or [],
                        },
                        ensure_ascii=False,
                        indent=2,
                    )
                    + "\n",
                    encoding="utf-8",
                )

snapshot_after, items_after = read_snapshot(notifications_json_path)
classification_after = classify(
    items_after,
    critical_types,
    manual_review_types,
    auto_ack_types,
    auto_ack_min_age_hours,
    keep_latest_per_auto_type,
    supersede_failure_types,
    supersede_success_types,
    historical_close_job_prefixes,
    historical_close_types,
    historical_close_min_age_hours,
    informational_keep_latest_types,
)

overall_status = "ok"
notes = []
if refresh_notifications and refresh_exit_code != 0:
    overall_status = "attention"
    notes.append("notifications refresh failed before governance check")
if classification_after["critical_count"] > 0:
    overall_status = "alert"
    notes.append("critical notifications still require manual action")
elif classification_after["manual_review_count"] > 0 or classification_after["unknown_count"] > 0:
    overall_status = "attention"
    notes.append("manual-review notifications remain in backlog")
elif classification_after["auto_ack_candidate_count"] > 0:
    overall_status = "attention"
    notes.append("safe auto-ack candidates remain unapplied")
elif classification_after["informational_keep_latest_count"] > 0:
    notes.append("only informational keep-latest notifications remain")
if failed:
    if overall_status == "ok":
        overall_status = "attention"
    notes.append("some auto-ack operations failed")
if not items_after:
    if overall_status == "ok":
        overall_status = "attention"
    notes.append("notifications snapshot is unavailable in current workspace")

payload = {
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "overall_status": overall_status,
    "notifications_json_path": notifications_json_path.as_posix(),
    "api_base": api_base,
    "refresh_notifications": refresh_notifications,
    "refresh_exit_code": refresh_exit_code,
    "refresh_generated_at": refresh_output.get("notifications_rebuild_generated_at"),
    "apply_auto_ack": apply_auto_ack,
    "allow_offline_snapshot_ack": allow_offline_snapshot_ack,
    "auto_ack_policy": {
        "critical_types": sorted(critical_types),
        "manual_review_types": sorted(manual_review_types),
        "auto_ack_types": sorted(auto_ack_types),
        "auto_ack_min_age_hours": auto_ack_min_age_hours,
        "keep_latest_per_auto_type": keep_latest_per_auto_type,
        "supersede_failure_types": sorted(supersede_failure_types),
        "supersede_success_types": sorted(supersede_success_types),
        "historical_close_job_prefixes": sorted(historical_close_job_prefixes),
        "historical_close_types": sorted(historical_close_types),
        "historical_close_min_age_hours": historical_close_min_age_hours,
        "informational_keep_latest_types": sorted(informational_keep_latest_types),
    },
    "notes": notes,
    "before": classification_before,
    "after": classification_after,
    "apply_result": {
        "apply_mode": apply_mode,
        "applied_count": len(applied),
        "failed_count": len(failed),
        "applied": applied,
        "failed": failed,
        "refresh_after_apply": refresh_after_apply,
    },
    "snapshot_generated_at": snapshot_after.get("generated_at") if isinstance(snapshot_after, dict) else None,
}

governance_json_path.parent.mkdir(parents=True, exist_ok=True)
governance_json_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

lines = [
    "# AppFactory Notification Governance",
    "",
    f"- generated_at: {payload['generated_at']}",
    f"- overall_status: {payload['overall_status']}",
    f"- notifications_json_path: {payload['notifications_json_path']}",
    f"- refresh_notifications: {payload['refresh_notifications']}",
    f"- apply_auto_ack: {payload['apply_auto_ack']}",
    f"- allow_offline_snapshot_ack: {payload['allow_offline_snapshot_ack']}",
    f"- auto_ack_min_age_hours: {payload['auto_ack_policy']['auto_ack_min_age_hours']}",
    f"- keep_latest_per_auto_type: {payload['auto_ack_policy']['keep_latest_per_auto_type']}",
        f"- historical_close_min_age_hours: {payload['auto_ack_policy']['historical_close_min_age_hours']}",
    f"- snapshot_generated_at: {payload.get('snapshot_generated_at') or '-'}",
  ]
if notes:
    lines.extend(["- notes:"] + [f"  - {item}" for item in notes])

lines.extend([
    "",
    "## Before",
    "",
    f"- total_unacknowledged_count: {classification_before['total_unacknowledged_count']}",
    f"- critical_count: {classification_before['critical_count']}",
    f"- manual_review_count: {classification_before['manual_review_count']}",
    f"- unknown_count: {classification_before['unknown_count']}",
    f"- auto_ack_candidate_count: {classification_before['auto_ack_candidate_count']}",
    f"- superseded_by_success_count: {classification_before['superseded_by_success_count']}",
    f"- historical_close_candidate_count: {classification_before['historical_close_candidate_count']}",
    f"- informational_keep_latest_count: {classification_before['informational_keep_latest_count']}",
    f"- blocked_keep_latest_count: {classification_before['blocked_keep_latest_count']}",
    f"- blocked_too_fresh_count: {classification_before['blocked_too_fresh_count']}",
    "",
    "## After",
    "",
    f"- total_unacknowledged_count: {classification_after['total_unacknowledged_count']}",
    f"- critical_count: {classification_after['critical_count']}",
    f"- manual_review_count: {classification_after['manual_review_count']}",
    f"- unknown_count: {classification_after['unknown_count']}",
    f"- auto_ack_candidate_count: {classification_after['auto_ack_candidate_count']}",
    f"- superseded_by_success_count: {classification_after['superseded_by_success_count']}",
    f"- historical_close_candidate_count: {classification_after['historical_close_candidate_count']}",
    f"- informational_keep_latest_count: {classification_after['informational_keep_latest_count']}",
    f"- blocked_keep_latest_count: {classification_after['blocked_keep_latest_count']}",
    f"- blocked_too_fresh_count: {classification_after['blocked_too_fresh_count']}",
    "",
    "## Apply Result",
    "",
    f"- apply_mode: {apply_mode}",
    f"- applied_count: {len(applied)}",
    f"- failed_count: {len(failed)}",
])

for section_title, rows in (
    ("Critical Notifications", classification_after["critical"][:20]),
    ("Manual Review Notifications", classification_after["manual_review"][:20]),
    ("Unknown Notifications", classification_after["unknown"][:20]),
    ("Auto Ack Candidates", classification_after["auto_ack_candidates"][:20]),
    ("Superseded By Success", classification_after["superseded_by_success"][:20]),
    ("Historical Close Candidates", classification_after["historical_close_candidates"][:20]),
    ("Informational Keep Latest", classification_after["informational_keep_latest"][:20]),
):
    lines.extend(["", f"## {section_title}", "", "| type | job_id | age_hours | suggested_action | summary |", "|---|---|---:|---|---|"])
    if rows:
        for item in rows:
            summary = str(item.get("summary") or "-").replace("|", "\\|")
            lines.append(
                f"| {item.get('type') or '-'} | {item.get('job_id') or '-'} | {item.get('age_hours') if item.get('age_hours') is not None else '-'} | {item.get('suggested_action') or '-'} | {summary} |"
            )
    else:
        lines.append("| - | - | - | - | - |")

lines.extend(["", "## Unacknowledged By Type", "", "| type | count |", "|---|---:|"])
for item_type, count in classification_after["unacknowledged_by_type"].items():
    lines.append(f"| {item_type} | {count} |")
if not classification_after["unacknowledged_by_type"]:
    lines.append("| - | 0 |")

governance_markdown_path.parent.mkdir(parents=True, exist_ok=True)
governance_markdown_path.write_text("\n".join(lines) + "\n", encoding="utf-8")

print(f"notification_governance_json={governance_json_path.as_posix()}")
print(f"notification_governance_markdown={governance_markdown_path.as_posix()}")
print(f"notification_governance_status={overall_status}")
print(f"notification_governance_before_unacknowledged_count={classification_before['total_unacknowledged_count']}")
print(f"notification_governance_after_unacknowledged_count={classification_after['total_unacknowledged_count']}")
print(f"notification_governance_after_critical_count={classification_after['critical_count']}")
print(f"notification_governance_after_manual_review_count={classification_after['manual_review_count']}")
print(f"notification_governance_after_auto_ack_candidate_count={classification_after['auto_ack_candidate_count']}")
print(f"notification_governance_after_superseded_by_success_count={classification_after['superseded_by_success_count']}")
print(f"notification_governance_after_historical_close_candidate_count={classification_after['historical_close_candidate_count']}")
print(f"notification_governance_after_informational_keep_latest_count={classification_after['informational_keep_latest_count']}")
print(f"notification_governance_applied_count={len(applied)}")

if overall_status == "alert":
    sys.exit(1)
PY