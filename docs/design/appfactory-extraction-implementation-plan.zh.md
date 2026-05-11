# OneAppFactory 断兼容重构实施方案

> 状态：Decisioned Execution Plan
>
> 日期：2026-05-10
>
> 目标：当前仓库不再作为 OneAppFactory 多功能产品继续演进，而是直接重构为 OneAppFactory-only 产品。该方案按“断兼容、重建产品壳、删除旧能力、逐批验证”的口径执行。

---

## 0. 审查结论

上一版方案方向正确，但仍偏保守，主要问题有四个：

| 问题 | 表现 | 新方案处理 |
|---|---|---|
| 兼容余地过多 | 仍给旧 `oneappfactory` / `oneappfactory-launcher` 留下继续存在的空间 | 不新增兼容入口；新 OneAppFactory 入口通过后，旧入口按批次删除 |
| 删除顺序偏后 | 非 AppFactory API、CLI、Web 页面在较后阶段才清理 | 第一阶段就切用户可见入口，第二阶段切 backend router，第三阶段删旧 CLI/包族 |
| 执行对象不够具体 | 只写“删除非 AppFactory 功能”，没有绑定真实目录和路由 | 本文把批次绑定到 `cmd/`、`web/frontend/src/routes`、`web/backend/api`、`pkg/`、`docker/`、`scripts/` |
| 验证闸门不够硬 | E0 偏记录，缺少每批必须执行的命令 | 每批都有最小验证、失败处理和退出标准；失败批次不扩大范围 |

本轮采用更激进的判断：

- 不考虑旧 OneAppFactory 用户入口兼容。
- 不把 `~/.appfactory` 作为 OneAppFactory 的默认目录或 fallback。
- 不保留 agent/chat/voice/channels/skills/MCP/tools/gateway 作为产品能力。
- 不以“先全仓大删再修编译”的方式执行；激进的是产品边界，不是无控制破坏。
- 每个阶段都必须能落成一个可提交、可验证的工作区状态。

## 1. 硬约束

### 1.1 产品边界

OneAppFactory 的唯一主链是：

```text
一句话需求 -> PRD/DomainModel/BuilderInput -> /jobs -> builder/runtime -> generated Flutter workspace -> analyze/test/build/live regression
```

除此之外的 OneAppFactory 个人助手能力都不是目标产品能力。

### 1.2 不兼容策略

以下内容不作为重构后的兼容承诺：

- `oneappfactory`、`oneappfactory-launcher`、`oneappfactory-launcher-tui` 二进制。
- `~/.appfactory` 默认目录。
- OneAppFactory Web Console 的 chat、channels、skills、tools、models、credentials、gateway 管理页面。
- `oneappfactory agent/auth/cron/gateway/model/onboard/skills/status/migrate` CLI 子命令。
- Docker 镜像 `sipeed/oneappfactory:*`、compose 服务 `oneappfactory-agent` / `oneappfactory-gateway`。
- 非 AppFactory API：pico websocket、gateway lifecycle、session history、OAuth/Weixin/WeCom、skills、tools、channels、通用 model management。

允许保留的不是“兼容层”，而是 OneAppFactory 仍然需要的基础能力，例如 provider 调用、配置读取、日志、文件工具、AppFactory job 通知。

### 1.3 验证纪律

每个批次必须满足：

- 批次开始前记录 `git status --short` 和本批目标。
- 批次只处理一个入口面或一个包族，不混入无关格式化。
- 批次结束后至少运行对应测试；涉及入口、配置或 router 时必须运行 build；涉及 `/jobs` 时必须运行 live regression。
- 如果验证失败，先修当前批次，不继续扩大删除面。
- 不为了让旧 OneAppFactory 能力继续可用而增加代码。

## 2. 目标终态

### 2.1 目录形态

目标目录应收敛为：

```text
cmd/
  oneappfactory/              # 主 CLI：prepare/run/orchestrator/version
  oneappfactory-launcher/     # 本地 Web 管理台入口
pkg/
  appfactory/                 # AppFactory 核心业务域
  config/                     # OneAppFactory-only 配置子集
  fileutil/                   # 最小文件工具
  providers/                  # builder-runtime 需要的 provider 子集
  envfile/                    # 仅保留 AppFactory 环境变量加载
  logger/                     # 服务日志
  httpx/                      # 如果 AppFactory/provider 仍使用则保留
web/
  backend/                    # OneAppFactory API server
  frontend/                   # OneAppFactory jobs/dashboard/config/logs 管理台
examples/
  appfactory/
docs/
  design/appfactory-*.zh.md
scripts/
  run-appfactory-*.sh
  verify-appfactory-*.sh
  check-appfactory-*.sh
  update-appfactory-*.sh
docker/
  Dockerfile.appfactory-builder
  docker-compose.appfactory.yml
config/
  oneappfactory.example.json
```

