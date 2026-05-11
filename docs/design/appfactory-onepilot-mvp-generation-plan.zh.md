# OnePilot MVP 生成收口计划

> 状态：In Progress
>
> 日期：2026-05-11
>
> 目标：执行本任务列表后，AppFactory 应能基于 OnePilot App PRD 与协议文档生成真实的 Flutter OnePilot 手机 App MVP，而不是再次生成可编译的 open-lite generic 通用记录壳。

---

## 1. 当前状态结论

最新 OnePilot `/jobs` 回归已经证明一件事：AppFactory 的生成链路可以完成 `compile -> create job -> deterministic emit -> flutter analyze -> flutter test -> release apk build`。

但该结果不能视为 OnePilot MVP 成功。当前生成物只是 `flutter-open-lite` generic skeleton：

- domain model 被压成 `通用记录` 与 `概览摘要`。
- persistence contract 被压成 `local-hive`。
- capability flags 只有 `navigation`、`local-storage`、`summary-card`、`list`、`detail`、`delete-record`、`filter` 等 open-lite 能力。
- Flutter 代码没有 `ApiClient`、`WsClient`、`Project`、`Conversation`、`Message`、`FileNode`、连接配置页、对话页、文件页或急停接口。
- `pubspec.yaml` 没有 `http`、`web_socket_channel`、`flutter_markdown`、`shared_preferences`、`provider`。

因此当前状态应标记为：

> 构建链路通过，产品语义失败。该 run 是 AppFactory deterministic 基线证据，不是 OnePilot App MVP 交付证据。

---

## 2. 问题点

| 优先级 | 问题 | 证据 | 影响 |
|---|---|---|---|
| P0 | OnePilot 语义在 compile 阶段丢失 | 输入 requirement 保留 REST/WS/Project/Conversation/File，生成 PRD 变成通用记录、本地持久化、概览/列表/详情 | 后续 emitter 即使稳定，也只会稳定生成错误 app |
| P0 | 离线/本地优先约束被错误继承 | generic profile 与 recognition 逻辑会注入“不把远端 API 作为默认前提”“默认离线单机运行”“不引入实时通信”等约束 | OnePilot 这类协议客户端需求会被 planning 阶段提前判成不需要网络/服务器 |
| P0 | 生成 App 不是 OnePilot 协议客户端 | 无网络客户端、无协议模型、无连接配置、无对话/文件页面 | App 不能连接 PC OnePilot Server，不能完成 PRD 核心流程 |
| P0 | acceptance 假阳性 | semantic checks 主要 grep generic 字段；widget test 只验证内存记录浏览 | `analyze/test/build apk` 通过不能证明产品需求通过 |
| P1 | generic 运行时仍有风险 | production main 使用 Hive custom object，但当前生成代码未证明 `Hive.initFlutter()` 与 adapter 注册 | 真机启动可能在持久化初始化处失败 |
| P1 | UI 有空按钮和未实现能力 | create action 是 no-op；状态筛选、删除确认不完整 | 即使作为 generic app 也不是完整 CRUD |
| P1 | 平台目标不完整 | PRD 提到优先 iOS，当前只生成 Android 工程/APK | 后续需要决定 iOS 是否纳入 MVP 验证闭环 |

---

## 3. 成功标准

执行完本计划后，同一份 OnePilot PRD/协议输入必须满足以下条件：

1. `domain-model.json` 不再出现固定 `通用记录` 作为主实体，而应包含：
   - `Project`
   - `Conversation`
   - `Message`
   - `MessageSegment`
   - `FileNode`
   - `ConnectionSettings`
2. `capability_flags` 至少包含：
   - `rest-api`
   - `websocket-streaming`
   - `connection-settings`
   - `project-switching`
   - `conversation-list`
   - `streaming-chat`
   - `emergency-stop`
   - `file-browser`
   - `read-only-file-preview`
3. `persistence_contract` 不应是业务数据 `local-hive`，而应表达：
   - 只本地保存连接设置、可选 token、上次 project/conversation。
   - Project/Conversation/Message/FileNode 主要来自 REST/WS，不做业务离线数据库。
4. `task-allocation.json` 必须包含协议客户端任务：
   - models
   - `ApiClient`
   - `WsClient`
   - connection/settings state
   - project state
   - conversation state
   - file state
   - connection page
   - main shell with tabs/project drawer
   - conversation list/detail
   - file browser/preview
   - widget/unit tests
