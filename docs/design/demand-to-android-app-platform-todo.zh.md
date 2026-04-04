# PicoClaw Android App 平台推进清单（任务中心重设计版，2026-04-03）

本文档本轮只服务一个目标：

- 重新设计整个任务中心页面，显著提升可读性、可操作性和问题定位效率。

这里的“任务中心”不只指状态展示，还包括：新建任务、任务列表、任务详情、执行状态、通知、产物、交付、编排器入口等整套 `/jobs` 页面体验。

## 1. 本轮目标

本轮不是继续零碎补文案，而是把 `/jobs` 从“功能堆叠页”重做成“任务中心工作台”。

目标结果：

- 用户进入页面后，先看到“现在应该关注什么”，而不是先被大量实现细节淹没。
- 新建任务入口足够短，只保留应用名称和需求说明，技术对象全部退到系统内部。
- 任务列表承担“筛选和切换”职责，不再承担完整详情页职责。
- 任务详情承担“定位问题和执行动作”职责，不再把运行时、交付、路径、状态上下文、失败上下文平铺成信息墙。
- 长耗时状态需要分层表达，既能一眼看懂当前卡在哪，也能继续展开看细节。
- 整个页面需要先按用户任务流组织，再按技术对象组织。

## 2. 当前问题诊断

基于当前实现，任务中心可用性差的核心原因不是单点 bug，而是信息架构错误。

### 2.1 页面层级错误

- 顶部同时暴露 runtime mode、summary、orchestrator 控制，页面一进入就像运维后台，不像任务中心。
- 左侧列表区同时承担筛选、状态说明、摘要阅读职责，卡片密度过高。
- 右侧详情区同时塞入通知、任务详情、执行拆解、运行时、预算、路径、状态上下文、失败上下文、交付表单、产物清单，缺少主次。

### 2.2 任务流断裂

- 新建任务、查看进度、处理失败、检查产物、做交付，这几个核心任务流没有被清晰串起来。
- 页面提供了很多信息，但没有明确告诉用户“下一步做什么”。
- 运行中的任务、失败任务、待人工处理任务，目前都混在同一浏览路径里。

### 2.3 技术细节前置过度

- 编排器、runtime、路径、builder 输入输出路径等底层对象暴露过早。
- 技术对象虽然真实，但不应该在默认视图占据黄金位置。
- 当前页面更像调试面板，而不是产品化工作台。

### 2.4 代码结构已经拖累交互演进

- 当前 `jobs-page.tsx` 已经演化成单文件超大组件，同时承载新建、列表、通知、详情、交付、产物、状态摘要、失败引导等多种职责。
- 如果不先拆页面信息架构，继续在现有布局上补功能，只会让后续维护更差。

## 3. 重设计原则

- 默认视图先回答三个问题：哪些任务需要我关注、当前卡在哪、下一步做什么。
- 把“技术真实”留在二级展开层，不在一级视图直接堆满。
- 把“列表页”和“详情页”彻底分工，避免同一信息重复出现三次。
- 所有状态文案都围绕用户可理解动作命名，不使用实现名词替代产品语义。
- 所有可执行动作都要靠近上下文，不让用户在页面不同区域来回找按钮。
- 文档、前端布局、状态模型、回归入口必须同时调整，不允许只改其中一层。

## 4. 目标信息架构

### 4.1 顶部工作台

顶部只保留任务中心真正需要的全局信息：

- 全局概览：进行中、待处理、失败、最近完成。
- 快捷动作：新建任务、刷新。
- 可折叠的“系统状态”入口：编排器、builder runtime 路由、异常锁等内容放进二级抽屉或折叠卡。

结论：orchestrator 不再占据首页主视觉区，而是退到“系统状态”里。

### 4.2 左侧任务列表

左侧列表只做三件事：

- 筛选任务
- 快速识别状态
- 切换当前任务

每张任务卡只保留：

- 应用名称
- 任务状态
- 当前阶段
- 当前步骤或失败摘要
- 最近更新时间

默认隐藏：

- PRD ID
- 模板 ID
- 审批数量
- 产物数量
- 运行时 phase 原始值

这些内容只在详情区查看，不再占用列表卡片空间。

### 4.3 右侧详情区

右侧详情区重构为 4 个固定标签页：

