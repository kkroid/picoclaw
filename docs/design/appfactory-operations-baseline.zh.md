# AppFactory 留存与值班升级基线

本文档用于给 item 10 补齐内部试运行阶段的两块运维底线：

- 哪些产物和状态页至少要保留多久。
- 哪些失败应该如何分级、何时升级、何时停止继续扩散问题样本。

当前目标不是引入新的调度系统，而是先把人工值班口径收敛，避免同一类故障每次都靠临场判断。

## 1. 适用范围

当前只覆盖 AppFactory 内部试运行主线：

- `workspace/appfactory/product-e2e/`
- `workspace/appfactory/platform-regression/`
- `workspace/appfactory/device-regression/`
- `workspace/appfactory/device-pool/`
- `workspace/appfactory/jobs/`
- `.runtime/builder-model-validation/`

## 2. 基础原则

1. `latest`、`summary`、`status` 这类状态页优先于临时目录，值班先看稳定对象，再进现场。
2. 成功样本用于建立基线，失败样本用于排障和回归；两者都不能只保留口头结论。
3. 现场保留不是无限期堆积，必须带“为什么保留”和“保留到什么时候”为前提。
4. 任何涉及凭据、设备截图、原始 logcat 的问题，一旦需要升级讨论范围，先按报告治理基线脱敏，再继续流转。

## 3. 最低留存周期

### 3.1 长期保留对象

以下对象视为当前内部试运行的基线资产，默认长期保留：

- `workspace/appfactory/product-e2e/latest.{json,md}`
- `workspace/appfactory/product-e2e/reports/product-flow-summary.{json,md}`
- `workspace/appfactory/platform-regression/latest.{json,md}`
- `workspace/appfactory/platform-regression/reports/platform-regression-summary.{json,md}`
- `workspace/appfactory/device-regression/latest.md`
- `workspace/appfactory/device-regression/index.json`
- `workspace/appfactory/device-pool/status.{json,md}`
- `.runtime/builder-model-validation/builder-runtime-validation-summary.latest.json`

### 3.2 运行归档对象

以下目录至少按“最近 14 天或最近 20 轮成功样本，加上所有未结案失败样本”保留：

- `workspace/appfactory/product-e2e/runs/`
- `workspace/appfactory/product-e2e/regression-runs/`
- `workspace/appfactory/platform-regression/runs/`
- `workspace/appfactory/device-regression/runs/`

说明：

- 当前这是人工留存策略，不代表仓库已经有自动清理脚本。
- 如果某一轮失败仍在定位中，即使超过 14 天也不应删除。

### 3.3 Job 现场与快照

以下对象按“问题是否已结案”处理：

- `workspace/appfactory/jobs/{job_id}/reports/`
- `workspace/appfactory/jobs/{job_id}/snapshots/preserved/`
- `workspace/appfactory/jobs/{job_id}/snapshots/archived/`

建议规则：

- 已进入 delivery / follow-up 稳定态且无争议的成功 run，保留正式 reports，preserved/archived snapshot 不作为长期资产。
- 失败 run 的 preserved snapshot 至少保留到失败原因明确、补救方案确定并且已有后续稳定样本覆盖。
- 若 preserved snapshot 只用于一次性排障，建议在问题关闭后 7 天内清理。

### 3.4 本地临时工作区

像 `preserved_temp_dir=/tmp/picoclaw-api-test-*` 这类本机临时目录，不属于长期资产。

建议规则：

- 仅在问题仍未定位清楚时保留。
- 一旦关键证据已复制进稳定 run 目录或 reports，对应临时目录应在 24 小时内删除。
- 临时目录不得作为团队共享证据根路径。

## 4. 值班分级

### 4.1 P0

满足任一条件时，视为 P0：

- 最新 `platform regression` 默认主线失败，且失败点位于 `product_flow` 或 control-plane 主链。
- 发现凭据、token、secret、未脱敏用户输入或敏感设备证据已被错误外发。
- 固定设备池状态异常导致所有设备回归完全不可用，且当前没有可替代设备。

处理要求：

- 立即停止继续扩散问题样本。
- 先锁住 latest / summary / result 路径，避免证据继续漂移。
- 涉及凭据时优先旋转 secret，再继续排障。

### 4.2 P1

满足任一条件时，视为 P1：

- `platform regression` 失败，但 `product_flow` 通过，失败集中在 `device_regression` 或 `builder_runtime_real`。
- `product-flow alerts` 或 `platform alerts` 连续两轮触发，且不是单次人工环境波动。
- 固定设备池可 claim，但验证阶段持续失败，无法形成稳定 device 证据。

处理要求：

- 当天内完成复跑和初步归因。
- 如果连续两轮复跑仍失败，应升级到平台主线负责人，而不是继续各自本地重试。

### 4.3 P2

满足任一条件时，视为 P2：

- 模板 full verify 不稳定，但主线已有替代模板或最近稳定样本。
- 单次 device 回归失败，但复跑已恢复。
- 报告治理、脱敏或留存策略发现缺口，但尚未造成错误外发。