5. 生成 Flutter 工程必须包含并使用 PRD 指定依赖：
   - `http`
   - `web_socket_channel`
   - `flutter_markdown`
   - `shared_preferences`
   - `provider`
   - `pubspec.yaml`、`main.dart`、`app.dart`、route/provider bootstrap 必须实际接入这些依赖，而不是只声明包名。
   - Android 构建必须限制为 `arm64-v8a` 单 ABI，不打包 `armeabi-v7a`、`x86` 或 `x86_64`。
   - iOS 平台目标必须记录为 iPhone 13 及以上 64 位设备；本轮 iOS build 仍不作为 gate。
6. acceptance 不能只验证 build。必须能拦住以下错误：
   - 缺 `GET /system/info`。
   - 缺 `GET /projects`。
   - 缺 `GET /conversations`。
   - 缺 `POST /conversations/{id}/turns`。
   - 缺 WebSocket subscribe/delta handling。
   - 缺 `POST /emergency-stop`。
   - 缺 files tree/read。
   - Android build config 缺少 `arm64-v8a` 单 ABI 约束，或显式包含 `armeabi-v7a`、`x86`、`x86_64`。
   - 出现 PRD 明确排除的项目新增/删除/启动/停止、文件搜索、扫码入口；文本文件保存是 MVP 允许能力。
   - 生成 PRD 或 manual constraints 仍声明“默认离线单机运行”“不把远端 API 作为默认前提”“不引入实时通信”。
7. live `/jobs` 通过后，必须同时满足：
   - `flutter analyze` 通过。
   - `flutter test` 通过。
   - Release APK build 通过。
   - semantic/protocol acceptance 通过。
   - generated source keyword scan 能看到 OnePilot 协议模型、客户端和页面。
   - App 默认入口不是 Flutter counter demo，且能在无真实服务端时进入连接配置或可恢复空状态。
   - Android release 允许局域网 HTTP，且仅打包 `arm64-v8a`。
   - iOS 侧保留本地网络/ATS 配置任务和 iPhone 13+ 64 位目标，即使本轮不把 iOS build 作为 gate。

---

## 4. 任务列表

### M0：冻结当前基线

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T0.1 | 提交当前 deterministic/topology/Gradle 环境分类修复 | 一个只包含当前稳定性修复的 commit | `go test ./pkg/appfactory/adapter ./pkg/appfactory/emitter ./pkg/appfactory/prepare -count=1` |
| T0.2 | 标记 OnePilot latest run 为 false-positive baseline | 文档或回归记录说明“构建链路通过，产品语义失败” | 复查 latest run 的 PRD/domain-model/task-allocation |
| T0.3 | 保留 OnePilot PRD/协议输入为固定 fixture | `workspace/appfactory/onepilot-mvp/requirement.md` 或测试 fixture | 后续每轮 compile 都使用同一输入做差异对比 |

退出标准：当前已通过的 generic deterministic 修复不再和 OnePilot 协议客户端改造混在一个 commit 里。

### M1：协议客户端需求识别

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T1.1 | 增加 protocol-client 识别 | prepare 能识别 REST、WebSocket、远端服务、host/port/token、项目/对话/文件 | OnePilot compile 不再走 generic open-lite 默认 profile |
| T1.2 | 增加 protocol-client capability classifier | 从 requirement 中抽取协议客户端能力，不按 App 名称分支 | OnePilot fixture 包含 `rest-api`、`websocket-streaming` 等能力；非 OnePilot fixture 不继承 OnePilot 专有能力 |
| T1.3 | 为远端协议需求阻断 local-record fallback | 当需求包含 REST/WS/endpoint 时，禁止降级为 `通用记录/local-hive` | 单元测试证明 OnePilot 输入不会生成 `entity-record` 主模型 |
| T1.4 | 收窄离线/本地优先约束 | `generic_domain_profiles` 与 `recognition` 中的离线、本地、无远端 API、无实时通信约束只对 generic local app 生效 | OnePilot artifacts 不再包含这些约束；generic local fixtures 仍保留离线约束 |

退出标准：compile artifacts 已能表达“远端协议客户端”，即使 emitter 还没有实现完整代码，也不能再输出 generic record PRD。