1. 概览
2. 执行
3. 产物
4. 交付

标签页职责：

- 概览：任务摘要、当前状态、下一步动作、失败原因摘要。
- 执行：时间线、阶段拆解、事件流、检查项。
- 产物：APK、报告、日志、本地路径。
- 交付：review / signing / release / follow-up。

结论：运行时参数、预算、底层路径、状态上下文等调试信息统一收纳到概览页底部的“高级信息”折叠区。

### 4.4 通知区

通知不再与任务详情并列抢空间，改为：

- 顶部“待处理事项”汇总条
- 或右侧详情区中的“待处理动作”模块

目标是让通知变成动作入口，而不是独立信息面板。

### 4.5 新建任务入口

新建任务弹窗固定只保留：

- 应用名称
- 需求说明

默认行为：

- 提交即自动编译并创建任务
- 系统自动生成 PRD / Job 等内部对象
- 不展示内部 ID、prepare bundle、builder-input 这些实现细节

如果后续要补高级选项，只允许放进“高级设置”折叠区，且默认关闭。

## 5. 本轮执行 TODO

### 5.1 信息架构与交互冻结

- [x] 先冻结任务中心四大一级区域：顶部工作台、任务列表、任务详情标签页、新建任务弹窗。
- [x] 明确哪些信息属于默认视图，哪些信息必须降级到折叠区或二级页。
- [x] 明确通知、失败引导、建议动作三者的统一入口，避免重复出现。

### 5.2 任务列表重做

- [x] 任务卡片改为“应用名称优先”，弱化内部 ID。

说明：当前公开任务记录已补齐稳定的 `title` 字段，来源直接取自准备链路里的 PRD 标题；任务列表已改为应用名称主展示、任务 ID 弱化展示，同时任务搜索也已纳入标题匹配，不再依赖内部 ID 充当首要识别信息。

- [x] 任务卡片只保留一个主状态和一个辅助摘要，不再并排堆多组元信息。
- [x] 补任务搜索与更清晰的筛选器，不只按状态切换。
- [x] 统一运行中、失败、待处理、已完成四类视觉层级。

补充：当前列表卡片已删除默认展示的模板 ID / 产物计数尾栏，把技术元信息从默认卡片中移除；应用名称现在作为首屏主标题，任务 ID 仅保留为弱化副文案，首屏重点收敛到状态徽标、阶段徽标、待处理数量、当前活动和辅助摘要。同时已基于现有 `status + attentionCount` 建立稳定的卡片视觉层级：失败态使用 rose tone，待处理态使用 amber tone，已完成态使用 emerald tone，选中态继续保留 primary tone；筛选器也新增了独立的“待处理 / Needs attention”聚合视图，把失败任务、待人工审批任务和存在建议动作的任务统一到任务流筛选中，不再只依赖原始状态值。

补充：本轮又把列表卡片的默认信息层级继续压成“三段式”结构：顶部状态带只保留状态、阶段、待处理数量和更新时间；中段只保留“应用名称 + 弱化 job ID”；底部只保留一行主摘要和一段可选副摘要，不再额外插入“当前状态”这类中间标题层，进一步把任务列表收敛成真正的任务切换器，而不是半个详情页。

补充：本轮继续把列表顶部控制区压缩成一体化操作带，把搜索与筛选器收进同一个紧凑容器，同时把任务卡片内外留白再缩一档，并把阶段从灰底标签降成弱化文本，避免列表默认视图继续向“信息墙”回弹。

### 5.3 任务详情重做

- [x] 将详情区改为“概览 / 执行 / 产物 / 交付”四标签页。
- [x] 概览页只保留用户决策真正需要的信息，并在首屏给出“下一步动作”。
- [x] 执行页同时支持简版阶段进度和展开后的事件流，不再把所有细节默认展开。
- [x] 失败态补标准化诊断面板：失败阶段、失败摘要、影响范围、建议动作、证据路径。

### 5.4 状态模型重做

- [x] 统一“状态 / 阶段 / 步骤 / 检查项 / 事件”五层模型的前端呈现边界。
- [x] 列表只显示阶段级摘要，详情页再展开到步骤和检查项。
- [x] 长耗时步骤增加稳定的中间态文案，避免用户误判为卡死。
- [x] 文件级执行事实已按保守口径接入，但仍不伪造完整文件状态机。

