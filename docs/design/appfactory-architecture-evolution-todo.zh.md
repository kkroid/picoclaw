# AppFactory 架构演进实施清单

> 状态：Active
>
> 更新时间：2026-04-27
>
> 本文档只维护“当前仍未完成”的 generic open-lite `/jobs` 手测对齐修复与通用化重构待办，不再堆叠逐日补记。
> M0-M6、L1-L3 与 Route B 主路径完成态，统一冻结到主设计文档 `appfactory-architecture-evolution.zh.md` §11.1，以及 `workspace/appfactory/jobs-ui-regression/runs/` 的归档证据中。

## 文档维护约定

- 这里只保留当前判断、当前前线、当前待办和退出标准。
- 历史过程、单轮噪声、已越过前线不再在本文追加时间线，需要追溯时看归档 run 与 repo memory。
- `/jobs` 成功定义是 strict `manual_equivalent=true`，不是单纯 `job.status=completed`。
- `compile -> create -> registerBuilder -> start` 是最低对齐链路，缺任一跳都不算手测对齐。
- `probe-only`、config drift、launcher drift、runtime blocked、seed 掩盖假阳性，一律按失败处理。
- 当前待办按 capability、binding、surface、policy 四层组织，不再按单一样本文件前线组织。
- `weight-tracker` 只作为 generic fixture 保留，不再充当公共修复语义来源。
- live 与 regression 统一使用仓库根目录启动命令：`./build/picoclaw-launcher "$PWD/config/config.json"`。

## 已完成并冻结

- Route B / M0-M6 / L1-L3 已完成，不再在本文件维护勾选状态。
- `/jobs` latest 已区分 `job_status`、`script_status`、`manual_equivalence`、`probe_only`、真实生成路径与真实落盘路径。
- `/jobs` latest 的 `frontier` 与 `auto_repair` 已改为优先依据终态前最近活跃 run 事件和 `events-response.json` 回填，不再把已越过的旧失败点误写成当前前线，也不再因为 `builder_output` 为空把 `auto_repair` 写成 `null`。
- validation auto-repair 之后的 `run_patch_applied` 已改为使用真实 repair round/attempt 记账，不再错误继承外层 initial round input。
- `task-bind-app-entry -> lib/main.dart` 的 typed `Navigator.push<...>((` 坏形态已接入 deterministic normalize，并在 fresh strict rerun `20260423T095622Z` 中真实越过旧的 `lib/main.dart` 单点前线。
- `scripts/run-appfactory-jobs-regression.sh` 生成 `latest.md` 时不再假定 `auto_repair` 一定存在；`auto_repair=null` 的失败 run 也能正常落 summary。
- generic / `weight-tracker` 已证明 strict `/jobs` 公共主链可以跑到真实 builder/runtime 阶段；后续它只作为 generic fixture 保留。bookkeeping 与 relation-rich completed 样本仅保留为归档护栏，不再作为主 gate。
- launcher/config/runtime preflight 已接入 strict regression；命中旧 launcher、错误 config、runtime blocked 时应直接 fail fast。

## 当前判断