### M2：协议客户端领域模型与协议合同

这一阶段不能把 `Project`、`Conversation`、`FileNode` 等 OnePilot 名词写进通用识别逻辑。M2 要做的是定义可复用的 protocol-client artifacts：实体来自输入协议，endpoint 来自输入协议，实时事件来自输入协议，持久化策略来自输入协议。OnePilot 只作为第一个 high-signal golden fixture，用来证明这套合同能承载复杂协议客户端。

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T2.1 | 定义 protocol domain contract schema | entity、field、relationship、source、read/write ownership、display hints | OnePilot golden 包含 `Project`、`Conversation`、`Message`、`FileNode`；非 OnePilot fixture 不出现这些实体 |
| T2.2 | 定义 endpoint contract schema | endpoint id、method、path params、query、body、response envelope、required/excluded 标记 | OnePilot golden 覆盖 required/excluded endpoints；非 OnePilot fixture 能生成自己的 endpoint set |
| T2.3 | 定义 realtime contract schema | transport、subscribe/unsubscribe、event type、payload mapping、reconnect/replay policy | OnePilot golden 覆盖 delta/turn/status 等事件；REST-only fixture 不被强制生成 WebSocket |
| T2.4 | 定义 client persistence contract schema | 本地只保存 client settings、auth/token、last selection、cache policy，不默认业务 Hive | OnePilot golden 证明只保存连接设置和上次选择；generic local fixtures 仍可保留 Hive |
| T2.5 | 增加非 OnePilot 协议客户端 fixture | 一个 REST-only 或 REST+SSE 的小型远端业务样例 | golden 证明 protocol-client 分支不依赖 OnePilot 名称、endpoint 或事件 |
| T2.6 | 定义 runtime/dependency contract schema | 依赖包、app bootstrap、routes、provider wiring、project context、token header、platform network config | OnePilot golden 覆盖 `http`/`web_socket_channel`/`flutter_markdown`/`shared_preferences`/`provider`、`/api/v1`、`project_id`、可选 token |

退出标准：`domain-model.json` 和 planning artifacts 足以驱动任意协议客户端代码生成，不需要 emitter 再从自然语言里猜协议，也不存在按 `OnePilot` 名称进入的专用分支。

### M3：任务分配与模板槽位

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T3.1 | 增加 protocol-client task bundle | models、clients、state、pages、tests 任务 | `task-allocation.json` 按 protocol contract 生成目标路径；OnePilot 只是其中一个 fixture |
| T3.2 | 定义 Flutter protocol-client slot map | model/client/state/page/test slots | `template-slot-map.json` 不再只包含 home/list/form/detail，也不包含 OnePilot 专用槽位 |
| T3.3 | 增加 allowed/protected paths | 允许 `lib/models/**`、`lib/services/**`、`lib/providers/**`、`lib/pages/**`、`test/**`、`pubspec.yaml` | builder input 能写入协议客户端所需路径 |
| T3.4 | 决定模板策略 | 复用 `flutter-open-lite` 作为 base，还是新增 `flutter-protocol-lite` | 文档和 registry 明确，不让 protocol-client 偷用 generic slot，也不新增 OnePilot 专用模板 |
| T3.5 | 增加 app/platform slot | `main.dart`、`app.dart`、routes、theme、Android Manifest、Android arm64-only build config、iOS Info.plist/entitlements | builder input 能写入入口、路由、主题、局域网网络配置和平台目标约束 |
| T3.6 | 增加 fake protocol test slots | fake REST client/server、fake WS stream、widget harness、golden fixtures | 不依赖真实 OnePilot Server 也能验证协议行为 |

退出标准：builder input 能把协议客户端 app 拆成明确任务；OnePilot fixture 只是其中一个验证样例，而不是把所有代码塞进 generic surface fallback。

