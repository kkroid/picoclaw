# AppFactory 产品级整链体验运行手册

本文档定义 item 9 的第一版固定产品级体验路径，用于把当前已经收口的输入链、审批链、public-job、review 和 delivery 串成一条可重复执行的主线，而不是继续依赖临场手工拼命令。

## 1. 目标

- 固定一条可重复体验的主路径：`requirement -> prepare / approval -> job -> run -> review -> delivery`。
- 把关键快照统一归档到 `workspace/appfactory/product-e2e/`，为后续持续回归和排障提供稳定 artifact 根目录。
- 显式执行 PRD 审批、模板审批和 `compile-prepare`，不再把这些门禁假设为“内部默认已经完成”。

## 2. 默认输入与输出

默认固定输入：

- requirement：`examples/appfactory/bookkeeping/requirement.md`
- template：`flutter-finance-lite`
- reviewer：`product-e2e-reviewer`
- delivery status：`approved_for_signing`

默认归档输出：

- 本次运行目录：`workspace/appfactory/product-e2e/runs/<timestamp>/`
- 最新结果 JSON：`workspace/appfactory/product-e2e/latest.json`
- 最新结果 Markdown：`workspace/appfactory/product-e2e/latest.md`

默认运行预算与缓存语义：

- `timeout_seconds` 默认值为 `2400`，目标是覆盖首次冷启动时 `flutter build apk --debug --no-pub` 的 Gradle 依赖下载、wrapper 预热和编译缓存建立成本。
- builder 镜像负责固定 Flutter SDK、Android SDK、JDK 与基础 precache；`PUB_CACHE` 与 `GRADLE_USER_HOME` 走宿主持久目录，避免每个新 job 都从零预热。
- 如果本机已经有热缓存，单次运行通常会明显快于首次冷启动；但默认预算仍以“首次必须成功”为准，而不是按热缓存平均时间倒推。

## 3. 入口

默认入口：

```bash
APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18800 \
make verify-appfactory-product-flow
```

界面入口：

- 打开 `http://127.0.0.1:18800/jobs`
- 使用任务中心顶部的“新建任务”按钮
- 在抽屉里填写 requirement 后，页面会自动执行 `POST /api/v1/prds:compile -> POST /api/v1/jobs`，并默认以 `real_checks=true` 和 `executor_image=picoclaw/appfactory-builder:local` 生成真实 Flutter 构建计划，而不是交接探针计划
- 如果同时开启“创建后立即启动”，页面会先自动调用 `POST /internal/v1/builders:register` 注册本地 builder，再触发 `POST /api/v1/jobs/{job_id}:start`
- 如果 backend 的 `appfactory.builder_runtime.enabled=false`，任务仍会回退到 thin executor，只验证输入包和探针文件，不会真的执行 Flutter 构建链路。

前置条件：

- 这组 `/api/v1/*` AppFactory 路由必须由当前仓库版本的 web backend 提供，不是任意旧 launcher 二进制都具备。
- 如果 `curl http://127.0.0.1:18800/api/v1/notifications` 返回 `404`，说明当前 `18800` 上跑的是旧 launcher，而不是当前仓库版本的 backend。
- 这种情况下，先单独启动当前仓库 backend，再把 `APPFACTORY_PRODUCT_FLOW_API_BASE` 指到它：

```bash
go run ./web/backend -port 18807 -no-browser config/config.json

APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18807 \
make verify-appfactory-product-flow
```

持续回归入口：

```bash
APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18800 \
make run-appfactory-product-flow-regression
```

等价脚本入口：

```bash
bash scripts/run-appfactory-product-e2e.sh \
  --api-base http://127.0.0.1:18800
```

如果想显式指定 requirement、job id 或 follow-up：

```bash
bash scripts/run-appfactory-product-e2e.sh \
  --api-base http://127.0.0.1:18800 \
  --requirement-file examples/appfactory/bookkeeping/requirement.md \
  --job-id job-product-e2e-demo \
  --prd-id prd-product-e2e-demo \
  --delivery-status released \
  --release-channel internal \
  --rollout-percent 100 \
  --follow-up-status monitoring
```

## 4. 脚本实际做的事

脚本会按固定顺序执行：

1. `POST /api/v1/prds:compile` 生成 requirement 对应的 prepare 输入。
2. `POST /api/v1/jobs` 创建 public job。
3. `POST /internal/v1/builders:register` 注册固定 builder。
4. 读取 job detail 的 `status_context.suggested_action`，自动补齐：
   - `submit_prd_approval`
   - `submit_template_approval`
   - `compile_prepare_bundle`
5. `POST /api/v1/jobs/{job_id}:start` 启动作业。
6. 轮询 `GET /api/v1/jobs/{job_id}`，直到进入终态。
7. 拉取 artifacts、events、notifications 快照。
8. `POST /internal/v1/reviews:prepare` 生成 review/handoff 产物。
9. `POST /internal/v1/deliveries:record` 写入 delivery record。
10. 如果显式传入 `--follow-up-status`，继续 `POST /internal/v1/deliveries:follow-up`。
11. 刷新最终 public job、artifacts、events、notifications，并写入 `latest.json` / `latest.md`。

## 5. 成功信号

脚本成功结束时，至少会固定产出以下对象：

- `compile-response.json`
- `job-create-response.json`
- `builder-register-response.json`
- `job-start-response.json`
- `job-final-response.json`
- `review-prepare-response.json`
- `delivery-record-response.json`
- `job-public-final-response.json`
- `artifacts-final-response.json`
- `events-final-response.json`
- `notifications-final-response.json`
- `product-flow-result.json`

