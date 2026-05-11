# OneAppFactory 断兼容重构清单

> 状态：R0 baseline inventory
>
> 日期：2026-05-10
>
> 基线 commit：`47e0a2e176f32ba634891b13eac8e1746554d90d`
>
> 基线分支：`feature/app-factory-v1`

## 1. 基线状态

### 1.1 工作区状态

R0 开始前 `git status --short`：

```text
 M docs/design/appfactory-architecture-evolution.zh.md
?? docs/design/appfactory-extraction-implementation-plan.zh.md
```

以上两项视为本轮重构输入或用户已有改动，重构过程不回滚。

### 1.2 基线验证

| 检查 | 命令 | 结果 |
|---|---|---|
| AppFactory 包测试 | `go test ./pkg/appfactory/... -count=1 -timeout 300s` | 通过 |
| 旧 CLI 构建 | `make build` | 通过，产物 `build/oneappfactory-linux-amd64` / `build/oneappfactory` |
| 旧 launcher 构建 | `make build-launcher` | 通过，产物 `build/oneappfactory-launcher` |
| 文件清单 | `rg --files cmd web pkg docker config scripts > /tmp/oneappfactory-files.txt` | 872 条 |
| Go 依赖清单 | `go list -deps ./pkg/appfactory/... ./web/backend` | 333 条，stderr 为空 |
| live `/jobs` 回归 | `APPFACTORY_JOBS_TEMPLATE_ID="flutter-open-lite" bash scripts/run-appfactory-jobs-regression.sh --api-base http://127.0.0.1:18800 --requirement "读书笔记，记录书名进度和摘抄" --title "/jobs oneappfactory smoke" --timeout-seconds 3600 --output-root /tmp/oneappfactory-jobs-test` | 通过：`script_status=passed`、`job_status=completed`、`manual_equivalent=true`、`runtime_mode=builder_runtime` |

## 2. Keep

保留对象是 OneAppFactory 主链和当前验证已经覆盖的 AppFactory 能力。

| 区域 | 保留内容 | 依据 |
|---|---|---|
| `pkg/appfactory/adapter` | builder runtime、runner、deterministic repair / emit 相关主链 | AppFactory 包测试通过，属于 `/jobs -> builder/runtime` 主链 |
| `pkg/appfactory/prepare` | PRD / DomainModel / builder-input 生成 | 主链第一跳，一句话需求到结构化制品 |
| `pkg/appfactory/runs` | build run 状态、持久化、执行记录 | `/internal/v1/build-runs` 依赖 |
| `pkg/appfactory/builders` | builder 注册、worker 分配 | `/internal/v1/builders` 与 orchestrator 依赖 |
| `pkg/appfactory/emitter` | deterministic emitter | 架构终态要求保留 |
| `web/backend/api/appfactory_internal.go` | `/api/v1/jobs`、`/api/v1/prds`、`/api/v1/templates`、internal builder/run API | OneAppFactory 公共与内部 API 主面 |
| `web/backend/api/appfactory_orchestrator.go` | public job orchestrator | launcher 和 CLI orchestrator 仍需 |
| `web/backend/api/log.go` | 服务与 job 日志入口 | OneAppFactory 管理台日志页需要 |
| `web/backend/api/launcher_config.go` | launcher 自身配置 | OneAppFactory launcher 仍需端口和访问控制 |
| `web/frontend/src/routes/jobs.tsx` | jobs/dashboard 主页面 | Web 首页目标入口 |
| `web/frontend/src/components/jobs/**` | jobs 组件 | Web 主体验 |
| `examples/appfactory/**` | 回归样例与 fixture | live / prepare 回归证据 |
| `scripts/*appfactory*` | AppFactory 验证、回归、告警脚本 | R8 回归和运维检查 |
| `docker/Dockerfile.appfactory-builder` | Flutter/Android builder 镜像 | builder/runtime 执行环境 |

## 3. Migrate

以下内容不是兼容层，但 OneAppFactory 仍需要其中的最小子集。迁移时只保留被 AppFactory 或 launcher 真实引用的部分。

