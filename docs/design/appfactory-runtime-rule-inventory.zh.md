# AppFactory Runtime Rule Inventory

> 状态：Active
>
> 更新时间：2026-04-24
>
> 本文档盘点 `pkg/appfactory/adapter/builder_runtime.go` 与 `pkg/appfactory/adapter/builder_runtime_open_lite_private.go` 当前仍承载的运行时规则入口，服务 `appfactory-architecture-evolution-todo.zh.md` 的 P5。

## 1. Public Runtime 入口盘点

### 1.1 route / retry / upgrade 入口

`pkg/appfactory/adapter/builder_runtime.go` 当前的公共 repair 主入口集中在以下几类：

- route 选择与启动：`executeBuilderRuntimeEditWithFailureContext(...)`
- 瞬时模型重试：`generateBuilderRuntimePatchWithProgressAndTransientRetry(...)`
- upgrade 判定：`shouldRetryBuilderRuntimeWithUpgrade(...)`
- upgrade 执行：`retryBuilderRuntimeWithUpgrade(...)`

这些入口当前主要按以下 reason 驱动升级：

- `parse_failure`
- `scope_violation`
- `semantic_conflict`
- `validation_failure`
- `schema_drift`
- `unrelated_edits`

### 1.2 schema / patch shape repair 入口

- `retryBuilderRuntimeSchemaRepair(...)`
- `retryBuilderRuntimePatchApplyRepair(...)`
- `normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(...)`

这部分主要处理：

- patch parse 失败
- schema drift
- patch apply 失败
- 受控工作区内的 fallback-only normalize

### 1.3 semantic / validation / coverage repair 入口

- `retryBuilderRuntimeSemanticConflictRepair(...)`
- `retryBuilderRuntimeTaskOutputValidationRepair(...)`
- `retryBuilderRuntimeTaskTargetCoverageRepair(...)`
- `retryBuilderRuntimeDirectFailureCoverageRepair(...)`

这部分主要处理：

- schema-conflicting symbols 反复引入
- task output validation 失败
- 当前 task target paths 未被真正覆盖
- 直接失败文件未被真正覆盖

### 1.4 prompt 入口

- `buildBuilderRuntimePrompt(...)`

当前 prompt 仍混有三类信息：

- 合法的公共主语义：`binding_refs`、`surface_refs`、`entity_refs`、`semantic_intent_refs`、`owned_paths`
- 合法的路径类别提示：`lib/views/*.dart`、`lib/controllers/*.dart`、`lib/repositories/*.dart`、`lib/template/*.dart`、`test/widget_test.dart`
- 尚待收缩的具体模板文件名提示：`home_page.dart`、`record_list_page.dart`、`record_form_page.dart`、`record_detail_page.dart`、`open_lite_copy.dart`、`dashboard_summary.dart`、`record.dart`

## 2. Template-Private 入口盘点

`pkg/appfactory/adapter/builder_runtime_open_lite_private.go` 当前主要承载以下 template-private 规则：

### 2.1 prompt guidance

- `appendBuilderRuntimeOpenLiteTemplateCopyPromptGuidance(...)`
- `appendBuilderRuntimeOpenLiteSchemaRemapGuidance(...)`
- `appendBuilderRuntimeOpenLiteWeightRecordTemplateCopyFailureGuidance(...)`

### 2.2 template copy normalize

- `normalizeBuilderRuntimeTemplateCopyContent(...)`
- `normalizeBuilderRuntimeOpenLiteCopyContent(...)`
- `normalizeBuilderRuntimeOpenLiteCopyLine(...)`

### 2.3 main normalize / app entry repair

- `normalizeBuilderRuntimeOpenLiteMainContent(...)`
- `normalizeBuilderRuntimeOpenLiteMainHomePageConstructorCalls(...)`
- `normalizeBuilderRuntimeOpenLiteMainDetailCallback(...)`
- `normalizeBuilderRuntimeOpenLiteMainCollectionCreateFlow(...)`
- `normalizeBuilderRuntimeOpenLiteMainNavigatorPushCalls(...)`
- `normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks(...)`

### 2.4 helper preservation / import canonicalize

- `builderRuntimeOpenLiteEnsureTopLevelHelper(...)`
- `normalizeBuilderRuntimeOpenLiteEnsureFormPageImport(...)`
- `normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(...)`
- `normalizeBuilderRuntimeOpenLiteEnsureNavigatorKey(...)`
- `normalizeBuilderRuntimeOpenLiteMainRepositoryImports(...)`
- `normalizeBuilderRuntimeOpenLiteMainUnusedRecordImport(...)`