其中 `latest.json` 与 `latest.md` 代表“最近一次固定产品级体验结果”，后续持续回归可以直接基于这两个路径做展示或阈值判定。

另外当前已经补了产品级整链 summary / alert 三件套：

- `make verify-appfactory-product-flow-summary`：聚合 `runs/*/product-flow-result.json`，输出冷/热缓存维度的总耗时、run 耗时和 `build-apk` 阶段耗时汇总。
- `make check-appfactory-product-flow-alerts`：基于 summary 检查 latest run 是否成功，以及是否超过配置的耗时阈值。当前默认阈值为：`latest script <= 300s`、`latest run <= 180s`、`latest build-apk <= 120s`；`warm build-apk p95 <= 90s` 只会在 warm 样本数达到 `10` 之后启用，避免少量历史异常值在样本尚小的时候把主线长期卡死。
- `make run-appfactory-product-flow-regression`：顺序执行 product flow、刷新 summary、执行 alerts，作为当前最小持续回归入口。

同时现在补了一条与 `/jobs` 页面参数组合对齐的独立入口：

- `make run-appfactory-jobs-regression`：直接按 `/jobs` 当前的 `compile(real_checks=true, executor_image=picoclaw/appfactory-builder:local) -> create -> register builder -> start` 组合跑一轮真实回归，并把结果固定落到 `workspace/appfactory/jobs-ui-regression/latest.{json,md}`。
- 这条入口与 `make run-appfactory-product-flow-regression` 共用 public job / builder runtime / Flutter validate 主语义，但不会覆盖 review / delivery 链，因此更适合专门盯 `/jobs` 页面建单和启动路径是否仍真实可用。

如果要进入更大的平台级回归主线，当前还有一个顶层入口：

- `make verify-appfactory-platform-regression-summary`：聚合 `workspace/appfactory/platform-regression/runs/*/platform-regression-result.json`，刷新 `workspace/appfactory/platform-regression/latest.json`、`latest.md` 与 `reports/platform-regression-summary.{json,md}`。
- `make check-appfactory-platform-regression-alerts`：基于平台级 summary 检查 latest run 的 product-flow、`/jobs` regression、builder-runtime real validation、builder-runtime summary 是否通过；如果 latest run 里 device regression 已启用，也会一并检查其是否通过。
- `make run-appfactory-platform-regression`：默认顺序执行 `product-flow regression -> /jobs regression -> builder-runtime real validation -> builder-runtime summary`。如果存在固定设备池配置 `config/appfactory-device-pool.json`，且未显式关闭 `APPFACTORY_PLATFORM_REGRESSION_RUN_DEVICE_REGRESSION`，脚本会自动把 device regression 以设备池模式并入主线；没有设备池配置时则继续默认跳过 device regression，避免无设备环境被误阻塞。平台 regression 结束后还会自动刷新 platform summary 并执行 platform alerts。

## 6. 当前边界

- 这条入口当前首先拉通的是 control-plane 语义、审批门禁、review 和 delivery 对象链。
- builder-runtime 的模型能力回归、真实 Flutter 工作区 analyze/test/build、device verify 仍然沿用已有入口：
  - `make validate-builder-runtime-ollama-real`
  - `make verify-appfactory-public-job-fast`
  - `make verify-appfactory-public-job-device-fast`
  - `make run-appfactory-public-job-device-regression-fast`
- `/jobs` 回归入口只覆盖页面当前采用的 compile/create/start 参数组合与真实 terminal job 结果；现在它已经并入 platform regression 顶层主线，但仍不覆盖 review / delivery 语义，这部分仍以 product-flow 为准。
- 因此这版产品级整链已经解决“固定主路径”和“固定 artifact 根目录”；当前 platform regression 也已经把 `/jobs` UI 路径并入顶层主线。后续剩余工作主要是继续提高 builder-runtime real validation 与 device regression 的长期稳定性，而不是再补一条平行入口。

## 7. 排障建议

- 如果脚本在审批或 prepare 阶段失败，先看 `job-readiness-*.json` 里最后一次 `status_context.suggested_action`。
- 如果脚本在运行阶段失败，优先看 `job-final-response.json`、`events-response.json` 和 `notifications-response.json`。
- 如果失败类别是 `environment_check_failed:check-flutter-pub-get`，优先确认当前 `picoclaw/appfactory-builder:local` 是否允许宿主 UID 写入 `/opt/flutter/bin/cache`，否则 Flutter 会在 `flutter_version_check.stamp` 上直接失败。
- 如果失败摘要是 `environment closure check check-flutter-build-apk failed: signal: killed`，先确认是否把 product flow 的外层 `timeout_seconds` 设得过小；当前默认值已经提高到 `2400`，低于这个量级时，首次 Gradle 预热容易被外层 run timeout 提前终止。
- 如果想判断当前是冷缓存退化、热缓存退化，还是单次偶发抖动，优先看 `workspace/appfactory/product-e2e/reports/product-flow-summary.json` 与 `product-flow-summary.md` 中的 `cache_mode_before`、`run_duration_seconds`、`build_apk_duration_seconds` 聚合结果。
- 如果脚本在 review / delivery 阶段失败，优先看 `review-prepare-response.json` 与 `delivery-record-response.json`。
- 如果需要机器可读的最近结果，直接读取 `workspace/appfactory/product-e2e/latest.json`，不要再从临时目录里手工找快照。