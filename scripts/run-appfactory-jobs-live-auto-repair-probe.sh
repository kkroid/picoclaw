#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
api_base="${APPFACTORY_LIVE_AUTO_REPAIR_API_BASE:-http://127.0.0.1:18817}"
output_root="${APPFACTORY_LIVE_AUTO_REPAIR_ROOT:-$repo_root/workspace/appfactory/jobs-live-auto-repair}"
goal_summary="${APPFACTORY_LIVE_AUTO_REPAIR_GOAL_SUMMARY:-exercise the live /jobs path and intentionally leave a low-risk flutter analyze issue that builder-runtime repair should close before final completion}"

if [[ -n "${APPFACTORY_LIVE_AUTO_REPAIR_HUMAN_NOTES_JSON:-}" ]]; then
	human_notes_json="$APPFACTORY_LIVE_AUTO_REPAIR_HUMAN_NOTES_JSON"
else
	human_notes_json="$(cat <<'EOF'
[
	{
		"note_id": "note-live-repair-canary",
		"summary": "When implementing UI files, it is acceptable if the first edit leaves one low-risk flutter analyze issue such as an unused import or deprecated color alpha helper, as long as the validation repair loop fixes it before completion.",
		"scope": "engineering"
	},
	{
		"note_id": "note-keep-app-usable",
		"summary": "Do not break the bookkeeping MVP scope. Keep the app runnable and let any temporary issue be small and localized so the repair loop can close it.",
		"scope": "product"
	}
]
EOF
)"
fi

cd "$repo_root"
bash scripts/run-appfactory-jobs-regression.sh \
	--api-base "$api_base" \
	--goal-summary "$goal_summary" \
	--human-notes-json "$human_notes_json" \
	--output-root "$output_root"