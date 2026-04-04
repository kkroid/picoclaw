# AppFactory 内部试运行基线手册

本文档用于收口 item 10 的第一批工程资产，目标不是重复描述底层设计，而是给内部试运行和值班同学一份单入口操作面：知道该跑什么、看什么、遇到失败先回哪里、哪些安全边界不能破。

## 1. 目标

- 给当前 AppFactory 主线提供一套内部试运行最小入口，而不是继续依赖口头经验。
- 把 product-flow、platform regression、device pool、delivery、排障和安全边界整理成同一套操作手册。
- 为后续治理、脱敏、审计和值班职责继续细化提供统一挂载点。

## 2. 当前主线入口

### 2.1 产品级整链入口

当前默认 launcher / backend 入口与产品级整链手册保持一致，统一使用 `http://127.0.0.1:18800`，不再使用旧的 `18807` 口径。

如果 `curl http://127.0.0.1:18800/api/v1/notifications` 返回 `404`，说明当前 `18800` 上运行的是旧 launcher，缺少当前仓库的 AppFactory 路由；这时应先单独启动当前仓库 backend，再把 `APPFACTORY_PRODUCT_FLOW_API_BASE` 指向新端口，例如：

```bash
go run ./web/backend -port 18807 -no-browser config/config.json

APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18807 \
make verify-appfactory-product-flow
```

默认入口：

```bash
APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18800 \
make verify-appfactory-product-flow
```

持续回归入口：

```bash
APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18800 \
make run-appfactory-product-flow-regression
```

`/jobs` 真实建单入口：

```bash
APPFACTORY_JOBS_API_BASE=http://127.0.0.1:18800 \
make run-appfactory-jobs-regression
```

用途：

- 验证 `requirement -> prepare / approval -> job -> run -> review -> delivery` 固定主路径。
- 刷新 `workspace/appfactory/product-e2e/latest.{json,md}` 与 `reports/product-flow-summary.{json,md}`。
- 作为“控制面整链是否还活着”的第一判断入口。
- 如果只想确认 `/jobs` 页面当前的 compile/create/start 真实建单路径是否仍可用，优先跑 `make run-appfactory-jobs-regression`；它不会覆盖 review / delivery，只专注页面建单与真实 Flutter terminal run。

### 2.2 平台级主线入口

默认入口：

```bash
APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18800 \
make run-appfactory-platform-regression
```

用途：

- 顺序执行 `product-flow regression -> builder-runtime real validation -> builder-runtime summary`。
- 当 `config/appfactory-device-pool.json` 存在且有可用设备时，自动并入 device regression。
- 刷新 `workspace/appfactory/platform-regression/latest.{json,md}` 与 `reports/platform-regression-summary.{json,md}`。
- 执行 platform top-level alerts，作为“当前平台主线是否可试运行”的最终入口。

### 2.3 设备回归入口

快速入口：

```bash
make run-appfactory-public-job-device-regression-fast
```

固定设备池入口：

```bash
make run-appfactory-public-job-device-regression-pool-fast
```

用途：

- 验证 public-job 真实 Flutter 工作区在设备侧是否仍可安装、拉起并采集最小证据。
- 刷新 `workspace/appfactory/device-regression/latest.md`、`index.json` 与设备池 `status.{json,md}`。
- 作为“真实设备执行面是否还活着”的专项入口。

### 2.4 试运行 freshness 检查入口

默认入口：

```bash
make check-appfactory-internal-trial-freshness
```

用途：

- 基于现有 `product-e2e/latest.json`、`platform-regression/latest.json` 与 `device-regression/index.json` 判断当前内部试运行主线是否仍处于“最近成功且可值守”的状态。
- 如果存在启用中的固定设备池配置，脚本会自动把 device regression freshness 一并纳入检查；否则只检查 product-flow 与 platform latest。
- 作为值班前的第一道快速判断，而不是每次手工读三份 latest 文件。

### 2.5 试运行状态页入口

默认入口：

```bash
make verify-appfactory-internal-trial-status
```

用途：

- 刷新 `workspace/appfactory/internal-trial/status.json` 与 `status.md`。
- 聚合 freshness 结果以及可选的 `workspace/appfactory/notifications/index.json`，形成单页值班状态摘要。
- 如果 notifications snapshot 已存在但 `generated_at` 缺失或快照过旧，状态页会把它标为 `attention`，避免值班只看到“没有未 ack”却忽略通知面已经失真。
- 如果当前 workspace 里尚未生成 notifications index，状态页会保留该事实，但不会阻塞 freshness 本身。

如果 backend 当前可访问，建议先刷新 notifications snapshot，再看状态页：

```bash
APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18800 \
make refresh-appfactory-notifications
```

如果状态页已经因为 notifications backlog 进入 `alert` 或 `attention`，下一步不要直接手工逐条 ack，先跑通知治理检查：

```bash
APPFACTORY_PRODUCT_FLOW_API_BASE=http://127.0.0.1:18800 \
make check-appfactory-notification-governance
```

