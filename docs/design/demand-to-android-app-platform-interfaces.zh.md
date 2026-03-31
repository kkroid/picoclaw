# PicoClaw 需求采集与产品研发编排平台接口设计（V1）

> 状态：Frozen
>
> 本文档是主设计文档的接口层补充，目标是把控制平面、审批流、Builder Adapter、Worker 之间的交互收敛为可实现的接口协议。

运行时状态、审批、Worker、存储和事件规则统一以主设计文档 [demand-to-android-app-platform.zh.md](demand-to-android-app-platform.zh.md) 为准。本文件只保留接口契约，不再重复运行时语义细节。

## 1. 文档目标

- 定义 V1 的公共接口与内部接口边界。
- 明确哪些接口同步返回，哪些接口必须异步 job 化。
- 明确接口与 schema 的对应关系，避免“接口长得像能用，实际字段随意加减”。
- 为后续 HTTP API、Gateway、Web UI、Worker 回调实现提供基线。

### 1.1 V1 接口范围

V1 接口范围如下：

- `POST /api/v1/prds:compile`
- `GET /api/v1/prds/{prd_id}`
- `POST /api/v1/prds/{prd_id}:submit-approval`
- `POST /api/v1/templates:match`
- `GET /api/v1/templates/{template_id}`
- `POST /api/v1/templates/{template_id}:submit-approval`
- `POST /api/v1/approvals`
- `GET /api/v1/approvals/{approval_id}`
- `POST /api/v1/approvals/{approval_id}:decision`
- `POST /api/v1/jobs`
- `GET /api/v1/jobs`
- `GET /api/v1/jobs/{job_id}`
- `POST /api/v1/jobs/{job_id}:start`
- `POST /api/v1/jobs/{job_id}:resume`
- `POST /api/v1/jobs/{job_id}:cancel`
- `GET /api/v1/jobs/{job_id}/artifacts`
- `GET /api/v1/jobs/{job_id}/events`
- `GET /api/v1/notifications`
- `POST /api/v1/notifications/{notification_id}:ack`
- `POST /api/v1/notifications:rebuild`

- `POST /internal/v1/builders:register`
- `POST /internal/v1/builders/{builder_id}/heartbeat`
- `POST /internal/v1/build-runs`
- `GET /internal/v1/build-runs/{run_id}`
- `POST /internal/v1/build-runs/{run_id}/heartbeat`
- `POST /internal/v1/build-runs/{run_id}/complete`
- `POST /internal/v1/build-runs/{run_id}/fail`
- `POST /internal/v1/build-runs/{run_id}/cancel`
- `POST /internal/v1/workers:allocate`
- `POST /internal/v1/workers/{worker_id}/preserve`
- `POST /internal/v1/workers/{worker_id}/release`
- `POST /internal/v1/artifacts:index`
- `POST /internal/v1/metrics:index`
- `POST /internal/v1/reviews:prepare`
- `POST /internal/v1/deliveries:record`

Job 执行采用异步 orchestrator 语义：`POST /api/v1/jobs/{job_id}:start` 与 `POST /api/v1/jobs/{job_id}:resume` 会先写入 `workspace/appfactory/orchestrator/jobs/{job_id}.json`，再由 orchestrator 推进 execution。execution record 至少包含 `attempt_count`、`attempts[]`、`lease_owner_id`、`lease_acquired_at`、`lease_expires_at` 五类接管字段，用于 claim、续租、交接和恢复。

同一个 `run_id` 不允许从 `running` 或任一终态回退到 `queued`；同一个 job 创建新的 execution record 前，上一条 execution 必须已经进入终态。

orchestrator 除 Job 级 execution record 外，还暴露最小观测与控制接口：

- `GET /internal/v1/orchestrator/status`：返回最近一次 orchestrator pass 的状态快照，当前至少包含 `state`、`last_trigger`、`last_pass_started_at`、`last_pass_finished_at`、`queued_recoveries`、`running_recoveries`、`foreign_live_lease_skips`、`last_error`。
- `POST /internal/v1/orchestrator:run`：同步触发一轮 recovery/dispatch pass，并返回更新后的状态快照。
- `POST /internal/v1/orchestrator:unlock-watch`：显式释放 `watch-lock.json`；默认只清理 stale 或当前实例自持有的锁，传 `force=true` 时允许人工强制清理活跃外部锁。
- `POST /internal/v1/orchestrator/watch:start`：在当前 HTTP Handler 进程内启动一个长期后台 watcher；请求体至少支持 `interval_seconds`。
- `POST /internal/v1/orchestrator/watch:stop`：停止当前 HTTP Handler 进程内的后台 watcher，并释放它持有的 `watch-lock.json`。

