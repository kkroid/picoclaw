# Generic Deterministic Route-Hint Rollout 实施清单

> 状态：R1-R3 已完成，进入持续矩阵验证
>
> 前置文档：`appfactory-architecture-evolution.zh.md`（统一确定性生成架构）与 `appfactory-architecture-evolution-todo.zh.md`（P1-P9 完成）。P5-P6 已将 deterministic emit 基础设施就位（5 个 from-scratch 生成器、emit_runner 回退链路、`selectBuilderRuntimeRoute` 修复、`emit_eligible` 收口），验证会话确认模型与仓储生成器可稳定工作。
>
> 本文件将剩余工作定义为 **配置级 rollout**——将 generic task bundle 中的任务从 `StrongModel` / 空 `RouteHint` 逐个迁移到 `Deterministic`，并在 live `/jobs` 环境下逐批验证。当前配置级 rollout 已完成，后续仅保留持续矩阵验证记录。

---

## 1. 问题描述

### 1.1 当前状态

P5-P9 的退出标准全部满足：registry 合同已冻结、consumer 迁移矩阵已完成、deterministic emit 基础设施已就位。代码层可确定性地生成以下核心文件：

| 生成器 | 函数 | 位置 |
|---|---|---|
| record 模型 | `deterministicGenericModelOperations` → `emitGenericRecordModel` | `emit_runner.go` |
| dashboard_summary 模型 | `deterministicGenericModelOperations` → `emitGenericSummaryModel` | `emit_runner.go` |
| record_repository | `emitGenericRepositoryContent` | `emit_runner.go` |
| home_controller | `builderRuntimeOpenLiteCanonicalGenericOverviewController` | `builder_runtime_open_lite_private.go` |
| home_page | `builderRuntimeOpenLiteCanonicalGenericOverviewPage` | 同上 |
| record_list_controller | `builderRuntimeOpenLiteCanonicalNoFilterListController` | 同上 |
| record_list_page | `builderRuntimeOpenLiteCanonicalNoFilterListPage` | 同上 |
| record_form_page | `builderRuntimeOpenLiteCanonicalGenericMutationPage` | 同上 |
| record_detail_page | `builderRuntimeOpenLiteCanonicalGenericInspectionPage` | 同上 |
| main.dart | `builderRuntimeOpenLiteCanonicalGenericAppEntry` | 同上 |

`recognition.go` 中 `buildGenericTaskBundleForTopology` 的 generic 任务已全部迁移到 `RouteHintDeterministic`。后续发现的 acceptance failure 已收口为确定性生成器缺口，并已修复：删除接线、summary/model 标识符、generic copy/test、repository in-memory test implementation、registry 不完整时的 surface class fallback。

### 1.2 已验证事实

在 `flutter-open-lite` 模板 + "读书笔记" 需求下，live 测试确认 **14 个确定性任务 / 15 个生成文件全部成功应用**，并通过 16/16 acceptance checks：

```
✅ lib/models/record.dart
✅ lib/models/dashboard_summary.dart
✅ lib/repositories/record_repository.dart
✅ lib/controllers/home_controller.dart
✅ lib/controllers/record_list_controller.dart
✅ lib/controllers/record_form_controller.dart
✅ lib/template/open_lite_copy.dart
✅ android/app/src/main/res/values/strings.xml
✅ android/app/build.gradle.kts
✅ lib/views/home_page.dart
✅ lib/views/record_list_page.dart
✅ lib/views/record_form_page.dart
✅ lib/views/record_detail_page.dart
✅ lib/main.dart
✅ test/widget_test.dart
```

验证结果：`result=COMPLETED`，`thin executor completed: 16/16 checks passed`，无 auto repair、无 LLM patch generation、无 probe-only 输出。

扩大矩阵新增 5 个领域用例，均已顺序通过 live `/jobs`：

| # | 需求 | 结果 |
|---|---|---|
| 1 | 课程作业追踪 app，记录课程、作业标题、截止日期、状态和备注，支持新增、列表、详情和删除。 | ✅ 16/16 checks passed |
| 2 | 宠物疫苗记录 app，记录宠物名、疫苗名称、接种日期、下次提醒日期和备注，支持新增、列表和详情。 | ✅ 16/16 checks passed |
| 3 | 植物浇水记录 app，记录植物名称、位置、上次浇水日期、状态和备注，首页显示需要浇水数量。 | ✅ 16/16 checks passed |
| 4 | 观影清单 app，记录电影名、类型、观看状态、评分和短评，支持新增、列表、详情和删除。 | ✅ 16/16 checks passed |
| 5 | 运动训练日志 app，记录训练项目、日期、时长、完成状态和备注，首页显示总次数和最近训练。 | ✅ 16/16 checks passed |

### 1.3 核心差距：已消除

`buildGenericTaskBundleForTopology` 的 14 个 generic 任务已全部设为 `RouteHintDeterministic`。`task-create-form-controller` 因 generic mutation 是 repository-driven 模式，不作为独立任务调度；deterministic form slot 会按需生成空壳 stub，避免旧 normalize/import 链路产生编译缺口。

---

## 2. 目标

将 generic `flutter-open-lite` 模板的全部可确定性任务从 LLM 路径迁移到 `Deterministic`，使 `/jobs` 全链在 **不依赖 LLM 的前提下** 稳定产出可编译 Flutter 工程。

### 2.1 范围

- **涉及** `recognition.go:buildGenericTaskBundleForTopology` 的 `RouteHint` 字段收口
- **涉及** generic deterministic emitters 与 canonical fallback 的稳定性修复
- **只覆盖** `flutter-open-lite` 模板（generic open-lite），不涉及 `flutter-finance-lite` 或 relation-rich 路径

