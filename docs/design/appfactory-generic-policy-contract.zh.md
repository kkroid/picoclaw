# AppFactory Generic Policy Contract

> 状态：Active
>
> 更新时间：2026-04-25
>
> 本文档收口 generic open-lite 的 binding-surface 语义表、repair taxonomy 和 policy registry 契约，服务 `appfactory-architecture-evolution-todo.zh.md` 的 P2-P4。

## 0. 当前边界

- `weight-tracker` 只作为 generic fixture，用于证明 generic `/jobs` 公共主链可跑；它不再反向定义公共 binding、surface、repair 语义。
- bookkeeping 与 relation-rich completed 样本只作为防回退护栏，不再覆盖 generic 主结论。
- 新的公共规则不得再以单个 live 文件名、单个实体名或单一样本拓扑命名。
- policy registry 只表达 generic 公共交集；flutter-open-lite 模板私有 canonicalize / normalize 继续留在 template-private repair policy，relation-rich / inventory 私有策略继续独立维护。

## 1. Binding-Surface 语义表

### 1.1 核心公共 binding

| 公共 binding / surface | `surface_refs` | 典型路径类别 | generic 允许触达范围 | 当前 template-private 例子 | 默认主策略 |
|---|---|---|---|---|---|
| `surface-overview` | `surface-overview` | `view` `controller` `model` | `lib/views/*.dart` `lib/controllers/*.dart` `lib/models/*.dart` 中的概览承载单元 | `open-lite` 的 `home_page.dart` / `home_controller.dart` / `dashboard_summary.dart`，`finance-lite` 的 overview 槽位 | `emit` |
| `surface-collection` | `surface-collection` | `view` `controller` | 列表页与列表控制器路径 | `record_list_page.dart` / `record_list_controller.dart`，`entry_list_page.dart` / `entry_list_controller.dart` | `emit` |
| `surface-mutation` | `surface-mutation` | `view` `controller` | 表单页与表单控制器路径 | `record_form_page.dart` / `record_form_controller.dart`，`entry_form_page.dart` / `entry_form_controller.dart` | `emit` |
| `surface-inspection` | `surface-inspection` | `view` | 详情页路径 | `record_detail_page.dart` 与 relation-rich detail 槽位 | `emit` |
| `app-entry` | 继承当前任务的 `surface_refs` | `app_entry` | `lib/main.dart` | `open-lite` / `finance-lite` 的 app-entry 槽位 | `emit` |
| `domain-copy` | 可为空；必要时继承当前任务的 `surface_refs` | `template_copy` `view` | `lib/template/*.dart` 与相关 `lib/views/*.dart` | `open_lite_copy.dart`、模板私有 copy helper | `emit`，fallback 到 template-private repair |
| `android-branding` | 无固定 `surface_refs` | `android_branding` `manifest` | `android/app/src/main/res/values/strings.xml` `android/app/build.gradle.kts` | open-lite / finance-lite branding 槽位 | `emit` |
| `widget-test` | 继承当前任务的 `surface_refs` | `widget_test` | `test/widget_test.dart` | 模板私有 widget test helper | `emit`，fallback 到 validation repair |
| `storage-boundary` | 通常不直接绑定单个 surface | `repository` `manifest` | `lib/repositories/*.dart` `pubspec.yaml` | `record_repository.dart`、`entry_repository.dart` | `generate`，fallback 到 template-private repair |

### 1.2 辅助公共 binding

| 公共 binding | 典型路径类别 | 说明 |
|---|---|---|
| `domain-model` | `model` | 领域模型与 schema 投影，不直接代表 UI surface。 |
| `flow-controller` | `controller` | 任务协调与控制流，不等价于具体 view 文件名。 |
| `interaction-surface` | `view` | 仅作为没有显式 `surface_refs` 时的中性公共回退，不允许冒充 `surface-overview` 等具体表面。 |
| `dependency-manifest` | `manifest` | 对 `pubspec.yaml` 之类依赖清单的公共语义占位。 |

### 1.3 路径类别定义

