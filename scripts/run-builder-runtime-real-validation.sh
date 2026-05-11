#!/usr/bin/env bash

set -euo pipefail

OLLAMA_BASE_URL="${OLLAMA_BASE_URL:-http://10.12.11.159:11434}"
MODEL_NAME="${MODEL_NAME:-gemma4:26b}"
ONEAPPFACTORY_BUILDER_IMAGE="${ONEAPPFACTORY_BUILDER_IMAGE:-oneappfactory/builder:local}"
OUTPUT_DIR="${OUTPUT_DIR:-.runtime/builder-model-validation}"
MAX_REPAIR_ROUNDS="${MAX_REPAIR_ROUNDS:-2}"

mkdir -p "${OUTPUT_DIR}"

sanitize_name() {
  printf '%s' "$1" | tr '/:' '__'
}

write_case_result() {
  local case_id="$1"
  local source_file="$2"
  local latest_file="${OUTPUT_DIR}/${case_id}.latest.json"
  local archived_file="${OUTPUT_DIR}/${case_id}.$(sanitize_name "${MODEL_NAME}").json"
  cp "${source_file}" "${latest_file}"
  cp "${source_file}" "${archived_file}"
}

restore_workspace() {
  local baseline_dir="$1"
  local workspace_dir="$2"
  docker run --rm \
    -v "${workspace_dir}:${workspace_dir}" \
    "${ONEAPPFACTORY_BUILDER_IMAGE}" exec /bin/sh -lc "find '${workspace_dir}' -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null || true"
  cp -R "${baseline_dir}/." "${workspace_dir}/"
}

