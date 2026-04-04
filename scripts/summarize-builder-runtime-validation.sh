#!/usr/bin/env bash

set -euo pipefail

OUTPUT_DIR="${OUTPUT_DIR:-.runtime/builder-model-validation}"
SUMMARY_FILE="${SUMMARY_FILE:-${OUTPUT_DIR}/builder-runtime-validation-summary.latest.json}"

mkdir -p "${OUTPUT_DIR}"

OUTPUT_DIR="${OUTPUT_DIR}" SUMMARY_FILE="${SUMMARY_FILE}" node <<'NODE'
const fs = require('fs');
const path = require('path');

const outputDir = process.env.OUTPUT_DIR;
const summaryFile = process.env.SUMMARY_FILE;

function safeReadDir(dirPath) {
  try {
    return fs.readdirSync(dirPath);
  } catch {
    return [];
  }
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function median(values) {
  if (values.length === 0) {
    return null;
  }
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  if (sorted.length % 2 === 1) {
    return sorted[middle];
  }
  return Number(((sorted[middle - 1] + sorted[middle]) / 2).toFixed(2));
}

function percentile(values, ratio) {
  if (values.length === 0) {
    return null;
  }
  const sorted = [...values].sort((left, right) => left - right);
  const index = Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * ratio) - 1));
  return sorted[index];
}

function summarizeDurations(values) {
  if (values.length === 0) {
    return {
      count: 0,
      min_seconds: null,
      max_seconds: null,
      avg_seconds: null,
      median_seconds: null,
      p95_seconds: null,
    };
  }
  const total = values.reduce((sum, value) => sum + value, 0);
  return {
    count: values.length,
    min_seconds: Math.min(...values),
    max_seconds: Math.max(...values),
    avg_seconds: Number((total / values.length).toFixed(2)),
    median_seconds: median(values),
    p95_seconds: percentile(values, 0.95),
  };
}

function passState(value) {
  return ['passed', 'applied', 'ok'].includes(String(value || '').toLowerCase());
}

function sanitizeModelName(value) {
  return String(value || 'unknown').replace(/[/:]/g, '_');
}

function summarizeRecords(records) {
  const durations = records
    .map((record) => Number(record.duration_seconds))
    .filter((value) => Number.isFinite(value));
  const attempts = records
    .map((record) => Number(record.attempts || 0))
    .filter((value) => Number.isFinite(value) && value > 0);
  const repairRounds = records
    .map((record) => Number(record.repair_rounds || 0))
    .filter((value) => Number.isFinite(value) && value > 0);
  const unrelatedRates = records
    .map((record) => Number(record.unrelated_operation_rate))
    .filter((value) => Number.isFinite(value));
  const passed = records.filter((record) => {
    if (typeof record.status === 'undefined' || record.status === null || record.status === '') {
      return true;
    }
    return passState(record.status);
  }).length;
  const schemaDriftCases = records.filter((record) => Number(record.schema_drift_count || 0) > 0).length;
  const parseFailureTotal = records.reduce((sum, record) => sum + Number(record.parse_failure_count || 0), 0);
  const scopeViolationTotal = records.reduce((sum, record) => sum + Number(record.scope_violation_count || 0), 0);
  return {
    total_cases: records.length,
    passed_cases: passed,
    pass_rate: records.length === 0 ? null : Number((passed / records.length).toFixed(4)),
    avg_attempts: attempts.length === 0 ? null : Number((attempts.reduce((sum, value) => sum + value, 0) / attempts.length).toFixed(2)),
    avg_repair_rounds: repairRounds.length === 0 ? null : Number((repairRounds.reduce((sum, value) => sum + value, 0) / repairRounds.length).toFixed(2)),
    avg_unrelated_operation_rate: unrelatedRates.length === 0 ? null : Number((unrelatedRates.reduce((sum, value) => sum + value, 0) / unrelatedRates.length).toFixed(4)),
    schema_drift_cases: schemaDriftCases,
    parse_failure_total: parseFailureTotal,
    scope_violation_total: scopeViolationTotal,
    durations: summarizeDurations(durations),
  };
}