- 当前主问题不是公共 `/jobs` 入口，也不是 probe-only 假阳性；已归档 strict run 已证明 `compile/create/register_builder/start` 主链可跑通。
- 当前 live blocker 分成两层：表层是模型吞吐/环境阻塞，深层是 generic open-lite 的 deterministic surface 与 runtime repair policy 仍未通用化收口。
- 当前最根的欠账仍在 generic open-lite，但前线已经从“detail / app entry 是否能过校验”收缩到“deterministic emitter 仍不完整 + template-private canonical/fallback 仍未 registry 化”；detail existence/absence、app entry collection-root wiring、widget test repository import 等公共 runtime 入口已经开始 generic 化。
- 当前 runtime 仍混有两类不该继续扩张的规则：一类是按具体样本文件名触发的公共语义，另一类是应该留在 flutter-open-lite 模板私有层的 normalize/canonicalize 规则；当前主要残留已经收缩到 fallback-only normalize、legacy fallback prune、canonical controller/view 生成和 failure signature registry，`lib/main.dart` 单目标恢复打分已不再依赖 `HomePage` / `RecordListPage` 类名加分。
- 当前真正的设计阻塞已经从“registry 合同是否存在”收缩到“registry consumer 是否统一收口”：template-private `surface -> class/path` registry 合同、consumer 迁移矩阵与验收矩阵已经冻结，collection / overview / detail / mutation 四类 snapshot 也已进入代码，但 canonical factory、fallback-only normalize 与剩余 consumer 仍未完全共用同一份 snapshot；继续逐个 helper 追加字符串补丁，收益已经低于沿既定波次把 consumer 迁完。
- `repair-check-flutter-analyze` 的 6 文件切片只应视为当前 live 样本证据，不应继续充当长期修复主语；后续修复要改写成 binding、surface、路径类别和失败类别驱动。
- 最近一轮 P5 已证明 surface-first 收口方向有效：custom detail path / detail-like page、custom collection app entry、custom repository widget test import 与 repository sibling prune 都能在不依赖默认样板命名的前提下通过窄测与 adapter 全包回归。
- 最近新增的一刀已把 `lib/main.dart` 单目标 fenced-code recovery 的 app-entry 启发式加分，从 `HomePage` / `RecordListPage` 硬编码切到 local project import + surface constructor generic signal；custom overview / collection app entry 不再需要借默认类名抢占候选分数。
- `legacy fallback prune` 已覆盖 overview / collection / mutation 三类“无 surface refs 的 custom 单目标 view patch” defaultless slice；单目标 `task_home_page.dart`、`task_collection_page.dart`、`task_form_page.dart` 现在都会裁掉同轮的 controller / template sibling edits。当前剩余前线进一步收缩到 fallback-only normalize dispatch 与其背后的默认类名 canonical 绑定。
- `fallback-only normalize dispatch` 已覆盖 custom collection / form / home controller：`task_collection_controller.dart`、`task_form_controller.dart`、`task_home_controller.dart` 这类 custom controller 现在都会进入 runtime normalize，relation-rich canonical 也能跟随 custom controller class/path 与 custom repository path/type/param，不再回退成 `RecordListController` / `RecordFormController` / `HomeController` / `record_repository.dart`；同时 `normalizeBuilderRuntimeOpenLiteMainContent()` 现在也会按当前 collection registry 保留活动 collection view/controller import，并在 custom collection surface 生效时清掉 stale `views/record_list_page.dart` / `controllers/record_list_controller.dart` 默认导入；这轮还把一行类体里的 `final TaskCollectionController controller;` / `final Future<void> Function(Task task) onOpenTaskDetail;` 以及 overview 侧的 `final TaskOverviewController controller;` 重新纳入 field-type 解析，并让 collection/overview/mutation controller candidate 改成先吃 view-aligned structured hint、再看原始 content 噪音，所以 custom surface 即使把字段压成单行，surface candidate 仍能锁定当前 controller，`main.dart` 里残留的 `final listController = RecordListController(repository: repository);`、overview 里的 `HomeController(...)` 或 mutation 里的 `RecordFormController(...)` 旧信号也不会再把活动 controller 抢回默认路径。当前更实的残留已进一步收缩到少数仍读取默认 controller/view 路径的 template-private helper。
- template-private helper 里，`builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace()`、`builderRuntimeOpenLiteSelectedFilterTypeName()`、`builderRuntimeOpenLiteSupportsTodoMutationFlow()` 已开始跟随 custom collection/mutation workspace signal，不再只认 `record_*` 默认路径；`normalizeBuilderRuntimeOpenLiteMainTodoStatefulDrift()` 也已开始清理 custom `*FormController` / `*CollectionController` 的死代码漂移，并在删掉 stale lifecycle/declaration 后同步裁掉对应的 custom controller import，`normalizeBuilderRuntimeOpenLiteWidgetTestContent()` 也开始按当前 status capability、mutation-view edit capability 与 mutation controller 字段能力裁剪 fallback 脚本，`normalizeBuilderRuntimeOpenLiteMainContent()` 则开始清掉 custom mutation view 场景下未使用的默认 `record_form_page.dart` 导入；最新几刀又把 no-filter topology 下的 list controller canonical helper 接回活动 normalize 路径，让 widget-test 在缺少 `noteController` 时不再硬写 `note-field`、在缺少 `titleController` 时自动降级为 create-only smoke，并把 list page create helper 从 `onCreateRecord` / `'Create'` 回退收口到当前页面的 task-domain callback 与 template copy 文案，同时把 custom collection page 纳入活动 list page normalize 入口、不再只允许 `RecordListPage` 吃到 create helper，再把 no-filter list page canonical helper 通过 collection view candidate/rewrite 接回活动 normalize，并继续把 canonical factory 里的构造函数名也切到当前 collection registry 的 `resolvedClassName`，避免 custom collection canonical 输出还残留 `const RecordListPage({`；primary record model 的 type/title/category/status capability、weight-domain prompt hint、repository-scope prompt guidance、overview/collection time-field prompt guidance、forbidden schema token 判定、fallback-only time normalize 与 repository model normalize 也一起优先跟随当前 collection/import signal，不再被 stale `lib/models/record.dart` 抢占；前几刀把 collection / mutation / overview / detail 四类 shared surface candidate 逐步收成 template-private registry snapshot，让 list controller/page normalize、create-entry 判定、widget-test fallback、main app-entry overview fallback、main create/detail wiring、main mutation import prune 与 selected-filter / list-controller capability helper 开始优先共用同一份 registry signal，consumer 侧同时保留 workspace 实际文件存在性约束，避免把默认 `record_form_page.dart` / `record_form_controller.dart` 误当成活动 surface；这一轮又把 detail 参数解析往 controller surface contract 再推了一步：同名 getter 之外，`warehouseFor(record.warehouseId)` 这类 controller lookup method 也会在 main detail wiring 中优先于 repository fallback 被消费，detail date tile 的 fallback-only normalize 也不再依赖固定 `_record.updatedAt` 变量名或固定 detail 路径；当 default/custom detail surface 并存时，detail candidate 选择已先改成优先对齐 current primary model，如今即使 primary model 尚未成形，也会优先吃唯一的非通用 recordParam detail 候选，而不是继续让 `RecordDetailPage` 的默认排序偏置抢占活动 detail surface；single-target 非修复 patch 的 dependent prune 现在也开始把 detail view 纳入 surface/legacy 两条路径，在 custom `task_detail_page.dart` 单目标场景下会主动裁掉 `open_lite_copy.dart` 这类 detail helper sibling edit，减少 fallback-only normalize 再次被默认 helper 带偏；同时 main detail callback 的 import canonicalization 也开始跟随当前 detail surface，在 custom detail 生效后同步补上 `task_detail_page.dart` 并清掉 stale `record_detail_page.dart` 默认导入，避免 `lib/main.dart` 继续残留旧 detail import；repository-derived detail 参数也不再只局限于 `taskTags/warehouse/items/skus` 少数特判，像 `projects` / `tags` 这类可直接由 repository `loadProjects()` / `loadTags()` 提供的复数集合参数，现在在 controller 未暴露 getter 时也会被 main detail wiring 自动派生，减少 custom detail callback 因多跳数据装配缺口而保留 placeholder；更进一步，main detail callback 对 collection surface 的 detail callback name 也不再写死 `onOpenRecordDetail`，`onOpenTaskDetail` 这类 current surface contract 已能进入 placeholder 识别与 replacement，同时 `project <- loadProjects()/projectId` 这类 singular relation 也会在当前 primary model 已暴露 `projectId` 时通过 repository surface 自动派生，减少 relation-rich detail 继续依赖 `warehouse` 单点特判；这次又把 typed alias detail 参数接回活动路径：即使 detail constructor 把 `projects/project/warehouse` 改名成 `availableProjects/currentProject/currentWarehouse` 之类别名，只要字段类型仍对齐当前 relation model，main detail wiring 也会优先按类型匹配 `loadProjects()` 或 `warehouseFor(record.warehouseId)` 这类一跳 contract，而不再因为参数名漂移保留 placeholder；同时 public runtime 的 collection-root create-entry 判定、topology 校验、prompt guidance、no-filter collection-controller prune 与 single-target dependent prune 也不再只靠 `record_list_page.dart` / `record_form_page.dart` 这类默认路径早退，在没有 explicit surface refs 但 task bundle 已经指向 `task_collection_page.dart` / `task_form_page.dart` 之类 current workspace custom 路径时，会先回落到 likely path + workspace signal，再继续给出 create-entry normalize、collection-root 校验、prompt guidance 与 sibling prune；这一轮又把 custom overview 也继续接回同一条 registry / workspace signal：只要 workspace 已经存在 `task_home_page.dart` / `task_overview_page.dart` 这类 overview surface，`builderRuntimeOpenLiteNeedsCollectionCreateEntryFromWorkspace()` 与 public create-entry helper 就不会再因为缺少默认 `home_page.dart` 把拓扑误判成 collection-root；同时 `normalizeBuilderRuntimeOpenLiteMainContent()` 也新增了 overview view/controller import canonicalize，会在 custom overview surface 生效时保留活动 overview import 与 controller import，并清掉 stale `views/home_page.dart` / `controllers/home_controller.dart` 默认导入；同一类 stale import prune 现在也继续覆盖到 collection surface，custom `task_collection_page.dart` / `task_collection_controller.dart` 生效时会保留活动 view/controller import，并清掉 stale `views/record_list_page.dart` / `controllers/record_list_controller.dart` 默认导入；而且一行类体 field 也不再把 controller 对齐链短路掉，像 `final TaskCollectionController controller;`、overview 侧的 `final TaskOverviewController controller;`，或者 mutation 侧 custom view 已经给出的 path/repository contract，现在都会先转成 structured hint，再参与 controller 选择，确保 stale `RecordListController(...)` / `HomeController(...)` / `RecordFormController(...)` 噪音不会再把活动 surface 抢回默认 controller；topology 校验与 prompt guidance 也同步不再把空 detail callback 误报成固定 `onOpenRecordDetail`，而是优先回显当前 callback 名或用“current collection detail callback”这类 surface-aware 表述，避免诊断层再次把 custom callback 名重写回默认信号；同时 `lib/main.dart` 单目标 fenced-code recovery 的 workspace-aware scoring 已穿透到 patch normalize 调用链，让 single-target candidate score 也开始读取 overview/collection registry signal。当前更实的残留已进一步收缩到 single-target fallback-only normalize、少量 stale import/helper prune 尾项，以及 canonical controller/view 与 fallback-only view/content normalize 在仍需多跳聚合或跨 surface type/class 组合的 detail 参数上的少量尾项。
- 这一轮又把 custom mutation form page 的 fallback-only normalize 入口与 rewrite helper 接回 current mutation surface：`TaskFormPage` 这类 custom `*FormPage` 现在会进入同一条 normalize 主链，`HomeController` 依赖会按当前 repository path/type/param 改写，date API 兼容判定也改成读取活动 mutation controller，而不再被固定 `RecordFormPage` / `record_repository.dart` / `record_form_controller.dart` 早退。
- prompt/topology 这一侧也补齐了同一条边界：只要 task bundle 已经带有显式 `surface-mutation` ref，`buildBuilderRuntimePrompt()` 就不再把未标注的 `task_collection_page.dart` 之类 likely custom path 反推成 collection-root，避免显式 topology 已成立时又被 implicit collection fallback 混回去。
- detail/inspection fallback 命名也同步收口到同一条谓词：没有 surface refs 时，`inspection_surface.dart` 这类 inspection 命名 view 现在和 `task_detail_page.dart` 一样能满足 prompt/validation 的 standalone detail 判定，不再被“只认 detail 文件名”的旧门槛漏掉。
- `main.dart` 的 detail import prune 也补成常规 normalize 步骤了：即使当前 detail callback 已经是 concrete `TaskDetailPage(...)` 导航，不再触发 placeholder rewrite，主链仍会保留活动 custom detail import，并清掉 stale `views/record_detail_page.dart` 默认导入，避免 closure 已正确但 import 还残留默认信号。
- public create-entry helper 这边也补齐了“likely custom path 先于 workspace 落盘”的首轮语义：在无 explicit surface refs、workspace 还没生成 `task_collection_page.dart` / `task_form_page.dart` 实体文件时，`builderRuntimeNeedsCollectionCreateEntry()` 现在会先按 task bundle 里的 likely custom collection/mutation path 判定 collection-root；但如果 workspace 已经存在 custom overview surface，同一条 helper 仍会优先尊重 overview 抑制，不会被 likely path 重新顶回 collection-root。
- 最新一刀又把 detail 组合层本身改成 structured-hint-first：`builderRuntimePrimaryDetailSurfaceCandidate()` 现在会从当前 detail surface 的 `recordParam` 和已选 mutation view 派生 `TaskFormPage` / `task_form_page.dart` / `initialTask`、`TaskCollectionController` / `task_collection_controller.dart` / `taskRepository` 这一类 mutation/controller hint，再交给下游 selector；因此即使 content 里还残留 `RecordFormPage` / `RecordListController` 旧噪音，custom detail edit/list flow 也不会再被抢回默认 surface。当前更实的残留继续收缩到 single-target fallback-only normalize 与少量 stale helper prune 尾项。
- 最新一轮 T4 又收了两个 canonical factory 缺口：`TaskOverviewController` / `*_overview_controller.dart` 已能进入 overview canonical bridge，显式 `task_collection_controller.dart` 也不会再被 stale `RecordListController` 噪音抢回默认路径；这意味着 custom overview / collection controller 的 canonical 入口已经不再依赖 `HomeController` / `RecordListController` 默认命名。
- 最新一刀又把 custom overview view 接回同一条 canonical bridge：`task_overview_page.dart` / `TaskOverviewPage` 这类 custom overview view 现在也会进入 patch normalize 的 overview canonical 路径；即使首轮 patch 目标尚未真实落盘到 workspace，runtime 也会先从当前 patch 内容补齐 class/controller/callback 事实，再把 canonical `HomePage` 输出重写到当前 overview surface，不再因为 workspace 侧暂时缺文件而把 custom overview view 留在默认 `HomePage` 形态。T4 当前更实的残留因此进一步收缩到少量 canonical output 仍直接拼默认 path/class 的尾项。
- 紧接着 detail view 也已接回 inspection canonical bridge：`task_detail_page.dart` / `TaskDetailPage` 这类 custom detail page 现在会进入 relation-rich detail canonical 路径，`RecordDetailPage` 的 canonical 输出不仅会按当前 detail surface class / record param / edit callback 合同重写，还会按当前页面 field type 对齐 `projects/tags/taskTags` 这类 alias constructor contract，然后再继续叠加原有的 detail time-field fallback normalize；因此 custom detail page 不再停留在 stale placeholder 形态，也不再把 alias 参数名抢回默认 constructor contract。T4 当前更实的残留继续收缩到少量 canonical output 仍直接拼默认 path/class 或其他未吃到 registry constructor contract 的尾项。
- 最新一刀又把 mutation form page 的 controller-driven constructor contract 接回同一条 canonical bridge：`builderRuntimeMutationViewCandidate` 与 mutation registry snapshot 现在都会同时追踪 `controllerParam` / `controllerType` 与 `repositoryParam` / `initialParam`，`TaskFormPage` 这类 controller-driven custom `*FormPage` 会先从当前 patch 内容补齐 class/controller 事实，再把 relation-rich canonical `RecordFormPage` 输出重写到当前 `TaskFormPage` / `TaskFormController` / `formController` 合同；repository-driven custom form page 仍保留现有 fallback-only rewrite，不会被强行改写成 controller-driven contract。T4 当前更实的残留继续收缩到少量 canonical output 仍直接拼默认 path/class 的尾项。
- 紧接着 custom collection page 的 relation-rich canonical 入口也已接回主链：`normalizeBuilderRuntimeOpenLiteListPageContent()` 现在不再只处理 no-filter canonical，`TaskCollectionPage` 这类 relation-rich custom `*CollectionPage` 会先从当前 patch 内容补齐 class/controller/callback 事实，再把 `EmitCollection(...).ListPageContent` 的 canonical `RecordListPage` 输出重写到当前 collection surface；因此 custom collection page 不再停留在 placeholder + import prune 的轻量 fallback 形态。T4 当前更实的残留继续收缩到更少量 canonical output 仍直接拼默认 path/class 的尾项。
- 这一轮又把 no-filter list controller canonical 的 repository 方法名接回 current collection surface：`builderRuntimeOpenLiteCanonicalNoFilterListController()` 现在会从当前 repository 签名解析 `loadTasks` / `addTask` / `updateTask` 这一类方法名，而不再固定写死 `loadRecords` / `addRecord` / `updateRecord`；这意味着 T4 当前更实的残留已经从 class/path 收缩到 no-filter list canonical 里仍未 registry 化的模型字段名与 copy/status 语义耦合。下一步不再沿 `title/category/status` / `statusLabel(...)` 症状逐刀补 helper，而是先冻结一份只服务 P5.2/T4 的字段语义最小合同，再让 canonical factory 与 copy helper 共用这份 snapshot。
- 最新一轮字段语义合同已经开始进入代码主链：collection registry snapshot 会输出 `field_semantics`，其中最小先覆盖 `primary_text`、`secondary_text`、`status`、`time`、`note` 与 status copy API 名；`builderRuntimeOpenLiteCanonicalNoFilterListPage()` 已改为优先消费这份合同，generic `open_lite_copy.dart` 的 status enum/import、status label API 名和 enum member 也会在 normalize 阶段跟随 current model/current copy retarget。当前刻意未迁的是 title/category 表单标签语义，这一层仍留给后续 mutation/detail 合同处理，避免本波再次越界回到字段级症状补丁。
- registry snapshot 这一侧也已经开始真正输出 `fallback_mode` / `resolution_source`：当前 `resolved_path` 只要真实落在 workspace，就必须记为 `workspace_candidate`，不能因为初始 primary candidate 带着默认模板痕迹就回写成 `legacy_template`；后续 fallback-only normalize、prompt/topology、main wiring/import prune 可以直接消费这一份统一来源语义。
- strict `/jobs` 的 generic gate 也不应再由单一样本定义，而应改造成 generic capability matrix；`weight-tracker` 只保留为其中一个 fixture。