### 2.5 widget test normalize

- `normalizeBuilderRuntimeOpenLiteWidgetTestContent(...)`

## 3. 当前边界判断

- public runtime 应只保留 binding / surface / path-class / failure-class 组合驱动的规则。
- `builder_runtime_open_lite_private.go` 应只保留 flutter-open-lite 模板私有 repair policy，不再反向塑造公共 runtime 主语义。
- 当前最重的 public 污染点仍集中在 `buildBuilderRuntimePrompt(...)` 与 `normalizeBuilderRuntimePatchForWorkspaceWithTaskBundleAndTargetPaths(...)` 内的具体文件名分支。

## 4. 本轮已完成的第一处迁移

- relation-rich repository guidance 已从 `record_repository.dart` 精确文件名触发改成 `lib/repositories/*.dart` repository-scope 触发。
- 相应 prompt 文案也已改成 repository-scope 表述，不再把 `record_repository.dart` 作为公共语义名词。

## 5. 本轮新增收口

- `buildBuilderRuntimePrompt(...)` 的 overview / collection / inspection topology 判断已改成 `allocation_transition.surface_refs` 优先，默认文件名只保留 legacy fallback。
- `buildBuilderRuntimePrompt(...)` 中 collection-surface 的无过滤约束已改成当前任务 surface 驱动，不再要求 `record_list_page.dart` / `record_list_controller.dart` 命中才能触发。
- `builderRuntimeNeedsCollectionCreateEntry(...)` 已改成 `surface-overview` / `surface-collection` / `surface-mutation` 优先，不再把 `home_page.dart` / `record_list_page.dart` / `record_form_page.dart` 作为唯一真相。
- `shouldUseCompactBuilderRuntimeOverviewBindPrompt(...)` 已改成单文件 `lib/views/*.dart` + `surface-overview` 驱动，`home_page.dart` 仅作为 legacy fallback。
- `validateBuilderRuntimeTopologyScopedTaskOutput(...)` 中 app entry 的 topology 判定已改成 `surface_refs` 优先，避免 prompt/normalize 与 validation 三层语义分叉。
- `validateBuilderRuntimeTopologyScopedTaskOutput(...)` 中 mutation / collection 的 controller/view 分支已改成 surface 路径集合驱动，不再只认 `record_form_controller.dart` / `record_list_controller.dart` / `record_list_page.dart`。
- `pruneUnexpectedNoFilterCollectionControllerOperations(...)` 已改成从 `surface-collection` 收集 controller/view 路径，只有缺少 surface 元数据时才退回 `record_list_*` 旧文件名。
- `pruneUnexpectedDependentOperationsFromSingleTargetNonRepairPatch(...)` 已改成 surface/path-class 优先，custom overview / collection / mutation view 任务会按 surface 收缩 controller/template dependents，旧文件名只保留 fallback。
- `shouldRetryBuilderRuntimeTaskOutputValidationRepair(...)` 已收紧：`SingleFileEdit` 任务上的 unresolved local import 不再默认进入 validation repair，避免 create task 被错误拉入下一轮 patch。
- `builderRuntimeMainViewConstructorSignatureError(...)` 已从固定 `HomePage/RecordListPage/RecordFormPage/RecordDetailPage` 文件名改成扫描 `lib/views/*.dart` 中实际被 `lib/main.dart` 构造的 view 类，custom view 文件名也会进入 app-entry constructor 校验。
- `builderRuntimeTaskOutputPathCanReferenceRecordDetail(...)` 已从固定 `record_list_page.dart` 放宽到 `lib/views/*.dart` path-class，custom collection view 在未声明 detail surface 时也会拦截 `RecordDetailPage` 引用。
- `builderRuntimeTaskOutputWiresCollectionCreateEntry(...)` 已从固定 `onCreateRecord` 放宽到通用 `onCreate*` 回调，task-domain / entry-domain collection root 不再依赖 record 语义命名。
- `normalizeBuilderRuntimeOpenLiteMainHomePageConstructorCalls(...)` 虽然名字还没改，但内部已经不再只认 `HomePage/home_page.dart`；它会扫描 `lib/main.dart` 实际构造的 `lib/views/*.dart` view 类，并对 custom overview view 注入缺失的 `controller` / `onCreate*` / `onViewAll*` fallback 参数。
- `builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace(...)` 已补一层 custom overview 识别：只要 `lib/main.dart` 已实际构造 `lib/views/*.dart` overview view，就不会再因为缺少 `home_page.dart` 把 workspace 误判成“仍需补 collection create entry”。
- `normalizeBuilderRuntimeOpenLiteMainRepositoryImports(...)`、`builderRuntimeOpenLiteRecordRepositoryTypeName(...)`、`builderRuntimeOpenLiteConcreteRecordRepositoryTypeName(...)`、`builderRuntimeOpenLitePreferredWidgetTestRepositoryTypeName(...)` 已开始共享通用 repository discovery：会扫描 `lib/repositories/*.dart` 中真实的 Repository 类型与实现类，custom `task_repository.dart` / `entry_repository.dart` 不再被强行改写回 `record_repository.dart`。
- `normalizeBuilderRuntimeOpenLiteMainDetailCallback(...)`、`normalizeBuilderRuntimeOpenLiteMainCollectionCreateFlow(...)`、`normalizeBuilderRuntimeOpenLiteMainRecordFormCallbacks(...)` 已开始共享 mutation view discovery：会扫描 `lib/views/*.dart` 中的 form-like view，提取 class 名、repository 参数名和 `initial*` 参数名，custom `task_form_page.dart` / `TaskFormPage(taskRepository:, initialTask:)` 已能进入 main callback canonicalize。
- `normalizeBuilderRuntimeOpenLiteEnsureListControllerVariable(...)` 与 `builderRuntimeOpenLiteListControllerSupportsUpdate/Init/Refresh(...)` 已开始共享 collection controller discovery：会扫描 `lib/controllers/*.dart` 中 list/collection controller，提取 class 名、repository 参数名和能力方法，custom `task_collection_controller.dart` / `TaskCollectionController(taskRepository: ...)` 已能进入 main callback canonicalize。
- detail surface 的 existence fallback 已开始共享 detail view discovery：public runtime 的 `hasDetailSurface` 在缺少 `surface_refs` 时不再只认 `record_detail_page.dart`，而是接受 `lib/views/*detail*.dart` 一类 custom detail path；`normalizeBuilderRuntimeOpenLiteMainDetailCallback(...)` 也会扫描 workspace 中的 detail-like view，已有 standalone detail view 时不再错误回落到 form flow。
- public runtime 的无-detail-surface 引用校验已不再只认 `RecordDetailPage`：`validateBuilderRuntimeTopologyScopedTaskOutput(...)` 现在会拦截 detail-like page 构造类型，custom `TaskDetailPage(...)` 也会走同一条 topology rule。
- `normalizeBuilderRuntimeOpenLiteWidgetTestContent(...)` 的 repository import 已开始共享 repository discovery：custom `task_repository.dart` widget test 不再一边实例化 `InMemoryTaskRepository()` 一边仍硬编码导入 `record_repository.dart`。
- app entry 的 surface wiring validation 已开始共享 `surface target paths + main view constructor discovery`：`validateBuilderRuntimeTopologyScopedTaskOutput(...)` 在缺少 overview 时不再只认 `RecordListPage`，custom `TaskCollectionPage` 也能通过 collection-root wiring 校验。
- `builderRuntimeSingleTargetLegacyPrunableDependentPaths(...)` 已开始对基础 `*_repository.dart` 共用 sibling prune 规则：custom `task_repository.dart` 的 `hive_*` / `in_memory_*` companion edits 也会被单目标 task 正确裁掉。