处理要求：

- 记录到当前 TODO / runbook。
- 在下一轮 item 10 收口中补规则，而不是立刻打断主线。

## 5. 升级顺序

建议内部值班按下面顺序处理：

1. 先确认失败入口：`product-flow`、`platform regression`、`device regression`、`template verify`。
2. 读取对应 `latest`、`summary`、`regression-result`，不要先翻临时目录。
3. 如果是单次失败，允许在同一环境复跑一次，用于区分抖动和稳定退化。
4. 如果复跑仍失败，按上面的 P0 / P1 / P2 分级升级。
5. 如果涉及敏感证据，先按报告治理基线缩减可分享材料，再发给更大范围的人。

## 6. 当前值班入口

推荐顺序：

1. `make check-appfactory-internal-trial-freshness`
2. `make refresh-appfactory-notifications`
3. `make check-appfactory-notification-governance`
4. `make apply-appfactory-notification-auto-ack`
5. `make verify-appfactory-internal-trial-status`
6. `make run-appfactory-platform-regression`
7. `make run-appfactory-product-flow-regression`
8. `make run-appfactory-public-job-device-regression-pool-fast`
9. `make check-appfactory-template-governance`
10. `make verify-appfactory-template` 或 `make verify-appfactory-template-fast`

其中：

- `internal trial freshness` 用于快速判断当前 latest/index 是否仍在可值守时间窗内。
- `refresh-appfactory-notifications` 用于把 backend 当前通知面显式刷新到 `workspace/appfactory/notifications/index.json`，避免 status 页长期停在 `unavailable` 降级分支。
- `check-appfactory-notification-governance` 用于把当前 backlog 拆成 `critical`、`manual review` 和 `safe auto-ack candidate` 三类，并把治理摘要写到 `workspace/appfactory/notifications/governance/latest.{json,md}`；默认是 dry-run，只有显式设置 `APPFACTORY_NOTIFICATION_GOVERNANCE_APPLY_AUTO_ACK=1` 才会去自动确认安全噪音通知。
- `apply-appfactory-notification-auto-ack` 用于执行当前默认安全 auto-ack 策略；它会显式允许 offline snapshot fallback，因此即使 backend 暂时不可达，也能先把本地 persisted snapshot 上的安全候选收敛掉。该入口执行后若仍有 `critical` / `manual review` backlog，仍会返回非零退出码，这是预期行为，不代表 auto-ack 本身失败。
- `internal trial status` 用于把 freshness 与可选 notifications ack 聚成一页状态摘要。
- `internal trial status` 还会额外检查 notifications snapshot 是否缺少 `generated_at` 或已经过旧，避免值班页只反映 ack 而不反映通知面新鲜度。
- `platform regression` 是总入口。
- `product-flow regression` 用于确认控制面整链。
- `device regression` 用于确认真实设备执行面。
- `template governance` 用于确认许可证证据、依赖风险和 Manifest 权限是否仍满足准入基线。
- `template verify` 用于确认模板是否仍满足准入基线。

当前这套入口已经有两轮真实治理样本：

- safe auto-ack 执行前：`60` 条未 ack，其中 `34` 条 critical、`9` 条 manual review、`12` 条 auto-ack candidate。
- 执行 `make apply-appfactory-notification-auto-ack` 后：已应用 `14` 条安全确认，未 ack 收敛到 `46`，当前剩余 `34` 条 critical、`9` 条 manual review、`0` 条 auto-ack candidate。
- 在第二轮规则补齐后，治理脚本已进一步把 `18` 条被后续成功样本覆盖的失败通知归到 `superseded_by_success`，并把 `25` 条超过 `72h` 的 controlled-e2e 历史失败归到 `historical_close_candidate`。
- 第二轮再次执行 `make apply-appfactory-notification-auto-ack` 后，额外确认了 `43` 条 superseded/historical backlog；当前只剩 `3` 条 `informational keep-latest` 通知，`critical` 与 `manual review` 都已清零。
- 当前 `make verify-appfactory-internal-trial-status` 已回到 `ok`；因此当前值班面里的非零 failure backlog 已经不再阻塞主线，只剩最新 signing / handoff / orchestrator watch 提示用于操作可见性。
- 当前默认保留策略是“每类信息型通知保留最新一条”：`delivery_ready_for_signing` 用于保留最新交付动作面，`execution_handoff` 用于保留最近一次 orchestrator handoff 审计痕迹，`orchestrator_watch_started` 用于保留最近一次 watch 启动审计痕迹。

## 7. 当前边界

本文档当前仍是人工运维基线，还没有自动提供：

- 按留存周期自动清理 runs / snapshots / 临时目录
- 自动 freshness 检查与值班轮值系统
- 通知 ack 的正式值班面板（当前已有脚本治理入口，但仍没有面板化操作面）
- 基于许可证、依赖来源和依赖风险的自动升级规则

这些缺口后续可以继续补，但当前内部试运行已经不应继续在“无留存规则、无升级规则”的状态下运行。