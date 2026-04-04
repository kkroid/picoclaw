# AppFactory 报告分享、脱敏与审计基线

本文档用于给 item 10 补第一版报告治理规范，重点解决两个问题：

- 哪些报告或产物可以直接用于内部沟通，哪些必须先脱敏。
- review / handoff / delivery 阶段至少要留下哪些可追溯审计信息。

## 1. 目标

- 避免继续把原始日志、设备截图、宿主机环境信息直接外发。
- 给 review、handoff、delivery 的人工动作提供最小审计字段要求。
- 让内部试运行阶段的问题共享优先基于稳定 artifact，而不是基于聊天截图或临时终端输出。

## 2. 默认分享顺序

当需要在内部同步问题或结果时，优先分享下面这组稳定对象：

1. `latest.json`
2. `summary.json`
3. `regression-result.json`
4. 对应 run 目录中的 `job-final-response.json`、`delivery-record.json` 或等价稳定报告

不要把以下内容作为第一手共享材料：

- 临时工作区完整打包
- 宿主机环境变量导出
- 未脱敏的终端整屏截图
- 设备截图原图
- 包含原始 token、URL、用户输入或设备标识细节的完整 `logcat`

## 3. 产物分级

### 3.1 可优先内部共享

- `workspace/appfactory/product-e2e/latest.json`
- `workspace/appfactory/platform-regression/latest.json`
- `workspace/appfactory/device-regression/runs/*/regression-result.json`
- `workspace/appfactory/*/reports/*summary*.json`
- `reports/delivery-record.json`
- `reports/release-follow-up.json`

前提：

- 内容中不再额外嵌入明文凭据。
- 路径或摘要未暴露宿主机私有目录结构之外的敏感材料。

### 3.2 共享前必须检查或脱敏

- `reports/review-bundle.md`
- `reports/handoff-checklist.md`
- `reports/build-report.md`
- `reports/smoke-test-report.md`
- `artifact-manifest.json`
- `events-response.json`
- `notifications-response.json`

检查重点：

- 是否包含 requirement 原文、人工说明、内部链接、私有 URL、账号标识。
- 是否包含宿主机绝对路径且会暴露不必要的本地环境细节。
- 是否包含带业务上下文的原始报错摘要，需要先压缩成机器可读 failure category。

### 3.3 默认不直接外发

- `device-logcat.txt`
- `device-screenshot.png`
- `app-debug.apk`
- `preserved_temp_dir` 对应的整个临时工作区

这些产物默认只用于定向排障，不作为常规问题同步材料。确实需要共享时，应先缩成摘要，或仅提取必要片段。

## 4. 最低脱敏规则

内部试运行阶段，至少遵守下面几条：

1. 所有凭据统一走 `~/.picoclaw/.security.yml`，不写入仓库和共享文档。
2. 对外共享前，确保工具输出仍启用 sensitive data filtering。
3. `device-logcat.txt` 只提取必要片段，默认不整份外发。
4. 设备截图默认视为敏感证据，除非明确确认不含账号、隐私或品牌敏感信息。
5. 如果报告里必须保留绝对路径，优先保留仓库内稳定路径，减少宿主机私有目录泄露。

## 5. review / handoff / delivery 的最小审计要求

### 5.1 review 阶段

至少保留：

- `run_id`
- `review_bundle_path`
- `handoff_checklist_path`
- `artifact_manifest_path`
- 审阅人或准备动作来源

### 5.2 delivery 阶段

`delivery-record.json` 至少应能回答：

- 谁做了交付决策：`reviewer_id`
- 当前状态是什么：`status`
- 决策依据是什么：`evidence_paths[]`
- 如果未通过，要求改什么：`required_changes[]`
- 如果进入签名或发布，签了什么：`signed_artifact_paths[]`

### 5.3 device verification / release follow-up

如果存在设备验证或发布后跟踪，至少保留：

- `record_path`
- `status`
- `summary`
- `evidence_paths[]`
- `verified_at` 或 `updated_at`
- `owner_id` 或 `reviewer_id`

## 6. 推荐问题同步模板

建议内部同步问题时，按下面顺序组织：

1. 失败入口：`product-flow` / `platform regression` / `device regression`
2. 对应 run id 或 result 路径
3. 机器可读状态：`status`、`failure_category`、`suggested_action`
4. 必要的 5 到 20 行日志摘录
5. 是否涉及敏感证据，若涉及则说明“仅线下可见，不进群”

## 7. 当前边界

本文档当前定义的是“内部试运行最低治理规则”，还没有覆盖：

- 正式审计留存周期
- 值班升级制度与责任分工
- 对外合作或外部客户共享时的更严格脱敏规则
- 自动化 redaction 工具链

这些内容后续可以继续补，但当前内部试运行至少不再允许无分级地分享原始报告和设备现场。