说明：当前公开事件契约已补齐四层执行事实。第一层是 run 创建/终态事实：`target_paths` 表示 run 创建时计划触达的目标文件，来自 `RunRecord.TaskBundle[].TargetPaths`；`affected_paths` 表示 run 终态时实际改动的文件，来自 `BuildOutput.ModifiedFiles` 与 `RoundOutputs[].WorkspacePatch`；`round_summaries` 则把 `BuildOutput.RoundOutputs` 收敛成终态轮次摘要，当前已包含每轮 `attempt`、轮次总结、当前 phase、`phase_trace`、轮次目标文件、失败检查、轮次改动文件，以及按保守口径生成的 `file_facts`。第二层是运行中 heartbeat 与轮次边界事实：`run_heartbeat` 事件现在也会携带当前 `round_id`、`attempt`、`current_phase`、`phase_trace`、`checkpoint_key` 与当轮目标文件；同时当 heartbeat 首次进入新轮次时，事件流还会显式写入 `run_round_started`，让执行时间线至少具备“哪一轮开始了”的真实边界。第三层是 patch 事实：runner 现在会在真正产出 patch 之前先写入 `run_patch_generation_started`，把“这一轮已经开始尝试生成 patch”这个事实先暴露出来；一旦结构化 patch 生成成功，会立刻写入 `run_patch_generated`；如果生成阶段就因为模型请求失败、patch 解析失败或捕获 patch 失败而中断，则会写入 `run_patch_generation_failed`；如果 patch 已生成但应用到工作区失败，则会写入 `run_patch_apply_failed`；只有 patch 真正落地后，才写入 `run_patch_applied`，把最终改动文件作为独立事件暴露出来，而不是只能等到 `run_completed` 再统一回放。这样 patch 生命周期已经从原来的两个锚点扩展到“生成尝试 / 生成成功 / 生成失败 / 应用失败 / 应用成功”五类公开事实。第四层是文件级执行事实：公开事件与轮次摘要现在都会携带 `file_facts`，并且只暴露当前事件源可以真实推导出的文件状态，不伪造完整状态机。当前 round summary 只提供 `targeted / modified` 两类保守事实；事件流则按 patch 生命周期暴露 `targeted / generation_started / generated / generation_failed / applied / apply_failed / finalized`，其中 `change_type` 仅在终态 `BuildOutput.ModifiedFiles` 可验证时才附带。对于历史 run，如果没有实时 patch 事件，终态聚合层仍会按 `RoundOutputs[].WorkspacePatch` 做一次兜底回放，并对已实时写过的轮次去重，避免 timeline 里出现重复 patch 事件。同时 `GET /jobs/{id}` 返回的 job 详情也会在运行中直接暴露 `current_round`，使概览页和执行概览不必等到时间线展开，已经能看见“当前跑到哪一轮、这一轮正在什么 phase、当前准确跑到哪个 checkpoint、原本打算触达哪些文件”。在此基础上，运行中的 `current_round` 已真正参与执行状态模型推导：live progress 的当前步骤、任务列表的运行中摘要，以及执行拆解里的 running checkpoint 现在会优先使用 runner 直接上报的 `checkpoint_key`，只有缺失时才回退到旧的 phase 级推断；在事件流尚未到达时，也会尽量使用 `current_round` 回填当前检查项。执行页已把这些事实显式展示为“当前轮次 / 目标文件 / 实际改动文件 / 轮次摘要 / 文件事实”证据块，并把“轮次活动”升级成可折叠的轮次时间线卡片：同一轮的 `run_round_started / run_patch_generation_started / run_patch_generated / run_patch_generation_failed / run_patch_apply_failed / run_patch_applied / run_heartbeat` 会先按 round 聚合成状态色卡片，再在展开态里展示事件链、checkpoint、phase 与文件证据，用户不必在原始时间线上自己拼接多条事件，也能更快看懂这一轮经历了什么、在哪一步失败、最终是否真正落地。这样用户进入执行标签后，不仅能看见单条事件，还能直接按轮次理解 builder 正在做什么，也能在事件和轮次摘要中看到真实文件事实。这一步仍然只解决“文件事实公开可见”，还没有形成稳定的文件级状态机，也没有为每个文件伪造运行中状态。后续若要继续细化文件状态，还需要 builder / runner 提供更细粒度、带时间序、并且能下钻到文件层的 patch 生命周期事实，而不是只在轮次级 patch 事件上继续堆 UI。