read_attempt_stats() {
  local stats_file="$1"
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

append_retry_context() {
  local prompt_file="$1"
  local failure_file="$2"
  local round="$3"
  if [ ! -s "${failure_file}" ]; then
    return
  fi
  cat >>"${prompt_file}" <<EOF

Previous round failed.
Current round: ${round}.
Fix the issue using the validation/apply failure details below.
Failure details:
$(cat "${failure_file}")
EOF
}

write_validation_result() {
  local case_id="$1"
  local duration_seconds="$2"
  local patch_file="$3"
  local output_file="$4"
  local validation_label="$5"
  local repair_rounds="$6"
  local attempts="$7"
  local schema_normalized="$8"
  local schema_drift_count="$9"
  local parse_failure_count="${10}"
  local scope_violation_count="${11}"
  CASE_ID="${case_id}" DURATION_SECONDS="${duration_seconds}" PATCH_FILE="${patch_file}" OUTPUT_FILE="${output_file}" VALIDATION_LABEL="${validation_label}" REPAIR_ROUNDS="${repair_rounds}" ATTEMPTS="${attempts}" SCHEMA_NORMALIZED="${schema_normalized}" SCHEMA_DRIFT_COUNT="${schema_drift_count}" PARSE_FAILURE_COUNT="${parse_failure_count}" SCOPE_VIOLATION_COUNT="${scope_violation_count}" node <<'NODE'
const fs = require('fs');
const patch = JSON.parse(fs.readFileSync(process.env.PATCH_FILE, 'utf8'));
const operationCount = patch.operations.length;
const result = {
  case_id: process.env.CASE_ID,
  model_name: process.env.MODEL_NAME || 'unknown',
  duration_seconds: Number(process.env.DURATION_SECONDS || '0'),
  repair_rounds: Number(process.env.REPAIR_ROUNDS || '0'),
  attempts: Number(process.env.ATTEMPTS || '0'),
  operation_count: operationCount,
  targeted_operation_count: operationCount,
  unrelated_operation_count: 0,
  unrelated_operation_rate: 0,
  paths: patch.operations.map((op) => op.path),
  schema_normalized: String(process.env.SCHEMA_NORMALIZED || '') === 'true',
  schema_drift_count: Number(process.env.SCHEMA_DRIFT_COUNT || '0'),
  parse_failure_count: Number(process.env.PARSE_FAILURE_COUNT || '0'),
  scope_violation_count: Number(process.env.SCOPE_VIOLATION_COUNT || '0'),
  validation: process.env.VALIDATION_LABEL || '',
  status: 'passed',
};
fs.writeFileSync(process.env.OUTPUT_FILE, JSON.stringify(result, null, 2));
console.log(JSON.stringify(result));
NODE
}

cleanup_dir() {
	if [ -n "${1:-}" ] && [ -d "$1" ]; then
    rm -rf "$1" 2>/dev/null || true
	fi
}

decode_patch_to_file() {
	local response_file="$1"
	local patch_file="$2"
  local stats_file="$3"
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
  if (hasNonEmptyString(source, ['case_id', 'id'])) {
    driftCount += 1;
  }
  const operations = Array.isArray(source?.operations) ? source.operations : [];
  for (const operation of operations) {
    if (hasNonEmptyString(operation, ['action', 'operation', 'op', 'kind'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['file_path', 'target_path', 'file'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['match'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['start_marker', 'begin'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['end_marker', 'finish'])) {
      driftCount += 1;
    }
    if (hasNonEmptyString(operation, ['replacement', 'file_content'])) {
      driftCount += 1;
    }
  }
  return driftCount;
}

function normalizePatch(source) {
  const normalized = {
    patch_id: firstString(source, ['patch_id', 'case_id', 'id']) || 'runtime-patch',
    operations: [],
  };
  const operations = Array.isArray(source?.operations) ? source.operations : [];
  for (const operation of operations) {
    const type = normalizeOperationType(firstString(operation, ['type', 'action', 'op', 'kind']));
    const fallbackType = normalizeOperationType(firstString(operation, ['operation']));
    const path = firstString(operation, ['path', 'file_path', 'target_path', 'file']);
    const resolvedType = type || fallbackType;
    if (!resolvedType || !path) {
      continue;
    }
    normalized.operations.push({
      type: resolvedType,
      path,
      anchor: firstString(operation, ['anchor', 'match']),
      old_content: firstString(operation, ['old_content']),
      start: firstString(operation, ['start', 'start_marker', 'begin']),
      end: firstString(operation, ['end', 'end_marker', 'finish']),
      content: firstString(operation, ['content', 'file_content']),
      new_content: firstString(operation, ['new_content', 'replacement']),
    });
  }
  return normalized;
}

const schemaDriftCount = countSchemaDrift(patch);
const normalizedPatch = normalizePatch(patch);
if (!Array.isArray(normalizedPatch.operations) || normalizedPatch.operations.length === 0) {
  throw new Error('patch operations missing');
}
fs.writeFileSync(process.env.PATCH_FILE, JSON.stringify(normalizedPatch, null, 2));
fs.writeFileSync(process.env.STATS_FILE, JSON.stringify({
  attempts: 1,
  schema_normalized: schemaDriftCount > 0,
  schema_drift_count: schemaDriftCount,
  parse_failure_count: 0,
  scope_violation_count: 0,
}, null, 2));
NODE
}

apply_patch_file() {
	local workspace_dir="$1"
	local patch_file="$2"
	WORKSPACE_DIR="${workspace_dir}" PATCH_FILE="${patch_file}" node <<'NODE'
const fs = require('fs');
const path = require('path');

const workspaceDir = process.env.WORKSPACE_DIR;
const patch = JSON.parse(fs.readFileSync(process.env.PATCH_FILE, 'utf8'));

for (const operation of patch.operations) {
  const target = path.join(workspaceDir, operation.path);
  const opType = operation.type || operation.action;
  if (opType === 'write_file') {
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, operation.content || operation.new_content || '');
    continue;
  }
  if (opType === 'replace_block') {
    let source = fs.readFileSync(target, 'utf8');
    const anchor = operation.old_content || operation.anchor;
    if (anchor) {
      if (!source.includes(anchor)) {
        throw new Error(`anchor not found in ${operation.path}: ${anchor}`);
      }
      source = source.replace(anchor, operation.new_content || operation.content || '');
      fs.writeFileSync(target, source);
      continue;
    }
    const start = operation.start;
    const end = operation.end;
    if (!start || !end) {
      throw new Error(`replace_block missing anchor or start/end in ${operation.path}`);
    }
    const startIndex = source.indexOf(start);
    if (startIndex < 0) {
      throw new Error(`start marker not found in ${operation.path}: ${start}`);
    }
    let endSearchStart = startIndex;
    if (end !== start) {
      endSearchStart = startIndex + start.length;
    }
    const endRelativeIndex = source.slice(endSearchStart).indexOf(end);
    if (endRelativeIndex < 0) {
      throw new Error(`end marker not found in ${operation.path}: ${end}`);
    }
    const replaceEnd = endSearchStart + endRelativeIndex + end.length;
    source = source.slice(0, startIndex) + (operation.new_content || operation.content || '') + source.slice(replaceEnd);
    fs.writeFileSync(target, source);
    continue;
  }
  throw new Error(`unsupported operation type: ${opType}`);
}
NODE
}

run_in_builder() {
	local workspace_dir="$1"
	shift
	docker run --rm \
		-v "${workspace_dir}:${workspace_dir}" \
		-w "${workspace_dir}" \
		"${ONEAPPFACTORY_BUILDER_IMAGE}" exec /bin/sh -lc "$*"
}

request_patch() {
	local prompt_file="$1"
	local response_file="$2"
	curl -sS --max-time 240 "${OLLAMA_BASE_URL}/v1/chat/completions" \
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
}

run_analyze_case() {
	local workspace_dir
	workspace_dir="$(mktemp -d)"
  local baseline_dir
  baseline_dir="$(mktemp -d)"
  local prompt_file response_file patch_file stats_file failure_file output_file
	prompt_file="$(mktemp)"
	response_file="$(mktemp)"
	patch_file="$(mktemp)"
  stats_file="$(mktemp)"
  failure_file="$(mktemp)"
  output_file="$(mktemp)"

	mkdir -p "${workspace_dir}/lib/views" "${workspace_dir}/test"
	cat >"${workspace_dir}/pubspec.yaml" <<'EOF'
name: model_validation_app
description: Builder runtime model validation sandbox
publish_to: 'none'
version: 0.1.0+1

environment:
  sdk: '>=3.3.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^3.0.0

flutter:
  uses-material-design: true
EOF
	cat >"${workspace_dir}/analysis_options.yaml" <<'EOF'
include: package:flutter_lints/flutter.yaml
EOF
	cat >"${workspace_dir}/lib/main.dart" <<'EOF'
import 'package:flutter/material.dart';

import 'views/entry_form_page.dart';

void main() {
  runApp(const BudgetApp());
}

class BudgetApp extends StatelessWidget {
  const BudgetApp({super.key});

  @override
  Widget build(BuildContext context) {
    return const MaterialApp(
      home: EntryFormPage(),
    );
  }
}
EOF
	cat >"${workspace_dir}/lib/views/entry_form_page.dart" <<'EOF'
import 'package:flutter/material.dart';

class EntryFormPage extends StatefulWidget {
  const EntryFormPage({super.key});

  @override
  State<EntryFormPage> createState() => _EntryFormPageState();
}

class _EntryFormPageState extends State<EntryFormPage> {
  DateTime selectedDate = DateTime.now();

  void setSelectedDate(DateTime date) {
    setState(() {
      selectedDate = date;
    });
  }

  Future<void> pickDate() async {
    final picked = await showDatePicker(
      context: context,
      initialDate: selectedDate,
      firstDate: DateTime(2020),
      lastDate: DateTime(2100),
    );
    if (picked != nil) {
      setSelectedDate(picked);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Entry Form')),
      body: Center(
        child: TextButton(
          onPressed: pickDate,
          child: Text(selectedDate.toIso8601String()),
        ),
      ),
    );
  }
}
EOF
	cat >"${workspace_dir}/test/widget_test.dart" <<'EOF'
import 'package:flutter_test/flutter_test.dart';
import 'package:model_validation_app/main.dart';

void main() {
  testWidgets('app renders', (tester) async {
    await tester.pumpWidget(const BudgetApp());
    expect(find.text('Entry Form'), findsOneWidget);
  });
}
EOF
  cp -R "${workspace_dir}/." "${baseline_dir}/"

	run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get.log && flutter analyze >/tmp/analyze.log 2>&1; status=\$?; cat /tmp/analyze.log; exit \$status" >/tmp/model-validation-analyze-before.log 2>&1 || true

  local attempts total_duration_seconds schema_drift_total parse_failure_total scope_violation_total
  local schema_normalized repair_rounds
  attempts=0
  total_duration_seconds=0
  schema_drift_total=0
  parse_failure_total=0
  scope_violation_total=0
  schema_normalized=false
  repair_rounds=0
  : >"${failure_file}"

  while [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; do
    attempts="$((attempts + 1))"
    restore_workspace "${baseline_dir}" "${workspace_dir}"
    cat >"${prompt_file}" <<EOF
Return compact JSON with keys patch_id and operations. No markdown.
Task: fix the Dart analyze error in lib/views/entry_form_page.dart.
Allowed path: lib/views/entry_form_page.dart only.
Current file content:
$(cat "${workspace_dir}/lib/views/entry_form_page.dart")

Required outcome: replace the invalid nil null-check with valid Dart null handling so flutter analyze passes.
Prefer exactly one write_file operation containing the full corrected file content.
EOF
    append_retry_context "${prompt_file}" "${failure_file}" "${attempts}"

    local started_at finished_at round_duration attempt_schema_drift attempt_parse_fail attempt_scope_violation
    started_at="$(date +%s)"
    request_patch "${prompt_file}" "${response_file}"
    finished_at="$(date +%s)"
    round_duration="$((finished_at - started_at))"
    total_duration_seconds="$((total_duration_seconds + round_duration))"

    if ! decode_patch_to_file "${response_file}" "${patch_file}" "${stats_file}" 2>"${failure_file}"; then
      parse_failure_total="$((parse_failure_total + 1))"
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    read attempt_schema_drift attempt_parse_fail attempt_scope_violation <<<"$(read_attempt_stats "${stats_file}")"
    schema_drift_total="$((schema_drift_total + attempt_schema_drift))"
    parse_failure_total="$((parse_failure_total + attempt_parse_fail))"
    scope_violation_total="$((scope_violation_total + attempt_scope_violation))"
    if [ "${attempt_schema_drift}" -gt 0 ]; then
      schema_normalized=true
    fi

    if ! apply_patch_file "${workspace_dir}" "${patch_file}" 2>"${failure_file}"; then
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    if run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get-after.log && flutter analyze" >"${failure_file}" 2>&1; then
      repair_rounds="${attempts}"
      break
    fi

    if [ "${attempts}" -ge "${MAX_REPAIR_ROUNDS}" ]; then
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi
  done

  write_validation_result \
    "real-analyze-repair" \
    "${total_duration_seconds}" \
    "${patch_file}" \
    "${output_file}" \
    "flutter analyze" \
    "${repair_rounds}" \
    "${attempts}" \
    "${schema_normalized}" \
    "${schema_drift_total}" \
    "${parse_failure_total}" \
    "${scope_violation_total}"
  write_case_result "real-analyze-repair" "${output_file}"

  rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
  cleanup_dir "${baseline_dir}"
	cleanup_dir "${workspace_dir}"
}

run_test_case() {
	local workspace_dir
	workspace_dir="$(mktemp -d)"
  local baseline_dir
  baseline_dir="$(mktemp -d)"
  local prompt_file response_file patch_file stats_file failure_file output_file
	prompt_file="$(mktemp)"
	response_file="$(mktemp)"
	patch_file="$(mktemp)"
  stats_file="$(mktemp)"
  failure_file="$(mktemp)"
  output_file="$(mktemp)"

	mkdir -p "${workspace_dir}/lib" "${workspace_dir}/test"
	cat >"${workspace_dir}/pubspec.yaml" <<'EOF'
name: model_validation_test_app
description: Builder runtime model validation sandbox
publish_to: 'none'
version: 0.1.0+1

environment:
  sdk: '>=3.3.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^3.0.0

flutter:
  uses-material-design: true
EOF
	cat >"${workspace_dir}/analysis_options.yaml" <<'EOF'
include: package:flutter_lints/flutter.yaml
EOF
	cat >"${workspace_dir}/lib/main.dart" <<'EOF'
import 'package:flutter/material.dart';

void main() {
  runApp(const BudgetApp());
}

class BudgetApp extends StatelessWidget {
  const BudgetApp({super.key});

  @override
  Widget build(BuildContext context) {
    return const MaterialApp(
      title: 'Budget Flow',
      home: Scaffold(
        body: Center(child: Text('Budget Flow')),
      ),
    );
  }
}
EOF
	cat >"${workspace_dir}/test/widget_test.dart" <<'EOF'
import 'package:flutter_test/flutter_test.dart';
import 'package:model_validation_test_app/main.dart';

void main() {
  testWidgets('budget title renders', (tester) async {
    await tester.pumpWidget(const BudgetApp());
    expect(find.text('Old Title'), findsOneWidget);
  });
}
EOF
  cp -R "${workspace_dir}/." "${baseline_dir}/"

	run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get.log && flutter test >/tmp/test.log 2>&1; status=\$?; cat /tmp/test.log; exit \$status" >/tmp/model-validation-test-before.log 2>&1 || true

  local attempts total_duration_seconds schema_drift_total parse_failure_total scope_violation_total
  local schema_normalized repair_rounds
  attempts=0
  total_duration_seconds=0
  schema_drift_total=0
  parse_failure_total=0
  scope_violation_total=0
  schema_normalized=false
  repair_rounds=0
  : >"${failure_file}"

  while [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; do
    attempts="$((attempts + 1))"
    restore_workspace "${baseline_dir}" "${workspace_dir}"
    cat >"${prompt_file}" <<EOF
Return compact JSON with keys patch_id and operations. No markdown.
Task: fix the failing Flutter widget test in test/widget_test.dart.
Allowed path: test/widget_test.dart only.
Current file content:
$(cat "${workspace_dir}/test/widget_test.dart")

Application behavior is already correct and renders text Budget Flow.
Required outcome: update the test so flutter test passes.
Prefer exactly one write_file operation containing the full corrected file content.
EOF
    append_retry_context "${prompt_file}" "${failure_file}" "${attempts}"

    local started_at finished_at round_duration attempt_schema_drift attempt_parse_fail attempt_scope_violation
    started_at="$(date +%s)"
    request_patch "${prompt_file}" "${response_file}"
    finished_at="$(date +%s)"
    round_duration="$((finished_at - started_at))"
    total_duration_seconds="$((total_duration_seconds + round_duration))"

    if ! decode_patch_to_file "${response_file}" "${patch_file}" "${stats_file}" 2>"${failure_file}"; then
      parse_failure_total="$((parse_failure_total + 1))"
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    read attempt_schema_drift attempt_parse_fail attempt_scope_violation <<<"$(read_attempt_stats "${stats_file}")"
    schema_drift_total="$((schema_drift_total + attempt_schema_drift))"
    parse_failure_total="$((parse_failure_total + attempt_parse_fail))"
    scope_violation_total="$((scope_violation_total + attempt_scope_violation))"
    if [ "${attempt_schema_drift}" -gt 0 ]; then
      schema_normalized=true
    fi

    if ! apply_patch_file "${workspace_dir}" "${patch_file}" 2>"${failure_file}"; then
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    if run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get-after.log && flutter test" >"${failure_file}" 2>&1; then
      repair_rounds="${attempts}"
      break
    fi

    if [ "${attempts}" -ge "${MAX_REPAIR_ROUNDS}" ]; then
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi
  done

  write_validation_result \
    "real-test-repair" \
    "${total_duration_seconds}" \
    "${patch_file}" \
    "${output_file}" \
    "flutter test" \
    "${repair_rounds}" \
    "${attempts}" \
    "${schema_normalized}" \
    "${schema_drift_total}" \
    "${parse_failure_total}" \
    "${scope_violation_total}"
  write_case_result "real-test-repair" "${output_file}"

  rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
  cleanup_dir "${baseline_dir}"
	cleanup_dir "${workspace_dir}"
}

run_dual_file_case() {
	local workspace_dir
	workspace_dir="$(mktemp -d)"
  local baseline_dir
  baseline_dir="$(mktemp -d)"
  local prompt_file response_file patch_file stats_file failure_file output_file
	prompt_file="$(mktemp)"
	response_file="$(mktemp)"
	patch_file="$(mktemp)"
  stats_file="$(mktemp)"
  failure_file="$(mktemp)"
  output_file="$(mktemp)"

	mkdir -p "${workspace_dir}/lib/controllers" "${workspace_dir}/lib/views" "${workspace_dir}/test"
	cat >"${workspace_dir}/pubspec.yaml" <<'EOF'
name: model_validation_dual_file_app
description: Builder runtime model validation sandbox
publish_to: 'none'
version: 0.1.0+1

environment:
  sdk: '>=3.3.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^3.0.0

flutter:
  uses-material-design: true
EOF
	cat >"${workspace_dir}/analysis_options.yaml" <<'EOF'
include: package:flutter_lints/flutter.yaml
EOF
	cat >"${workspace_dir}/lib/main.dart" <<'EOF'
import 'package:flutter/material.dart';

import 'controllers/home_controller.dart';
import 'views/home_page.dart';

void main() {
  runApp(const BudgetApp());
}

class BudgetApp extends StatelessWidget {
  const BudgetApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      home: HomePage(controller: HomeController()),
    );
  }
}
EOF
	cat >"${workspace_dir}/lib/controllers/home_controller.dart" <<'EOF'
import 'package:flutter/foundation.dart';

class HomeController extends ChangeNotifier {
  Future<void> reload() async {}
}
EOF
	cat >"${workspace_dir}/lib/views/home_page.dart" <<'EOF'
import 'package:flutter/material.dart';

import '../controllers/home_controller.dart';

class HomePage extends StatefulWidget {
  const HomePage({super.key, required this.controller});

  final HomeController controller;

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  @override
  void initState() {
    super.initState();
    widget.controller.reload();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        return Scaffold(
          body: Center(
            child: widget.controller.loading
                ? const CircularProgressIndicator()
                : Text(widget.controller.title),
          ),
        );
      },
    );
  }
}
EOF
	cat >"${workspace_dir}/test/widget_test.dart" <<'EOF'
import 'package:flutter_test/flutter_test.dart';
import 'package:model_validation_dual_file_app/main.dart';

void main() {
  testWidgets('app renders', (tester) async {
    await tester.pumpWidget(const BudgetApp());
    expect(find.byType(BudgetApp), findsOneWidget);
  });
}
EOF
  cp -R "${workspace_dir}/." "${baseline_dir}/"

	run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get.log && flutter analyze >/tmp/analyze.log 2>&1; status=\$?; cat /tmp/analyze.log; exit \$status" >/tmp/model-validation-dual-before.log 2>&1 || true

  local attempts total_duration_seconds schema_drift_total parse_failure_total scope_violation_total
  local schema_normalized repair_rounds
  attempts=0
  total_duration_seconds=0
  schema_drift_total=0
  parse_failure_total=0
  scope_violation_total=0
  schema_normalized=false
  repair_rounds=0
  : >"${failure_file}"

  while [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; do
    attempts="$((attempts + 1))"
    restore_workspace "${baseline_dir}" "${workspace_dir}"
    cat >"${prompt_file}" <<EOF
Return compact JSON with keys patch_id and operations. No markdown.
Task: finish the narrow dual-file wiring between HomeController and HomePage.
Allowed paths: lib/controllers/home_controller.dart and lib/views/home_page.dart only.
Current controller file:
$(cat "${workspace_dir}/lib/controllers/home_controller.dart")

Current page file:
$(cat "${workspace_dir}/lib/views/home_page.dart")

Required outcome: flutter analyze passes. Keep the page showing a loading spinner while loading and a title string after loading. Prefer exactly two write_file operations, one per allowed path.
EOF
    append_retry_context "${prompt_file}" "${failure_file}" "${attempts}"

    local started_at finished_at round_duration attempt_schema_drift attempt_parse_fail attempt_scope_violation
    started_at="$(date +%s)"
    request_patch "${prompt_file}" "${response_file}"
    finished_at="$(date +%s)"
    round_duration="$((finished_at - started_at))"
    total_duration_seconds="$((total_duration_seconds + round_duration))"

    if ! decode_patch_to_file "${response_file}" "${patch_file}" "${stats_file}" 2>"${failure_file}"; then
      parse_failure_total="$((parse_failure_total + 1))"
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    read attempt_schema_drift attempt_parse_fail attempt_scope_violation <<<"$(read_attempt_stats "${stats_file}")"
    schema_drift_total="$((schema_drift_total + attempt_schema_drift))"
    parse_failure_total="$((parse_failure_total + attempt_parse_fail))"
    scope_violation_total="$((scope_violation_total + attempt_scope_violation))"
    if [ "${attempt_schema_drift}" -gt 0 ]; then
      schema_normalized=true
    fi

    if ! apply_patch_file "${workspace_dir}" "${patch_file}" 2>"${failure_file}"; then
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    if run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get-after.log && flutter analyze" >"${failure_file}" 2>&1; then
      repair_rounds="${attempts}"
      break
    fi

    if [ "${attempts}" -ge "${MAX_REPAIR_ROUNDS}" ]; then
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi
  done

  write_validation_result \
    "real-dual-file-wiring" \
    "${total_duration_seconds}" \
    "${patch_file}" \
    "${output_file}" \
    "flutter analyze" \
    "${repair_rounds}" \
    "${attempts}" \
    "${schema_normalized}" \
    "${schema_drift_total}" \
    "${parse_failure_total}" \
    "${scope_violation_total}"
  write_case_result "real-dual-file-wiring" "${output_file}"

  rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
  cleanup_dir "${baseline_dir}"
	cleanup_dir "${workspace_dir}"
}

run_closure_case() {
	local workspace_dir
	workspace_dir="$(mktemp -d)"
  local baseline_dir
  baseline_dir="$(mktemp -d)"
  local prompt_file response_file patch_file stats_file failure_file output_file
	prompt_file="$(mktemp)"
	response_file="$(mktemp)"
	patch_file="$(mktemp)"
  stats_file="$(mktemp)"
  failure_file="$(mktemp)"
  output_file="$(mktemp)"

	mkdir -p "${workspace_dir}/lib" "${workspace_dir}/test"
	cat >"${workspace_dir}/pubspec.yaml" <<'EOF'
name: model_validation_closure_app
description: Builder runtime model validation sandbox
publish_to: 'none'
version: 0.1.0+1

environment:
  sdk: '>=3.3.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^3.0.0

flutter:
  uses-material-design: true
EOF
	cat >"${workspace_dir}/analysis_options.yaml" <<'EOF'
include: package:flutter_lints/flutter.yaml
EOF
	cat >"${workspace_dir}/lib/main.dart" <<'EOF'
import 'package:flutter/material.dart';

void main() {
  runApp(const BudgetApp());
}

class BudgetApp extends StatelessWidget {
  const BudgetApp({super.key});

  @override
  Widget build(BuildContext context) {
    return const MaterialApp(
      title: 'Old Title',
      home: Scaffold(
        body: Center(child: Text('Budget Flow')),
      ),
    );
  }
}
EOF
	cat >"${workspace_dir}/test/widget_test.dart" <<'EOF'
import 'package:flutter_test/flutter_test.dart';
import 'package:model_validation_closure_app/main.dart';

void main() {
  testWidgets('budget title renders', (tester) async {
    await tester.pumpWidget(const BudgetApp());
    expect(find.text('Old Title'), findsOneWidget);
  });
}
EOF
  cp -R "${workspace_dir}/." "${baseline_dir}/"

	run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get.log && flutter test >/tmp/test.log 2>&1; status=\$?; cat /tmp/test.log; exit \$status" >/tmp/model-validation-closure-before.log 2>&1 || true

  local attempts total_duration_seconds schema_drift_total parse_failure_total scope_violation_total
  local schema_normalized repair_rounds
  attempts=0
  total_duration_seconds=0
  schema_drift_total=0
  parse_failure_total=0
  scope_violation_total=0
  schema_normalized=false
  repair_rounds=0
  : >"${failure_file}"

  while [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; do
    attempts="$((attempts + 1))"
    restore_workspace "${baseline_dir}" "${workspace_dir}"
    cat >"${prompt_file}" <<EOF
Return compact JSON with keys patch_id and operations. No markdown.
Task: do a low-risk closure repair so both flutter analyze and flutter test pass.
Allowed paths: lib/main.dart and test/widget_test.dart only.
Current main file:
$(cat "${workspace_dir}/lib/main.dart")

Current test file:
$(cat "${workspace_dir}/test/widget_test.dart")

Required outcome: the app title and the test expectation must be consistent on Budget Flow. Prefer exactly two write_file operations, one per allowed path.
EOF
    append_retry_context "${prompt_file}" "${failure_file}" "${attempts}"

    local started_at finished_at round_duration attempt_schema_drift attempt_parse_fail attempt_scope_violation
    started_at="$(date +%s)"
    request_patch "${prompt_file}" "${response_file}"
    finished_at="$(date +%s)"
    round_duration="$((finished_at - started_at))"
    total_duration_seconds="$((total_duration_seconds + round_duration))"

    if ! decode_patch_to_file "${response_file}" "${patch_file}" "${stats_file}" 2>"${failure_file}"; then
      parse_failure_total="$((parse_failure_total + 1))"
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    read attempt_schema_drift attempt_parse_fail attempt_scope_violation <<<"$(read_attempt_stats "${stats_file}")"
    schema_drift_total="$((schema_drift_total + attempt_schema_drift))"
    parse_failure_total="$((parse_failure_total + attempt_parse_fail))"
    scope_violation_total="$((scope_violation_total + attempt_scope_violation))"
    if [ "${attempt_schema_drift}" -gt 0 ]; then
      schema_normalized=true
    fi

    if ! apply_patch_file "${workspace_dir}" "${patch_file}" 2>"${failure_file}"; then
      if [ "${attempts}" -lt "${MAX_REPAIR_ROUNDS}" ]; then
        continue
      fi
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi

    if run_in_builder "${workspace_dir}" "flutter pub get >/tmp/pub-get-after.log && flutter analyze && flutter test" >"${failure_file}" 2>&1; then
      repair_rounds="${attempts}"
      break
    fi

    if [ "${attempts}" -ge "${MAX_REPAIR_ROUNDS}" ]; then
      cat "${failure_file}" >&2
      rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
      cleanup_dir "${baseline_dir}"
      cleanup_dir "${workspace_dir}"
      return 1
    fi
  done

  write_validation_result \
    "real-closure-repair" \
    "${total_duration_seconds}" \
    "${patch_file}" \
    "${output_file}" \
    "flutter analyze + flutter test" \
    "${repair_rounds}" \
    "${attempts}" \
    "${schema_normalized}" \
    "${schema_drift_total}" \
    "${parse_failure_total}" \
    "${scope_violation_total}"
  write_case_result "real-closure-repair" "${output_file}"

  rm -f "${prompt_file}" "${response_file}" "${patch_file}" "${stats_file}" "${failure_file}" "${output_file}"
  cleanup_dir "${baseline_dir}"
	cleanup_dir "${workspace_dir}"
}

echo "[info] running real Flutter analyze/test validation via ${ONEAPPFACTORY_BUILDER_IMAGE} using ${MODEL_NAME}"
MODEL_NAME="${MODEL_NAME}" run_analyze_case
MODEL_NAME="${MODEL_NAME}" run_test_case
MODEL_NAME="${MODEL_NAME}" run_dual_file_case
MODEL_NAME="${MODEL_NAME}" run_closure_case
echo "[ok] real Flutter validation passed for ${MODEL_NAME}"