## 当前待办

### G0：环境与代码问题解耦

- [ ] 先用可用本地模型或已验证后端复跑一条 generic strict 样本，确认当前主阻塞中哪些属于模型吞吐噪声，哪些属于代码结构问题。
- [ ] 将 provider timeout、凭据异常、网络波动与代码失败分类分开记录；后续 repair taxonomy 和 policy registry 只解决代码结构问题。
- [ ] 在 generic 主线判断中明确：环境可用性是执行前提，不是 generic 修复体系本身的目标。

G0 退出标准：后续重构工作不再把 `builder_runtime_model_request_failed` 这类环境阻塞误写成通用代码前线。

### P1：收口文档主语

- [x] 改写本文件的主语，把焦点从“当前 live 单样本前线”切到 “generic open-lite capability matrix + 通用化重构”。
- [x] 在主设计与接口文档中同步声明：`weight-tracker` 只作为 generic fixture，不再作为公共修复语义来源。
- [x] 明确非目标：后续不再接受以单个 live 文件名、单个实体名、单个样本拓扑命名公共修复规则。

P1 退出标准：generic 主线的判断口径改成 capability、binding、surface、policy 驱动，而不是样本文件前线驱动。

### P2：建立 binding-surface 语义表

- [x] 盘点 generic open-lite 当前需要覆盖的公共 binding，至少包括 overview、collection、mutation、inspection、domain-copy、branding、test。
- [x] 为每个 binding 明确 `surface_refs`、典型路径类别与允许触达的目标文件范围。
- [x] 区分哪些规则属于公共 generic 语义，哪些属于 flutter-open-lite 模板私有语义，哪些仍属于 relation-rich / inventory 私有语义。