## 6. 下一刀优先级

当前真正的剩余硬点已经进一步集中在 template-private canonical path、fallback-only normalize 与启发式评分：

- detail surface 的 existence 与 absence 两侧主链已经开始 generic 化，但 template-private canonical path 里仍保留默认 detail/list/repository 样板命名；尤其是 widget test 与 canonical controller/view 生成还没有接入同一套 `surface -> class/path` registry。
- detail surface 的 existence 与 absence 两侧主链已经开始 generic 化，app entry surface wiring 也开始按真实 view 构造校验；但 fallback-only 的 view/content normalize、启发式打分以及 template-private canonical controller/view 生成仍保留默认 detail/list/repository 样板命名。
- `builderRuntimeOpenLiteCanonicalNoFilterListController(...)`、`builderRuntimeOpenLiteCanonicalNoFilterListPage(...)`、fallback-only 的 `normalizeBuilderRuntimeRecordListPageContent(...)` / `normalizeBuilderRuntimeRecordDetailPageContent(...)` 等路径仍残留 `RecordListController` / `record_list_page.dart` / `record_detail_page.dart` 一类样板命名，需要后续和新 discovery/registry 对齐。
- `builderRuntimeSingleTargetDartCandidateScore(...)` 已经不再直接奖励 `HomePage` / `RecordListPage` 样板命名；它目前只看 local project import + surface constructor generic signal，但还没有升级为消费统一 registry snapshot。