| 路径类别 | 含义 |
|---|---|
| `app_entry` | 应用入口与路由装配，例如 `lib/main.dart`。 |
| `view` | `lib/views/*.dart` 一类 UI 承载文件。 |
| `controller` | `lib/controllers/*.dart` 一类交互编排文件。 |
| `model` | `lib/models/*.dart` 一类领域模型文件。 |
| `repository` | `lib/repositories/*.dart` 一类本地持久化或数据访问边界。 |
| `template_copy` | `lib/template/*.dart` 一类模板私有 copy helper。 |
| `widget_test` | `test/widget_test.dart` 一类公共 widget test。 |
| `android_branding` | Android branding 资源与 gradle 配置。 |
| `manifest` | `pubspec.yaml` 等依赖/声明文件。 |

## 2. Repair Taxonomy

### 2.1 失败类别

| 失败类别 | 类型 | 典型证据 | 默认处置方向 |
|---|---|---|---|
| `schema_drift` | 代码结构 | patch JSON 结构不合法、schema repair 触发 | 先走 schema / patch repair，再决定是否升级模型 |
| `patch_parse` | 代码结构 | patch 解析失败、operation 形态损坏 | 走 patch repair，必要时升级模型 |
| `analyze` | 代码结构 | `flutter analyze` 报错 | 按 binding / surface / path-class 选 repair slice |
| `test` | 代码结构 | `flutter test` 失败 | 优先 widget-test / 相关 surface repair |
| `closure` | 代码结构 | helper 缺失、导入闭包断裂、引用未收口 | 按 path-class + helper guard 处理 |
| `scope_violation` | 代码结构 | patch 越界、allowed paths / owned paths 违规 | 立即收口到 policy guard rails |
| `semantic_conflict` | 代码结构 | 语义守卫命中、文案/路由/实体漂移 | 按 binding / surface 触发语义 repair |
| `model_request_failure` | 环境 | provider timeout、凭据异常、网络波动 | 只记录为环境阻塞，不进入 generic 代码修复 taxonomy 主路径 |

### 2.2 绑定到 binding / surface / 路径类别的第一版矩阵

| 失败类别 | 首选 binding / surface 维度 | 首选路径类别 | 备注 |
|---|---|---|---|
| `schema_drift` | 当前任务 `binding_refs` | 当前任务全部路径类别 | 不允许重新退回样本文件名分支。 |
| `patch_parse` | 当前任务 `binding_refs` | 当前任务全部路径类别 | 先修 patch 结构，再决定是否扩大 repair slice。 |
| `analyze` | `surface-*` / `app-entry` / `storage-boundary` / `domain-copy` | `app_entry` `view` `controller` `repository` `template_copy` `widget_test` | 由当前 task + 直接失败文件共同裁定。 |
| `test` | `widget-test` + 相关 `surface_refs` | `widget_test` `view` `controller` | widget test 不能再孤立为单文件语义。 |
| `closure` | 当前 task `binding_refs` | `controller` `view` `repository` `template_copy` | 需要 helper preservation guard。 |
| `scope_violation` | 当前 task `binding_refs` | 当前 task 全部路径类别 | 本质是 guard rail 命中，不是文件名命中。 |
| `semantic_conflict` | `surface-*` / `domain-copy` / `android-branding` | `view` `template_copy` `android_branding` | 语义冲突优先回到 binding-surface 语义表。 |
| `model_request_failure` | 无 | 无 | 只在 G0 环境层记录，不升级为代码前线。 |

### 2.3 当前 live 6 文件切片的表达方式

当前 live analyze 6 文件切片，不再被解释成“六个特殊文件规则”，而是被表达为以下矩阵示例：

| live 切片角色 | 对应路径类别 | 绑定到的公共语义 |
|---|---|---|
| `lib/main.dart` | `app_entry` | `app-entry`，并继承当前任务的 `surface_refs` |
| 两个 `lib/controllers/*.dart` | `controller` | `surface-collection` / `surface-mutation` / `flow-controller` |
| 两个 `lib/views/*.dart` | `view` | `surface-collection` / `surface-mutation` |
| `test/widget_test.dart` | `widget_test` | `widget-test`，并继承相关 `surface_refs` |