### 2.2 明确删除的目录和入口

以下目录和文件不应出现在最终产品面：

| 类型 | 删除对象 |
|---|---|
| CLI | `cmd/oneappfactory` 中除 AppFactory 命令可迁移代码外的全部旧入口；`cmd/oneappfactory-launcher-tui` |
| Web 前端 | `web/frontend/src/routes/agent*`、`channels*`、`credentials.tsx`、`models.tsx`、默认 chat 首页、非 AppFactory API client/hook/store |
| Web 后端 | `web/backend/api/pico.go`、`gateway.go`、`session.go`、`oauth.go`、`models.go`、`channels.go`、`skills.go`、`tools.go`、`weixin.go`、`wecom.go` 以及对应测试 |
| Go 包族 | `pkg/agent`、`pkg/asr`、`pkg/tts`、`pkg/voice`、`pkg/channels`、`pkg/commands`、`pkg/cron`、`pkg/mcp`、`pkg/skills`、`pkg/tools`、`pkg/routing`、`pkg/session`、`pkg/gateway` 等非 AppFactory 包 |
| Docker | `docker/docker-compose.yml` 中 agent/gateway 服务，`docker/docker-compose.full.yml`、`docker/docker-compose.asr-tts.yml`、full/heavy 非 AppFactory Dockerfile |
| 脚本/安装器 | macOS/Windows 安装器里 OneAppFactory launcher 文案和旧二进制名；非 AppFactory release/build 脚本 |
| 配置/数据 | `config/config.json` 作为主配置、`~/.appfactory` 默认路径、OneAppFactory workspace/skills 默认目录 |

### 2.3 保留或重建的能力

| 能力 | 目标处理 |
|---|---|
| `/api/v1/jobs`、`/api/v1/prds`、`/api/v1/templates` | 保留为 OneAppFactory 原生公共 API，不是兼容 OneAppFactory |
| `/internal/v1/builders`、`/internal/v1/build-runs`、orchestrator | 保留，作为 builder/runtime 内部协议 |
| `web/frontend/src/components/jobs/**` | 保留并改成首页主体验 |
| config/preflight | 重建为 OneAppFactory provider/runtime/config 页面，不保留通用助手配置 |
| logs | 保留服务与 job 运行日志，去掉 gateway/chat 语义 |
| provider 调用 | 裁剪到 builder-runtime 所需模型调用，不保留通用聊天模型管理产品面 |
| appfactory scripts | 保留并统一默认 `oneappfactory/builder:local`、`~/.appfactory`、`oneappfactory-launcher` |

## 3. 模块与命名决策

默认目标命名如下，除非后续用户明确改组织名：

| 项 | 目标值 |
|---|---|
| 产品名 | OneAppFactory |
| CLI | `oneappfactory` |
| Web launcher | `oneappfactory-launcher` |
| Go module path | `github.com/sipeed/oneappfactory` |
| 默认配置 | `config/oneappfactory.json` / `~/.appfactory/config.json` |
| 默认数据目录 | `~/.appfactory` |
| Docker builder image | `oneappfactory/builder:local` |
| Web 首页 | `/jobs`，根路径 `/` 直接展示 jobs/dashboard |

`/api/v1/jobs` 这类 API 路径可以保留，因为它们是 AppFactory 产品语义，不是 OneAppFactory 兼容语义。`/api/pico`、gateway、session、skills、tools、channels 等路径必须删除。

## 4. 可执行阶段

### R0：基线和删除清单

目标：在动刀前形成一份可复现基线，并产出第一版 `keep/remove/migrate/unknown` 清单。

执行：

- [ ] 记录当前 commit、branch、`git status --short`。
- [ ] 运行 `go test ./pkg/appfactory/... -count=1 -timeout 300s`。
- [ ] 运行 `make build && make build-launcher`。
- [ ] 启动当前 launcher，跑一条 `flutter-open-lite` `/jobs` live regression。
- [ ] 生成清单文档 `docs/design/oneappfactory-refactor-inventory.zh.md`。