function summarizeLeafSamples() {
  const grouped = new Map();
  for (const entry of safeReadDir(outputDir)) {
    const match = entry.match(/^(\d{8}-\d{6})-(.+)\.jsonl$/);
    if (!match) {
      continue;
    }
    const [, timestamp, modelKey] = match;
    const current = grouped.get(modelKey);
    if (!current || timestamp > current.timestamp) {
      grouped.set(modelKey, {
        timestamp,
        file_name: entry,
        file_path: path.join(outputDir, entry),
      });
    }
  }

  const byModel = {};
  for (const [modelKey, metadata] of grouped.entries()) {
    const lines = fs
      .readFileSync(metadata.file_path, 'utf8')
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean);
    const records = lines.map((line) => JSON.parse(line));
    const sampleSummary = summarizeRecords(records);
    byModel[modelKey] = {
      latest_file: metadata.file_name,
      sample_count: records.length,
      passed_cases: sampleSummary.passed_cases,
      pass_rate: sampleSummary.pass_rate,
      avg_attempts: sampleSummary.avg_attempts,
      avg_repair_rounds: sampleSummary.avg_repair_rounds,
      avg_unrelated_operation_rate: sampleSummary.avg_unrelated_operation_rate,
      durations: sampleSummary.durations,
      schema_drift_cases: sampleSummary.schema_drift_cases,
      parse_failure_total: sampleSummary.parse_failure_total,
      scope_violation_total: sampleSummary.scope_violation_total,
    };
  }

  return byModel;
}

function collectArchivedCaseResults(prefix) {
  const grouped = new Map();
  for (const entry of safeReadDir(outputDir)) {
    if (!entry.startsWith(`${prefix}.`) || !entry.endsWith('.json') || entry.includes('.latest.')) {
      continue;
    }
    const suffix = entry.slice(prefix.length + 1, -'.json'.length);
    if (!suffix || suffix === 'latest') {
      continue;
    }
    const record = readJson(path.join(outputDir, entry));
    const modelKey = suffix;
    if (!grouped.has(modelKey)) {
      grouped.set(modelKey, []);
    }
    grouped.get(modelKey).push(record);
  }
  if (grouped.size === 0) {
    const latestPath = path.join(outputDir, `${prefix}.latest.json`);
    if (fs.existsSync(latestPath)) {
      const record = readJson(latestPath);
      const modelKey = sanitizeModelName(record.model_name);
      grouped.set(modelKey, [record]);
    }
  }
  return grouped;
}

function summarizeNamedCases(caseIds) {
  const byModel = {};
  for (const caseId of caseIds) {
    const grouped = collectArchivedCaseResults(caseId);
    for (const [modelKey, records] of grouped.entries()) {
      if (!byModel[modelKey]) {
        byModel[modelKey] = { cases: {} };
      }
      const summary = summarizeRecords(records);
      byModel[modelKey].cases[caseId] = {
        runs: records.length,
        source: records.length > 1 || fs.existsSync(path.join(outputDir, `${caseId}.${modelKey}.json`)) ? 'archived' : 'latest_only',
        latest_status: records[records.length - 1]?.status || null,
        latest_duration_seconds: Number(records[records.length - 1]?.duration_seconds ?? NaN),
        avg_attempts: summary.avg_attempts,
        avg_repair_rounds: summary.avg_repair_rounds,
        avg_unrelated_operation_rate: summary.avg_unrelated_operation_rate,
        schema_drift_cases: summary.schema_drift_cases,
        parse_failure_total: summary.parse_failure_total,
        scope_violation_total: summary.scope_violation_total,
        pass_rate: summary.pass_rate,
        durations: summary.durations,
      };
    }
  }

  for (const modelKey of Object.keys(byModel)) {
    const records = caseIds.flatMap((caseId) => collectArchivedCaseResults(caseId).get(modelKey) || []);
    byModel[modelKey].overall = summarizeRecords(records);
  }

  return byModel;
}

