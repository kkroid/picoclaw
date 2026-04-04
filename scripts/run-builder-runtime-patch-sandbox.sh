#!/usr/bin/env bash

set -euo pipefail

OLLAMA_BASE_URL="${OLLAMA_BASE_URL:-http://127.0.0.1:11434}"
MODEL_NAME="${MODEL_NAME:-qwen2.5-coder:14b}"
OUTPUT_DIR="${OUTPUT_DIR:-.runtime/builder-model-validation}"
MAX_REPAIR_ROUNDS="${MAX_REPAIR_ROUNDS:-2}"

mkdir -p "${OUTPUT_DIR}"

workspace_dir="$(mktemp -d)"
baseline_dir="$(mktemp -d)"
response_file="$(mktemp)"
patch_file="$(mktemp)"
stats_file="$(mktemp)"
prompt_file="$(mktemp)"
failure_file="$(mktemp)"

cleanup() {
  rm -f "${response_file}" "${patch_file}" "${stats_file}" "${prompt_file}" "${failure_file}"
  rm -rf "${baseline_dir}"
	rm -rf "${workspace_dir}"
}
trap cleanup EXIT

restore_workspace() {
  rm -rf "${workspace_dir}"
  mkdir -p "${workspace_dir}"
  cp -R "${baseline_dir}/." "${workspace_dir}/"
}

mkdir -p "${workspace_dir}/lib"
cat >"${workspace_dir}/lib/main.dart" <<'EOF'
import 'package:flutter/material.dart';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Old Title',
      home: const Placeholder(),
    );
  }
}
EOF

cp -R "${workspace_dir}/." "${baseline_dir}/"
decode_response() {
	RESPONSE_FILE="${response_file}" PATCH_FILE="${patch_file}" STATS_FILE="${stats_file}" node <<'NODE'
const fs = require('fs');

const raw = fs.readFileSync(process.env.RESPONSE_FILE, 'utf8');
const response = JSON.parse(raw);
let content = response?.choices?.[0]?.message?.content;
if (typeof content !== 'string' || content.trim() === '') {
  throw new Error('assistant content missing');
}
content = content.trim();
if (content.startsWith('```')) {
  content = content.replace(/^```[a-zA-Z0-9_-]*\n?/, '');
  content = content.replace(/\n?```$/, '');
  content = content.trim();
}
const firstBrace = content.indexOf('{');
const lastBrace = content.lastIndexOf('}');
if (firstBrace >= 0 && lastBrace > firstBrace) {
  content = content.slice(firstBrace, lastBrace + 1);
}
const patch = JSON.parse(content);

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
    case 'replace_block':
    case 'replace':
    case 'edit_block':
    case 'update_block':
      return 'replace_block';
    case 'write_file':
    case 'write':
    case 'create_file':
    case 'replace_file':
    case 'update_file':
      return 'write_file';
    default:
      return '';
  }
}

function hasNonEmptyString(object, keys) {
  return keys.some((key) => typeof object?.[key] === 'string' && object[key].trim() !== '');
}

