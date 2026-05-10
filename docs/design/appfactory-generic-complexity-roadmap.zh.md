# AppFactory Generic App 复杂度升级路线图

> 状态：Proposed
>
> 日期：2026-05-10
>
> 前置结论：`generic-deterministic-rollout.zh.md` 已证明 `flutter-open-lite` generic deterministic 链路能稳定生成可编译 Flutter 工程；本文件接续定义下一阶段目标：先做到语义保真，再在同一条 deterministic 主链上提升数据、交互、状态、工作流复杂度。

---

## 1. 当前结论

### 1.1 已经达成

- `/jobs` live 验证 5/5 通过，均完成 `flutter analyze`、`flutter test`、Debug APK build。
- deterministic route 已绕开 LLM patch generation，避免模型超时和 hallucination 影响已知 generic open-lite 表面。
- 生成文件结构稳定：model、repository、controller、home/list/form/detail/main/copy/test 均能闭环。
- 当前代码已经适合作为“可编译多页面工具类 skeleton”的基线。

### 1.2 主要缺口

| 缺口 | 证据 | 影响 |
|---|---|---|
| 领域字段被压成通用记录 | 五个新用例的 `domain-model.json` 都是 `title/category/status/updated_at/note` | 课程截止日、疫苗提醒日、电影评分、训练时长等核心需求没有落地 |
| 生成代码横向同质化 | 五个项目的 `record.dart`、`record_form_page.dart`、`widget_test.dart` hash 一致 | 不能证明“一句话需求 -> 具体 app”，只能证明“一句话需求 -> 通用 CRUD app” |
| 表单没有覆盖模型字段 | form 只写 `title` 和 `note` | `category/status/updatedAt` 基本不可编辑，summary/filter 变成装饰性功能 |
| 持久化没有真实闭环 | 生产入口使用 Hive custom object，但未初始化 Hive Flutter/adapter；widget test 使用 in-memory repo | 编译通过不等于真机持久化可用 |
| acceptance 偏结构，不偏语义 | 当前 16/16 主要保证文件、编译、导航骨架 | 无法拦住字段缺失、删除越界、筛选缺失、品牌名不对等问题 |

### 1.3 根因判断

问题优先发生在 prepare 阶段，而不是单纯发生在 emitter 阶段。

当前 `genericDomainSignals` 的默认 profile 把未知 generic 需求统一映射成“通用记录”。`DomainModel` 结构本身已经支持 `Entities`、`SummaryMetrics`、`SemanticAcceptanceRules`，但当前字段提取、字段角色、字段级验收没有被真实填充。deterministic emitter 只是稳定地消费了这个过度简化后的 schema。

---

## 2. 总目标

下一阶段目标分两层，不把“字段变多”误判为“app 复杂度提升”。

第一层是语义保真：

> 对单句需求，系统能稳定产出带领域字段、领域文案、领域表单、领域列表/详情/摘要、真实持久化和行为测试的 Flutter MVP。

第二层是复杂度提升：

> 在语义保真的基础上，系统能逐步生成带多字段校验、状态流转、条件交互、聚合指标、轻关系模型和可验证工作流的 Flutter app。

衡量标准从 “5/5 build passed” 升级为：

- 需求中的核心名词/字段进入 `domain-model.json`，且字段具备角色、类型、校验和展示意图。
- 生成 Dart model、repository、表单、列表、详情、摘要、测试全部跟 `domain-model.json` 对齐。
- 复杂行为能被测试证明：字段校验、排序/筛选、状态变化、聚合统计、删除确认、持久化恢复。
- 未被需求声明的能力不越界生成，例如未要求删除时不出现删除 UI，禁用首页时不生成 home route。
- 五个不同需求的关键文件不再完全同构，并且差异来自领域字段和行为，不只是文案差异。
- L1/L3 稳定后，能进入轻关系 L4，而不是把所有需求继续塞进单个 `Record`。

---

## 3. 复杂度定义与量化指标

复杂度分为四个正交维度。后续任何“更复杂 app”需求，都必须说明具体提升了哪个维度，而不是只说页面更多或字段更多。

