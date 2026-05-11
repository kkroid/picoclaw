# OneAppFactory 需求采集与产品研发编排平台接口设计（V1）

> 状态：Frozen
>
> 本文档是主设计文档的接口层补充，目标是把控制平面、审批流、Builder Adapter、Worker 之间的交互收敛为可实现的接口协议。

运行时状态、审批、Worker、存储和事件规则统一以主设计文档 [demand-to-android-app-platform.zh.md](demand-to-android-app-platform.zh.md) 为准。需求拆解、任务分配和 `builder-input` 生成的对象模型统一以 [appfactory-architecture-evolution.zh.md](appfactory-architecture-evolution.zh.md) 为准。本文件只保留接口契约，不再重复运行时语义细节。

当前 active execution contract 已收口为 binding / surface-first 语义：`binding_id`、`surface_ref`、`surface_refs`、`target_paths` 是运行时唯一有效主语义。历史 `screen_list`、`screen_ref`、`screen_refs`、`legacy_screen_ref` 只允许在离线归一化或历史产物迁移里被一次性吸收，不再作为新的 public / internal request 与 prepare artifact 的合法字段。

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
- `POST /api/v1/jobs/{job_id}:compile-prepare`
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
- `POST /internal/v1/deliveries:follow-up`

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
- `GET /api/v1/notifications`：把 watcher 审计事件投影成同名通知类型，供 UI 和运维侧直接观察后台 watcher 生命周期与人工干预动作；当前外部建议动作至少覆盖 `inspect_orchestrator_status`、`inspect_orchestrator_lock_owner`、`restart_orchestrator_watch`。

jobs 运维页与 header 的 watcher 状态展示必须共用同一套 `watch_runner_state + watch_lock_state` 判定，不允许同一份 orchestrator status 在不同 UI 入口被解释成不同运行态或不同人工动作建议。

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

对于 `/jobs` 页面触发的 compile 请求，当前额外约定如下：

- 当 `requirement_source=jobs-ui` 且 `real_checks=true` 时，如果 compile 结果已经具备结构化页面、结构化实体和真实 Flutter structural checks，则允许 generic domain 直接进入 `flutter-open-lite` 主链。
- 不再要求 generic requirement 必须包含 bookkeeping / finance 关键词；待办、体重记录、习惯打卡这类单任务工具类需求都应能进入同一 generic 模板链路。
- `weight-tracker` 只作为 generic fixture，不再作为 public compile / start / resume 契约的公共语义来源；bookkeeping 与 relation-rich completed 样本只做 guardrail，不再覆盖 generic 主结论。
- 新的 public / internal prepare artifact 与 runtime contract 不得再以单个样本文件名、实体名或样本拓扑推导公共规则。
- 如果 compile 结果未来再次退化成只包含 prepare-level smoke probes 的占位包，则才应拒绝 `real_checks=true` 的 public 入口。

接口约束：

- `POST /api/v1/prds:compile` 负责生成 `prepare/` 目录下的 `PRD.md`、`PRD.json`、审批快照、`template-fit-report.md`、`builder-input.json`，不创建 run，也不占用 Builder。
- `POST /api/v1/prds:compile` 生成的 active prepare / execution artifacts 必须使用 binding / surface-first 语义；新产物不得继续输出 `screen_list`、`screen_ref`、`screen_refs`、`legacy_screen_ref` 作为执行主语义。
- `GET /api/v1/prds/{prd_id}` 返回冻结后的 `PRD.json` 内容，并可通过 `?version=` 约束版本。
- `POST /api/v1/prds/{prd_id}:submit-approval` 负责创建 PRD approval record，并镜像回对应的 `prepare/prd-approval.json`。
- `POST /api/v1/jobs/{job_id}:compile-prepare` 负责基于当前 Job 的 `prepare/requirement.md`、`PRD.json` 与模板选择重新编译 `prepare/` 目录；这是 `status_context.suggested_action=compile_prepare_bundle` 的正式 public action，不要求 UI 重新上传 `requirement_text`。

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

对于 generic domain，`template_id` 当前允许显式传 `flutter-open-lite`，也允许省略后由 compile + registry 自动解析到 `flutter-open-lite`。

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