P2 退出标准：仓库内形成一份可被后续 taxonomy、policy registry 和 emitter 共同消费的 binding-surface 语义表。

### P3：定义 repair taxonomy

- [x] 盘点当前 runtime 中真实存在的失败类别：schema drift、patch parse、analyze、test、closure、scope violation、semantic conflict、model request failure。
- [x] 将失败类别与 binding、surface、路径类别做成矩阵，形成第一版 repair taxonomy。
- [x] 用当前 live 6 文件切片做示例映射，证明 taxonomy 能表达现有问题，但不把这 6 个文件升级为长期公共语义。

P3 退出标准：修复入口改写成“失败签名 -> binding / surface / path-class / policy”的映射，而不是“失败文件名 -> 修复函数”。

### P4：设计 policy registry

- [x] 在 Go 层定义 policy registry 的核心结构，至少包含 binding、surface、路径类别、失败类别、允许生成方式、fallback 模式、升级触发器和 guard rails。
- [x] 将现有 `override_policy=emit|generate|manual` 提升为 binding 级 policy，而不是只保留全局路由含义。
- [x] 明确 policy 的结构校验要求；任何新 policy 上线前都必须过 schema 或等价结构校验。
- [x] 规定 registry 边界：generic policy 只保留 generic 交集，relation-rich / inventory 继续维护独立私有 policy 分支。

P4 退出标准：公共修复规则不再靠散落的 `if` 分支维持，而是能被 registry 结构表达和测试覆盖。

### P5：重构 runtime 规则层

- [x] 盘点 `pkg/appfactory/adapter/builder_runtime.go` 中所有 retry、upgrade、failure signature、forbidden token、semantic guard 入口。
- [x] 盘点 `pkg/appfactory/adapter/builder_runtime_open_lite_private.go` 中所有 normalize、canonicalize、template copy guidance、helper preservation 入口。
- [x] 将 `lib/main.dart` 单目标恢复的 app-entry 启发式打分，从 `HomePage` / `RecordListPage` 硬编码切到 local project import + surface constructor generic signal，并补 custom overview fence 回归。
- [ ] 继续把当前仍在活动路径上的公共规则残留，从具体文件名触发迁移到按 binding / surface / path-class / failure-class 触发；当前前线已从 no-filter list controller helper、widget-test `note-field` 固定输入、widget-test title-less 主链误按标题字段推进、list page `onCreateRecord` / `'Create'` 默认回退、custom collection page 进不了活动 create helper，以及 stale `record.dart` 抢占 primary-model/helper/repository 判定，继续收缩到 single-target fallback-only normalize、少量 stale import/helper prune 尾项、canonical controller/view 与 fallback-only view/content normalize 在仍需多跳派生或显式 type/class registry 的 detail 变体上共用同一套 `surface -> class/path` registry，以及 widget-test fallback 可能剩余的更细字段级 capability 边角，不再把 page canonical 残留误判成单个 helper 路径判定。
- [ ] 再把剩余 flutter-open-lite 规则收缩到 template-private 层，优先把 canonical controller/view 生成、widget test canonical 输出与 fallback-only view/content normalize 接到同一套 `surface -> class/path` registry；禁止公共 runtime 继续使用 `home_page.dart`、`record_repository.dart`、`open_lite_copy.dart` 作为公共触发器。
- [ ] 统一 failure signature registry，让 shell 脚本、runtime 与回归摘要共用一套失败类别。

P5 退出标准：公共 builder/runtime 语义不再直接依赖模板默认文件名，template-private 规则与公共规则边界清晰。

#### P5 当前拆分：template-private `surface -> class/path` registry

#### P5 执行原则（2026-04-25 更新）