CLI 与 HTTP watcher 共用同一套 watcher runner 与 `status.json` / `watch-lock.json` 语义，确保 watch lock、runner 状态、退出语义和周期性 pass 的落盘格式一致。

`GET /internal/v1/orchestrator/status` 同时暴露两层 watcher 语义：

- `watch_lock_*` 字段描述基于 `watch-lock.json` 的跨进程 best-effort 单实例约束，包括 `watch_lock_state`、`watch_lock_owner_id`、`watch_lock_mode`、`watch_lock_acquired_at`、`watch_lock_updated_at`、`watch_lock_expires_at`。
- `watch_runner_*` 字段描述最近一次持有 orchestrator watcher 角色的运行态，包括 `watch_runner_state`、`watch_runner_mode`、`watch_runner_owner_id`、`watch_runner_interval_seconds`、`watch_runner_started_at`、`watch_runner_stopped_at`、`watch_runner_last_error`。

`watch_lock_state` 当前约定为 `unlocked`、`held_by_self`、`held_by_other`、`stale` 四种值，用于判断是否已经有另一个常驻 orchestrator watcher 在运行；`watch_runner_state` 当前约定为 `not_running`、`running`、`stopping` 三种值，用于区分当前 watcher 生命周期。`watch_runner_mode` 当前至少有 `cli_watch` 与 `internal_api_watch` 两种值，用于标识最近一次实际运行 watcher 的角色来源。

除状态接口外，watcher 控制面还会形成两类附加可观测物：

- `workspace/appfactory/orchestrator/watch-events.jsonl`：记录 `orchestrator_watch_started`、`orchestrator_watch_stopped`、`orchestrator_watch_unlocked` 三类控制动作，字段至少包含 `action`、`remote_addr`、`user_agent`、`force`、`summary`、`watch_runner_state`、`watch_runner_mode`、`watch_lock_state`、`created_at`。
- `GET /api/v1/notifications`：把 watcher 审计事件投影成同名通知类型，供 UI 和运维侧直接观察后台 watcher 生命周期与人工干预动作。

## 2. 设计原则

- 接口只传结构化对象，不传整段自由对话历史。
- 一切长任务都通过 job 驱动，不允许前端长连接卡住等待完整构建结束。
- 对外接口优先稳定、窄口、少字段；对内接口优先可观测、可恢复。
- 所有关键对象都必须能落到正式 schema。
- Builder 和 Worker 的边界必须显式，不允许“控制平面顺手去跑构建”。
- 当前接口依赖的 schema 都视为 `0.1.0` 草案契约，待架构冻结后再升级正式版。

## 3. 参与方

| 参与方 | 角色 |
|---|---|
| `gateway/ui` | 人工提交需求、查看 PRD、审批、查看任务与产物 |
| `demand/prd services` | 需求整理、PRD 编译 |
| `template-registry` | 提供模板检索、模板详情、健康状态 |
| `job-orchestrator` | 创建任务、推进状态机、绑定审批与 Builder 运行 |
| `approval-gate` | 创建审批记录、记录审批结论 |
| `builder-adapter` | 启动 Builder、接收回调、归档执行结果 |
| `worker-manager` | 分配/释放 worker、保留或销毁现场 |
| `builder-runtime` | 真实执行代码改造与验证；长期默认实现为 thin executor |
| `artifact-state-store` | 存储 PRD、任务状态、报告、APK、截图与日志 |

## 4. 公共 API

公共 API 面向 Gateway、Web UI 或后续外部调用方。