| 维度 | L1 基线 | L2/L3 增强 | L4+ 复杂化信号 | 验收方式 |
|---|---|---|---|---|
| 数据复杂度 | 单实体、4-6 个领域字段、基础类型 | 字段角色、派生字段、枚举、日期、数值指标 | 2 个实体、外键/引用、父子聚合 | `domain-model.json` golden + generated model/repository test |
| 交互复杂度 | 创建、列表、详情 | 编辑、筛选、排序、条件按钮、字段校验 | 跨页面选择、批量操作、主从导航 | widget test 覆盖主流程和负向能力 |
| 状态复杂度 | 简单状态字段 | 状态流转、到期/逾期、完成/未完成、评分/进度 | 状态机、软删除、归档、撤销 | behavior acceptance + semantic rule evidence |
| 工作流复杂度 | 单次 CRUD | 聚合摘要、最近记录、提醒候选、周期判断 | 多步骤流程、关系聚合、周期任务 | live `/jobs` semantic matrix + persistence smoke |

复杂度提升的最低门槛：每一阶段至少提升一个维度，并且新增复杂度必须被自动验收捕获。

---

## 4. 复杂度分层

后续不要一次性追求“复杂 app”，先分层推进。

| 层级 | 名称 | 能力定义 | 例子 | 退出标准 |
|---|---|---|---|---|
| L0 | 稳定骨架 | 当前 generic CRUD skeleton，可编译、可导航、可测试 | 通用记录 app | 已完成 |
| L1 | 领域单实体 | 单实体字段能按需求生成，含 string/int/double/date/bool/enum 与字段角色 | 课程作业、宠物疫苗、观影清单 | 5 个领域字段级 golden 通过 |
| L2 | 字段驱动行为 | 表单控件、字段校验、列表副标题、详情、排序/筛选、摘要指标按字段角色生成 | 电影评分筛选、植物需浇水数、训练总次数 | widget test 覆盖 create/list/detail/summary/delete gating |
| L3 | 单实体工作流 | 状态流转、到期/逾期、周期判断、聚合摘要、条件操作 | 作业逾期、疫苗下次提醒候选、浇水周期 | behavior acceptance 覆盖状态变化和派生指标 |
| L4 | 轻关系模型 | 允许 2 个实体和简单关系，支持主从选择、关系详情、父子聚合 | 宠物 -> 疫苗记录，课程 -> 作业 | 关系选择/详情聚合通过 live 验证 |
| L5 | 复杂能力 | 提醒、导出、归档、软删除、批量操作、多步骤流程 | 疫苗提醒、训练计划归档、批量完成 | 需要新 capability contract，不作为当前阶段默认目标 |

当前应该先完成 L1/L2，并在 P1/P2 就预留 L3/L4 的字段角色、能力标记和关系合同。不要在 L0 骨架还未语义保真前扩展更多 live case 数量。

---

## 5. 优化路线

### P1：prepare 语义保真

目标：让 `domain-model.json` 不再退化为固定“通用记录”。

改动方向：

- 在 generic prepare 中引入字段提取层，输出领域字段 schema。
- 扩展 generic profile catalog，让它保存“领域字段预设”，而不是完整 app 模板。
- 给字段补充语义角色：`primary_text`、`secondary_text`、`status`、`date`、`due_date`、`amount`、`rating`、`duration`、`note`、`metric_source`。
- topology 能力必须从需求和字段共同推导：有 enum/status 才考虑筛选；明确说删除才生成删除；明确说不需要首页则关闭 overview。
- 输出复杂度画像：`complexity_level`、`complexity_dimensions`、`capability_flags`，至少区分 create/edit/delete/filter/sort/summary/readonly/persistence。
- 即使 L1 只生成单实体，也要预留 relation contract：实体可声明 `entity_role`、候选 `relation_refs`、父子聚合意图，避免后续 L3/L4 被迫重写 prepare artifact。

首批字段 profile：