清单最低内容：

```text
keep:     AppFactory 核心和已经验证的 jobs/runtime/emitter 链路
migrate:  config/fileutil/providers/envfile/logger/httpx 中被 AppFactory 真实引用的子集
remove:   非 AppFactory CLI/Web/API/pkg/docker/docs/scripts
unknown:  暂无法判断是否被 AppFactory 间接依赖的包，必须用 go list/rg 证明后再移动
```

推荐命令：

```bash
git status --short
go test ./pkg/appfactory/... -count=1 -timeout 300s
make build && make build-launcher
rg --files cmd web pkg docker config scripts > /tmp/oneappfactory-files.txt
go list -deps ./pkg/appfactory/... ./web/backend 2>/tmp/oneappfactory-go-list.err > /tmp/oneappfactory-go-list.txt
```

退出标准：基线验证通过；清单能明确第一批删除目标；如果 live regression 不通过，先修 AppFactory 主链，不进入 R1。

### R1：建立 OneAppFactory 新产品壳

目标：先让新名字的 CLI 和 launcher 可构建、可启动，再删除旧 OneAppFactory 壳。新壳不是兼容入口，是最终入口。

执行：

- [ ] 新增 `cmd/oneappfactory`，只注册 AppFactory 相关命令：`prepare`、`run-requirement`、`run-executor`、`orchestrator`、`version`。
- [ ] 从 `cmd/oneappfactory/internal/appfactory` 迁移必要命令实现，删除对 `internal.GetConfigPath()` 中 OneAppFactory 默认路径的依赖。
- [ ] 新增 `cmd/oneappfactory-launcher`，从 `web/backend` 启动 OneAppFactory Web 管理台。
- [ ] 把默认配置路径切为 `~/.appfactory/config.json`。
- [ ] Makefile 新增并切换主目标：`build` -> `oneappfactory`，`build-launcher` -> `oneappfactory-launcher`。
- [ ] 构建产物只输出 `build/oneappfactory` 和 `build/oneappfactory-launcher`。

验证：

```bash
go test ./cmd/oneappfactory/... ./pkg/appfactory/... -count=1 -timeout 300s
make build && make build-launcher
./build/oneappfactory --help
./build/oneappfactory-launcher -console -no-browser config/oneappfactory.json
```

退出标准：新入口构建和启动通过；后续验证不再依赖 `./build/oneappfactory-launcher`。

### R2：前端断舍离

目标：Web 管理台第一屏就是 jobs/dashboard，不再出现 OneAppFactory 控制台能力。

执行：

- [ ] 把 `web/frontend/src/routes/index.tsx` 改为 jobs/dashboard 入口，或直接把 `/` 路由指向 jobs 页面。
- [ ] 删除导航组：chat、model、channels、agent、skills、tools、credentials。
- [ ] 删除或停止引用以下页面：
  - `web/frontend/src/routes/agent.tsx`
  - `web/frontend/src/routes/agent/skills.tsx`
  - `web/frontend/src/routes/agent/tools.tsx`
  - `web/frontend/src/routes/channels/**`
  - `web/frontend/src/routes/credentials.tsx`
  - `web/frontend/src/routes/models.tsx`
  - 当前 chat 首页和相关 feature/store/hooks
- [ ] 保留并强化：`web/frontend/src/routes/jobs.tsx`、jobs components、config/preflight、logs。
- [ ] 清理 `routeTree.gen.ts` 和 i18n 中不再使用的导航文案。
- [ ] 删除前端非 AppFactory API client：channels、gateway、oauth、pico、sessions、skills、tools、models 中不再被 config/jobs 使用的部分。

验证：

```bash
cd web/frontend
pnpm test -- --run
pnpm build:backend
```

退出标准：前端 build 通过；根路径不再加载 chat；sidebar 只显示 OneAppFactory 任务、配置、日志等必要入口。

### R3：后端 router 切成 AppFactory-only

目标：后端只暴露 OneAppFactory API，删除 OneAppFactory Web Console 的控制面。

执行：

- [ ] 将 `web/backend/api/router.go` 的 `RegisterRoutes` 改为 AppFactory-only router。
- [ ] 保留：
  - `registerAppFactoryInternalRoutes`
  - OneAppFactory config 子集
  - launcher config 子集
  - logs/status 中不依赖 gateway/chat 的部分