### 4.1 需求与 PRD

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /api/v1/demands:ingest` | `gateway/ui` | 写入一批原始需求 | 需求源对象列表 | `source_ids[]` |
| `POST /api/v1/prds:compile` | `gateway/ui`、`job-orchestrator` | 由需求集合生成结构化 PRD | `source_ids[]`、可选目标约束 | `prd_id`、`version`、`status` |
| `GET /api/v1/prds/{prd_id}` | `gateway/ui` | 读取 PRD | 无 | `PRD.json`，符合 `prd.schema.json` |
| `POST /api/v1/prds/{prd_id}:submit-approval` | `gateway/ui`、系统 | 对 PRD 发起审批 | `version`、`summary`、`evidence_paths[]` | `approval_id` |

`POST /api/v1/prds:compile` 建议请求体：

```json
{
  "source_ids": ["src-001", "src-002"],
  "stack": "flutter",
  "android_required": true,
  "title_hint": "轻量记账 App"
}
```

接口约束：

- `POST /api/v1/prds:compile` 负责生成 `prepare/` 目录下的 `PRD.md`、`PRD.json`、审批快照、`template-fit-report.md`、`builder-input.json`，不创建 run，也不占用 Builder。
- `GET /api/v1/prds/{prd_id}` 返回冻结后的 `PRD.json` 内容，并可通过 `?version=` 约束版本。
- `POST /api/v1/prds/{prd_id}:submit-approval` 负责创建 PRD approval record，并镜像回对应的 `prepare/prd-approval.json`。

### 4.2 模板匹配

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /api/v1/templates:match` | `gateway/ui`、`planning-engine` | 基于 PRD 做模板检索与打分 | `prd_id`、`prd_version`、`max_results` | 候选模板列表 |
| `GET /api/v1/templates/{template_id}` | `gateway/ui`、`planning-engine` | 查看模板详情 | 无 | 模板对象，符合 `template-registry-entry.schema.json` |
| `POST /api/v1/templates/{template_id}:submit-approval` | `gateway/ui`、系统 | 对选中的模板发起审批 | `prd_id`、`job_id`、`summary` | `approval_id` |

接口约束：

- `POST /api/v1/templates:match` 读取已批准 PRD 中的模板约束，返回候选模板列表与打分结果。
- `GET /api/v1/templates/{template_id}` 返回模板注册表中的冻结模板元数据。
- `POST /api/v1/templates/{template_id}:submit-approval` 负责创建模板 approval record，并镜像回对应的 `prepare/template-approval.json`。

`POST /api/v1/templates:match` 建议响应体：

```json
{
  "items": [
    {
      "template_id": "flutter-finance-lite",
      "score": 82,
      "hard_gate_passed": true,
      "reasons": [
        "capability_match: list/detail/form",
        "build_proof: passed",
        "risk_penalty: outdated_dependency"
      ]
    }
  ]
}
```