- `POST /api/v1/jobs`、`GET /api/v1/jobs`、`GET /api/v1/jobs/{job_id}` 基于 `prepare/PRD.json`、`builder-input.json`、审批快照和 run 记录构建稳定 Job 视图，并对外统一暴露 `status_context`、`failure_context`、`resume_context`、artifact、event 与 approval 信息；如果 approval snapshot 的 `subject_version` 已不匹配当前 `prepare` 输入，Job 视图必须退回对应 approval phase，而不能继续假装“可启动”或“可恢复”。其中 PRD 审批的 `subject_version` 不只冻结 `PRD.json`，还要冻结当前 `PRD.md` 与 `requirement.md` 的内容；模板审批的 `subject_version` 不只冻结 `template_id + pinned_ref`，还要冻结当前 `template-fit-report.md` 的内容；因此 PRD 的人工说明、原始需求补充、模板匹配结果文字、差距说明或人工补充结论变化时，都应先回到对应审批分支。相对地，fresh-start builder 分支只处理 `builder-input.json` 语义变化、`implementation-plan.md`、`manual-constraints.md` 等 prepare 支撑文件内容变化，以及当前 `template_source_dir` / 默认模板 seed 源码树内容变化。若当前 PRD 已经变化到让 `builder-input.json` 的编译来源版本失效，或模板审批已经追上最新冻结版本但 `prepared_template_subject_version` 仍绑定旧模板编译来源，Job 视图都必须进入 `awaiting_prepare_recompile` / `prepare` phase，而不是继续暴露 builder phase。相对地，如果只是 `builder-input.json` 当前已经切到新的 `template_id`，则在重新完成模板审批后应回到 fresh-start builder 分支，而不是误判成必须重编译。`status_context`、`resume_context`、`delivery_context` 等对外动作字段统一使用 `suggested_action`，`failure_context` 统一承载 `failure_signature`、`failure_domain`、`failure_category`、`last_error_summary`、`retryable` 这组失败主语义；其中 execution 因 dispatcher 丢失而中断时，public `event.type`、notification `type`、job `failure_category` 统一使用 `execution_interrupted`，避免同一中断语义在不同 facade 上再出现 `execution_dispatcher_lost` / `execution_interrupted` 的双写。对于 recovery pass 或 handler 重启后直接把 run 收敛成 `failed` / `cancelled` 的终态场景，job detail 仍必须继续投影同一套 `status_context.reason_code + summary + suggested_action`，不能只在 execution override 分支上暴露终态动作语义；同理，terminal execution 对应的 notifications 也必须继续投影稳定的 `notification.type + suggested_action`，至少覆盖 `execution_interrupted`、`execution_failed`、`execution_cancelled`、`execution_recovery_failed` 这组外部可观测终态。delivery 侧的 `delivery_context` 现已扩展为正式交付对象：主记录仍由 `delivery_record_path + status + suggested_action + release_channel + rollout_percent + reviewer_id + evidence_paths + required_changes + signed_artifact_paths + recorded_at` 构成，同时允许嵌套 `device_verification` 与 `release_follow_up` 两个子对象；前者至少包含 `record_path`、`status`、`summary`、`evidence_paths`、`verified_at`、`suggested_action`，后者至少包含 `record_path`、`status`、`summary`、`owner_id`、`evidence_paths`、`updated_at`、`suggested_action`，并与独立 artifact/report 路径保持一致。
- `builder-input.json` 的 canonical path 固定为 `workspace/appfactory/jobs/{job_id}/prepare/builder-input.json`。
- `POST /api/v1/jobs/{job_id}:start` 只能在 PRD 与模板审批均通过且 approval snapshot 的 `subject_version` 仍匹配当前 `prepare` 输入时执行；接口会创建 execution record，写入 orchestrator 路径，并在已有 `queued` 或 `running` execution 时返回 `JOB_START_CONFLICT`。如果旧 run 已因重编译后的新 `prepare` 输入，或因当前 `builder-input.json` 已发生语义变化，或因当前模板 seed 源码树已变化而失效，`start` 必须允许以当前输入重新开启一轮 execution。但如果当前 PRD 已变化到让 `builder-input.json` 的编译来源版本过期，或者模板编译来源版本已经落后于当前已审批模板冻结版本，`start` 必须拒绝并明确要求先重新 compile prepare bundle。
- `POST /api/v1/jobs/{job_id}:compile-prepare` 在 `awaiting_prepare_recompile` 分支下负责把当前 Job 从“编译来源失效”推进到新的 `prepare/` 快照；重编译完成后，Job 视图必须立刻按新 `builder-input.json`、审批快照和最近一次 run 重新求值，通常会回到新的 builder / fresh-start 分支，而不是继续停留在 `awaiting_prepare_recompile`。
- `POST /api/v1/jobs/{job_id}:cancel` 负责终止非终态 run，并在需要时把 execution record 收敛到 `cancelled`；如果请求带 `preserve_workspace=true`，平台必须把现场归档到 `jobs/{job_id}/snapshots/preserved/*` 并加入 artifact 列表。
- `POST /api/v1/jobs/{job_id}:resume` 只允许从 `failed` Job 恢复；接口受 `resume_context` 门禁控制，并在 `requires_human_confirmation=true` 或 `resume_allowed=false` 时拒绝直接恢复。若 `requires_preserved_workspace=true`，恢复必须基于 preserved snapshot 执行；恢复前若 canonical workspace 已有内容，必须先归档到 `jobs/{job_id}/snapshots/archived/*`。已有 `queued` 或 `running` execution 时返回 `JOB_RESUME_CONFLICT`。如果失败现场对应的当前输入链已发生 approval version drift，则接口必须先拒绝恢复，等待对当前输入重新提审；只有在重提审后当前 prepare 仍与旧 failed run 对齐时，才允许恢复到 failed-run 语义。如果当前 `prepare` 已因重编译晚于最近一次 failed run，或当前 `builder-input.json` 的语义已不同于 failed run 采用的输入，或当前 prepare 支撑文件内容已不同于 failed run 使用的上下文，或当前模板 seed 源码树已经变化，`resume` 也必须拒绝，并明确要求改走 `start`。如果当前 PRD 已变化到让 `builder-input.json` 的编译来源版本过期，或者模板编译来源版本已经落后于当前已审批模板冻结版本，`resume` 必须直接拒绝并要求先重新 compile prepare bundle。
- `GET /api/v1/jobs/{job_id}/artifacts` 返回该 Job 的 `artifact-manifest.json`；如果还没有 run，返回空 manifest 占位对象。
- `GET /api/v1/jobs/{job_id}/events` 返回 Job 时间线，除 run events 外，还要投影 approval、orchestrator、resume 与 workspace preserve/restore 的 synthetic signal。
- 当前 Flutter P0 profile 还支持一个 env-gated 的最小设备验证扩展：当 `APPFACTORY_DEVICE_VERIFICATION_ENABLED=1` 时，acceptance checks 会在 `flutter build apk --debug` 之后继续追加 `check-adb-device-ready`、`check-install-debug-apk` 与 `check-launch-app-and-capture-logcat`，并通过 `ONEAPPFACTORY_REPORTS_DIR` 写出 `device-logcat.txt` 与可选 `device-screenshot.png`。这些文件与 `change-summary.md`、`build-report.md`、`smoke-test-report.md`、可选 `workspace/build/app/outputs/flutter-apk/app-debug.apk` 一起进入 `artifact-manifest.json`，形成可追溯的运行时设备证据层；device shell commands 现在会显式写出 `__oneappfactory_failure_signature__:*` marker，run 侧据此把失败收敛为稳定语义键，例如 `environment_check_failed:adb_binary_unavailable`、`environment_check_failed:debug_apk_missing`、`environment_check_failed:android_app_id_missing`、`device_check_failed:adb_device_unavailable`、`device_check_failed:apk_install_failed`、`device_check_failed:app_launch_failed`、`device_check_failed:app_runtime_crash`、`device_check_failed:log_capture_failed`，并在 public facade 中继续映射到稳定的 `failure_domain`。同时 `metrics.json` 现在允许额外写出 `device_failure_categories[]`，把这些设备相关失败按 `category + failure_domain + count` 聚合沉淀，供回归统计、告警或固定设备池看板使用；`smoke-test-report.md` 也会同步带出同一组设备失败类别摘要，避免调用方重复解析原始 `failure_signatures`。在固定设备池模式下，`run-appfactory-device-regression.sh` 还会依据 `config/appfactory-device-pool.json` 或等价配置自动 claim/release 设备租约，把 `serial`、`label`、`claim_status`、`owner_id`、`lease_file` 写入 `regression-result.json.device`，并同步刷新 `workspace/appfactory/device-pool/status.json` / `status.md`，让设备占用、最近回归结果和失败告警拥有同一套 machine-readable 运维视图。

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

