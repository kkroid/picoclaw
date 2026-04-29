#!/usr/bin/env bash

set -euo pipefail

OLLAMA_BASE_URL="${OLLAMA_BASE_URL:-http://10.12.11.159:11434}"
MODEL_NAME="${MODEL_NAME:-gemma4:26b}"
OUTPUT_DIR="${OUTPUT_DIR:-.runtime/builder-model-validation}"

mkdir -p "${OUTPUT_DIR}"

sanitize_name() {
  printf '%s' "$1" | tr '/:' '__'
}

archive_failure_artifacts() {
  local case_id="$1"
  local prompt_file="$2"
  local response_file="$3"
  local diagnostics_file="$4"
  local failure_dir
  failure_dir="${OUTPUT_DIR}/failures"
  mkdir -p "${failure_dir}"

  local base_name
  base_name="$(sanitize_name "${MODEL_NAME}")-${case_id}"
  cp "${prompt_file}" "${failure_dir}/${base_name}.prompt.txt"
  cp "${response_file}" "${failure_dir}/${base_name}.raw-response.json"
  cp "${diagnostics_file}" "${failure_dir}/${base_name}.error.txt"
}

run_case() {
	local case_id="$1"
  local task_type="$2"
	local allowed_paths_csv="$3"
	local prompt_file
	prompt_file="$(mktemp)"
	cat >"${prompt_file}"

	local started_at
	started_at="$(date +%s)"
	local response_file
	response_file="$(mktemp)"
  local diagnostics_file
  diagnostics_file="$(mktemp)"
  local result_file
  result_file="$(mktemp)"

	curl -sS --max-time 180 "${OLLAMA_BASE_URL}/v1/chat/completions" \
		-H 'Content-Type: application/json' \
		-d @- >"${response_file}" <<EOF
{
  "model": "${MODEL_NAME}",
  "temperature": 0,
  "messages": [
    {
      "role": "system",
      "content": "You are a builder-runtime patch generator. Output only compact JSON. No markdown. Never touch paths outside the allowed path set. For replace_block, use only supported keys: type, path, and either anchor, old_content, or start/end, plus new_content or replacement. Never use line-number fields such as start_line or end_line. Before sending, verify the content is valid JSON with all arrays and objects closed."
    },
    {
      "role": "user",
      "content": $(node -e "const fs=require('fs'); const p=process.argv[1]; process.stdout.write(JSON.stringify(fs.readFileSync(p,'utf8')));" "${prompt_file}")
    }
  ]
}
EOF

	local finished_at
	finished_at="$(date +%s)"
	local duration_seconds
	duration_seconds="$((finished_at - started_at))"

  if ! CASE_ID="${case_id}" TASK_TYPE="${task_type}" ALLOWED_PATHS_CSV="${allowed_paths_csv}" DURATION_SECONDS="${duration_seconds}" RESPONSE_FILE="${response_file}" node <<'NODE' >"${result_file}" 2>"${diagnostics_file}"
const fs = require('fs');

function fail(message, details) {
  console.error(`[error] ${message}`);
  if (details) {
    console.error(details);
  }
  process.exit(1);
}

function sliceAround(text, position) {
  if (!Number.isFinite(position) || position < 0) {
    return text;
  }
  const start = Math.max(0, position - 120);
  const end = Math.min(text.length, position + 120);
  return text.slice(start, end);
}

const caseId = process.env.CASE_ID;
const taskType = process.env.TASK_TYPE;
const allowedPaths = (process.env.ALLOWED_PATHS_CSV || '').split(',').filter(Boolean);
const durationSeconds = Number(process.env.DURATION_SECONDS || '0');
const raw = fs.readFileSync(process.env.RESPONSE_FILE, 'utf8');

let response;
try {
  response = JSON.parse(raw);
} catch (error) {
  fail(`response for ${caseId} is not valid JSON`, raw);
}

const content = response?.choices?.[0]?.message?.content;
if (typeof content !== 'string' || content.trim() === '') {
  fail(`response for ${caseId} has no assistant content`, raw);
}

let payload;
try {
  let normalized = content.trim();
  if (normalized.startsWith('```')) {
    normalized = normalized.replace(/^```[a-zA-Z0-9_-]*\n?/, '');
    normalized = normalized.replace(/\n?```$/, '');
    normalized = normalized.trim();
  }
  const firstBrace = normalized.indexOf('{');
  const lastBrace = normalized.lastIndexOf('}');
  if (firstBrace >= 0 && lastBrace > firstBrace) {
    normalized = normalized.slice(firstBrace, lastBrace + 1);
  }
  payload = JSON.parse(normalized);
} catch (error) {
  const match = /position (\d+)/.exec(String(error?.message || ''));
  const position = match ? Number(match[1]) : -1;
  const details = [String(error?.message || 'unknown parse error')];
  if (position >= 0) {
    details.push(`context: ${JSON.stringify(sliceAround(content, position))}`);
  }
  details.push(content);
  fail(`assistant content for ${caseId} is not valid JSON`, details.join('\n'));
}

function firstString(object, keys) {
  for (const key of keys) {
    if (typeof object?.[key] === 'string' && object[key].trim() !== '') {
      return object[key].trim();
    }
  }
  return '';
}

function normalizeOperationType(value) {
  switch (String(value || '').trim().toLowerCase()) {
    case 'write_file':
    case 'write':
    case 'create_file':
    case 'replace_file':
    case 'update_file':
      return 'write_file';
    case 'replace_block':
    case 'replace':
    case 'edit_block':
    case 'update_block':
      return 'replace_block';
    case 'delete_file':
    case 'delete':
    case 'remove_file':
      return 'delete_file';
    default:
      return '';
  }
}

function hasNonEmptyString(object, keys) {
  return keys.some((key) => typeof object?.[key] === 'string' && object[key].trim() !== '');
}

function countSchemaDrift(source) {
  let driftCount = 0;
  if (hasNonEmptyString(source, ['taskType'])) {
    driftCount += 1;
  }
  const operations = Array.isArray(source?.operations) ? source.operations : [];
  for (const operation of operations) {
    if (hasNonEmptyString(operation, ['action', 'operation', 'op', 'kind', 'operation_type'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['file_path', 'target_path', 'file'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['match', 'old_content'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['start_marker', 'begin'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['end_marker', 'finish'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['replacement', 'content', 'file_content'])) {
      driftCount += 1;
    }
  }
  return driftCount;
}

function normalizePatchPayload(source) {
  const normalizedTaskType = firstString(source, ['task_type', 'taskType']);
  const operations = Array.isArray(source?.operations) ? source.operations.map((operation) => {
    const path = firstString(operation, ['path', 'file_path', 'target_path', 'file']);
    const anchor = firstString(operation, ['anchor', 'match', 'old_content']);
    const start = firstString(operation, ['start', 'start_marker', 'begin']);
    const end = firstString(operation, ['end', 'end_marker', 'finish']);
    const replacement = firstString(operation, ['new_content', 'replacement', 'content', 'file_content']);
    return {
      type: normalizeOperationType(firstString(operation, ['type', 'action', 'operation', 'op', 'kind', 'operation_type'])),
      path,
      anchor,
      start,
      end,
      replacement,
    };
  }) : [];
  return {
    task_type: normalizedTaskType,
    operations,
    upgrade_recommended: Boolean(source?.upgrade_recommended),
  };
}

const schemaDriftCount = countSchemaDrift(payload);
payload = normalizePatchPayload(payload);

if (payload.task_type !== taskType) {
  fail(`task_type mismatch for ${caseId}: got ${payload.task_type}, want ${taskType}`, content);
}

if (!Array.isArray(payload.operations) || payload.operations.length === 0) {
  fail(`operations missing for ${caseId}`, content);
}

for (const operation of payload.operations) {
  if (!operation.type) {
    fail(`operation type missing or unsupported for ${caseId}`, content);
  }
  if (typeof operation.path !== 'string' || operation.path.trim() === '') {
    fail(`operation path missing for ${caseId}`, content);
  }
  if (!allowedPaths.includes(operation.path)) {
    fail(`operation path ${operation.path} is outside allowed set for ${caseId}`, content);
  }
  if (operation.type === 'replace_block') {
    const hasAnchor = operation.anchor !== '';
    const hasRange = operation.start !== '' && operation.end !== '';
    if (!hasAnchor && !hasRange) {
      fail(`replace_block location missing for ${caseId}`, content);
    }
    if (operation.replacement === '') {
      fail(`replace_block replacement missing for ${caseId}`, content);
    }
  }
}

const result = {
  case_id: caseId,
  task_type: taskType,
  duration_seconds: durationSeconds,
  repair_rounds: 1,
  attempts: 1,
  operation_count: payload.operations.length,
  targeted_operation_count: payload.operations.length,
  unrelated_operation_count: 0,
  unrelated_operation_rate: 0,
  paths: payload.operations.map((operation) => operation.path),
  schema_normalized: schemaDriftCount > 0,
  schema_drift_count: schemaDriftCount,
  parse_failure_count: 0,
  scope_violation_count: 0,
  upgrade_recommended: Boolean(payload.upgrade_recommended),
};

console.log(JSON.stringify(result));
NODE
  then
    archive_failure_artifacts "${case_id}" "${prompt_file}" "${response_file}" "${diagnostics_file}"
    cat "${diagnostics_file}" >&2
    rm -f "${prompt_file}" "${response_file}" "${diagnostics_file}" "${result_file}"
    return 1
  fi

  cat "${result_file}"

  rm -f "${prompt_file}" "${response_file}" "${diagnostics_file}" "${result_file}"
}

echo "[info] running first five builder-runtime leaf task samples via ${OLLAMA_BASE_URL} using ${MODEL_NAME}"

results_file="${OUTPUT_DIR}/$(date +%Y%m%d-%H%M%S)-${MODEL_NAME//[:\/]/_}.jsonl"

run_case "single-file-edit" "single_file_edit" "lib/main.dart" <<'EOF' >>"${results_file}"
Return compact JSON with keys: case_id, task_type, upgrade_recommended, operations.
Set case_id to single-file-edit.
Set task_type to single_file_edit.
Set upgrade_recommended to false.
Allowed path: lib/main.dart only.
Task: replace the MaterialApp title in lib/main.dart from 'Old Title' to 'Budget Flow'.
Output operations as an array with exactly one object.
That operation must use type replace_block, path lib/main.dart, anchor exactly "title: 'Old Title',", and new_content exactly "title: 'Budget Flow',".
EOF

run_case "dual-file-wiring" "dual_file_wiring" "lib/views/home_page.dart,lib/controllers/home_controller.dart" <<'EOF' >>"${results_file}"
Return compact JSON with keys: case_id, task_type, upgrade_recommended, operations.
Set case_id to dual-file-wiring.
Set task_type to dual_file_wiring.
Set upgrade_recommended to false.
Allowed paths: lib/views/home_page.dart and lib/controllers/home_controller.dart only.
Task: wire a HomeController loading state into HomePage.
Output exactly two operations: one for lib/controllers/home_controller.dart and one for lib/views/home_page.dart.
Use only type replace_block or write_file.
Do not mention any other path.
Before sending, verify the operations array is valid JSON, contains exactly two objects, and ends with ].
EOF

run_case "analyze-repair" "analyze_repair" "lib/views/entry_form_page.dart" <<'EOF' >>"${results_file}"
Return compact JSON with keys: case_id, task_type, upgrade_recommended, operations.
Set case_id to analyze-repair.
Set task_type to analyze_repair.
Set upgrade_recommended to false.
Allowed path: lib/views/entry_form_page.dart only.
Given analyze error: "The method 'setSelectedDate' can't be unconditionally invoked because the receiver can be 'null'".
Produce exactly one replace_block operation on lib/views/entry_form_page.dart that guards the nullable value before calling setSelectedDate.
Use keys type, path, and either anchor or old_content, plus new_content or replacement. Do not use line numbers.
EOF

run_case "test-repair" "test_repair" "test/widget_test.dart" <<'EOF' >>"${results_file}"
Return compact JSON with keys: case_id, task_type, upgrade_recommended, operations.
Set case_id to test-repair.
Set task_type to test_repair.
Set upgrade_recommended to false.
Allowed path: test/widget_test.dart only.
Task: update a widget test so it asserts text 'Budget Flow' instead of 'Old Title'.
Produce exactly one replace_block operation on test/widget_test.dart.
Use keys type, path, and either anchor or old_content, plus new_content or replacement. Do not use line numbers.
EOF

run_case "closure-repair" "closure_repair" "lib/main.dart,pubspec.yaml" <<'EOF' >>"${results_file}"
Return compact JSON with keys: case_id, task_type, upgrade_recommended, operations.
Set case_id to closure-repair.
Set task_type to closure_repair.
Set upgrade_recommended to false.
Allowed paths: lib/main.dart and pubspec.yaml only.
Task: perform a low-risk closure repair for a Flutter app where the app title is stale and a missing dependency entry prevents build closure.
Output one or two operations, only on the allowed paths.
Use only type replace_block or write_file.
Do not mention any other path.
For replace_block, use only type, path, and either anchor or old_content, plus new_content or replacement. Do not use line numbers.
EOF

echo "[ok] leaf task sample results written to ${results_file}"
cat "${results_file}"