- [ ] 删除路由注册：
  - `registerPicoRoutes`
  - `registerGatewayRoutes`
  - `registerSessionRoutes`
  - `registerOAuthRoutes`
  - `registerModelRoutes`
  - `registerChannelRoutes`
  - `registerSkillRoutes`
  - `registerToolRoutes`
  - `registerWeixinRoutes`
  - `registerWecomRoutes`
- [ ] 删除 `web/backend/main.go` 中的 `EnsurePicoChannel` 和 `TryAutoStartGateway`。
- [ ] 删除对应 API 文件和测试，或先迁移其中被 OneAppFactory config 需要的极小函数。

验证：

```bash
go test ./web/backend/... -count=1 -timeout 300s
make build-launcher
curl -sS http://127.0.0.1:18800/api/v1/notifications
```

退出标准：后端不再注册非 AppFactory API；`/api/v1/jobs`、`/api/v1/notifications` 和 internal builder/run API 仍可用。

### R4：删除旧 CLI、TUI 和非 AppFactory 包族

目标：从 Go 编译图中移除 OneAppFactory 个人助手域。

执行顺序：

1. 删除旧 CLI/TUI：`cmd/oneappfactory`、`cmd/oneappfactory-launcher-tui`。
2. 删除明显非 AppFactory 包族：`pkg/agent`、`pkg/asr`、`pkg/tts`、`pkg/voice`、`pkg/channels`、`pkg/commands`、`pkg/cron`、`pkg/mcp`、`pkg/skills`、`pkg/tools`、`pkg/routing`、`pkg/session`、`pkg/gateway`。
3. 对 `pkg/auth`、`pkg/credential`、`pkg/identity`、`pkg/memory`、`pkg/state`、`pkg/media`、`pkg/devices` 先用 `go list` 和 `rg` 证明是否被 OneAppFactory 真实需要；不需要即删，需要则迁移最小子集。
4. 裁剪 `pkg/config`：删除 agents、channels、tools、skills、MCP、ASR/TTS、voice、cron、gateway 等字段。
5. 裁剪 `pkg/providers`：只保留 builder-runtime LLM 调用所需 provider。

验证：

```bash
go list ./... >/tmp/oneappfactory-go-list-all.txt
go test ./... -count=1 -timeout 600s
make build && make build-launcher
```

退出标准：`go list ./...` 不再需要已删除包族；`cmd/oneappfactory` 不存在；AppFactory 主链仍可构建。

### R5：配置、状态目录和脚本重置

目标：OneAppFactory 不再读取 OneAppFactory 默认目录或完整旧配置。

执行：

- [ ] 新增 `config/oneappfactory.example.json`，只保留 AppFactory provider/runtime/jobs/builder 配置。
- [ ] 若仓库需要本地样例配置，新增 `config/oneappfactory.json`；避免混用旧 `config/config.json`。
- [ ] 默认目录统一为 `~/.appfactory`。
- [ ] 脚本默认值替换：
  - `oneappfactory/appfactory-builder:local` -> `oneappfactory/builder:local`
  - `$HOME/.appfactory/workspace/appfactory` -> `$HOME/.appfactory/workspace/appfactory`
  - `oneappfactory-launcher` -> `oneappfactory-launcher`
  - `oneappfactory_executor_probe.dart` -> `oneappfactory_executor_probe.dart`
- [ ] 不提供自动旧目录迁移；如确需保留历史数据说明，只写文档化手动导入步骤。

验证：

```bash
rg "oneappfactory|OneAppFactory|ONEAPPFACTORY|\.appfactory" config scripts pkg web cmd
go test ./pkg/appfactory/... ./web/backend/... -count=1 -timeout 300s
make build && make build-launcher
```

退出标准：默认运行和 regression 脚本不再依赖 `~/.appfactory` 或旧二进制名。

### R6：Docker、发布物和安装器重建

目标：发布链只构建 OneAppFactory。

执行：

- [ ] 保留并改名/更新 `docker/Dockerfile.appfactory-builder`。
- [ ] 新增 `docker/docker-compose.appfactory.yml`，只包含 OneAppFactory launcher/service 和 AppFactory builder 需要的挂载。
- [ ] 删除或归档：`docker/docker-compose.full.yml`、`docker/docker-compose.asr-tts.yml`、`docker/Dockerfile.full`、`docker/Dockerfile.heavy`。
- [ ] 重写 `docker/Dockerfile` 和 goreleaser Dockerfile，使其只构建 `oneappfactory` / `oneappfactory-launcher`。
- [ ] 更新 macOS/Windows 安装脚本中的名称、bundle id、输出文件和图标。