function countSchemaDrift(source) {
  let driftCount = 0;
  const operations = Array.isArray(source?.operations) ? source.operations : [];
  for (const operation of operations) {
    if (hasNonEmptyString(operation, ['action', 'operation', 'op', 'kind'])) {
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

const operations = Array.isArray(patch.operations) ? patch.operations.map((operation) => ({
  type: normalizeOperationType(firstString(operation, ['type', 'action', 'operation', 'op', 'kind'])),
  path: firstString(operation, ['path', 'file_path', 'target_path', 'file']),
  anchor: firstString(operation, ['anchor', 'match', 'old_content']),
  start: firstString(operation, ['start', 'start_marker', 'begin']),
  end: firstString(operation, ['end', 'end_marker', 'finish']),
  new_content: firstString(operation, ['new_content', 'replacement', 'content', 'file_content']),
})) : [];
const schemaDriftCount = countSchemaDrift(patch);

if (operations.length !== 1) {
  throw new Error(`expected exactly one operation, got ${operations.length}`);
}
const operation = operations[0];
if (operation.type !== 'replace_block') {
  throw new Error(`expected replace_block, got ${operation.type}`);
}
if (operation.path !== 'lib/main.dart') {
  throw new Error(`expected lib/main.dart, got ${operation.path}`);
}
fs.writeFileSync(process.env.PATCH_FILE, JSON.stringify({ patch_id: patch.patch_id || 'sandbox-patch', operations }, null, 2));
fs.writeFileSync(process.env.STATS_FILE, JSON.stringify({
  attempts: 1,
  schema_normalized: schemaDriftCount > 0,
  schema_drift_count: schemaDriftCount,
  parse_failure_count: 0,
  scope_violation_count: 0,
}, null, 2));
NODE
}

apply_and_record() {
  WORKSPACE_DIR="${workspace_dir}" PATCH_FILE="${patch_file}" STATS_FILE="${stats_file}" DURATION_SECONDS="${duration_seconds}" OUTPUT_DIR="${OUTPUT_DIR}" MODEL_NAME="${MODEL_NAME}" REPAIR_ROUNDS="${repair_rounds}" ATTEMPTS="${attempts}" SCHEMA_NORMALIZED="${schema_normalized}" SCHEMA_DRIFT_COUNT="${schema_drift_total}" PARSE_FAILURE_COUNT="${parse_failure_total}" SCOPE_VIOLATION_COUNT="${scope_violation_total}" node <<'NODE'
const fs = require('fs');
const path = require('path');

const workspaceDir = process.env.WORKSPACE_DIR;
const patch = JSON.parse(fs.readFileSync(process.env.PATCH_FILE, 'utf8'));
const operation = patch.operations[0];
const target = path.join(workspaceDir, operation.path);
let source = fs.readFileSync(target, 'utf8');
if (operation.anchor) {
  if (!source.includes(operation.anchor)) {
    throw new Error(`anchor not found: ${operation.anchor}`);
  }
  source = source.replace(operation.anchor, operation.new_content);
} else {
  if (!operation.start || !operation.end) {
    throw new Error('replace_block requires anchor or start/end');
  }
  const startIndex = source.indexOf(operation.start);
  if (startIndex < 0) {
    throw new Error(`start marker not found: ${operation.start}`);
  }
  let endSearchStart = startIndex;
  if (operation.end !== operation.start) {
    endSearchStart = startIndex + operation.start.length;
  }
  const endRelativeIndex = source.slice(endSearchStart).indexOf(operation.end);
  if (endRelativeIndex < 0) {
    throw new Error(`end marker not found: ${operation.end}`);
  }
  const replaceEnd = endSearchStart + endRelativeIndex + operation.end.length;
  source = source.slice(0, startIndex) + operation.new_content + source.slice(replaceEnd);
}
fs.writeFileSync(target, source);
if (!source.includes("title: 'Budget Flow',")) {
  throw new Error('patched file does not contain expected title');
}

const result = {
  case_id: 'sandbox-single-file-apply',
  model_name: process.env.MODEL_NAME || 'unknown',
  duration_seconds: Number(process.env.DURATION_SECONDS || '0'),
  repair_rounds: Number(process.env.REPAIR_ROUNDS || '0'),
  attempts: Number(process.env.ATTEMPTS || '0'),
  operation_count: 1,
  targeted_operation_count: 1,
  unrelated_operation_count: 0,
  unrelated_operation_rate: 0,
  workspace_path: workspaceDir,
  patched_file: 'lib/main.dart',
  patch_id: patch.patch_id || '',
  schema_normalized: String(process.env.SCHEMA_NORMALIZED || '') === 'true',
  schema_drift_count: Number(process.env.SCHEMA_DRIFT_COUNT || '0'),
  parse_failure_count: Number(process.env.PARSE_FAILURE_COUNT || '0'),
  scope_violation_count: Number(process.env.SCOPE_VIOLATION_COUNT || '0'),
  status: 'applied',
};

const latestPath = path.join(process.env.OUTPUT_DIR, 'sandbox-single-file-apply.latest.json');
const archivedPath = path.join(
  process.env.OUTPUT_DIR,
  `sandbox-single-file-apply.${(process.env.MODEL_NAME || 'unknown').replace(/[/:]/g, '_')}.json`,
);
const serialized = JSON.stringify(result, null, 2);
fs.writeFileSync(latestPath, serialized);
fs.writeFileSync(archivedPath, serialized);
console.log(JSON.stringify(result));
NODE
}

read_attempt_stats() {
  STATS_FILE="${stats_file}" node <<'NODE'
const fs = require('fs');
const stats = JSON.parse(fs.readFileSync(process.env.STATS_FILE, 'utf8'));
console.log([
  Number(stats.schema_drift_count || 0),
  Number(stats.parse_failure_count || 0),
  Number(stats.scope_violation_count || 0),
].join(' '));
NODE
}

attempts=0
repair_rounds=0
duration_seconds=0
schema_drift_total=0
parse_failure_total=0
scope_violation_total=0
schema_normalized=false
: >"${failure_file}"

while [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; do
  attempts="$((attempts + 1))"
  restore_workspace
  cat >"${prompt_file}" <<'EOF'
Generate a WorkspacePatch JSON object for this exact task. Allowed path: lib/main.dart only. Update MaterialApp title from 'Old Title' to 'Budget Flow'. Output format: {patch_id:string, operations:[{type:string,path:string,anchor:string,new_content:string}]}. Use exactly one replace_block operation on lib/main.dart. Use anchor exactly "title: 'Old Title'," and new_content exactly "title: 'Budget Flow',".
EOF
  if [ -s "${failure_file}" ]; then
    cat >>"${prompt_file}" <<EOF

Previous round failed.
Current round: ${attempts}.
Failure details:
$(cat "${failure_file}")
EOF
  fi
  started_at="$(date +%s)"
  curl -sS --max-time 180 "${OLLAMA_BASE_URL}/v1/chat/completions" \
    -H 'Content-Type: application/json' \
    -d @- >"${response_file}" <<EOF
{
  "model": "${MODEL_NAME}",
  "temperature": 0,
  "messages": [
    {
      "role": "system",
      "content": "You are a builder-runtime patch generator. Output only compact JSON. No markdown. Stay inside the allowed path set. Before sending, verify the content is valid JSON with all arrays and objects closed."
    },
    {
      "role": "user",
      "content": $(node -e "const fs=require('fs'); process.stdout.write(JSON.stringify(fs.readFileSync(process.argv[1],'utf8')));" "${prompt_file}")
    }
  ]
}
EOF
  finished_at="$(date +%s)"
  duration_seconds="$((duration_seconds + finished_at - started_at))"

  if ! decode_response 2>"${failure_file}"; then
    parse_failure_total="$((parse_failure_total + 1))"
    if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
      continue
    fi
    cat "${failure_file}" >&2
    exit 1
  fi

  read attempt_schema_drift attempt_parse_fail attempt_scope_violation <<<"$(read_attempt_stats)"
  schema_drift_total="$((schema_drift_total + attempt_schema_drift))"
  parse_failure_total="$((parse_failure_total + attempt_parse_fail))"
  scope_violation_total="$((scope_violation_total + attempt_scope_violation))"
  if [ "${attempt_schema_drift}" -gt 0 ]; then
    schema_normalized=true
  fi

  repair_rounds="${attempts}"
  if apply_and_record >"${failure_file}" 2>&1; then
    cat "${failure_file}"
    echo "[ok] sandbox patch validation passed for ${MODEL_NAME}"
    exit 0
  fi

  if [ "${attempts}" -ge "${MAX_REPAIR_ROUNDS}" ]; then
    cat "${failure_file}" >&2
    exit 1
  fi
done

exit 1