这个示例只证明 taxonomy 可以表达当前 live 问题，不把这些文件名升级为长期公共语义。

## 3. Policy Registry 契约

### 3.1 Go 层草案

Go 层第一版 contract 已落到 `pkg/appfactory/adapter/policy_registry.go`。当前只定义结构、枚举、校验与 generic open-lite draft registry，不直接替换现有 runtime 路由。

registry 入口字段包括：

- `binding_refs`
- `surface_refs`
- `path_classes`
- `failure_classes`
- `override_policy`
- `primary_generation_mode`
- `allowed_generation_modes`
- `fallback_mode`
- `upgrade_triggers`
- `guard_rails`

### 3.2 `override_policy` 的公共含义

| `override_policy` | 含义 |
|---|---|
| `emit` | 当前 binding 优先走 deterministic emit / canonical factory。 |
| `generate` | 当前 binding 允许模型生成，但必须受 path-class / failure-class / guard rails 约束。 |
| `manual` | 当前 binding 不允许自动代码修复，只允许人工或环境处置。 |

### 3.3 生成方式与 fallback

| 字段 | 允许值 |
|---|---|
| `primary_generation_mode` / `allowed_generation_modes` | `deterministic_emit` `model_generate` `template_private_repair` `validation_repair` `manual_review` |
| `fallback_mode` | `none` `upgrade_model` `template_private_repair` `validation_repair` `manual_review` |

### 3.4 Guard Rails

第一版公共 guard rails 统一表达以下限制：

- 必须遵守 `owned_paths`。
- 必须遵守 `allowed_paths`。
- 必须保留仍被引用的 helper。
- 禁止重新引入 legacy `screen_refs` 作为公共语义。
- 禁止把 template-private 路径提升为公共触发器。

### 3.5 结构校验要求

任何新 policy 上线前至少要满足：

- `policy_id` 全局唯一。
- `override_policy`、`generation_mode`、`fallback_mode`、`failure_class`、`path_class` 必须命中受控枚举。
- `primary_generation_mode` 必须包含在 `allowed_generation_modes` 内。
- `emit` / `generate` / `manual` 三类 `override_policy` 必须分别和 `deterministic_emit` / `model_generate` / `manual_review` 对齐。
- 任何 entry 至少要锚定一项匹配维度：binding、surface、路径类别、失败类别或 task type。

### 3.6 与 template-private `surface -> class/path` registry 的边界

public policy registry 负责决定“某类 binding / surface / path-class / failure-class 允许走哪条修复或生成路径”；它不负责决定具体应该落到哪个模板文件、哪个类名、哪个 constructor 形态。

后者属于 template-private `surface -> class/path` registry，当前约束如下：

- registry 的公共输入只能来自 `binding_refs`、`surface_refs`、`path_classes` 与模板扫描结果，不能重新退回 live 文件名驱动。
- registry 派生键建议稳定为 `binding_ref + surface_ref + path_class + template_role`，其中 `template_role` 只保留模板私有含义，不得上升为新的 public contract。
- canonical controller/view 生成、fallback-only normalize、widget test canonical 输出与启发式评分必须共用同一份 registry；否则 policy registry 上层已经 generic 化，runtime 仍会在下层重新长出默认 `record_*` / `home_*` 分支。
- public API、prepare artifact 与 runtime task contract 仍只暴露 binding / surface-first 语义；`resolved_path`、`resolved_class_name`、`template_role` 只能留在 template-private repair policy 或执行期 snapshot 内部。

#### 3.6.1 Template-Private Registry Snapshot 最小合同