### 5.5 新建任务体验重做

- [x] 固化“应用名称 + 需求说明”的最简创建表单。
- [x] 新建成功后默认跳到新任务概览，并显式展示“已开始处理”的反馈。
- [x] 失败时用产品文案解释失败位置，不直接暴露底层内部术语。
- [x] 高级创建选项如确有必要，必须进入折叠区，不得重新污染主表单。

说明：当前创建弹窗继续维持“应用名称 + 需求说明”作为默认主表单，只把两个已经存在且确实会改变执行策略的控制项下沉到默认折叠的“高级创建选项”里：`real_checks` 用来决定是否启用真实检查，`auto_start_builder` 用来决定创建后是否自动注册 builder 并直接启动任务。默认路径仍保持自动启动和真实检查全开，只有明确展开折叠区时才允许覆盖，不再把技术参数重新堆回首屏表单。

### 5.6 通知与动作入口重做

- [x] 把 notification 面板改造成“待处理动作中心”，优先展示需要用户立即处理的事项。
- [x] 状态上下文、建议动作、通知建议动作统一收敛到同一套 action 组件。
- [x] 所有动作执行后都需要回到对应任务上下文，不允许跳区后丢失焦点。

补充：本轮已把通知建议动作、交付建议动作、failure guidance 跳转，以及 review 准备/交付记录/follow-up 成功后的页面回跳统一收敛到同一套“先切对应标签，再滚动并补焦点”的导航 helper，执行页也补齐了 `jobs-events` 锚点，避免动作跳到隐藏区域或跳区后丢失焦点；同时新增统一的建议动作按钮组件，把 `status_context`、通知行、交付上下文、resume 上下文里原本分散实现的 `runSuggestedAction` 按钮收敛到同一套渲染逻辑，并补上 failure guidance 跳转到执行区/产物区的页面级回归覆盖。

### 5.7 工程重构与回归

- [x] 拆分 `jobs-page.tsx`，至少拆成工作台、新建任务、任务列表、任务详情、交付区、状态模型工具函数几层。

说明：本轮已先抽出一层通用展示块，`SummaryCard / DetailSection / KeyValue / FieldBlock / LoadingBlock / ErrorBlock / EmptyBlock` 已迁到独立文件；任务列表区、执行区、交付上下文区、交付表单区、系统状态/编排器控制区，以及通知行/产物行明细视图都已拆到独立业务组件；执行状态与失败诊断的纯推导逻辑已迁到独立状态模型文件，列表排序、工作台焦点、状态徽标与任务进度摘要等页面状态派生逻辑也已迁到独立页面状态模块；交付表单默认值/校验与动作文案/可执行判断也已拆到独立工具文件，后续只剩少量零散页面工具函数收尾。

补充：本轮继续抽出 `jobs-page-utils.ts`，把 builder runtime 解析、自动 builder 请求构造、日期/错误处理、启动冲突判断等纯工具从页面主文件移出，`jobs-page.tsx` 继续向组合层收敛；同时顶部工作台摘要/焦点卡、概览页里的待处理动作中心、实时进度卡、状态上下文、恢复上下文、失败诊断/引导区、高级信息折叠区、创建任务弹窗视图、详情区卡壳/标签切换容器，以及产物标签页里的最终交付物/路径展示也已拆成独立组件，创建任务与交付提交流的校验和 mutation 编排也已下沉到独立 hook；本轮再把页面派生视图数据统一收敛到 `jobs-page-view-model.ts`，通知/状态动作的 mutation 与跳转编排收敛到 `jobs-workspace-actions.ts`，编排器 start/stop/unlock 与 busy/lock 派生状态收敛到 `jobs-orchestrator-flow.ts`，页面筛选外的本地 UI state、有效任务选中逻辑、创建弹窗重置逻辑也已收敛到 `jobs-page-ui-state.ts`，顶部 header 动作条与 builder runtime 提示条继续拆成 `jobs-page-toolbar.tsx`、`jobs-runtime-mode-banner.tsx`；本轮又把详情区标签页内容组合层收敛到 `jobs-detail-content.tsx`，状态徽标视图收敛到 `jobs-status-badge.tsx`，并删掉了页面里重复的交付表单重置副作用，`jobs-page.tsx` 已基本只剩查询装配、筛选 state 和少量页面级 glue code。
- [x] 补页面级回归用例，覆盖创建、运行、失败、产物查看、交付五条主链。