### 2.2 不做

- 不改进 LLM prompt（确定性路径成功后 LLM 路径将逐渐退场）
- 不改 relation-rich emitter 合同（relation-rich 已有独立的 emitter 体系）
- 不改变任务依赖顺序或分配逻辑

---

## 3. 任务分类

generic task bundle 的可确定性任务已全部迁移完成。当前分类仅用于说明最终归属：

| 类别 | 任务 | 状态 | 说明 |
|---|---|---|---|
| Domain/model | `task-create-record-model`、`task-create-summary-model`、`task-create-repository` | ✅ 完成 | 从 `domain-model.json` 生成稳定 ASCII Dart 类型、默认构造值、Hive + in-memory repository。 |
| Controller | `task-create-home-controller`、`task-create-list-controller` | ✅ 完成 | registry 不完整时使用稳定 generic fallback，按 topology 生成删除接线。 |
| Surface | `task-bind-overview-surface`、`task-bind-collection-surface`、`task-bind-mutation-surface`、`task-bind-inspection-surface` | ✅ 完成 | 生成 homepage/list/form/detail；form 为 repository-driven，按需补 `record_form_controller.dart` stub。 |
| App entry | `task-bind-app-entry` | ✅ 完成 | 使用稳定 generic `AppFactoryApp`、repository、controller、surface import 与 CRUD navigation。 |
| Copy/branding/build/test | `task-create-copy`、Android branding/build config、`task-create-test` | ✅ 完成 | copy/test 走 deterministic emitter；generic widget test 与 `AppFactoryApp`/`InMemoryRecordRepository` 对齐。 |

`task-create-form-controller` 不再作为独立 generic 任务存在；它的文件只作为 mutation slot 的兼容 stub 生成。

---

## 4. 分阶段实施

### R1：全部确定性任务一次迁移 ✅ 完成

14 个 generic 任务（除已移除的 `task-create-form-controller`）全部设为 `Deterministic`，live 验证 14 个任务 / 15 个文件通过。

### R2：跨领域验证 ✅ 完成

使用 5 个不同领域 fixture 验证稳定性。最新扩大矩阵覆盖课程作业、宠物疫苗、植物浇水、观影清单、运动训练，全部 `COMPLETED`。

### R3：flutter analyze acceptance check 收尾 ✅ 完成

确定性生成文件已通过 `flutter analyze`、`flutter test` 与 Debug APK 构建验收。

---

## 5. 验证方法

### 5.1 单元验证

每轮 `RouteHint` 修改后，运行：

```bash
go test ./pkg/appfactory/adapter/ -count=1 -timeout 300s
go test ./pkg/appfactory/prepare/ -count=1 -timeout 60s
```

### 5.2 Live 集成验证

使用单一 generic fixture 做端到端验证：

```bash
APPFACTORY_JOBS_TEMPLATE_ID="flutter-open-lite" \
  bash scripts/run-appfactory-jobs-regression.sh \
  --api-base http://127.0.0.1:18800 \
  --requirement "读书笔记，记录书名进度和摘抄" \
  --title "/jobs deterministic rollout" \
  --timeout-seconds 3600 \
  --output-root /tmp/oneappfactory-jobs-test \
  --allow-non-manual-equivalent
```

验证事件流中 `run_patch_applied` 数量递增，且无 `builder_runtime_model_request_failed` 或 `dart format rejected` 错误。

### 5.3 多元化矩阵验证

使用至少 3 个不同领域的 fixture 作为矩阵验证。当前已完成 5 个新领域顺序验证，输出目录为 `/tmp/oneappfactory-jobs-expanded/runs/*`。

---

## 6. 退出标准

- [x] A 类 10 个任务全部 `RouteHintDeterministic`（实际 14 个）
- [x] R1 在 live 环境下 14 个任务 / 15 个文件通过确定性生成
- [x] 无 LLM 依赖的编译错误
- [x] `go test ./pkg/appfactory/adapter/` 全包 PASS
- [x] 至少 3 个不同领域 fixture 各通过编译链（当前 5/5 通过）

---

## 7. 风险与已知限制

| 风险 | 缓解措施 |
|---|---|
| `task-create-form-controller` 无法独立确定性生成 | generic mutation page 是 repository-driven；slot emit 时生成空壳 stub，任务本身不独立调度 |
| registry 在 workspace 文件尚未完整时误判 class/import | generic fallback 对 app entry、form、list、detail 使用稳定默认类名和 import，避免读取 seed/半成品 surface |
| 不同领域 fixture 的 domain-model.json 差异导致生成器输出不通用 | 生成器只消费标准化字段（`entity_id`、`name`、`type`、`required`），模型构造器为通用字段提供默认值 |
| 后续模板或 relation-rich 路径受 generic fallback 影响 | 保持 scope 在 `flutter-open-lite` generic；relation-rich emitter 与 legacy fallback 继续走既有路径 |

## 8. 与前置文档的关系

| 文档 | 状态 | 关系 |
|---|---|---|
| `appfactory-architecture-evolution.zh.md` | Frozen | 统一确定性生成架构定义 |
| `appfactory-architecture-evolution-todo.zh.md` | 已完成 | P1-P9 退出标准已满足；可冻结 |
| **本文档** | **Active** | 配置级 rollout 新前线 |
| `appfactory-generic-policy-contract.zh.md` | Active | binding-surface 语义表 + repair taxonomy |
| `appfactory-generic-complexity-roadmap.zh.md` | Proposed | 下一阶段：从稳定通用 CRUD 骨架升级到领域字段、语义验收和轻复杂度 app |