用途：

- 把当前未 ack 通知拆成 `critical`、`manual review`、`safe auto-ack candidate` 三类，而不是只给一个总数。
- 默认 dry-run 输出 `workspace/appfactory/notifications/governance/latest.{json,md}`，作为值班和升级讨论时的稳定摘要页。
- 如果确认要自动收敛安全噪音通知，可显式加 `APPFACTORY_NOTIFICATION_GOVERNANCE_APPLY_AUTO_ACK=1`，或者直接跑 `make apply-appfactory-notification-auto-ack`；当前默认仅自动候选 `delivery_ready_for_signing`、`execution_handoff`、`orchestrator_watch_started` 这类非失败型且超过年龄阈值的通知，并默认为每个 auto-ack 类型保留最新一条，避免把最新现场也一起抹掉。
- 如果 backend API 临时不可达，但本地 persisted snapshot 已经存在，`make apply-appfactory-notification-auto-ack` 会显式启用 offline snapshot fallback，只修改本地 `workspace/appfactory/notifications/index.json` 里的安全候选 ack 状态；后续 backend 恢复后再次 `refresh` / `rebuild` 仍会沿当前 ack merge 语义保留这些确认结果。

当前已验证过两轮真实收敛样本：

- dry-run 阶段曾稳定识别出 `34` 条 `critical`、`9` 条 `manual review`、`12` 条 `safe auto-ack candidate`。
- `make apply-appfactory-notification-auto-ack` 已成功应用 `14` 条安全确认，并把未 ack 总数从 `60` 收敛到 `46`。
- 第二轮治理已补上 `superseded_by_success` 与 `historical_close_candidate` 规则，并再次执行 apply；本轮额外确认 `43` 条已被后续成功样本覆盖或已超过时间窗的历史失败通知。
- 当前最新治理摘要已经收敛为 `0` 条 `critical`、`0` 条 `manual review`、`0` 条 `safe auto-ack candidate`，仅剩 `3` 条 `informational keep-latest` 通知被保留。
- 当前状态页已回到 `internal_trial_status=ok`；剩余 `3` 条通知都属于信息型 keep-latest，不再视为当前主线告警。

当前保留策略：

- `delivery_ready_for_signing`：保留最新一条，避免最新交付待签名动作在值班面完全消失。
- `execution_handoff`：保留最新一条，方便排查最近一次 orchestrator handoff 是否正常发生。
- `orchestrator_watch_started`：保留最新一条，作为 watch 已启动的最近审计痕迹。
- 这三类通知当前只承担操作可见性，不再参与 `critical` / `manual review` 判定，也不再阻塞 `internal_trial_status=ok`。

## 3. 当前稳定样本

截至 2026-04-02，本地已确认的稳定样本如下：

- platform regression：`workspace/appfactory/platform-regression/runs/20260402T064127Z-platform/`
- device regression：`workspace/appfactory/device-regression/runs/20260402T064504Z-fast/`
- product flow：`workspace/appfactory/product-e2e/runs/20260402T064128Z/`

当前口径下：

- platform latest 已经包含 `product_flow`、`builder_runtime_real`、`builder_runtime_summary`、`device_regression`、`alerts` 全部 passed 的完整样本。
- product-flow 最新 warm 样本 `build_apk_duration_seconds=44.564`，当前仍处于正常热缓存区间。
- 真实固定设备池已经完成自动 claim / release、adb 安装、应用拉起和 `device-logcat.txt` 产物归档。

## 4. 值班检查顺序

建议内部值班或试运行同学按下面顺序判断，而不是一上来就翻临时目录：

1. 先看 `workspace/appfactory/platform-regression/latest.md`。
2. 如果只是想快速判断主线是否仍新鲜可值守，先跑 `make check-appfactory-internal-trial-freshness`。
3. 如果 freshness 正常但 internal trial status 因 notifications backlog 进入 `alert` 或 `attention`，先跑 `make check-appfactory-notification-governance`，看 `critical`、`manual review` 和 `auto-ack candidate` 的拆分。
4. 如果 platform latest 失败，再看 `workspace/appfactory/platform-regression/runs/<run_id>/platform-regression-result.json`。
5. 如果是 `product_flow` 失败，再回 `workspace/appfactory/product-e2e/latest.json` 与对应 `runs/<timestamp>/`。
6. 如果是 `device_regression` 失败，再看 `workspace/appfactory/device-regression/latest.md`、`workspace/appfactory/device-pool/status.md` 和对应 `runs/<timestamp>-fast/`。
7. 如果是 builder-runtime real validation 失败，再看 `.runtime/builder-model-validation/` 下最新 summary 和 real validation 归档。

不要跳过 latest / summary 直接去翻临时工作区。临时目录只用于已经明确到某一轮失败后的深挖。

## 5. 排障入口

### 5.1 product-flow 失败

优先看：