| 区域 | 迁移策略 |
|---|---|
| `cmd/oneappfactory/internal/appfactory` | 迁移到 `cmd/oneappfactory/internal/appfactory`，去掉 `appfactory` 二级命令外壳，保留 `prepare`、`run-requirement`、`run-executor`、`orchestrator` |
| `cmd/oneappfactory/internal/version` | 迁移为 `oneappfactory version`，文案改为 OneAppFactory |
| `cmd/oneappfactory/internal/helpers.go` | 重建为 OneAppFactory helper，默认目录改为 `~/.appfactory`，环境变量改为 OneAppFactory 语义 |
| `web/backend/main.go` | 拆成 `cmd/oneappfactory-launcher` 或迁移 launcher 入口，删除 gateway auto-start 和 pico channel 初始化 |
| `web/backend/api/config.go` | 裁剪为 provider/runtime/jobs/builder 配置子集，不保留通用助手配置产品面 |
| `pkg/config` | 先保留构建和 provider 所需字段，R5 再裁剪 agents/channels/tools/skills/MCP/ASR/TTS/voice/cron/gateway |
| `pkg/providers` | 只保留 builder-runtime LLM 调用需要的 provider 子集 |
| `pkg/envfile` | 仅用于 AppFactory 环境变量加载 |
| `pkg/fileutil`、`pkg/logger`、`pkg/httpx` | 保留被 AppFactory / web backend 真实引用的最小工具子集 |
| `config/config.example.json` | 投影为 `config/oneappfactory.example.json`，字段后续按 R5 裁剪 |
| `Makefile` | `build` / `build-launcher` 切到 OneAppFactory 产物，保留 AppFactory 验证目标 |

## 4. Remove

第一轮可删除或停止引用的对象如下；删除顺序按 R2/R3/R4 分批执行，避免前端、router、Go 包族失败面叠加。

### 4.1 Web 前端旧入口

| 类型 | 文件 |
|---|---|
| routes | `web/frontend/src/routes/agent.tsx` |
| routes | `web/frontend/src/routes/agent/skills.tsx` |
| routes | `web/frontend/src/routes/agent/tools.tsx` |
| routes | `web/frontend/src/routes/channels/$name.tsx` |
| routes | `web/frontend/src/routes/channels/route.tsx` |
| routes | `web/frontend/src/routes/credentials.tsx` |
| routes | `web/frontend/src/routes/models.tsx` |
| chat home | `web/frontend/src/routes/index.tsx` 中的 OneAppFactory chat 首页 |
| API clients | `web/frontend/src/api/pico.ts`、`sessions.ts`、`channels.ts`、`gateway.ts`、`oauth.ts`、`skills.ts`、`tools.ts`、`models.ts` 中不再被 config/jobs 使用的部分 |
| stores/hooks | `web/frontend/src/store/chat.ts`、`gateway.ts`、`hooks/use-pico-chat.ts`、`use-session-history.ts`、`use-sidebar-channels.ts`、`use-chat-models.ts`、`use-gateway.ts`、`use-gateway-logs.ts` 中非 AppFactory 引用 |

### 4.2 Web 后端旧 API

| 类型 | 文件 |
|---|---|
| pico websocket/chat | `web/backend/api/pico.go`、`pico_test.go` |
| gateway lifecycle | `web/backend/api/gateway.go`、`gateway_host.go`、`gateway_test.go`、`gateway_host_test.go` |
| session history | `web/backend/api/session.go`、`session_test.go` |
| OAuth | `web/backend/api/oauth.go`、`oauth_test.go` |
| model management | `web/backend/api/models.go`、`models_test.go`、`model_status.go`、`model_status_test.go` |
| channels | `web/backend/api/channels.go` |
| skills/tools | `web/backend/api/skills.go`、`skills_test.go`、`tools.go`、`tools_test.go` |
| Weixin/WeCom | `web/backend/api/weixin.go`、`weixin_test.go`、`wecom.go` |
| autostart | `web/backend/api/startup.go`、`startup_test.go`，除非 OneAppFactory launcher 明确需要保留 |

### 4.3 Go CLI 与包族