const realCaseIds = [
  'real-analyze-repair',
  'real-test-repair',
  'real-dual-file-wiring',
  'real-closure-repair',
];

const summary = {
  generated_at: new Date().toISOString(),
  output_dir: outputDir,
  notes: [
    'repair_rounds_available: 当前本地验证脚本已经落盘 repair rounds，汇总会输出平均 repair rounds。',
    'real_and_sandbox_use_archived_per_model_results: real/sandbox 统计依赖 *.model.json 归档文件。',
  ],
  leaf_samples: summarizeLeafSamples(),
  real_validation: summarizeNamedCases(realCaseIds),
  sandbox_validation: summarizeNamedCases(['sandbox-single-file-apply']),
};

fs.writeFileSync(summaryFile, JSON.stringify(summary, null, 2));

function printSection(title) {
  console.log(`\n## ${title}`);
}

function printModelSummary(source, data) {
  const models = Object.keys(data).sort();
  if (models.length === 0) {
    console.log('- no data');
    return;
  }
  for (const modelKey of models) {
    const item = data[modelKey];
    if (source === 'leaf') {
      console.log(`- ${modelKey}: ${item.passed_cases}/${item.sample_count} passed, avg ${item.durations.avg_seconds ?? 'n/a'}s, p95 ${item.durations.p95_seconds ?? 'n/a'}s, avg attempts ${item.avg_attempts ?? 'n/a'}, avg repair rounds ${item.avg_repair_rounds ?? 'n/a'}, avg unrelated rate ${item.avg_unrelated_operation_rate ?? 'n/a'}, schema drift cases ${item.schema_drift_cases}, parse failures ${item.parse_failure_total}, scope violations ${item.scope_violation_total}, latest ${item.latest_file}`);
      continue;
    }
    const overall = item.overall;
    console.log(`- ${modelKey}: ${overall.passed_cases}/${overall.total_cases} passed, avg ${overall.durations.avg_seconds ?? 'n/a'}s, p95 ${overall.durations.p95_seconds ?? 'n/a'}s, avg attempts ${overall.avg_attempts ?? 'n/a'}, avg repair rounds ${overall.avg_repair_rounds ?? 'n/a'}, avg unrelated rate ${overall.avg_unrelated_operation_rate ?? 'n/a'}, schema drift cases ${overall.schema_drift_cases}, parse failures ${overall.parse_failure_total}, scope violations ${overall.scope_violation_total}`);
    const caseIds = Object.keys(item.cases).sort();
    for (const caseId of caseIds) {
      const caseSummary = item.cases[caseId];
      const latestDuration = Number.isFinite(caseSummary.latest_duration_seconds) ? caseSummary.latest_duration_seconds : 'n/a';
      console.log(`  - ${caseId}: latest ${caseSummary.latest_status || 'unknown'} in ${latestDuration}s, runs ${caseSummary.runs}, pass rate ${caseSummary.pass_rate ?? 'n/a'}, avg attempts ${caseSummary.avg_attempts ?? 'n/a'}, avg repair rounds ${caseSummary.avg_repair_rounds ?? 'n/a'}, schema drift cases ${caseSummary.schema_drift_cases}`);
    }
  }
}

console.log('# builder-runtime validation summary');
console.log(`- summary_file: ${summaryFile}`);
console.log('- missing_metrics: none');
printSection('Leaf samples');
printModelSummary('leaf', summary.leaf_samples);
printSection('Real validation');
printModelSummary('real', summary.real_validation);
printSection('Sandbox validation');
printModelSummary('sandbox', summary.sandbox_validation);
NODE