| 字段 | 含义 | 约束 |
|---|---|---|
| `registry_key` | `binding_ref + surface_ref + path_class + template_role` 组成的稳定派生键 | 只能由 `TemplateSlotMap + template scan + workspace candidate detection` 派生，不允许人工写死到 public task contract |
| `binding_ref` | 当前 snapshot 归属的公共 binding | 必填；必须命中公共 binding-surface 语义表 |
| `surface_ref` | 当前 snapshot 对应的具体 surface | 对 UI surface 必填；只有 `template_copy` / `manifest` 一类非表面路径允许为空 |
| `path_class` | 当前条目对应的路径类别 | 必填；必须命中公共 `path_class` 枚举 |
| `template_role` | 模板私有角色，例如 `collection_view`、`mutation_controller` | 必填；只能停留在 template-private 层 |
| `resolved_path` | 当前条目应消费的具体 workspace 相对路径 | 只允许落在当前 binding 允许触达的目标目录范围内 |
| `resolved_class_name` | 当前路径承载的具体类名 | `view` / `controller` / `app_entry` 条目必填；`template_copy` / `manifest` 可为空 |
| `constructor_contract` | 当前条目可被 wiring / canonical / repair 依赖的构造参数、callback 和必填字段事实 | 只能来自模板扫描、workspace 代码或 prepare artifact，不允许 invent 不存在的参数 |
| `capability_flags` | 当前 surface 暴露的能力，例如是否支持 edit、delete、status、title、note | 只能来自 workspace/template 事实，不允许由样本命名推断 |
| `field_semantics` | 当前 surface 依赖的字段语义映射，例如 `primary_text`、`secondary_text`、`status`、`time`、`note`，以及 status 对应的 enum/copy API | 只能来自 workspace/template/prepare 事实，不允许按 `title/category/status` 样本名反推公共语义 |
| `fallback_mode` | 当前 `resolved_path` / `resolved_class_name` 的来源级别 | 必填；语义见 3.6.4 |
| `resolution_source` | 本次解析命中的证据来源与冲突裁决说明 | 执行期可选缓存字段；用于调试和冲突解释，不上升为 public API |

补充约束：`mutation_view.constructor_contract` 必须允许 `controllerParam` / `controllerType` 与 `repositoryParam` / `initialParam` 并存。前者服务 relation-rich canonical `FormPage` 向当前 custom view contract 的重写，后者继续服务 repository-driven fallback-only normalize、main wiring 与 edit capability 判定；任何 consumer 都不得假定 mutation view 只存在单一路径的构造合同。

补充约束：当前 P5.2/T4 的字段语义最小合同只允许先覆盖 `primary_text`、`secondary_text`、`status`、`time`、`note` 五类角色。`status` 条目如存在，可附带 `enum_type`、`copy_label_method_name`、`filter_label_keys` 等 template-private 子字段；它服务 no-filter list canonical、detail/list subtitle、copy helper 与后续 widget-test capability 判定。任何 consumer 都不得在合同已存在时重新通过 `record.title`、`record.category`、`record.status` 或固定 `statusLabel(...)` 名称猜测字段语义。

#### 3.6.2 输入源优先级

template-private registry 只能按以下优先级解释输入，不允许 consumer 自己重排：

| 优先级 | 来源 | 典型证据 | 说明 |
|---|---|---|---|
| P0 | 显式任务/规划证据 | `surface_refs`、`binding_refs`、`path_classes`、`registry_projection_hints`、direct task-bundle topology | 属于当前轮最强信号；一旦存在，不允许被 workspace/legacy fallback 覆盖 |
| P1 | workspace candidate detection | 当前 workspace 已存在的 view/controller/repository 文件、类名、constructor contract、import 对齐 | 用于确认当前 active surface 的 concrete path/class |
| P2 | likely custom path fallback | `task_collection_page.dart`、`task_form_page.dart`、`inspection_surface.dart` 这类路径/类名启发式 | 只用于无显式 ref 且 workspace 证据不足时的过渡判定 |
| P3 | legacy template fallback | `record_list_page.dart`、`record_form_page.dart`、`home_page.dart` 等模板默认命名 | 仅允许作为 template-private 兜底，不得反向宣布“当前 surface 已存在” |

补充规则：