说明：前端已补上 Vitest + Testing Library 测试底座，并新增 `JobsPage` 页面级回归测试，当前已覆盖默认概览失败态、通知确认、通知重建成功与失败、通知建议动作切换执行标签并聚焦事件区、failure guidance 跳转到执行区/产物区、执行标签页、执行空态、执行事件证据字段展示、运行中任务的当前轮次展示、交付标签页、交付建议动作聚焦 follow-up 区块、review 准备成功自动切到产物标签、交付表单前端校验拦截、review 准备失败、交付提交失败、follow-up 提交失败、产物标签页、产物空态、创建流校验与成功提交、创建编译失败分支、创建成功但启动失败警告分支、创建弹窗高级选项默认折叠与关闭自动启动分支、概览建议动作失败分支、任务列表默认信息压缩、任务列表中间摘要层继续压缩、任务列表失败态与待处理态视觉分层、“待处理”聚合筛选，以及按应用名称搜索等主链；当前 `JobsPage` 回归总数已提升到 36 条，同时新增 `jobs-orchestrator-flow.test.tsx`、`jobs-page-view-model.test.ts`、`jobs-page-ui-state.test.tsx` 三组模块级测试，相关总回归数为 48 条，把编排器控制流、页面派生视图模型和页面 UI 状态协调也纳入回归护栏。
- [x] 更新文案、设计文档、回归脚本和运行时状态映射，保持同一口径。

说明：当前 `/jobs` 的统一基线已经固定到同一套前端事实源上：运行时状态映射统一以 `web/frontend/src/lib/orchestrator-status.ts` 为准，中文/英文文案分别以 `web/frontend/src/i18n/locales/zh.json` 与 `web/frontend/src/i18n/locales/en.json` 为准，任务列表新增的“待处理 / Needs attention”筛选也已纳入同一翻译口径；回归入口统一收敛到 `pnpm -C web/frontend exec tsc --noEmit --pretty false` 与 `pnpm -C web/frontend test:run -- src/components/jobs/jobs-page.test.tsx src/components/jobs/jobs-orchestrator-flow.test.tsx src/components/jobs/jobs-page-view-model.test.ts src/components/jobs/jobs-page-ui-state.test.tsx`，不再存在文档、运行态文案与测试入口各写一套的情况。

## 6. 本轮 review 输出要求

以下 review 产物现已补齐，并作为当前 `/jobs` 重设计的冻结基线：

- 一版任务中心线框图或模块草图
- 一版字段保留 / 下沉 / 删除清单
- 一版状态模型映射表
- 一版组件拆分计划

只有这四项过 review，才进入实现阶段。

### 6.1 任务中心线框图 / 模块草图

页面结构冻结为以下四层：

1. 顶部工作台
	- 页面标题
	- 快捷动作：新建任务、刷新、重建通知
	- runtime 提示条
	- 可折叠系统状态卡
2. 左侧任务列表
	- 状态筛选
	- 搜索输入
	- 任务卡列表
3. 右侧任务详情
	- 固定标签：概览 / 执行 / 产物 / 交付
	- 详情头：任务 ID、状态徽标、阶段徽标
	- 标签内容组合层
4. 新建任务弹窗
	- 应用名称
	- 需求说明

对应线框可简化为：

```text
Jobs Workbench
├─ Header
│  ├─ Create
│  ├─ Refresh
│  └─ Rebuild Notifications
├─ Runtime Banner
├─ Workspace Overview
├─ System Status (collapsible)
└─ Main Grid
	├─ Job List
	│  ├─ Filters
	│  ├─ Search
	│  └─ Cards
	└─ Job Detail
		├─ Overview
		├─ Execution
		├─ Artifacts
		└─ Delivery
```

### 6.2 字段保留 / 下沉 / 删除清单