| 需求类型 | 建议主实体字段 |
|---|---|
| 课程作业追踪 | `course_name`、`assignment_title`、`due_date`、`status`、`note` |
| 宠物疫苗记录 | `pet_name`、`vaccine_name`、`vaccinated_at`、`next_due_at`、`note` |
| 植物浇水记录 | `plant_name`、`location`、`last_watered_at`、`water_status`、`note` |
| 观影清单 | `movie_title`、`genre`、`watch_status`、`rating`、`review` |
| 运动训练日志 | `workout_name`、`workout_date`、`duration_minutes`、`completion_status`、`note` |

P1 退出标准：

- `go test ./pkg/appfactory/prepare/ -count=1` 中新增 5 个字段级 golden。
- 五个需求的 `domain-model.json` 不再完全相同。
- `SemanticAcceptanceRules.FieldRefs` 覆盖每个需求的核心字段。

### P2：schema-driven deterministic emit

目标：emitter 不再假设 `title/category/status/updatedAt/note` 固定存在，而是消费 `domain-model.json`。

改动方向：

- `emitGenericRecordModel` 生成任意单实体字段：支持 `String`、`int`、`double`、`bool`、`DateTime`、enum。
- `emitGenericSummaryModel` 从 `summary_metrics` 或字段角色生成实际指标，而不是固定 `totalCount/doneCount`。
- form generator 按字段类型生成控件：
  - string/text -> `TextFormField`
  - int/double -> number keyboard + parser validation
  - DateTime -> date picker
  - bool -> switch/checkbox
  - enum -> dropdown/segmented control
- list/detail generator 按字段角色选择展示：primary、secondary、status、time、metric、note。
- 行为 generator 按字段角色生成默认复杂行为：日期排序、到期/逾期判断、enum 筛选、数值聚合、评分展示、完成状态切换。
- copy emitter 根据字段 label 生成领域文案。
- app entry 使用 typed callback，去掉 `dynamic`。

P2 退出标准：

- 五个项目的 `record.dart`、`record_form_page.dart`、`widget_test.dart` 不再完全相同。
- 生成 Dart 文件没有 stale `RecordStatus`、`title`、`category`、`statusLabel` 引用。
- 未声明字段不出现在 UI 和测试里。

### P3：真实持久化闭环

目标：编译通过之外，生产 repository 真的可初始化、可保存、可恢复，并为多实体/关系/迁移打基础。

推荐策略：短期不要用 Hive custom object adapter 作为 generic 默认路径。

可选方案：

| 方案 | 优点 | 风险 | 建议 |
|---|---|---|---|
| Hive `Box<Map>` / `Box<String>` JSON DTO | 不需要代码生成 adapter；schema-driven 简单 | 类型转换要严谨 | 首选 |
| 为每个 model 生成 `TypeAdapter` | 性能和类型更强 | adapter typeId、注册、代码复杂度高 | L2 后再评估 |
| 纯 in-memory | 测试简单 | 不满足持久化 | 只用于 widget test 注入 |

P3 退出标准：

- production `main()` 明确初始化持久化环境。
- repository 有明确 DTO mapping、schema version 和迁移策略占位。
- repository 单元测试覆盖 add/update/delete/reload。
- live acceptance 新增 persistence smoke：创建记录、重启 repository、重新读取字段。

### P4：语义与复杂度验收升级

目标：acceptance 不再只看 compile/test/build，而要证明需求语义和新增复杂行为都被保留。

新增检查：

- `check-domain-model-field-coverage`：需求核心字段必须进入 `domain-model.json`。
- `check-generated-field-coverage`：model/form/detail/test 必须引用核心字段。
- `check-negative-capability`：未要求删除时不能生成删除 action；禁用首页时不能生成 home route。
- `check-behavior-complexity`：排序、筛选、状态流转、聚合摘要、到期判断等复杂行为必须有证据。
- `check-relation-readiness`：L4 场景必须证明两个实体、引用字段、主从导航和聚合指标存在。
- `check-diversity`：同一矩阵内核心生成文件不能全部 hash 一致。
- `check-persistence-runtime`：真实 repository 持久化 smoke。

P4 退出标准：

- 五个新领域 5/5 通过语义验收。
- 任意字段缺失会导致 acceptance 失败，而不是只进入人工 review。
- auto repair 仍保持 0；已知 schema-driven 场景不允许退回 LLM patch。