- direct task-bundle topology 属于 P0 证据，不属于 `likely path`；凡 task 自己已明确点名的 collection/mutation/overview 组合，都必须先于 workspace 抑制逻辑解释。
- `resolution_source` 必须能解释当前 snapshot 究竟由哪一级证据产生，避免 prompt、topology、normalize 和 import prune 对同一 surface 使用不同来源。
- 执行期一旦已经选出 `resolved_path`，`workspace_candidate` 与 `legacy_template` 的边界必须按该最终路径是否真实存在于 workspace 判定，不能只看初始 primary candidate 带不带默认模板痕迹。

#### 3.6.3 冲突决策

当多个候选同时存在时，统一按以下顺序裁决：

1. 高优先级证据永远覆盖低优先级证据，P0 > P1 > P2 > P3。
2. 同优先级下，优先选择 workspace 已真实存在的文件，而不是仅靠命名猜测的候选。
3. 同优先级且都存在实体文件时，优先选择与当前 primary model、record param、repository param、controller type 对齐的候选。
4. 同优先级且 path/class 都可用时，优先选择 `constructor_contract` 更完整、能解释更多现有 wiring 事实的候选。
5. `legacy template fallback` 不能覆盖任何 explicit / workspace / likely custom 候选，也不能单独满足 topology、import ownership、widget-test ownership 或 “surface 已存在” 判定。
6. 对 task-driven consumer 而言，workspace overview 抑制只能压制 `likely custom path fallback`，不能反向推翻 direct task-bundle topology 已明确给出的 collection-root 结论。

#### 3.6.4 `fallback_mode` 语义与 consumer 义务

| `fallback_mode` | 含义 | 允许消费的 consumer |
|---|---|---|
| `explicit_surface` | 当前 `resolved_path/class` 由 P0 显式 surface/topology 证据锁定 | 所有 consumer 均可直接消费 |
| `workspace_candidate` | 当前条目由 workspace 真实文件/类/contract 确认 | 所有 consumer 可消费，但仍需遵守文件存在性与 ownership 约束 |
| `likely_path` | 当前条目仅由 likely custom path / class 启发式得到 | 仅允许 prompt、topology、create-entry、scoring、single-target fallback dispatch 等过渡 consumer 消费；main wiring、import prune、widget-test ownership 仍需额外 workspace 或 explicit 佐证 |
| `legacy_template` | 当前条目仅来自模板默认命名兜底 | 仅允许 canonical factory 或 template-private fallback repair 使用；不得拿来宣告 active surface、不得满足 topology、不得保留 import ownership |
| `unresolved` | 当前 surface 没有足够证据选出 concrete path/class | consumer 必须 no-op、保留 placeholder 或显式继续等待更强证据 |

任何 consumer 只要跨越了上述消费边界，就视为重新引入了隐藏的默认文件名语义。

执行补充：当前 open-lite registry snapshot 已经开始在运行时代码中落地 `fallback_mode` / `resolution_source`；后续任何 consumer 迁移都必须直接消费这两个字段，而不是重新各自推断默认 path/class 来源。

## 4. 当前结论

- P2-P4 的第一版 contract 已经具备仓库内落点：设计文档看本文，Go 结构看 `pkg/appfactory/adapter/policy_registry.go`。
- 下一阶段进入 P5-P7 时，`builder_runtime.go` 与 `builder_runtime_open_lite_private.go` 的公共规则迁移，必须以本文的 binding / surface / path-class / failure-class 组合为边界；而 template-private 的 concrete path/class 解析，应统一下沉到同一份 `surface -> class/path` registry，并严格遵守 3.6.1-3.6.4 的字段、优先级、冲突决策和 `fallback_mode` 语义，而不是继续在 helper 内分散硬编码。
- 当前 P5.2/T4 的下一刀必须先冻结 `field_semantics` 最小合同，再迁 no-filter list canonical 与 copy/status 语义；未完成前，不再接受字段级 helper 症状修补进入 generic 主线。
- 任何无法被本文 taxonomy 与 policy registry 表达的新 live 修复，都不应继续直接进入 generic 主线。