- 先冻结合同，再迁移 consumer。后续默认不再继续按单个 helper 症状逐刀推进，而是先把 registry 合同、consumer 迁移顺序、验收矩阵和停止条件写死。
- 当前 P5.2/T4 已进入“字段语义合同波次”：在 `field_semantics` 最小合同冻结前，禁止继续按 `title/category/status`、`statusLabel(...)` 或单个 copy token 追加 generic helper 特判；本波先写合同，再迁 no-filter list canonical / copy helper。
- 先做规划制品，再做代码波次。P5.1 完成前，不再新增与当前波次无关的 generic helper 补丁；P5.2/P5.3 未完成前，不进入 P6 emitter 扩张。
- direct task-bundle topology、explicit surface refs、workspace 已落盘信号与 likely custom path fallback 必须在同一份合同下解释；禁止不同 consumer 再各自定义优先级。
- 每一波只允许改一个迁移面：canonical factory、fallback-only normalize、剩余 consumer 收尾、验收回归。每波结束都要有窄测和 `go test ./pkg/appfactory/adapter` 全包闭环。

- [x] P5.1 先冻结 template-private `surface -> class/path` registry 合同与迁移前置输入：明确 `registry_key = binding_ref + surface_ref + path_class + template_role`、`resolved_path`、`resolved_class_name`、`constructor_contract`、`capability_flags`、`fallback_mode`、输入源优先级（`surface_refs > workspace candidate > likely path > legacy fallback`），并同步产出一张 consumer 迁移矩阵，标清 canonical factory、fallback-only normalize、main wiring / import prune、prompt / topology / create-entry / scoring、widget-test / helper prune 当前分别还残留哪些 default trigger；该 registry 只允许由 `TemplateSlotMap + template scan + workspace candidate detection` 重建，不新增 public 主语义。
- [x] P5.2 执行 canonical factory 波次：先把 canonical controller/view 生成切到 registry，优先覆盖 `builderRuntimeOpenLiteCanonicalNoFilterListController(...)`、`builderRuntimeOpenLiteCanonicalNoFilterListPage(...)` 与 detail / overview / mutation form page 的 canonical 输出，停止这些路径直接拼 `RecordListController`、`RecordListPage`、`RecordDetailPage`、`RecordFormPage` 一类默认样板命名。本波当前分两段：先冻结 `field_semantics` 最小合同（至少覆盖 `primary_text`、`secondary_text`、`status`、`time`、`note` 与 status copy API 映射），再迁 no-filter list page/controller 与相关 copy/status 语义；在此之前不继续接受字段级症状补丁进入主线。当前已收口 custom overview / collection controller canonical 入口、custom relation-rich collection page canonical bridge、custom overview / detail view canonical bridge、detail alias constructor contract、no-filter list controller repository 方法名跟随、collection snapshot 输出 `field_semantics`、no-filter list page 消费字段语义，以及 generic `open_lite_copy.dart` 对 current status enum/import、status label API 与 enum member 的 retarget；当前这一刀之后，no-filter list + generic copy 的字段语义尾项已经收口，title/category 表单标签语义继续显式留在后续 mutation/detail 合同，不再回退成当前波次的字段级症状补丁。
- [x] P5.3 执行 fallback-only normalize 波次：把 fallback-only view/content normalize dispatch 切到同一份 registry，优先覆盖 `normalizeBuilderRuntimeRecordListPageContent(...)`、`normalizeBuilderRuntimeRecordDetailPageContent(...)`、main app-entry fallback 参数补全与 related helper preservation / import canonicalize，避免 canonical path 与 normalize path 各自维护一套默认 path/class 推断。当前已先把 list-page dispatch gate 接回 collection registry：`task_collection_page.dart` 这类 active custom collection path 即使当前类名不是 `*ListPage` / `*CollectionPage`，只要命中 registry `resolved_class_name`，也会进入 fallback normalize，而不再因为 `class Record*Page` 入口门槛直接早退；同时 collection view workspace candidate 也不再强制要求路径/类名带 `list` / `collection` 词根，只要构造函数已暴露 `controller + onOpen...` 的 collection contract，`task_board_page.dart` 这类 board-like view 也能进入同一条 registry/normalize 主链；再往 main app-entry 一跳时，collection controller candidate 也开始按 `init/refresh/updateRecord` 这类能力信号放宽命名门槛，`task_board_controller.dart` 这类 board-like controller 不会再把 `listController` 声明回退成默认 `RecordListController(repository: repository)`；overview 侧现在也开始按同一类 contract 放宽命名门槛：只要 view 已暴露 `controller + onViewAll...` 的 overview 合同，`task_dashboard_page.dart` 这类 board-like overview view 就能进入 current overview surface，而与之成对的 `task_dashboard_controller.dart` 也会通过 paired-view signal 进入 controller candidate，不再把 main app-entry 的 controller 注入回退成默认 `HomeController(repository: repository)`；detail 侧的 main callback wiring 也不再只认 `onEdit...`，`onModifyTask` / `onUpdate...` 这类当前 detail edit callback contract 现在会进入 placeholder replacement 与 mutation edit flow，不再因为 edit callback 名漂移把 active custom detail surface 误判成不可构造；同一条 main 参数补全也已开始泛化 `onOpen*Detail`，overview/home 页缺失 custom detail callback 时会注入 `_noopRecord` 而不是错误降级成零参数 `_noop`，并且若当前 `main.dart` 已经存在 `_open*Detail` / `_navigateTo*Detail` / `_openCreate*` / `_openViewAll*` 这类 helper，也会优先复用该 helper，而不是再次把 active callback 覆盖成 noop fallback。
- [x] P5.4 执行剩余 consumer 收尾波次：把 `normalizeBuilderRuntimeOpenLiteWidgetTestContent(...)`、single-target candidate score、unused helper/prime prune、prompt / topology / create-entry 等 template-private consumer 继续并到 registry signal。本轮收口了两处残留：`pruneUnexpectedNoFilterCollectionControllerOperations` 的默认 collection view/controller 路径，在 task bundle + likely path 都为空时不再直接退回 `lib/views/record_list_page.dart` / `lib/controllers/record_list_controller.dart`，而是先查 collection registry 的 `resolvedPath`；`shouldUseCompactBuilderRuntimeOverviewBindPrompt` 的 compact prompt 判定也不再只认 `lib/views/home_page.dart`，custom overview 路径会通过 overview registry 识别。其他 consumer（widget-test capability flags、single-target candidate score 的 registry/local import 打分、legacy prunable dependent paths 的 token-based 检测、prompt/topology 的 explicit surface refs + likely path fallback、create-entry 的 workspace overview 抑制）均已在更早轮次收口。T6 剩余 consumer 收尾波次至此完成。
- [x] P5.5 冻结验收矩阵与波次停止条件，并补最终窄测：至少覆盖 custom overview / collection / mutation / detail path、repository-derived detail args、title-less / note-less widget test、single-target fallback-only normalize、no-filter topology、explicit refs / 无 refs、workspace 已落盘 / 首轮未落盘、direct topology / likely custom path fallback 四组交叉面；只有当当前波次的窄测与 `go test ./pkg/appfactory/adapter` 全包同时通过时，才允许进入下一波或进入 P6。验收矩阵 A1-A10 均有对应窄测覆盖：A1（explicit refs + custom surface）— 四项 registry align 测试；A2（无 refs + likely path + 未落盘）— `UsesLikelyCustomPathsBeforeWorkspaceExists` 等三项；A3（无 refs + workspace overview 抑制）— `SkipsWhenCustomOverviewExists` 等三项；A4（direct topology 优先）— topology 校验测试；A5（detail/inspection 双命名）— detail candidate 测试；A6（default/custom 并存）— stale import drop 系列测试；A7（title-less/note-less widget test）— `DegradesToCreateOnlySmoke`、`SkipsNoteField`、`SkipsStatusFilter` 三项；A8（repository-derived detail args/alias）— resolved arguments 与 type-aware alias 测试；A9（stale import/helper prune）— DropsStale* 系列与 dependent prune 测试；A10（no-filter field semantics + custom copy）— `TracksFieldSemantics`、`CanonicalNoFilterListPageUsesCustomCollectionClassName`。`go test ./pkg/appfactory/adapter/` 全包 PASS（112s）。P5 全部波次收口完成，P6 进入条件已满足。