### P5：复杂度扩展到轻关系和工作流

目标：在 L1/L2 稳定后，允许小规模 relation-rich generic app，并把 L3 单实体工作流升级到 L4 轻关系。

进入条件：

- L1/L2 五域矩阵连续通过。
- 字段角色、控件映射、持久化、语义验收都已固化。
- 单实体 schema-driven emitter 不再依赖默认 record 字段。

首批轻关系场景：

- 宠物与疫苗记录：`Pet` + `VaccineDose`
- 课程与作业：`Course` + `Assignment`
- 训练计划与训练日志：`WorkoutPlan` + `WorkoutSession`

首批工作流增强场景：

- 课程作业：按 `due_date` 推导逾期，支持状态从待办到已完成。
- 植物浇水：按 `last_watered_at` 和周期字段推导需要浇水。
- 观影清单：按 `watch_status` 和 `rating` 聚合已看数量与平均评分。

P5 不应提前解决日历同步、导出、账号、多端同步等 L5 能力。

---

## 6. 验证矩阵

### 6.1 L1/L3 固定矩阵

| # | 需求 | 数据复杂度 | 行为/状态复杂度 | 负向/边界验收 |
|---|---|---|---|---|
| 1 | 课程作业追踪 app，记录课程、作业标题、截止日期、状态和备注，支持新增、列表、详情和删除。 | `course_name`、`assignment_title`、`due_date`、`status`、`note` | 按截止日期排序；能标记完成；逾期状态可被推导或展示 | 删除必须二次确认；无关字段不出现 |
| 2 | 宠物疫苗记录 app，记录宠物名、疫苗名称、接种日期、下次提醒日期和备注，支持新增、列表和详情。 | `pet_name`、`vaccine_name`、`vaccinated_at`、`next_due_at`、`note` | 首页或列表能突出下一次接种；按 `next_due_at` 排序 | 需求未声明删除，不能出现删除入口 |
| 3 | 植物浇水记录 app，记录植物名称、位置、上次浇水日期、状态和备注，首页显示需要浇水数量。 | `plant_name`、`location`、`last_watered_at`、`water_status`、`note` | summary 显示需浇水数量；列表能优先显示需浇水植物 | 无详情需求时不得强制详情；状态文案必须是浇水语义 |
| 4 | 观影清单 app，记录电影名、类型、观看状态、评分和短评，支持新增、列表、详情和删除。 | `movie_title`、`genre`、`watch_status`、`rating`、`review` | 支持按观看状态筛选；详情展示评分；summary 可显示已看数量或平均评分 | 删除必须二次确认；评分校验范围明确 |
| 5 | 运动训练日志 app，记录训练项目、日期、时长、完成状态和备注，首页显示总次数和最近训练。 | `workout_name`、`workout_date`、`duration_minutes`、`completion_status`、`note` | summary 显示总次数和最近训练；按日期排序；时长做数值校验 | 不生成与训练无关的分类/status 文案 |

### 6.2 负向矩阵

| 需求 | 预期 |
|---|---|
| 便签，列表编辑不需首页 | 无 home route；列表是入口；支持编辑 |
| 只读电影榜单，不允许新增和删除 | 无 FAB/create/delete；只读列表和详情 |
| 简单喝水计数，不需要详情页 | 无 detail route；主页面能增加计数 |

### 6.3 L4 轻关系预备矩阵

| 需求 | 必须证明的关系复杂度 |
|---|---|
| 宠物疫苗管理，维护宠物资料和每只宠物的疫苗记录 | `Pet` 与 `VaccineDose` 两个实体；疫苗记录引用宠物；宠物详情聚合疫苗记录 |
| 课程作业管理，维护课程并给课程添加作业 | `Course` 与 `Assignment` 两个实体；作业引用课程；课程详情显示作业列表和未完成数 |
| 训练计划和训练日志，计划下记录每次训练 | `WorkoutPlan` 与 `WorkoutSession` 两个实体；日志引用计划；计划详情聚合完成次数和总时长 |