### 4.3 Job 与执行控制

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /api/v1/jobs` | `gateway/ui`、系统 | 创建生成任务 | `prd_id`、`prd_version`、`template_id` | Job 对象 |
| `GET /api/v1/jobs` | `gateway/ui` | 查看任务列表 | 可选 `status` 过滤 | `items[]` |
| `GET /api/v1/jobs/{job_id}` | `gateway/ui` | 查看任务详情 | 无 | Job 对象，符合 `job.schema.json` |
| `POST /api/v1/jobs/{job_id}:start` | `gateway/ui`、系统 | 启动任务 | 可选启动备注 | 最新 Job 状态 |
| `POST /api/v1/jobs/{job_id}:resume` | `gateway/ui`、系统 | 从保留现场恢复任务 | `resume_mode`、`note` | 最新 Job 状态 |
| `POST /api/v1/jobs/{job_id}:cancel` | `gateway/ui` | 取消任务 | `reason`、`preserve_workspace` | 最新 Job 状态 |
| `GET /api/v1/jobs/{job_id}/artifacts` | `gateway/ui` | 查看产物索引 | 无 | `artifact-manifest.json` |

`POST /api/v1/jobs` 建议请求体：

```json
{
  "prd_id": "prd-20260325-001",
  "prd_version": "0.1.0",
  "template_id": "flutter-finance-lite",
  "goal_summary": "生成一个可运行的 Android Debug App",
  "human_notes": [
    {
      "note_id": "note-001",
      "summary": "首页必须保留底部导航",
      "scope": "product"
    }
  ]
}
```

接口约束：

- `POST /api/v1/jobs`、`GET /api/v1/jobs`、`GET /api/v1/jobs/{job_id}` 基于 `prepare/PRD.json`、`builder-input.json`、审批快照和 run 记录构建稳定 Job 视图，并对外统一暴露 `resume_context`、artifact、event 与 approval 信息。
- `builder-input.json` 的 canonical path 固定为 `workspace/appfactory/jobs/{job_id}/prepare/builder-input.json`。
- `POST /api/v1/jobs/{job_id}:start` 只能在 PRD 与模板审批均通过后执行；接口会创建 execution record，写入 orchestrator 路径，并在已有 `queued` 或 `running` execution 时返回 `JOB_START_CONFLICT`。
- `POST /api/v1/jobs/{job_id}:cancel` 负责终止非终态 run，并在需要时把 execution record 收敛到 `cancelled`；如果请求带 `preserve_workspace=true`，平台必须把现场归档到 `jobs/{job_id}/snapshots/preserved/*` 并加入 artifact 列表。
- `POST /api/v1/jobs/{job_id}:resume` 只允许从 `failed` Job 恢复；接口受 `resume_context` 门禁控制，并在 `requires_human_confirmation=true` 或 `resume_allowed=false` 时拒绝直接恢复。若 `requires_preserved_workspace=true`，恢复必须基于 preserved snapshot 执行；恢复前若 canonical workspace 已有内容，必须先归档到 `jobs/{job_id}/snapshots/archived/*`。已有 `queued` 或 `running` execution 时返回 `JOB_RESUME_CONFLICT`。
- `GET /api/v1/jobs/{job_id}/artifacts` 返回该 Job 的 `artifact-manifest.json`；如果还没有 run，返回空 manifest 占位对象。
- `GET /api/v1/jobs/{job_id}/events` 返回 Job 时间线，除 run events 外，还要投影 approval、orchestrator、resume 与 workspace preserve/restore 的 synthetic signal。

### 4.4 审批接口

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /api/v1/approvals` | 系统、`gateway/ui` | 创建审批记录 | 审批类型、目标对象、版本、摘要 | 审批记录 |
| `GET /api/v1/approvals/{approval_id}` | `gateway/ui` | 查询审批记录 | 无 | 审批记录 |
| `POST /api/v1/approvals/{approval_id}:decision` | `gateway/ui` | 提交审批结论 | `decision`、`reviewer_id`、`comment` | 最新审批记录 |

`POST /api/v1/approvals/{approval_id}:decision` 建议请求体：

```json
{
  "decision": "changes_requested",
  "reviewer_id": "user-001",
  "comment": "模板风险过高，请更换更简单的模板",
  "required_changes": [
    "replace_template",
    "regenerate_fit_report"
  ]
}
```

接口约束：

- `POST /api/v1/approvals` 创建 `pending` approval record；`prd` 与 `template` 类型需要自动补齐 `subject_version` 与默认证据。
- `GET /api/v1/approvals/{approval_id}` 返回冻结后的审批记录。
- `POST /api/v1/approvals/{approval_id}:decision` 只允许对 `pending` 记录写入 `approved`、`changes_requested` 或 `rejected` 结论，并同步回写全局审批索引与 `prepare` 快照。

### 4.5 事件与通知接口

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `GET /api/v1/jobs/{job_id}/events` | `gateway/ui` | 查询任务事件时间线 | 无 | 事件列表 |
| `GET /api/v1/notifications` | `gateway/ui` | 查询待处理通知 | 可选过滤条件 | 通知列表 |
| `POST /api/v1/notifications/{notification_id}:ack` | `gateway/ui` | 确认通知 | 空对象 | 最新通知对象 |
| `POST /api/v1/notifications:rebuild` | `gateway/ui`、运维面 | 重建通知快照 | 空对象 | `items[]` |

事件与通知是两层概念：

- 事件描述“系统发生了什么”。
- 通知描述“哪些事件必须明确提醒人工处理”。

接口约束：

- `GET /api/v1/notifications` 读取并返回 `workspace/appfactory/notifications/index.json` 中的通知快照；通知来源包括审批、失败 Job、orchestrator、watcher、execution 与 delivery 投影。
- 失败 Job 通知必须附带 `resume_context` 相关字段，包括 `failure_category`、`recommended_resume_mode`、`requires_human_confirmation`、`requires_preserved_workspace`。
- `POST /api/v1/notifications/{notification_id}:ack` 负责把通知标记为已确认，并回写 `acknowledged`、`acknowledged_at`。
- `POST /api/v1/notifications:rebuild` 负责重建通知快照，并保留既有 ack 状态。
- 成功 run 需要生成 `reports/review-bundle.metadata.json`、`reports/review-bundle.md` 与 `reports/handoff-checklist.md`，并通过 artifact manifest 暴露。
- `POST /internal/v1/reviews:prepare` 按 `run_id` 重放生成 review / handoff 产物，并回写 artifact manifest 与 metrics。
- `POST /internal/v1/deliveries:record` 负责写入 `reports/delivery-record.json`，重新索引 artifact manifest，并把交付状态投影到 public job detail、events 与 notifications。

事件模型的正式语义定义见主设计文档 [demand-to-android-app-platform.zh.md](demand-to-android-app-platform.zh.md)。

## 5. 内部接口

内部接口面向 `job-orchestrator`、`builder-adapter`、`worker-manager` 和 `builder-runtime`。

补充说明：这里的 `builder-runtime` 是泛化术语，表示实际执行输入包的运行时。长期默认实现是 thin executor。

### 5.1 Builder Run 接口

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /internal/v1/builders:register` | `builder-runtime`、`builder-adapter` | 注册或刷新执行节点元信息 | Builder ID、能力标签、worker profile | Builder 节点摘要 |
| `POST /internal/v1/builders/{builder_id}/heartbeat` | `builder-runtime` | 上报执行节点心跳 | 当前状态、当前 Run、失败摘要 | 最新 Builder 节点摘要 |
| `POST /internal/v1/build-runs` | `job-orchestrator` | 创建一次 Builder 运行 | `worker_id`、`lease_id`、`builder_input` | `run_id`、`worker_id`、状态、launch spec |
| `GET /internal/v1/build-runs/{run_id}` | `job-orchestrator` | 查询 Builder 运行状态 | 无 | 运行状态摘要 |
| `POST /internal/v1/build-runs/{run_id}/heartbeat` | `builder-runtime` | 上报阶段心跳与进展 | 当前阶段、迭代数、失败签名、token 使用 | `ack` |
| `POST /internal/v1/build-runs/{run_id}/complete` | `builder-runtime` | 回传最终输出包 | `builder_output` | `ack` |
| `POST /internal/v1/build-runs/{run_id}/fail` | `builder-runtime` | 上报致命失败 | 错误摘要、恢复建议 | `ack` |
| `POST /internal/v1/build-runs/{run_id}/cancel` | `job-orchestrator` | 取消运行 | `reason` | `ack` |

这里的关键点是：

- `POST /internal/v1/build-runs` 使用封装请求体：顶层携带 `worker_id` 和 `lease_id`，其中 `builder_input` 主体必须符合 `builder-input.schema.json`。
- `POST /internal/v1/build-runs/{run_id}/complete` 使用封装请求体：顶层 `builder_output` 主体必须符合 `builder-output.schema.json`。
- `POST /internal/v1/build-runs` 成功后，控制平面会把 `workspace_path`、`artifact_dir` 和 `context_files.*` 规范化到 Job 目录下，并在响应中返回 `runner_script_path`、`launch_command`、`launch_args`、`log_path` 这组 launch spec，供 builder adapter、thin executor 或过渡 builder runtime 在自己的执行环境里触发。
- `context_files.supporting_files` 除原始 requirement 外，还要携带 `prd-approval.json` 与 `template-approval.json` 两份结构化审批快照。
- adapter runner 在执行 `thin-prepare` 或任意 acceptance check 前，必须先校验 `builder-input.json` 中声明的审批快照；只要快照缺失、结构损坏或 `status != approved`，run 就要直接失败。
- `POST /internal/v1/build-runs/{run_id}/complete`、`fail`、`cancel` 必须同步释放对应 Builder 租约。
- `heartbeat` 只传增量进展，不回传大日志正文。
- `builder-runtime` 对瞬时模型请求失败应先内部重试，只有超过阈值后才通过 `heartbeat` 或 `fail` 上报升级状态。

### 5.2 Worker 管理接口

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /internal/v1/workers:allocate` | `builder-adapter`、`job-orchestrator` | 分配 worker / builder | `job_id`、capability、model tier、budget class、worker profile | `worker_id`、`lease_id`、`decision` |
| `POST /internal/v1/workers/{worker_id}/preserve` | `builder-adapter` | 保留失败现场 | `reason`、`ttl_minutes` | `ack` |
| `POST /internal/v1/workers/{worker_id}/release` | `builder-adapter` | 释放 worker | `archive_artifacts`、`destroy_workspace` | `ack` |

Worker 默认应基于统一 Docker 镜像创建，`input`、源码工作区、缓存和产物目录通过挂载方式注入容器。

调度策略采用三层约束：

- `required_capability_tags` 作为硬过滤，必须全部命中。
- `preferred_capability_tags`、`priority`、`last_assigned_at` 作为排序键，优先选择更贴近任务的 builder，并对最近未分配的节点做最小负载均衡。
- `model_tier`、`budget_class`、`worker_profile_name` 作为最小准入门禁；当前至少支持用 `budget_class=low` 排除 `flagship` 模型池，避免低预算任务误落到高成本节点。

`workers:allocate` 响应里的 `decision` 返回 `selected_builder_id`、`candidate_builder_ids`、`decision_reason`、`fallback_count`，用于排障与成本策略演进。

### 5.3 产物与归档接口

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /internal/v1/artifacts:index` | `builder-adapter`、`android-validation` | 写入产物索引 | `artifact-manifest.json` | `ack` |
| `POST /internal/v1/metrics:index` | `builder-adapter` | 写入 metrics | `metrics.json` | `ack` |
| `POST /internal/v1/reviews:prepare` | `job-orchestrator` | 重放生成 review / handoff 交付包 | `run_id` | `review_bundle_metadata_path`、`review_bundle_path`、`handoff_checklist_path`、`artifact_manifest_path` |
| `POST /internal/v1/deliveries:record` | `job-orchestrator`、人工交付面 | 写入交付后结构化记录并回填交付信号 | `run_id`、`reviewer_id`、`status`、可选发布/反馈字段 | `delivery_record_path`、`status`、`next_action` |

`artifacts:index` 和 `metrics:index` 都采用封装请求体：顶层增加 `run_id`，分别把 `manifest` 和 `metrics` 作为 schema 主体下发，以便控制平面精确定位目标 Run。

### 5.4 事件与通知接口

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /internal/v1/events:publish` | `job-orchestrator`、`builder-adapter`、`worker-manager` | 记录事件 | 事件对象 | `ack` |
| `POST /internal/v1/notifications:dispatch` | `job-orchestrator`、`approval-gate`、`worker-manager` | 分发人工通知 | 通知对象 | `ack` |

以下情况必须至少触发一次人工通知：

- Job 进入 `failed`
- Job 进入 `escalated`
- Worker 进入保留现场状态
- 阻塞型审批超时
- 长时间无法连接大模型

## 6. 核心错误模型

公共接口和内部接口都应统一错误返回结构：

```json
{
  "error_code": "JOB_PRECONDITION_FAILED",
  "message": "template approval is required before start",
  "retryable": false,
  "details": {
    "job_id": "job-001",
    "missing_approval_type": "template"
  }
}
```

推荐的错误码分层如下：

- `VALIDATION_*`：字段校验失败、schema 不通过。
- `APPROVAL_*`：审批缺失、审批拒绝、审批版本不匹配。
- `JOB_*`：状态机非法转换、任务不存在、任务不可恢复。
- `BUILD_*`：Builder 启动失败、Builder 回调不合法、模型不可用或持续限流，如 `BUILD_MODEL_UNAVAILABLE`、`BUILD_MODEL_RATE_LIMITED`。
- `WORKER_*`：worker 分配失败、worker 心跳超时、worker 环境损坏。
- `ARTIFACT_*`：产物缺失、产物索引失败、归档失败。

## 7. 幂等与异步语义

V1 建议所有会改变系统状态的公共接口都支持 `Idempotency-Key`。

特别是以下接口：

- `POST /api/v1/jobs`
- `POST /api/v1/jobs/{job_id}:start`
- `POST /api/v1/jobs/{job_id}:resume`
- `POST /api/v1/approvals`
- `POST /internal/v1/build-runs`

长任务接口统一采用异步语义：

- 创建成功返回 Job 或 Run 标识。
- 真正执行进度通过查询接口或事件流获得。
- 不允许通过一次 HTTP 请求直接等待“Builder 完整执行结束”。

事件流的物理落盘和目录组织见主设计文档 [demand-to-android-app-platform.zh.md](demand-to-android-app-platform.zh.md)。

## 8. 接口与 Schema 的映射关系

| 接口 | 核心 Schema |
|---|---|
| `GET /api/v1/prds/{prd_id}` | `prd.schema.json` |
| `GET /api/v1/templates/{template_id}` | `template-registry-entry.schema.json` |
| `GET /api/v1/jobs/{job_id}` | `job.schema.json` |
| `GET /api/v1/approvals/{approval_id}` | `approval-record.schema.json` |
| `POST /internal/v1/build-runs` | `builder-input.schema.json` |
| `POST /internal/v1/build-runs/{run_id}/complete` | `builder-output.schema.json` |
| `POST /internal/v1/artifacts:index` | `artifact-manifest.schema.json` |
| `POST /internal/v1/metrics:index` | `builder-metrics.schema.json` |