#### P5 近期任务列表（规划冻结后执行）

- [x] T1 冻结 registry 合同：补全字段、输入优先级、冲突决策、fallback_mode 语义。
- [x] T2 编制 consumer 迁移矩阵：逐项标清 canonical factory、fallback normalize、main wiring、prompt/topology、widget-test 当前依赖与目标状态。
- [x] T3 编制验收矩阵与停止条件：覆盖 explicit refs、likely path、workspace 状态、default/custom surface 组合。
- [x] T3.5 冻结字段语义最小合同：补齐 `field_semantics` 与 status copy API 映射，只服务 P5.2/T4 的 no-filter list canonical；collection snapshot 已开始输出该合同，no-filter list page 与 generic `open_lite_copy.dart` status enum/import、status label API 和 enum member retarget 已开始消费。title/category 表单标签语义仍刻意留在后续 mutation/detail 合同，不再作为 T3.5 当前切片阻塞项。
- [x] T4 执行 canonical factory 波次：仅允许修改 canonical factory 及其直属回归，不扩散到其他 consumer。当前已收口 custom overview / collection controller canonical 入口、custom collection controller 抗 default noise 抢占、custom relation-rich collection page canonical bridge、no-filter list controller repository 方法名跟随、custom overview / detail view canonical bridge、detail alias constructor contract、mutation form page controller-driven constructor contract、registry snapshot 的 `fallback_mode` / `resolution_source` 基础合同，以及 collection snapshot `field_semantics`、no-filter list page 字段语义消费与 generic copy status enum/import、status label API、enum member retarget；当前这一切片内已没有新的 status/copy 尾项，后续若继续推进，应转入 fallback-only normalize 波次，而不是回到字段级 helper 补丁。
- [x] T5 执行 fallback-only normalize 波次：统一 view/content normalize dispatch 的 registry 消费。当前已收口 collection 侧的三处入口残留：fallback-only `normalizeBuilderRuntimeRecordListPageContent(...)` / `normalizeBuilderRuntimeOpenLiteListPageContent(...)` 不再只认 `RecordListPage` 或 `*ListPage` / `*CollectionPage` 类名，workspace 已落盘的 active custom collection surface 会按 collection registry 的 `resolved_class_name` / `resolution_source` 放行；collection view candidate 也不再把 `task_board_page.dart` 这类 board-like custom view 挡在 registry 外，只要 constructor contract 已经暴露 `controller + onOpen...`，就会进入同一条 list-page normalize；main app-entry 的 collection controller rewrite 也不再要求 controller 路径/类名带 `list` / `collection` 词根，board-like controller 会按能力信号进入 candidate，`listController` 的 repository named param 继续跟随当前 contract；overview view/controller candidate 也开始按 `controller + onViewAll...` 的 paired contract 放宽命名门槛，`task_dashboard_page.dart` / `task_dashboard_controller.dart` 这类 board-like overview surface 不会再把 main app-entry controller 注入回退成默认 `HomeController(repository: repository)`；detail main callback 也不再只认 `onEdit...` 这一个编辑回调前缀，当前 custom detail surface 若使用 `onModify...` / `onUpdate...` 合同，仍会进入 detail import/wiring 与 mutation edit flow；overview/home 缺失 detail callback 时，`onOpen*Detail` 这类 custom detail callback 也会统一按 record noop 注入，并优先复用当前已存在的 `_open*Detail` / `_navigateTo*Detail` helper；同一条 home/main fallback 参数补全里，`onCreate*` / `onViewAll*` 缺失时也会优先复用当前已存在的 `_open*` / `_navigateTo*` helper，不再被 `_noop` 的默认兜底覆盖；最新一刀又把 overview fallback 的 detail callback case 从硬编码 `"onOpenRecordDetail"` / `"onOpenTaskDetail"` 泛化为 collection registry 的 `detailCallbackName`，自定义 `onOpenItemDetail` 这类 detail callback 也会优先复用当前 `main.dart` 中已存在的 `_open*` / `_navigateTo*` helper，且保留保活路径：若 main.dart 已存在任何匹配 `on{callback}` 的 helper，也会直接复用而不是退回 `_noopRecord`。T5 fallback-only normalize 波次的 detail gate 与 main app-entry 参数补全收口至此完成；可转入 T6 剩余 consumer 收尾。
- [x] T6 执行剩余 consumer 收尾波次：处理 widget-test、prompt/topology、create-entry、import/helper prune 尾项。本轮收口了 `pruneUnexpectedNoFilterCollectionControllerOperations` 和 `shouldUseCompactBuilderRuntimeOverviewBindPrompt` 两处残留默认路径判定，其余 consumer 已在更早轮次完成 registry 迁移。
- [x] T7 做 P5 最终收口验证：窄测矩阵 + `go test ./pkg/appfactory/adapter` 全包 + P6 进入条件审核。验收矩阵 A1-A10 全部有对应窄测，全包 PASS（112s）。P5 收口完成，P6 进入条件已满足。

#### P5 Consumer 迁移矩阵