---

## 7. 实施顺序建议

1. 先补 prepare golden：证明字段能进入 `domain-model.json`。
2. 再改 model/repository emitter：字段类型和持久化先闭环。
3. 再改 form/list/detail/home/copy/test：UI 和测试消费 schema。
4. 最后升级 live acceptance：让语义问题自动失败。
5. L1/L2 连续通过后，进入 L3 单实体工作流；L3 稳定后，再进入 L4 轻关系。

这个顺序的核心理由：如果 prepare 仍输出固定“通用记录”，后面的 emitter 越强，只会越稳定地生成错误抽象。

---

## 8. 阶段里程碑

| 里程碑 | 范围 | 必须产出 | 不允许通过的情况 |
|---|---|---|---|
| M1：语义保真 | L1 数据复杂度 | 五个领域的 `domain-model.json`、generated model、form、detail、test 都含领域字段 | 核心字段缺失但 build passed |
| M2：行为复杂度 | L2/L3 交互与状态复杂度 | 每个 app 至少一个字段驱动行为：排序、筛选、状态切换、聚合、到期判断之一 | 只有字段展示，没有行为差异 |
| M3：关系预备 | L4 轻关系合同 | prepare artifact 能表达两个实体、引用字段、主从导航和聚合验收 | 继续把父子实体压进单个 `Record` |
| M4：复杂 app 基线 | L4 live 验证 | 至少 2 个轻关系 app live 通过 semantic + compile + persistence acceptance | 只能依赖 LLM repair 或人工判定通过 |

M1/M2 是近期主线；M3 只做合同预埋和小样本验证；M4 要等 L1-L3 连续稳定后再进入。

---

## 9. 不做事项

- 不把更多 prompt repair 当作主路径；已知 generic open-lite 表面应继续 deterministic。
- 不用单个垂直样本反向定义公共模板合同。
- 不在 L1-L3 未稳定前引入账号、云同步、推送、地图、支付、导出等复杂能力。
- 不以“build passed” 单独判定成功；语义字段和行为必须成为硬验收。

---

## 10. 近期任务拆分

| 任务 | 文件/模块 | 验证 |
|---|---|---|
| T1 字段 profile 设计 | `pkg/appfactory/prepare/generic_domain_profiles.json` / `generic_domain_profiles.go` | 5 个需求 `domain-model.json` golden |
| T2 字段角色合同 | `pkg/appfactory/prepare/compiler.go` 或 execution contract 扩展 | `SemanticAcceptanceRules` 覆盖核心字段 |
| T3 schema model/repository emitter | `pkg/appfactory/adapter/emit_runner.go` | Go unit + generated Dart analyze |
| T4 schema form/list/detail/home emitter | `pkg/appfactory/adapter/builder_runtime_open_lite_private.go` | widget test 覆盖 create/list/detail/summary |
| T5 持久化 runtime smoke | acceptance plan / generated repository tests | create + reload + assert field values |
| T6 行为复杂度验收 | acceptance plan / matrix script | 排序、筛选、状态、聚合、到期判断至少覆盖 5 类中的 3 类 |
| T7 live 语义矩阵 | `/jobs` regression script 或 matrix script | 5/5 semantic + behavior + compile acceptance passed |
| T8 轻关系合同预研 | prepare artifact / execution contract | 2 实体 + relation refs golden，不要求先 live 全链 |

---

## 11. 阶段性成功定义

下一次可以称为“复杂度提升有效”的标准：

- 5 个领域需求均生成不同的领域模型和表单。
- 每个 app 至少有 4 个领域字段进入 create/list/detail/test。
- 每个 app 至少有 1 个字段驱动复杂行为被测试覆盖。
- 删除、首页、详情、筛选等能力按需求启停。
- production repository 的持久化 smoke 通过。
- prepare artifact 已能表达轻关系所需的实体角色、引用字段和聚合验收。
- 仍保持无 LLM repair、无 model timeout 对已知 generic surface 的影响。

达到这个标准后，再扩大到更复杂的 app：轻关系、周期任务、状态机、聚合统计和导出才有可靠基础。