继续安全推进前，需要把 template-private canonical 输出也拉到显式 `surface -> class/path` registry；在那之前，继续只围绕默认样板命名做零散字符串替换，收益已经明显下降。 

## 7. 下一阶段设计：registry-first 过渡

### 7.1 registry 定位

当前缺的不是再多一条 custom helper，而是同一份 template-private `surface -> class/path` registry，让 canonical controller/view 生成、fallback-only normalize、widget test canonical 输出和启发式评分共用一份 concrete path/class 描述。

建议 registry 至少包含以下字段：

| 字段 | 含义 |
|---|---|
| `registry_key` | `binding_ref + surface_ref + path_class + template_role` 的稳定派生键 |
| `binding_ref` / `surface_ref` | 回指公共 binding / surface |
| `path_class` | `app_entry` / `view` / `controller` / `template_copy` / `widget_test` / `repository` |
| `template_role` | `overview_view`、`collection_controller`、`mutation_view`、`inspection_view`、`copy_helper`、`widget_test` 等模板私有角色 |
| `resolved_path` / `resolved_class_name` | 当前 workspace 或模板里真正应消费的 path / class |
| `constructor_contract` | required params、callback params、repository/controller/record 参数 |
| `capability_flags` | create/edit/delete/filter/status/title/note/refresh/update 等能力位 |
| `fallback_mode` | `emit_only` / `canonicalize` / `preserve` / `skip` |

约束：

- 它是 template-private 派生表，不是新的 public contract。
- 任何 consumer 都不允许在拿到 registry 后再自己回退到 `record_*` / `home_*` 默认触发器。
- registry snapshot 可以缓存到执行期，但必须能从 `TemplateSlotMap + template scan + workspace candidate detection` 重建。

### 7.2 第一批 consumer

| consumer 切面 | 当前残留 | 切到 registry 后的目标 |
|---|---|---|
| `builderRuntimeOpenLiteCanonicalNoFilterListController(...)` | 默认产出 `RecordListController` / `record_list_controller.dart` | 直接消费 `collection_controller` role 的 `resolved_path/class/capabilities` |
| `builderRuntimeOpenLiteCanonicalNoFilterListPage(...)` | 默认产出 `RecordListPage` / `record_list_page.dart` | 直接消费 `collection_view` role 的 constructor contract 与 callback names |
| `normalizeBuilderRuntimeRecordListPageContent(...)` / `normalizeBuilderRuntimeRecordDetailPageContent(...)` | fallback-only normalize 仍夹带默认 page class/path | 统一改看 `collection_view` / `inspection_view` role |
| `normalizeBuilderRuntimeOpenLiteWidgetTestContent(...)` | canonical 输出仍需局部能力硬判 | 由 `widget_test` + `mutation/collection` role capability flags 共同决定脚本 |
| `builderRuntimeSingleTargetDartCandidateScore(...)` | 已改成 local import + surface constructor generic signal，但仍未直接消费 registry snapshot | 继续升级为奖励 registry signal、surface match 和 constructor contract 完整度 |

### 7.3 迁移顺序

1. 先落 registry derive helper 与 snapshot 测试，只证明 `binding/surface/path_class -> resolved path/class/contract` 可重建。
2. 再把 canonical controller/view 生成切到 registry，优先收 `NoFilterListController`、`NoFilterListPage`、detail / overview canonical 输出。
3. 再把 fallback-only view/content normalize 与 widget test canonical 输出切到同一份 registry，停止各自维护默认路径分支。
4. 最后清 heuristic scoring、prompt fallback、single-target candidate ranking 里的默认类名奖励，让它们只看 registry signal。

### 7.4 迁移验收

- template-private canonical path 不再需要直接引用 `record_list_page.dart`、`record_detail_page.dart`、`home_page.dart` 等默认文件名。
- fallback-only normalize、widget test canonical 与 candidate score 在 custom class/path workspace 上共享同一份 registry snapshot，通过窄测覆盖。
- runtime private helper 的剩余默认文件名判断只允许作为 legacy fallback，并且有明确删除清单。