### M4：确定性生成协议客户端基础层

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T4.1 | 生成协议模型 | 按 domain contract 生成 Dart models | model unit tests 覆盖 OnePilot 与非 OnePilot JSON parse |
| T4.2 | 生成 `ApiClient` | base URL、headers/token、response envelope、error handling | fake HTTP client tests 覆盖 contract 中声明的 required endpoints |
| T4.3 | 生成 realtime client | 按 realtime contract 生成 WebSocket/SSE client；REST-only 合同不生成实时客户端 | fake stream tests 覆盖 OnePilot delta/replay_truncated；REST-only fixture 证明不强制 WS |
| T4.4 | 生成 settings store | SharedPreferences 读写 contract 声明的连接设置和 last selection | unit tests 覆盖保存/恢复/默认值 |
| T4.5 | 生成 app bootstrap 与 DI | `main.dart`/`app.dart`、Provider tree、route table、fake client 注入点 | widget tests 能替换 fake clients；启动页不是 counter demo |
| T4.6 | 生成 dependency 与平台配置 | `pubspec.yaml` 依赖、Android cleartext/LAN 配置、Android `arm64-v8a` 单 ABI、iOS 本地网络/ATS 与 iPhone 13+ 目标占位 | `flutter pub get`、Android release build、ABI source scan 与平台目标 source scan 通过 |

退出标准：网络和设置基础层可独立测试，不依赖真实 OnePilot Server。

### M5：生成状态管理与 UI surfaces

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T5.1 | 生成 state/providers | 按 domain/endpoint/realtime contract 生成连接状态、资源状态和操作状态 | provider tests 覆盖 loading/error/success；OnePilot fixture 生成 Connection/Project/Conversation/File 状态 |
| T5.2 | 生成连接配置页 | 按 connection contract 生成 host/port/token 等输入、保存和健康检查 | widget test 覆盖失败提示与成功进入主页 |
| T5.3 | 生成主页 shell | 按 navigation contract 生成连接状态、主资源切换、tab/drawer | widget test 覆盖 OnePilot 项目切换和对话/文件 tab；非 OnePilot fixture 生成自己的导航 |
| T5.4 | 生成集合与创建 surface | 按 endpoint contract 生成 list/refresh/create 等允许操作 | fake API widget test 覆盖 OnePilot conversation create；excluded create 不生成入口 |
| T5.5 | 生成主交互详情 surface | 按 endpoint/realtime contract 生成详情、发送、流式增量和中止动作 | widget test 覆盖 OnePilot user bubble、assistant delta、reasoning/tool_call/stop |
| T5.6 | 生成辅助资源 surface | 按 resource contract 生成 tree、breadcrumb、read-only preview、复制、图片缩放、大文件提示 | widget test 覆盖 OnePilot 目录进入、文本预览、图片 URL；无文件资源 fixture 不生成文件 tab |
| T5.7 | 生成消息渲染组件 | Markdown 气泡、reasoning 折叠块、tool_call 状态、tool_result 隐藏策略、usage 记录 | widget test 覆盖 agent_message/reasoning/tool_call/tool_result/turn_completed |
| T5.8 | 生成设置、生命周期和错误空状态 | 主题、连接地址、关于页、冷启动恢复、项目切换重连、网络错误/HTTP 错误/空状态 | widget/provider tests 覆盖无连接、连接失败、replay_truncated、last project/conversation 恢复 |

退出标准：不用真实服务端，fake client 注入下能跑完 OnePilot MVP 主流程。

### M6：强化验收门禁

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T6.1 | 增加 protocol semantic checks | 按 artifacts 检查 endpoint、client、models、providers、pages | 缺任何 required endpoint 会失败 |
| T6.2 | 增加 negative capability checks | 按 PRD excluded capabilities 禁止 endpoint/UI | OnePilot 出现项目新增/删除/启动/停止、文件搜索、扫码入口会失败；文本文件保存必须保留 |
| T6.3 | 增加 behavior widget tests | 按 contract 生成连接、资源切换、主交互、终止、资源浏览等行为测试 | `flutter test` 覆盖 OnePilot 协议行为；非 OnePilot fixture 覆盖自己的主流程 |
| T6.4 | 更新 jobs regression summary | 明确区分 build pass、semantic pass、protocol pass | latest summary 不再把 build-only 当完成 |
| T6.5 | 增加 fake protocol acceptance harness | fake REST/WS fixtures、错误 envelope、重连/replay、默认入口检查 | 无真实服务端时仍能证明 system/projects/conversations/files/WS 主流程；counter demo 会失败 |

退出标准：再次生成 generic record 壳时，acceptance 必须失败。

### M7：live `/jobs` 回归与产物 review