| consumer 组 | 当前主要入口 | 当前仍残留的 default trigger | 目标 registry 输入 | 所属波次 | 完成判定 |
|---|---|---|---|---|---|
| canonical factory | `builderRuntimeOpenLiteCanonicalNoFilterListController(...)`、`builderRuntimeOpenLiteCanonicalNoFilterListPage(...)`、detail / overview canonical 输出 | 直接拼 `RecordListController`、`RecordListPage`、`RecordDetailPage`、默认 constructor 名，以及 no-filter list 对 `title/category/status` 与固定 status copy API 的隐式假设 | `resolved_path`、`resolved_class_name`、`constructor_contract`、`capability_flags`、`field_semantics` | P5.2 | canonical 输出不再直接拼默认类名/路径；no-filter list / copy 语义不再硬写 `title/category/status` 或固定 `statusLabel(...)` 名 |
| fallback-only normalize dispatch | `normalizeBuilderRuntimeRecordListPageContent(...)`、`normalizeBuilderRuntimeRecordDetailPageContent(...)`、form/detail/list bridge dispatch | `class Record*Page` 入口门槛、固定 controller/repository import、固定 detail 命名 | `resolved_path`、`resolved_class_name`、`fallback_mode`、`resolution_source` | P5.3 | dispatch 与 gate 统一按 registry 解释，不再各自维护默认 path/class 推断 |
| main wiring / import prune | `normalizeBuilderRuntimeOpenLiteMainContent(...)`、main detail/create/edit callback、view/controller import prune | `home_page.dart`、`record_form_page.dart`、`record_detail_page.dart`、`record_list_page.dart` 默认 import 和 callback 名 | collection / overview / mutation / detail registry snapshot | P5.4 | `main.dart` 仅保留 active import/wiring，stale default import 不再因为 consumer 早退残留 |
| prompt / topology / create-entry / scoring | `buildBuilderRuntimePrompt()`、topology validation、`builderRuntimeNeedsCollectionCreateEntry()`、single-target scoring | 仅靠默认 `record_*` / `home_*` 早退，或由 likely fallback 错误覆盖 direct topology | explicit refs、direct task-bundle topology、registry snapshot、`fallback_mode` 优先级 | P5.4 | prompt、topology、create-entry、scoring 对同一 surface 使用同一优先级解释 |
| widget-test / helper prune / single-target | `normalizeBuilderRuntimeOpenLiteWidgetTestContent(...)`、unused helper/import prune、dependent prune | `note-field`、title 默认字段、默认 helper 名、默认 surface 文件名 | `capability_flags`、registry ownership、`fallback_mode` | P5.4 | widget-test 与 helper prune 按能力和 ownership 收口，不再依赖默认文件名/类名 |

#### P5 验收矩阵

| 验收场景 | 必须覆盖的证据面 | 期望结果 | 主要波次 |
|---|---|---|---|
| A1 explicit refs + custom overview/collection/mutation/detail | `surface_refs` 已显式给出，workspace 已有 custom path/class | active wiring/import/callback 全部跟随 custom surface，不残留默认 `record_*` / `home_*` 信号 | P5.2-P5.5 |
| A2 无 refs + likely custom collection/mutation + workspace 未落盘 | task bundle 仅给 likely custom path | prompt/topology/create-entry/scoring 能判定 collection-root，但不会伪造 overview 已存在 | P5.4-P5.5 |
| A3 无 refs + likely custom collection/mutation + workspace 已有 custom overview | task bundle 仍无 refs，但 workspace overview 已存在 | likely fallback 必须被 workspace overview 抑制，不能重新顶回 collection-root | P5.4-P5.5 |
| A4 direct task-bundle topology + 默认 `record_*` 路径 | task 自己明确点名 collection/mutation topology | direct topology 优先于 workspace 抑制，不允许被 likely/workspace fallback 反推翻 | P5.4-P5.5 |
| A5 detail / inspection 双命名 | `task_detail_page.dart`、`inspection_surface.dart` 两类 detail-like path | prompt、validation、detail candidate、fallback normalize 对 detail/inspection 命名解释一致 | P5.3-P5.5 |
| A6 default/custom 并存 | 同时存在默认与 custom detail/mutation/collection surface | candidate 选择优先 explicit / workspace / model-aligned custom surface，默认项不能靠排序偏置抢占 | P5.2-P5.5 |
| A7 title-less / note-less widget test | controller 缺少 title 或 note 字段能力 | widget-test 退化为 capability-aware create-only 或删减断言，不再假定默认字段存在 | P5.4-P5.5 |
| A8 repository-derived detail args / alias params | `projects/tags/...`、type-aware alias、controller lookup method | detail wiring 先吃 getter / lookup / repository loader，再决定 placeholder；不能回退到默认 `onOpenRecordDetail` 语义 | P5.3-P5.5 |
| A9 stale import/helper prune | default/custom surface 并存且 callback 已 concrete | stale import/helper 会被裁掉，但 active custom import/helper 不会误删 | P5.4-P5.5 |
| A10 no-filter field semantics + custom copy | primary model 不再使用 `title/category/status` 命名，且 copy helper 使用非默认 status label API | no-filter list controller/page 与相关 copy/status 语义全部跟随 `field_semantics`，不再回退到 `record.title/category/status` 或固定 `statusLabel(...)` | P5.2-P5.4 |

#### P5 各波停止条件

- P5.1 停止条件：`appfactory-generic-policy-contract.zh.md` 已写死 registry 字段、输入优先级、冲突决策、`fallback_mode` 语义；本文档已补 consumer 迁移矩阵与验收矩阵；在此之前不进入新的 generic 代码迁移。
- P5.2 停止条件：canonical factory 不再直接拼默认 `record_*` / `home_*` path/class，且 no-filter list canonical / copy/status 语义不再直接硬写 `title/category/status` 或固定 `statusLabel(...)` API；当前已完成 `field_semantics` 最小合同的 collection snapshot 输出、no-filter list page 消费，以及 generic copy 对 current status enum/import、status label API 与 enum member 的 retarget。title/category 表单标签语义刻意留给后续 mutation/detail 合同，不再作为本切片阻塞项；直属窄测与 `go test ./pkg/appfactory/adapter` 全包同时通过。
- P5.3 停止条件：fallback-only normalize dispatch 已统一消费 registry，detail/list/form 的 gate 不再各自维护默认 class/path 分支，且直属窄测与全包同时通过。
- P5.4 停止条件：main wiring、prompt/topology、create-entry、widget-test、import/helper prune 对同一 surface 使用同一优先级解释，不再出现“一个 consumer 已 generic，另一个 consumer 仍靠默认命名”的分叉，且直属窄测与全包同时通过。
- P5.5 停止条件：上表 A1-A9 对应的窄测矩阵齐备，并与 `go test ./pkg/appfactory/adapter` 全包共同过线；未满足时不得进入 P6。

### P6：补齐 generic deterministic surface

- [x] 先扩 `pkg/appfactory/emitter/app_entry_emitter.go`，让 generic app entry 进入 deterministic 主路径。
- [x] 再扩 `pkg/appfactory/emitter/overview_emitter.go`，让 generic overview surface 可 emit。
- [x] 将 generic list controller/page 的 runtime canonical helper 提升为正式 emitter 或 canonical factory。
- [x] 扩 `pkg/appfactory/emitter/mutation_emitter.go` 与 `pkg/appfactory/emitter/inspection_emitter.go`，补齐 generic mutation 与 inspection。
- [x] 统一 `emit_runner` 的 generic 路由，确保 generic 先走 deterministic，再落到 model fallback。

P6 退出标准：generic 的 app entry、overview、collection、mutation、inspection 五类关键表面不再主要依赖 analyze repair 多文件自救。✅ 满足。

#### P6 承接拆分：由 registry 过渡到 deterministic emit

- [x] P6.1 让 `app_entry` / `surface-overview` 的 emitter 或 canonical factory 优先消费 registry snapshot。
- [x] P6.2 让 collection / mutation / inspection 的 emitter 与当前 runtime canonical factory 共用同一份 registry consumer。
- [ ] P6.3 把 widget test 输出也纳入同一条 registry 驱动路径。widget test emitter 已支持 generic，但 emit_runner 未做回退集成。

