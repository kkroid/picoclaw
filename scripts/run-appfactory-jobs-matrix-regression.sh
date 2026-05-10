#!/usr/bin/env bash
# AppFactory Generic Jobs Matrix Regression
#
# 将多个 generic fixture 依次跑通 /jobs 全链，汇总 matrix 结果。
# 每个 fixture 调用 run-appfactory-jobs-regression.sh，使用独立输出目录避免覆盖。
#
# 用法：
#   scripts/run-appfactory-jobs-matrix-regression.sh [--api-base http://127.0.0.1:18800]
#
# 环境变量：
#   APPFACTORY_JOBS_MATRIX_TIMEOUT_SECONDS  每个 fixture 超时秒数（默认 5400）
#   APPFACTORY_JOBS_MATRIX_FIXTURES         逗号分隔 fixture 名（默认全部）
#   APPFACTORY_JOBS_MATRIX_ONLY             只跑指定 topology（逗号分隔）

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
matrix_started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

api_base="${APPFACTORY_JOBS_API_BASE:-${APPFACTORY_PRODUCT_FLOW_API_BASE:-http://127.0.0.1:18800}}"
output_root="${APPFACTORY_JOBS_MATRIX_OUTPUT_ROOT:-$repo_root/workspace/appfactory/jobs-ui-regression}"
matrix_output_dir="$output_root/matrix"
fixture_timeout="${APPFACTORY_JOBS_MATRIX_TIMEOUT_SECONDS:-5400}"
selected_fixtures="${APPFACTORY_JOBS_MATRIX_FIXTURES:-}"
topology_filter="${APPFACTORY_JOBS_MATRIX_ONLY:-}"

# fixture 矩阵：name|topology|requirement_file
declare -a fixture_defs=(
  "weight-tracker|classic-list-detail-form|examples/appfactory/generic/weight-tracker/requirement.md"
  "todo-lite|classic-list-detail-form|examples/appfactory/generic/todo-lite/requirement.md"
  "habit-checkin|classic-list-detail-form|examples/appfactory/generic/habit-checkin/requirement.md"
  "todo-lite-no-home-no-detail|no-home-no-detail|examples/appfactory/generic/todo-lite-no-home-no-detail/requirement.md"
)

usage() {
  cat <<'EOF'
Usage:
  scripts/run-appfactory-jobs-matrix-regression.sh [--api-base URL]

Environment:
  APPFACTORY_JOBS_MATRIX_FIXTURES   comma-separated fixture names
  APPFACTORY_JOBS_MATRIX_ONLY       comma-separated topology names
  APPFACTORY_JOBS_MATRIX_TIMEOUT_SECONDS  per-fixture timeout (default 5400)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-base) api_base="$2"; shift 2 ;;
    --output-root) output_root="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 1 ;;
  esac
done

run_dir="$matrix_output_dir/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$run_dir"

console_log="$run_dir/console.log"
exec > >(tee -a "$console_log") 2>&1

echo "=== AppFactory Generic Jobs Matrix Regression ==="
echo "started_at=$matrix_started_at"
echo "api_base=$api_base"
echo "output_dir=$run_dir"

# 筛选 fixture
declare -a fixtures=()
for def in "${fixture_defs[@]}"; do
  IFS='|' read -r name topology req <<< "$def"
  if [[ -n "$selected_fixtures" ]]; then
    found=false
    IFS=',' read -ra wanted <<< "$selected_fixtures"
    for w in "${wanted[@]}"; do
      [[ "$name" == "$(echo "$w" | xargs)" ]] && found=true && break
    done
    [[ "$found" != "true" ]] && continue
  fi
  if [[ -n "$topology_filter" ]]; then
    found=false
    IFS=',' read -ra wanted <<< "$topology_filter"
    for w in "${wanted[@]}"; do
      [[ "$topology" == "$(echo "$w" | xargs)" ]] && found=true && break
    done
    [[ "$found" != "true" ]] && continue
  fi
  fixtures+=("$def")
done