| ID | 任务 | 产出 | 验证 |
|---|---|---|---|
| T7.1 | 跑 OnePilot compile-only snapshot | 新 artifacts 与当前 false-positive artifacts 对比 | domain/task/acceptance diff 符合协议客户端预期 |
| T7.2 | 跑 full `/jobs` regression | 生成 OnePilot Flutter workspace 和 APK | latest json: completed + manual_equivalent + protocol_pass |
| T7.3 | 代码 review 生成产物 | review models/services/providers/pages/tests | 不允许 generic record/local-hive/no-op buttons |
| T7.4 | 记录回归结果 | 文档追加 run id、失败点、剩余风险 | 后续可复现 |

退出标准：生成物从源码结构到测试行为都能证明它是 OnePilot MVP，而不是 open-lite generic app。

---

## 5. 自我审核任务列表

### 5.1 初始任务列表的问题

初始列表是：

1. Freeze current regression baseline
2. Add protocol requirement recognition
3. Generate protocol contract artifacts
4. Plan protocol client tasks
5. Emit Flutter protocol client
6. Build protocol-client UI surfaces
7. Strengthen acceptance gates
8. Run real MVP regression

这组任务方向正确，但粒度不够，存在三个风险：

- `Emit Flutter protocol client` 太大，容易一次性塞进 services、providers、pages、tests，最后无法定位失败层。
- `Strengthen acceptance gates` 放得太靠后。至少 protocol semantic checks 应在 full UI 生成前就建立，否则又可能出现 build-only 假阳性。
- `Generate protocol domain model` 只说实体，不够约束 endpoint/WebSocket/persistence/excluded capabilities，无法保证“按协议生成”。

### 5.2 审核后的调整

本计划将任务拆成 8 个里程碑：

- M0 先冻结当前基线，避免把已有稳定修复和 OnePilot 大改混在一起。
- M1 先阻断 generic fallback。
- M2 把实体、endpoint、WS、persistence 都前移成 planning contract。
- M3 单独处理 task allocation 和 slot map，防止 emitter 没有执行结构。
- M4 先生成可独立测试的协议基础层。
- M5 再生成 UI 和 state。
- M6 建立语义/负向/行为验收，确保 fake app 不能再通过。
- M7 才跑 full live regression 和产物 review。

### 5.3 仍需决策的问题

| 决策 | 推荐 | 原因 |
|---|---|---|
| 是否新增模板 | 推荐新增 `flutter-protocol-lite` 或至少新增 protocol-client profile；不新增 OnePilot 专用模板 | `flutter-open-lite` 的 slot 语义是 generic record，不适合承载协议客户端；OnePilot 只能作为 fixture |
| iOS 是否纳入本轮验收 | 先不纳入 build gate，但 prepare/PRD 不应删除 iOS 优先信息 | 当前基础设施主要验证 Android；先保证 Android 可编译与协议语义正确 |
| 是否要求真实 OnePilot Server | 不要求 | PRD 明确网络错误应有空状态/重试入口；用 fake client/test server 验证行为更稳定 |
| 是否保留 Hive | 不保留业务数据 Hive | OnePilot MVP 只保存连接设置和上次选择，SharedPreferences 足够 |

### 5.4 最小可执行闭环

如果要控制第一轮范围，最低可执行闭环是：

1. 完成 M0。
2. 完成 M1 + M2，使 compile artifacts 正确。

---

## 6. 测试页功能对齐追加项（2026-05-11）

布局仍以手机 App PRD 为准，不复刻测试页的桌面左栏布局；测试页只作为功能语义参考。

需要补入 PRD 与生成逻辑的 MVP 行为：

- 项目切换必须清空当前 conversation、关闭旧 WebSocket、以新 project_id 重连、刷新 conversations、刷新 file tree，并隐藏当前文件预览。
- 进入 conversation detail 后 subscribe；退出 detail 或切换 conversation/project 时 unsubscribe。
- `turn_completed` 必须保存 usage，并在 UI 中至少以轻量状态或消息元信息展示。
- `conversation_update` 与 `instance_status` 必须更新用户可见状态。
- 急停按钮触发前必须确认。
- 若当前没有 conversation，发送消息可以自动创建普通 thread 并订阅新 conversation。
- 文件能力从只读扩展为“文本文件编辑保存”：`GET /files/read` 读取，`PUT /files/write` 保存；仍不做文件搜索、冲突检测、二进制编辑或项目生命周期管理。

需要同步加强验收：