验证：

```bash
make build-appfactory-builder
docker compose -f docker/docker-compose.appfactory.yml config
make build && make build-launcher
```

退出标准：Docker/installer/release 路径不再构建 OneAppFactory agent/gateway/chat 产品。

### R7：Go module path 和全仓命名统一

目标：技术命名与产品名一致。

执行：

- [ ] 将 `go.mod` module 从 `github.com/sipeed/oneappfactory` 改为 `github.com/sipeed/oneappfactory`。
- [ ] 全仓 Go imports 同步替换。
- [ ] README、docs、package comments、Web title、i18n、Makefile help、Docker image 文案统一为 OneAppFactory。
- [ ] 删除或改写旧 OneAppFactory license/header 之外的产品描述。

验证：

```bash
go mod tidy
go test ./... -count=1 -timeout 600s
make build && make build-launcher
rg "github.com/sipeed/oneappfactory|OneAppFactory|oneappfactory|ONEAPPFACTORY" .
```

退出标准：除历史迁移说明、许可证或必须保留的第三方文本外，全仓不再出现 OneAppFactory 命名。

### R8：OneAppFactory 主链回归矩阵

目标：证明重构后产品不是“能编译”，而是 AppFactory 主链仍能完成生成。

最低矩阵：

- generic boolean/summary：打包清单或喝水打卡。
- numeric/summary：观影评分或训练时长。
- list-detail-form：读书笔记或课程作业。
- no-home/no-detail 负向拓扑：便签或简单计数。
- 当前 schema-driven widget test 的 packing-list 用例。

每个用例成功定义：

```text
script_status=passed
job_status=completed
manual_equivalent=true
auto_repair_observed=false
flutter analyze/test/build 均通过
```

退出标准：至少 5 条 live regression 通过，且生成产物、脚本输出和 Web UI 都使用 OneAppFactory 命名。

## 5. 第一轮推荐执行批次

为避免一次性修改过大，第一轮只执行到 R3，但每一步都应是可提交状态。

### Batch 1：R0 基线和清单

产出：`docs/design/oneappfactory-refactor-inventory.zh.md`。

验证：AppFactory Go tests、现有入口 build 仅作为删除前基线、1 条 live regression。

### Batch 2：R1 新入口

产出：`cmd/oneappfactory`、`cmd/oneappfactory-launcher`、Makefile 新 build 目标、`config/oneappfactory.example.json` 初稿。

验证：新入口 help/build/launcher smoke。

### Batch 3：R2 前端切换

产出：根路由 jobs 化，sidebar 只剩 OneAppFactory 入口，删除 chat/agent/channels/models/credentials 页面引用。

验证：frontend tests/build，launcher 打开首页为 jobs。

### Batch 4：R3 后端 router 切换

产出：AppFactory-only router，删除 gateway/pico/session/oauth/skills/tools/channels 注册。

验证：`go test ./web/backend/...`、launcher smoke、`/api/v1/notifications`、1 条 live regression。

完成 Batch 1-4 后，再进入 R4 大包族删除；否则容易把前端、router、CLI 和 shared package 四个失败面叠在一起。

## 6. 验证矩阵

### 6.1 每批通用检查

```bash
git diff --check
go test ./pkg/appfactory/... -count=1 -timeout 300s
```

### 6.2 入口相关批次

```bash
make build
make build-launcher
./build/oneappfactory --help
./build/oneappfactory-launcher -console -no-browser config/oneappfactory.json
```

### 6.3 Web 前端批次

```bash
cd web/frontend
pnpm test -- --run
pnpm build:backend
```

### 6.4 Web 后端批次

```bash
go test ./web/backend/... -count=1 -timeout 300s
curl -sS http://127.0.0.1:18800/api/v1/notifications
```

### 6.5 删除大包族后

```bash
go list ./...
go test ./... -count=1 -timeout 600s
make build && make build-launcher
```

### 6.6 live regression

```bash
APPFACTORY_JOBS_TEMPLATE_ID="flutter-open-lite" \
  bash scripts/run-appfactory-jobs-regression.sh \
  --api-base http://127.0.0.1:18800 \
  --requirement "读书笔记，记录书名进度和摘抄" \
  --title "/jobs oneappfactory smoke" \
  --timeout-seconds 3600 \
  --output-root /tmp/oneappfactory-jobs-test
```