| 类别 | 字段 / 信息 | 处理策略 | 说明 |
| --- | --- | --- | --- |
| 保留 | 应用名称 / 任务 ID | 列表主标识 | 应用名称主展示，任务 ID 作为弱信息保留 |
| 保留 | 状态、阶段、最新摘要、更新时间 | 列表默认显示 | 满足“快速识别 + 切换任务” |
| 保留 | 下一步动作、失败摘要、通知动作 | 概览默认显示 | 满足任务流驱动 |
| 保留 | 执行阶段拆解、事件流、最终产物、交付表单 | 对应标签页显示 | 按任务流分区 |
| 下沉 | PRD ID、模板 ID、预算、runtime、路径 | 高级信息区 | 不占列表和概览首屏 |
| 下沉 | 编排器锁、runner 状态、recoveries 计数 | 系统状态折叠卡 | 不再抢主视觉 |
| 下沉 | 设备验证证据、follow-up 证据、签名产物路径 | 交付标签内二级表单 | 只在交付链出现 |
| 删除 | 列表卡上的并排多组技术元信息 | 从默认列表移除 | 避免卡片信息墙 |
| 删除 | 独立 notification 面板 | 改为待处理动作中心 | 统一到概览动作入口 |

### 6.3 状态模型映射表

| 层级 | 面向谁 | 默认出现位置 | 典型来源 | 当前策略 |
| --- | --- | --- | --- | --- |
| 状态 Status | 所有用户 | 列表、详情头 | `job.status` | 只表达整体结果与运行态 |
| 阶段 Stage | 所有用户 | 列表摘要、详情头、执行页 | `job.phase` + 最新事件 | 列表只显示阶段级 |
| 步骤 Step | 排障用户 | 执行页 | 事件流推导 | 不放到默认概览 |
| 检查项 Checkpoint | 排障用户 | 执行页 | 执行拆解推导 | 支撑“卡在哪” |
| 事件 Event | 深度排障 | 执行页 | `job-events` | 作为最低层事实流 |

补充映射规则：

- 列表只消费状态 + 阶段级摘要，不直接消费事件原文。
- 概览页消费状态上下文、失败诊断、建议动作，不直接消费完整事件流。
- 执行页是步骤 / 检查项 / 事件三层展开的唯一入口。
- 产物页和交付页只消费与交付闭环强相关的状态，不重复渲染执行层事实。

### 6.4 组件拆分计划

已完成的拆分层级：

- 页面外壳与装配：`jobs-page.tsx`
- 页面状态：`jobs-page-ui-state.ts`
- 页面视图模型：`jobs-page-view-model.ts`
- 页面动作流：`jobs-workspace-actions.ts`
- 编排器控制流：`jobs-orchestrator-flow.ts`
- 顶部动作条：`jobs-page-toolbar.tsx`
- runtime 提示条：`jobs-runtime-mode-banner.tsx`
- 详情容器：`jobs-detail-panel.tsx`
- 详情内容组合层：`jobs-detail-content.tsx`
- 状态徽标：`jobs-status-badge.tsx`
- 各业务面板：列表、概览动作、执行、产物、交付、失败诊断、状态上下文、恢复上下文等独立组件

剩余建议收口点：

- 当前工程级收口点已基本完成，后续如果继续推进，应优先进入事件源级能力建设，而不是继续做页面层微调。

### 6.5 文案 / 状态 / 回归统一口径基线

当前 `/jobs` 的同口径基线如下，后续如果要改运行态文案或交互判断，必须同步改代码、翻译和回归，不允许只改其中一层。

#### 运行时状态映射

| 运行时条件 | 标签文案 key | 详情文案 key | 视觉 tone | 事实源 |
| --- | --- | --- | --- | --- |
| `data` 缺失 | `header.orchestrator.status.loading` | `header.orchestrator.menu.loading` | `bg-muted-foreground/40` | `getOrchestratorLabel / Detail / ToneClass` |
| `watch_runner_state = running` | `header.orchestrator.status.running` | `header.orchestrator.menu.runningDetail` | `bg-emerald-500` | `orchestrator-status.ts` |
| `watch_runner_state = stopping` | `header.orchestrator.status.stopping` | `header.orchestrator.menu.stoppingDetail` | `bg-amber-500` | `orchestrator-status.ts` |
| `watch_lock_state = held_by_other` | `header.orchestrator.status.locked` | `header.orchestrator.menu.lockedDetail` | `bg-amber-500` | `orchestrator-status.ts` |
| `watch_lock_state = stale` | `header.orchestrator.status.stale` | `header.orchestrator.menu.staleDetail` | `bg-orange-500` | `orchestrator-status.ts` |
| 其他可用态 | `header.orchestrator.status.idle` | `header.orchestrator.menu.idleDetail` | `bg-slate-400` | `orchestrator-status.ts` |