| 类型 | 对象 |
|---|---|
| CLI | `cmd/oneappfactory`，待 `cmd/oneappfactory` 验证后删除 |
| TUI | `cmd/oneappfactory-launcher-tui` |
| 明显非 AppFactory 包族 | `pkg/agent`、`pkg/asr`、`pkg/tts`、`pkg/voice`、`pkg/channels`、`pkg/commands`、`pkg/cron`、`pkg/mcp`、`pkg/skills`、`pkg/tools`、`pkg/routing`、`pkg/session`、`pkg/gateway` |

### 4.4 Docker / scripts / config 旧产品面

| 类型 | 对象 |
|---|---|
| Docker compose | `docker/docker-compose.yml` 中 agent/gateway 服务、`docker/docker-compose.full.yml`、`docker/docker-compose.asr-tts.yml` |
| Dockerfile | `docker/Dockerfile.full`、`docker/Dockerfile.heavy`，以及非 OneAppFactory goreleaser 路径 |
| 旧默认配置 | `config/config.json` 作为主配置入口 |
| 旧默认目录 | `~/.appfactory`、`ONEAPPFACTORY_HOME`、`ONEAPPFACTORY_CONFIG` 默认路径语义 |
| 旧镜像名 | `oneappfactory/appfactory-builder:local`、`sipeed/oneappfactory:*` |

## 5. Unknown

以下包当前出现在 `go list -deps ./pkg/appfactory/... ./web/backend` 依赖图中，不能在 R4 前直接删除，需要先由 R3 router 裁剪和 `rg` 反向引用证明是否仍被 OneAppFactory 需要。

```text
github.com/sipeed/oneappfactory/pkg/credential
github.com/sipeed/oneappfactory/pkg/auth
github.com/sipeed/oneappfactory/pkg/commands
github.com/sipeed/oneappfactory/pkg/identity
github.com/sipeed/oneappfactory/pkg/media
github.com/sipeed/oneappfactory/pkg/channels
github.com/sipeed/oneappfactory/pkg/channels/weixin
github.com/sipeed/oneappfactory/pkg/skills
```

处理原则：这些依赖若只由被删除 API 或旧配置页面引入，随 R3/R4 删除；若 provider/runtime/config 真实需要其中的小函数，则迁移最小子集到保留包，而不是保留整个 OneAppFactory 包族。

## 6. 第一批执行边界

### Batch 2 / R1

- 新增 `cmd/oneappfactory`，顶层只挂 `prepare`、`run-requirement`、`run-executor`、`orchestrator`、`version`。
- 新增 `cmd/oneappfactory-launcher`，以 OneAppFactory 文案启动 web backend，默认配置路径切到 `~/.appfactory/config.json`。
- Makefile 主构建目标切到 `build/oneappfactory` 与 `build/oneappfactory-launcher`。
- 新增 `config/oneappfactory.example.json` 初稿。

### Batch 3 / R2

- 根路由改为 jobs/dashboard。
- Sidebar 删除 chat、model、channels、agent、skills、tools、credentials。
- 删除或停止引用旧 routes、API clients、stores、hooks。

### Batch 4 / R3

- `web/backend/api/router.go` 改为 AppFactory-only router。
- 删除 `EnsurePicoChannel` / `TryAutoStartGateway` 启动逻辑。
- 删除非 AppFactory API 注册和对应文件测试。

## 7. R0 结论

R0 基线验证已通过，可以进入 R1 新入口迁移。

- 当前 launcher 使用仓库 `config/config.json`，`preflight_uses_user_home_config=false`。
- `/jobs` live regression 完成于 `20260510T123946Z`，job `job-jobs-ui-regression-20260510T123946Z`，run `81351fab-524a-4bf8-935d-ff3ffb76989f`。
- 主链为真实 `builder_runtime`，非 probe-only；生成路径覆盖 Flutter workspace 主文件。
- baseline 仍出现 `auto_repair_observed=true`（`test_repair`），后续 R8 最终矩阵需要继续压到 `auto_repair_observed=false`。
- 旧 launcher 启动时仍自动 `EnsurePicoChannel` / `TryAutoStartGateway`，这是 R3 必删启动副作用。