- source scan 必须要求 `PUT /files/write` / `writeFile` 存在。
- negative capability scan 不再禁止 `writeFile`，但仍禁止 `searchFiles`、扫码和项目新增/删除/启动/停止。
- fake protocol widget/provider test 必须覆盖文本保存、项目切换联动、unsubscribe、usage 保存和急停确认。
3. 完成 M6.1 + M6.2，使 generic false-positive 立即失败。
4. 跑 compile-only regression。

只有当 artifacts 已经正确表达 OnePilot 协议客户端后，才进入 M4/M5 生成真实 Flutter app。

---

## 6. 禁止通过条件

后续任何 OnePilot `/jobs` run 只要出现以下情况，即使 APK build 成功，也必须判定失败：

- `domain-model.json` 主实体仍是 `通用记录`。
- `persistence_contract.mode` 仍是业务数据 `local-hive`。
- PRD、manual constraints 或 supporting assumptions 仍包含“默认离线单机运行”“不把远端 API 作为默认前提”“不引入实时通信”等 generic local 约束。
- `pubspec.yaml` 缺 `http` 或 `web_socket_channel`。
- 源码缺 `ApiClient` 或 `WsClient`。
- 源码缺 `Project` 或 `Conversation` 模型。
- 源码缺连接配置页。
- 源码缺对话详情发送消息路径。
- 源码缺 WebSocket delta 处理。
- 源码缺 emergency stop 调用。
- 源码缺文件 tree/read 路径。
- 出现项目新增/删除/启动/停止入口。
- 出现文件搜索或扫码连接入口；文本文件写入保存是 MVP 允许能力，不应作为负向能力拦截。

---

## 7. 离线/无网络约束审核

本轮排查确认：OnePilot 被压成 generic app，不只是因为文档里写过离线限制。相关约束已经固化在 prepare 代码和 generic profile 中，会进入 PRD、manual constraints、supporting assumptions 与 builder input。

已确认的来源包括：

- `pkg/appfactory/prepare/generic_domain_profiles.go`：默认 generic profile 写死 `local_storage`、`通用记录`，并声明“不接入登录、支付、广告、推送、地图、实时通信”“不把远端 API 作为默认前提”。
- `pkg/appfactory/prepare/generic_domain_profiles.json`：多个 generic profile 都声明“默认离线本地优先”或“不接入云同步/远端账号/外部 API”。
- `pkg/appfactory/prepare/recognition.go`：generic domain spec 注入 `NonGoals` 与 `SupportingAssumptions`，包括“不把远端 API 或自建服务器当作模板基础依赖”“默认离线单机运行”“本地持久化是当前默认数据源”。
- `pkg/appfactory/prepare/recognition.go`：generic persistence contract 固定为 `local-hive`，task bundle 固定生成本地 repository。

处理原则：

- 不全局删除这些约束。它们对“离线个人工具类 generic app”仍然是正确边界。
- 必须把它们从全局 fallback 收窄到 `generic-local-app` 分支。
- 当 requirement 出现 REST endpoint、WebSocket、server、host/port/token、远程项目/对话/文件等信号时，compile 必须切到 `protocol-client` 分支，且不得继承 generic local 的 NonGoals、ManualConstraints、SupportingAssumptions、PersistenceContract。
- `CommandProfile.NetworkPolicy = disabled` 不等同于“生成的 App 不能写网络代码”。它限制的是 prepare/acceptance 命令执行网络行为。后续 protocol acceptance 应优先用 fake client/test server 或静态语义检查验证，不要求真实 OnePilot Server 在线。

需要去掉的不是“所有离线文案”，而是“协议客户端需求仍继承 generic 离线文案”的路径。

---

## 8. 与现有文档关系

| 文档 | 关系 |
|---|---|
| `generic-deterministic-rollout.zh.md` | 证明 deterministic 链路能稳定 build，但不证明 OnePilot 产品语义 |
| `appfactory-generic-complexity-roadmap.zh.md` | 解决 generic app 从通用记录走向字段/行为保真；本文解决协议客户端类需求 |
| `appfactory-architecture-evolution.zh.md` | 本文是 M6 之后的具体 protocol-client 扩展计划 |
| `appfactory-generic-policy-contract.zh.md` | 本文的 negative capability 和 protocol acceptance 应复用其 policy registry 思路 |