- `workspace/appfactory/product-e2e/latest.json`
- 对应 run 目录下的 `job-final-response.json`
- 对应 run 目录下的 `events-response.json`
- 对应 run 目录下的 `notifications-response.json`

如果是耗时类退化：

- 看 `workspace/appfactory/product-e2e/reports/product-flow-summary.json`
- 区分 latest 单次异常、warm 样本整体退化、还是 cold cache 首次预热

### 5.2 device regression 失败

优先看：

- `workspace/appfactory/device-regression/latest.md`
- 对应 run 目录下的 `regression-result.json`
- `workspace/appfactory/device-pool/status.md`

如果需要进现场：

- 看 `regression-result.json.preserved_temp_dir`
- 看对应 job `reports/` 下的 `device-logcat.txt`
- 必要时再看 `app-debug.apk`、`device-failure-summary.json`

### 5.3 platform alerts 失败

优先看：

- `workspace/appfactory/platform-regression/reports/platform-regression-summary.json`
- `workspace/appfactory/platform-regression/latest.json`

当前 platform alerts 已改为检查本轮 latest，而不是上一轮 latest；如果 alerts 与 run 结果不一致，应优先怀疑 summary 产物未刷新，而不是直接怀疑 stage 本身。

### 5.4 notifications backlog 处理

优先看：

- `workspace/appfactory/internal-trial/status.json`
- `workspace/appfactory/notifications/governance/latest.json`
- `workspace/appfactory/notifications/governance/latest.md`

建议顺序：

1. 先确认 `critical` 是否仍存在；这类通知不进入自动 ack 候选，必须保留人工动作面。
2. 再看 `manual review`，当前默认包括 `builder_failed`、`execution_cancelled` 这类需要人工判断是否已闭环的通知。
3. 只有 `safe auto-ack candidate` 才适合走 `APPFACTORY_NOTIFICATION_GOVERNANCE_APPLY_AUTO_ACK=1`，不要把 execution failure 之类失败型通知也混进去。
4. 如果需要直接执行当前默认安全策略，优先跑 `make apply-appfactory-notification-auto-ack`，不要手工逐条改 snapshot。
5. 自动 ack 之后，再重跑 `make verify-appfactory-internal-trial-status`，确认状态页是否从噪音告警回落为真实待办。

当前最新本地状态：

- `workspace/appfactory/notifications/governance/latest.json` 已显示 `before_unacknowledged_count=46`，其中 `superseded_by_success_count=18`、`historical_close_candidate_count=25`。
- 在新规则 apply 之后，治理摘要已收敛为 `after_unacknowledged_count=3`、`critical_count=0`、`manual_review_count=0`、`superseded_by_success_count=0`、`historical_close_candidate_count=0`。
- `workspace/appfactory/internal-trial/status.json` 已显示 `internal_trial_status=ok`、`internal_trial_notification_unacknowledged_count=3`、`internal_trial_notification_critical_unacknowledged_count=0`，并明确说明“only informational keep-latest notifications remain”。
- 因此当前值班面剩下的已经不是 failure backlog，而只是 3 条保留给操作面的最新信息型通知：最新 signing、最新 execution handoff、最新 orchestrator watch audit。

## 6. 安全与治理最低要求

内部试运行阶段，至少遵守下面几条：

1. 所有 API key、token、secret 不进入仓库配置明文，统一放进 `~/.picoclaw/.security.yml`。
2. `.security.yml` 权限保持 `600`，不要截图、不要发群、不要进 wiki。
3. 工具输出默认启用 sensitive data filtering，不要为了排障关闭全局脱敏再把原始日志外发。
4. 如果必须分享问题样本，优先分享 `latest.json`、`summary.json`、`regression-result.json` 和脱敏后的日志，不要直接转发带凭据的宿主机环境。
5. 需要加密配置时，统一走 `enc://` 与 `PICOCLAW_KEY_PASSPHRASE` 机制，不再新增散落的自定义 secret 存储方式。

对应手册：

- 安全配置：[docs/security_configuration.md](../security_configuration.md)
- 敏感信息过滤：[docs/sensitive_data_filtering.md](../sensitive_data_filtering.md)
- 凭据加密：[docs/credential_encryption.md](../credential_encryption.md)
- 模板治理基线：[docs/design/appfactory-template-governance.zh.md](appfactory-template-governance.zh.md)
- 报告分享与审计基线：[docs/design/appfactory-report-governance.zh.md](appfactory-report-governance.zh.md)
- 留存与值班升级基线：[docs/design/appfactory-operations-baseline.zh.md](appfactory-operations-baseline.zh.md)

## 7. 当前边界

这份手册当前只收口“如何试运行”和“如何值班”，还没有完整覆盖：

- 模板许可证与依赖治理自动化
- review / handoff / delivery 报告的更细粒度逐字段脱敏规范
- 审计留存周期和值班升级制度自动化
- CI / cron / watcher 的正式接入规范

这些内容仍属于 item 10 的后续分解项，但不再影响当前内部试运行入口的可用性。