## 7. 风险和处理

| 风险 | 触发点 | 处理 |
|---|---|---|
| 新入口还未成熟就删除旧入口 | R1/R4 | R1 必须先让 `oneappfactory` 和 `oneappfactory-launcher` 独立 build/smoke；R4 才删旧入口 |
| Web 删除页面但 API client 仍引用旧 endpoint | R2 | 前端 batch 必须跑 build；先删导航和 routes，再删 client/hook/store |
| 后端 router 删除影响 jobs orchestration | R3 | AppFactory routes 单独列白名单；删除 gateway/pico/session 时保留 orchestrator startup |
| shared 包误删 | R4 | 删除前用 `go list -deps` 和 `rg` 证明无 AppFactory 引用；未知包先进 `unknown` |
| 配置裁剪破坏 builder-runtime provider | R5 | provider/runtime 配置先加测试，再删旧字段 |
| 全仓 module path 替换引入大面积噪声 | R7 | 放到旧包族删除后执行；替换后只接受 gofmt/go mod tidy 必要变更 |
| live regression 失败但 Go tests 通过 | R8 | 以 live regression 为 AppFactory 主链最终闸门；失败先修 generator/runtime，不继续清理 |

## 8. 后续任务清单

### 立即执行

- [ ] R0-1：跑基线验证并记录结果。
- [ ] R0-2：创建 `oneappfactory-refactor-inventory.zh.md`。
- [ ] R0-3：确认第一批 remove 清单只包含无 AppFactory 反向引用的前端入口。
- [ ] R1-1：创建 `cmd/oneappfactory`。
- [ ] R1-2：创建 `cmd/oneappfactory-launcher`。
- [ ] R1-3：新增 OneAppFactory Makefile targets。

### 第一轮删除

- [ ] R2-1：根路由改为 jobs。
- [ ] R2-2：删除 sidebar 非 AppFactory 导航。
- [ ] R2-3：删除 chat/agent/channels/models/credentials 页面引用。
- [ ] R3-1：router 注册改为 AppFactory-only。
- [ ] R3-2：删除 `EnsurePicoChannel` / `TryAutoStartGateway` 启动逻辑。
- [ ] R3-3：删非 AppFactory API 文件和测试。

### 第二轮删除

- [ ] R4-1：删除 `cmd/oneappfactory`。
- [ ] R4-2：删除 `cmd/oneappfactory-launcher-tui`。
- [ ] R4-3：删除明显非 AppFactory `pkg/*` 包族。
- [ ] R5-1：裁剪配置 schema 并切到 `~/.appfactory`。
- [ ] R5-2：替换脚本默认二进制、镜像和数据目录。

### 最终收口

- [ ] R6-1：重建 Docker/installer/release 路径。
- [ ] R7-1：改 Go module path。
- [ ] R7-2：全仓品牌命名统一。
- [ ] R8-1：跑 5 条 OneAppFactory live regression。
- [ ] R8-2：`rg "OneAppFactory|oneappfactory|ONEAPPFACTORY|\.appfactory"` 残留审计。

## 9. 不做事项

- [ ] 不保留 `oneappfactory` / `oneappfactory-launcher` 作为发布入口。
- [ ] 不提供旧 CLI 子命令兼容。
- [ ] 不自动读取或迁移 `~/.appfactory`。
- [ ] 不保留非 AppFactory Web 页面作为隐藏入口。
- [ ] 不继续维护 ASR/TTS/voice/channel/MCP/skills/tools/agent/gateway 产品能力。
- [ ] 不在一个批次里同时改 module path、删 router、删 pkg 包族和改配置目录。

## 10. 成功定义

重构完成的判断标准不是“OneAppFactory 功能还能不能跑”，而是：

- `oneappfactory` 和 `oneappfactory-launcher` 是唯一主入口。
- Web 首页就是 jobs/dashboard。
- 后端 router 只暴露 OneAppFactory API。
- 默认目录是 `~/.appfactory`。
- Docker/Makefile/scripts 不再使用 OneAppFactory 命名。
- `go test ./...`、build、5 条 live regression 通过。
- 全仓 `OneAppFactory/oneappfactory/ONEAPPFACTORY/.appfactory` 只剩明确允许的历史说明或许可证上下文。