total=${#fixtures[@]}
passed=0
failed=0
declare -a results=()

echo "fixtures_selected=$total"

# 公共 preflight
echo ""
echo "=== Matrix preflight ==="
probe_status="$(curl -sS -o /dev/null -w '%{http_code}' --connect-timeout 3 --max-time 30 "$api_base/api/v1/notifications" 2>/dev/null || echo "000")"
if [[ "$probe_status" != 2* ]]; then
  echo "FATAL: API unreachable (status=$probe_status)"
  exit 1
fi
echo "api_reachable=true"

# 逐 fixture 运行
index=0
for def in "${fixtures[@]}"; do
  index=$((index + 1))
  IFS='|' read -r name topology req_file <<< "$def"

  echo ""
  echo "=== [$index/$total] $name ($topology) ==="
  started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  fixture_output_root="$run_dir/$name"
  mkdir -p "$fixture_output_root"
  fixture_job_id="job-matrix-${name}-$(date -u +%Y%m%dT%H%M%SZ)"
  fixture_prd_id="prd-matrix-${name}-$(date -u +%Y%m%dT%H%M%SZ)"

  reg_script="$repo_root/scripts/run-appfactory-jobs-regression.sh"
  [[ -x "$reg_script" ]] || chmod +x "$reg_script" 2>/dev/null || true

  # 每个 fixture 使用独立输出目录，避免 latest.md 被覆盖
  set +e
  APPFACTORY_JOBS_API_BASE="$api_base" \
  APPFACTORY_JOBS_REQUIREMENT_FILE="$repo_root/$req_file" \
  APPFACTORY_JOBS_TITLE_HINT="/jobs matrix $name" \
  APPFACTORY_JOBS_JOB_ID="$fixture_job_id" \
  APPFACTORY_JOBS_PRD_ID="$fixture_prd_id" \
  APPFACTORY_JOBS_GOAL_SUMMARY="generic fixture $name ($topology)" \
  APPFACTORY_JOBS_TIMEOUT_SECONDS="$fixture_timeout" \
  APPFACTORY_JOBS_REGRESSION_ROOT="$fixture_output_root" \
    bash "$reg_script" > "$fixture_output_root/sub-console.log" 2>&1
  exit_code=$?
  set -e

  finished="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  # 从子脚本 latest.md 提取状态
  job_status="unknown"
  manual_eq=""
  probe_only=""
  failure_sig=""
  failure_cat=""

  if [[ -f "$fixture_output_root/latest.md" ]]; then
    latest_md="$(cat "$fixture_output_root/latest.md")"
    job_status="$(echo "$latest_md" | grep -oP 'terminal_job_status=\K\S+' 2>/dev/null || echo "unknown")"
    manual_eq="$(echo "$latest_md" | grep -oP 'manual_equivalence=\K\S+' 2>/dev/null || echo "")"
    probe_only="$(echo "$latest_md" | grep -oP 'probe_only=\K\S+' 2>/dev/null || echo "")"
    failure_sig="$(echo "$latest_md" | grep -oP 'failure_signature=\K\S+' 2>/dev/null || echo "")"
    failure_cat="$(echo "$latest_md" | grep -oP 'failure_category=\K\S+' 2>/dev/null || echo "")"
  fi

  fixture_passed="false"
  if [[ "$exit_code" -eq 0 ]] && [[ "$job_status" == "completed" ]]; then
    if [[ "$manual_eq" == "true" ]]; then
      fixture_passed="true"
      passed=$((passed + 1))
    else
      failed=$((failed + 1))
    fi
  else
    failed=$((failed + 1))
  fi

  results+=("{\"fixture\":\"$name\",\"topology\":\"$topology\",\"job_id\":\"$fixture_job_id\",\"job_status\":\"$job_status\",\"manual_equivalence\":\"$manual_eq\",\"probe_only\":\"$probe_only\",\"passed\":$fixture_passed,\"failure_signature\":\"$failure_sig\",\"failure_category\":\"$failure_cat\",\"exit_code\":$exit_code,\"started_at\":\"$started\",\"finished_at\":\"$finished\"}")

  echo "  job_status=$job_status manual_eq=$manual_eq probe_only=$probe_only passed=$fixture_passed"
done

# ── 汇总 ──────────────────────────────────────────────────────────────────────
matrix_finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# matrix.json
{
  echo "{"
  echo "  \"started_at\": \"$matrix_started_at\","
  echo "  \"finished_at\": \"$matrix_finished_at\","
  echo "  \"total\": $total,"
  echo "  \"passed\": $passed,"
  echo "  \"failed\": $failed,"
  echo "  \"results\": ["
  first=true
  for r in "${results[@]}"; do
    [[ "$first" == "true" ]] || echo "    ,"
    echo -n "    $r"
    first=false
  done
  echo ""
  echo "  ]"
  echo "}"
} > "$run_dir/matrix.json"

# matrix.md
{
  echo "# AppFactory Generic Jobs Matrix Regression"
  echo ""
  echo "| # | Fixture | Topology | Job Status | Manual Equiv | Passed | Failure |"
  echo "|---|---|---|---|---|---|---|"
  idx=0
  for r in "${results[@]}"; do
    idx=$((idx + 1))
    name="$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['fixture'])" 2>/dev/null || echo "?")"
    topo="$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['topology'])" 2>/dev/null || echo "?")"
    js="$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['job_status'])" 2>/dev/null || echo "?")"
    me="$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['manual_equivalence'])" 2>/dev/null || echo "")"
    ps="$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['passed'])" 2>/dev/null || echo "false")"
    fs="$(echo "$r" | python3 -c "import sys,json; print(json.load(sys.stdin)['failure_signature'])" 2>/dev/null || echo "")"
    echo "| $idx | $name | $topo | $js | $me | $ps | $fs |"
  done
  echo ""
  echo "**Total**: $total | **Passed**: $passed | **Failed**: $failed"
} > "$run_dir/matrix.md"

ln -sfn "$run_dir" "$matrix_output_dir/latest" 2>/dev/null || true
ln -sf "$run_dir/matrix.json" "$matrix_output_dir/latest-matrix.json" 2>/dev/null || true
ln -sf "$run_dir/matrix.md" "$matrix_output_dir/latest-matrix.md" 2>/dev/null || true

echo ""
echo "=== Matrix complete ==="
echo "total=$total passed=$passed failed=$failed"
echo "matrix_json=$run_dir/matrix.json"
echo "matrix_md=$run_dir/matrix.md"

[[ $failed -eq 0 ]] || exit 1