#### 任务中心文案事实源

| 领域 | 当前事实源 | 说明 |
| --- | --- | --- |
| 编排器状态文案 | `header.orchestrator.status.*` / `header.orchestrator.menu.*` | 与 `orchestrator-status.ts` 一一对应 |
| 任务列表筛选文案 | `jobs.filters.*` | 包括新增的 `jobs.filters.attention` |
| runtime 模式提示 | `jobs.runtimeMode.*` | 顶部 runtime banner 的唯一文案入口 |

#### 当前统一回归入口

| 类型 | 命令 / 文件 | 作用 |
| --- | --- | --- |
| 类型检查 | `pnpm -C web/frontend exec tsc --noEmit --pretty false` | 保证页面拆分后的 TS 装配一致 |
| 页面主链回归 | `src/components/jobs/jobs-page.test.tsx` | 覆盖创建、列表、详情标签、失败诊断、建议动作、交付主链 |
| 编排器控制流 | `src/components/jobs/jobs-orchestrator-flow.test.tsx` | 覆盖 start / stop / unlock 与 busy/lock 派生状态 |
| 页面视图模型 | `src/components/jobs/jobs-page-view-model.test.ts` | 覆盖列表项、通知、checklist、failure guidance，以及 round summary / current_round checkpoint 回填派生数据 |
| 页面 UI 状态 | `src/components/jobs/jobs-page-ui-state.test.tsx` | 覆盖有效选中任务与创建弹窗重置逻辑 |
| 页面 workspace controller | `src/components/jobs/jobs-workspace-controller.test.ts` | 覆盖统一跳转 controller 的选中任务、标签切换与查询失效行为 |

当前冻结口径：页面级 36 条回归 + 模块级 12 条回归，共 48 条；如果后续继续拆页或调整状态映射，至少要同步更新这里列出的四组回归入口之一。

## 9. 当前进度评估（2026-04-04）

按当前 TODO 严格勾选口径计算，整体进度约为 `100%`。

计算方式：

- 5.1：3/3 已完成
- 5.2：4/4 已完成，默认列表信息层级已压缩，四类视觉层级已建立，筛选器已支持待处理聚合视图，应用名称也已成为列表主展示信息
- 5.3：4/4 已完成
- 5.4：4/4 已完成
- 5.5：4/4 已完成
- 5.6：3/3 已完成
- 5.7：3/3 已完成

合计为 `25 / 25 = 100%`。

如果只看当前这一轮 `/jobs` 重设计的实现闭环，页面层信息架构、动作入口、列表密度、执行诊断、页面级 workspace controller、运行中 checkpoint 的显式状态映射，以及保守口径的文件级执行事实都已经完成收口。当前剩余的工作不再属于这一轮 redesign TODO，而属于下一阶段事件源能力升级，例如是否要让 builder / runner 输出更细粒度、可排序、可下钻的文件级 patch 生命周期事实。

主要后续项：

- 如需更强文件级执行理解，需要后端事件源继续升级为真正的文件状态机事实，而不是继续在前端补推断。

## 7. 已完成归档摘要

- [x] 平台级 public-job 主链、builder-runtime、验证链与交付闭环已拉通。
- [x] `/jobs` 页面已具备真实创建、真实执行、真实失败、真实产物的基础能力。
- [x] `/jobs` 新建任务入口已收敛为只输入应用名称与需求说明。
- [x] `jobs-ui` 自动生成的 PRD / Job ID 已改为共享同一毫秒级时间戳的简化格式。

## 8. 当前不做的事

- iOS 生成
- 应用商店发布自动化
- 多租户账号体系与复杂组织权限
- 完整分布式多活协调协议
- 任意技术栈并行扩展
- 无人工门禁的全自动发布闭环