- `POST /api/v1/approvals` 创建 `pending` approval record；`prd` 与 `template` 类型需要自动补齐 `subject_version` 与默认证据。其中 PRD 的 `subject_version` 不能只是裸 `version`，而应同时冻结当前 `PRD.json`、`PRD.md` 与 `requirement.md` 的实际内容，用于识别“结构化 PRD 未变，但人工说明或原始需求证据已经漂移”的场景；模板审批的 `subject_version` 也不能只写 `template_id + pinned_ref`，而应能冻结当前 `template-fit-report.md` 的内容，用于识别“模板匹配结果已经变化但模板源码版本未变”的场景。
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
- delivery 通知必须保留 `delivery_status`、`delivery_record_path`、可选 `release_channel` / `rollout_percent` 与稳定 `suggested_action`；当存在设备验证或发布后跟踪时，还应继续附带 `device_verification_status`、`release_follow_up_status`，以保证 jobs 运维页可以直接区分“灰度待验证”“验证失败”“发布后观察中”“发布后发现问题”等不同人工动作入口，而不是只暴露裸通知类型字符串。

### 4.6 Review / Delivery 内部接口

| 接口 | 调用方 | 说明 | 请求主体 | 响应主体 |
|---|---|---|---|---|
| `POST /internal/v1/reviews:prepare` | `gateway/ui`、内部运维页 | 为指定 `run_id` 生成 review bundle / handoff checklist | `run_id` | `review_bundle_path`、`handoff_checklist_path`、`artifact_manifest_path` |
| `POST /internal/v1/deliveries:record` | `gateway/ui`、内部运维页 | 写入主交付记录，并可同时附带设备验证结果 | `run_id`、`reviewer_id`、`status`、可选 `release_channel` / `rollout_percent` / `evidence_paths[]` / `required_changes[]` / `signed_artifact_paths[]` / `device_verification_*` | `delivery_record_path`、`status`、`next_action` |
| `POST /internal/v1/deliveries:follow-up` | `gateway/ui`、内部运维页 | 对已存在 delivery record 的 run 写入 release follow-up | `run_id`、`owner_id`、`status`、可选 `summary` / `evidence_paths[]` | `follow_up_record_path`、`status` |