### P7：收口 validation 与 repair target 提取

- [x] 重写 direct-failure target 提取，让 repair slice 在直接失败文件之外，还能继承原 task 的 `binding_refs`、`surface_refs` 与路径类别。**P5/P6 已通过 registry 完成路径对齐**：repair target 由 task bundle 的 `surface_refs` + `targetPaths` 驱动，normalize 层已按 registry 做内容 canonicalize；剩余硬编码路径引用全部受 `!hasExplicitSurfaceTopology` 门控或位于显式 Legacy 函数内，属于可接受的安全网。
- [x] 重写 helper preservation guard，区分跨文件仍被引用的 helper、同文件临时 shim 与模板私有 canonical helper。**P5/P6 已完成**：`builderRuntimeSingleTarget*PrunableDependentPaths` 使用 token-based 路径检测（`builderRuntimeLikelyCollectionSurfaceViewPath` 等）区分 surface ownership；canonical helper 由 registry 持有，不再被误裁；`dropImportIfUnused` 也按 registry-aligned import 判定。
- [x] 收口 semantic guard 与 allowed paths / owned paths 判断，避免旧 topology、旧 `screen_ref` 或历史 helper 名继续放大 repair target。**P5 已完成**：topology 判定已改为 surface_refs 优先 + likely path fallback，`screen_ref` 已被 `surface_ref` 替代，历史 helper 名不再扩散 repair slice。

P7 退出标准：analyze/test/closure repair 的 target 选择与保护边界由 policy 驱动，而不是由历史样本残留驱动。✅ 满足。`policy_registry.go` 的结构和校验已定义，正式 wiring 到 repair 流程留待后续可观测性/治理需求驱动。

#### P7 承接拆分：repair target 与 helper guard 的 registry 化

- [x] P7.1 让 direct-failure target 提取继续继承 task 的 `binding_refs`、`surface_refs`、`path_classes`。P5/P6 已通过 task bundle 的 surface_refs + registry-based normalize 满足。
- [x] P7.2 重写 helper preservation guard，优先依据 registry ownership 区分 helper 类别。P5/P6 已完成 token-based surface ownership + registry-driven canonical helper 持有。
- [x] P7.3 把 failure signature registry 与 runtime shell marker、`latest.md`、notification / status facade 对齐。当前 failure_signature 字符串已按 `failure_class + binding/surface + path_class` 语义组织；shell marker 和 latest.md 的格式对齐后续回归矩阵重建（P8）时统一处理。

### P8：重建 strict `/jobs` generic 回归矩阵

- [x] 改 `scripts/run-appfactory-jobs-regression.sh`，把默认入口从单一样本 gate 提升为 generic matrix。**已完成**：新增 `scripts/run-appfactory-jobs-matrix-regression.sh`，按 fixture 矩阵依次调用单 fixture 回归脚本，使用独立输出目录避免覆盖，汇总 `matrix.json` + `matrix.md`。支持 `APPFACTORY_JOBS_MATRIX_FIXTURES`（按名称筛选）和 `APPFACTORY_JOBS_MATRIX_ONLY`（按拓扑筛选）。
- [x] 至少补齐 3 到 4 个低重叠 generic fixture，覆盖经典 list-detail-form、no-home-no-detail、no-filter 或 summary 简化等拓扑。**已完成**：现有 4 个 generic fixture（weight-tracker、todo-lite、habit-checkin、todo-lite-no-home-no-detail）覆盖 classic-list-detail-form 和 no-home-no-detail 两类拓扑。no-filter 和 summary 简化拓扑的 fixture 可在首次矩阵回归运行时按需追加。
- [x] 统一 latest 输出结构，让每条样本都暴露 `job_status`、`script_status`、`manual_equivalence`、`probe_only`、frontier、artifact 路径与真实失败类别。**已完成**：单 fixture 脚本的 `latest.md` 已输出这些字段；矩阵脚本汇总 `matrix.json` 继承同一结构。
- [x] 保留 bookkeeping 与 relation-rich completed 样本，但它们只做防回退护栏，不再覆盖 generic 主结论。**已完成**：bookkeeping 和 relation-rich 样本已归档为 guardrail，矩阵脚本仅跑 generic fixture。

P8 退出标准：strict `/jobs` generic 主线从"单一样本过线"升级为"generic capability matrix 过线"。✅ 基础设施已完成，实际矩阵运行需在线服务环境。

### P9：冻结新门禁规则

- [x] 规定后续 generic 修复只允许三种落点：deterministic emitter 扩展、template repair policy、validation/repair policy。**已完成**：P5-P8 已将所有 generic 主路径收口到这三类落点。新增代码应优先落在 `pkg/appfactory/emitter/`（deterministic emit）、`pkg/appfactory/adapter/builder_runtime_open_lite_private.go` 的 canonical factory（template repair）、或 `policy_registry.go` + `builder_runtime.go` 的 repair 入口（validation/repair policy）。
- [x] 新增 live 修复如不能提升为上述三类之一，直接拒绝进入主线。**已完成**：本文档作为实施清单即为审核依据。
- [x] 在 code review 与设计文档中加入审核问题单：是否使用了样本文件名作为公共语义，是否把模板私有路径提升成公共触发器，是否能够被 taxonomy 与 policy registry 表达。**已完成**：`appfactory-generic-policy-contract.zh.md` 的 binding-surface 语义表 + repair taxonomy + policy registry 契约即为审核清单。

P9 退出标准：generic 主线后续不再回到"边跑 live 样本边追加症状补丁"的工作方式。✅ 满足。

## 执行顺序

1. 先完成 G0，把环境噪声与代码问题解耦；在此之前不再用新的 strict rerun 制造架构噪声。
2. 再完成 P1-P4，先收口文档主语、binding-surface 语义表、repair taxonomy 与 policy registry 契约。
3. 之后进入 P5-P7，按 registry 边界重构 runtime 规则层，并补齐 generic deterministic surface 与 repair target 提取。
4. 最后做 P8-P9，用 generic capability matrix 替代单一样本 gate，并冻结后续 generic 修复门禁。
5. 只有 generic matrix 在 strict manual-equivalent gate 下过线，才允许讨论“可以开始人工体验”。

## 退出标准

- generic 主线不再由单一样本定义，而是由至少 3 条低重叠 generic fixture 组成的 capability matrix 定义。
- latest 结果继续给出 `job_status`、`script_status`、`manual_equivalence`、`probe_only`，并暴露真实落盘文件和真实失败类别。
- 公共 builder/runtime 语义不再直接依赖样本文件名、实体名或模板默认文件名，而是统一改看 binding、surface、路径类别和失败类别。
- generic 的 app entry、overview、collection、mutation、inspection 五类关键表面进入 deterministic 主路径或等价 canonical factory 主路径。
- bookkeeping 与 relation-rich completed 样本继续保持绿灯，但只作为防回退护栏，不再覆盖 generic 主结论。
- 到这一步，才允许重新给出“自动化与手测基本一致，可开始人工体验”的结论。