接口约束：

- `POST /internal/v1/deliveries:record` 负责维护 `reports/delivery-record.json`，并在需要时同步生成 `reports/device-verification.json`；device verification 当前允许 `pending`、`passed`、`failed` 三种状态，并继续通过 public `delivery_context.device_verification`、job events、notifications 投影给 UI。
- `POST /internal/v1/deliveries:follow-up` 只能在该 `run_id` 已存在 delivery record 时调用；接口会写入 `reports/release-follow-up.json`，并把 `monitoring`、`stable`、`issue_detected` 等 follow-up 状态继续投影到 public `delivery_context.release_follow_up`、job events 与 notifications。
- `POST /api/v1/notifications/{notification_id}:ack` 负责把通知标记为已确认，并回写 `acknowledged`、`acknowledged_at`。
- `POST /api/v1/notifications:rebuild` 负责重建通知快照，并保留既有 ack 状态。
- 成功 run 需要生成 `reports/review-bundle.metadata.json`、`reports/review-bundle.md` 与 `reports/handoff-checklist.md`，并通过 artifact manifest 暴露。
- `POST /internal/v1/reviews:prepare` 按 `run_id` 重放生成 review / handoff 产物，并回写 artifact manifest 与 metrics。
- `POST /internal/v1/deliveries:record` 负责写入 `reports/delivery-record.json`，重新索引 artifact manifest，并把交付状态投影到 public job detail、events 与 notifications；`GET /api/v1/jobs/{job_id}` 返回的 `delivery_context` 应与对应 delivery event / notification 的 `delivery_status` 与 `suggested_action` 保持一致。

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
