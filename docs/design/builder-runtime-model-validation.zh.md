# builder-runtime 模型验证记录

本文档用于记录 builder-runtime 本地模型验证的当前结论、环境前提、首轮结果与后续动作。

## 1. 当前结论

- builder-runtime 的长期目标不是绑定单一模型或单一推理后端，而是接入统一模型池，并按任务类型路由。
- 当前首轮验证以后端改造成本最低为优先，先以 `Ollama` 作为本地推理后端验证 builder-runtime 的最小调用链。
- 当前首轮 A/B 之后，默认验证模型仍优先选 `qwen2.5-coder:14b`，用于承担单文件实现、低风险 analyze/test 修复和受限 `WorkspacePatch` 生成这类高频窄任务。
- `qwen2.5-coder:32b` 已在本地环境可用，并已完成同任务集首轮 A/B；当前更适合作为复杂修复和失败升级层，而不是直接替换默认层。
- 若后续证明需要更好的统一路由、并发和可观测能力，再评估切到 `vLLM` 或 `LiteLLM`；本轮不提前引入额外中间层。

## 2. 环境前提

- 项目运行在 WSL。
- 宿主机运行 `Ollama`。
- WSL 内通过 `http://127.0.0.1:11434` 可访问本地 `Ollama`。
- builder-runtime 首轮验证先走 `Ollama` 的 OpenAI 兼容接口 `/v1/chat/completions`。

## 3. 首轮验证结果（2026-04-01）

### 3.1 连通性

- `GET /api/tags` 成功返回模型列表。
- `GET /v1/models` 成功返回 OpenAI 兼容模型列表。
- 当前已确认可见模型：
  - `qwen2.5-coder:14b`
  - `qwen2.5-coder:32b`
  - `qwen3.5-27b-claude-opus-v2:q4_k_m`

说明：`qwen2.5-coder:32b` 已经就绪，因此当前文档不再只记录 14B 基线，而是开始记录 14B / 32B 的同口径对比结果。

### 3.2 结构化输出验证

- `qwen2.5-coder:14b` 已成功按要求返回纯 JSON。
- 已成功返回受限的 patch 风格 JSON，对象结构可映射到后续 `WorkspacePatch` 生成流程。
- 在当前最小样本下，patch 风格 JSON 返回耗时约 1 秒。

### 3.3 叶子任务样本验证

- 已新增可复跑入口：`bash scripts/run-builder-runtime-model-samples.sh`，`make validate-builder-runtime-ollama-samples`。
- 2026-04-01 已完成首批五类叶子任务样本探测，并补做了 14B / 32B 同口径首轮对比。
- 当前已验证通过的样本类型：
  - 单文件局部修改
  - 双文件接线
  - `flutter analyze` 风格修复
  - `flutter test` 风格修复
  - 低风险 closure repair
- 14B 当前在五类叶子任务样本上 5/5 返回了可解析的结构化 patch JSON，且所有 operation path 都保持在 allowed paths 内；本轮重跑结果文件为 `.runtime/builder-model-validation/20260401-192551-qwen2.5-coder_14b.jsonl`，平均耗时约 3 秒每题。
- 在脚本对齐 runtime 归一化规则之后，14B 叶子样本复跑结果文件为 `.runtime/builder-model-validation/20260401-202038-qwen2.5-coder_14b.jsonl`，五类任务仍为 5/5 通过，但 analyze/test 两类提示的单题耗时已抬升到 31 秒和 64 秒，说明默认层虽然仍可用，但延迟波动不能再只看平均值。
- 32B 在首轮同脚本复跑里曾卡在双文件叶子样本，结果文件为 `.runtime/builder-model-validation/20260401-202045-qwen2.5-coder_32b.jsonl`；对原始响应留档分析后已确认，直接故障是 assistant content 在尾部产出了坏 JSON：`operations` 数组最后一个元素之后缺少 `]`，解析器会在末尾报 `Expected ',' or ']' after array element`。
- 针对这个问题，本轮又收紧了 builder-runtime 正式提示与三条本地验证入口的 prompt contract：明确要求模型在输出前自检 JSON 闭合，`replace_block` 只能使用 `type/path + anchor|old_content|start/end + new_content|replacement` 这组受支持字段，禁止 `start_line/end_line` 这类行号补丁表达。
- 在提示收紧后，32B 叶子样本最新复跑结果文件为 `.runtime/builder-model-validation/20260401-210846-qwen2.5-coder_32b.jsonl`，五类任务已经恢复为 5/5 通过。这说明前一个卡点主要是 prompt contract 过松，而不是 32B 在窄任务上完全不可用。
- 为了继续补长期指标，本轮又重新复跑了带统计字段的叶子样本，最新结果文件分别为 `.runtime/builder-model-validation/20260401-220148-qwen2.5-coder_14b.jsonl` 与 `.runtime/builder-model-validation/20260401-220158-qwen2.5-coder_32b.jsonl`。这轮结果除了继续 5/5 通过之外，还把 `attempts`、`schema_drift_count`、`parse_failure_count`、`scope_violation_count` 真正写入样本产物：14B 叶子样本当前 schema drift case 为 0，而 32B 在 `analyze-repair` 与 `test-repair` 两类样本上各出现 1 次 schema drift。
- 从当前叶子样本看，14B 的速度和延迟波动仍明显更适合作为默认高频窄任务模型；32B 虽然现在也能通过五类叶子任务，但耗时仍更高，而且在叶子样本上开始暴露出比 14B 更多的 schema drift，更适合作为升级层而不是默认层。
- 新增一个需要纳入 builder-runtime 实现层的现实约束：即使系统提示明确要求“只输出 JSON”，模型仍可能偶发返回 fenced JSON，因此 runtime 侧必须做输出清洗与严格解析，不能直接把原始文本当作最终 patch payload。

### 3.4 沙箱 patch 落地验证

- 已新增可复跑入口：`bash scripts/run-builder-runtime-patch-sandbox.sh`，`make validate-builder-runtime-ollama-sandbox`。
- 2026-04-01 已完成首个真实沙箱 patch 应用验证：由 `qwen2.5-coder:14b` 生成单文件 `WorkspacePatch`，并在临时 Flutter 风格工作区中成功将 `lib/main.dart` 的 `title: 'Old Title',` 替换为 `title: 'Budget Flow',`。
- 当前结果文件会同时写入 `.runtime/builder-model-validation/sandbox-single-file-apply.latest.json` 与按模型归档文件 `.runtime/builder-model-validation/sandbox-single-file-apply.<model>.json`，避免双模型复跑后只保留最后一次结果。
- 这说明 14B 已经越过“只会描述 patch”的阶段，至少在单文件、单 anchor、低风险替换场景里可以生成可实际应用的 patch。
- 在提示 contract 收紧后，sandbox 入口已再次复跑，14B 与 32B 仍然都能通过单文件 patch 应用；为补长期指标，本轮又用新产物格式重新复跑了一次，最新按模型归档结果分别约 83 秒和 101 秒，并且结果文件里已经带上 `attempts`、`schema_drift_count`、`parse_failure_count`、`scope_violation_count`。当前两边 sandbox 仍未出现 schema drift 或 parse/scope failure，说明 patch apply 链虽然有时延波动，但结构仍稳定。

### 3.5 容器内真实 Flutter 修复验证

- 已新增可复跑入口：`bash scripts/run-builder-runtime-real-validation.sh`，`make validate-builder-runtime-ollama-real`。
- 2026-04-01 已完成四类容器内真实修复验证，均复用本地 `picoclaw/appfactory-builder:local` 镜像执行真实 Flutter 工具链。
- 当前已验证通过的真实场景：
  - `flutter analyze` 修复：模型仅修改 `lib/views/entry_form_page.dart`，修复 Dart 空值判断错误后，容器内 `flutter analyze` 通过。
  - `flutter test` 修复：模型仅修改 `test/widget_test.dart`，修正错误断言后，容器内 `flutter test` 通过。
  - 双文件接线修复：模型仅修改 `lib/controllers/home_controller.dart` 和 `lib/views/home_page.dart`，补齐 loading/title 接线后，容器内 `flutter analyze` 通过。
  - 低风险 closure repair：模型仅修改 `lib/main.dart` 和 `test/widget_test.dart`，统一标题与测试期望后，容器内 `flutter analyze` 与 `flutter test` 同时通过。
- 当前结果文件已写入：
  - `.runtime/builder-model-validation/real-analyze-repair.latest.json`
  - `.runtime/builder-model-validation/real-test-repair.latest.json`
  - `.runtime/builder-model-validation/real-dual-file-wiring.latest.json`
  - `.runtime/builder-model-validation/real-closure-repair.latest.json`
  - 以及对应的按模型归档文件 `.runtime/builder-model-validation/real-*.{model}.json`
- 这说明 `qwen2.5-coder:14b` 已经不只是“能生成结构化 patch”，而是在四类真实 Flutter 叶子任务场景里能生成可应用且可通过真实验收的 patch。
- 当前又新增一个需要纳入 builder-runtime 实现层的现实约束：模型在 patch schema 中可能返回 `action` 字段而不是 `type` 字段，因此 runtime 侧需要做受限 schema 归一化，不能把这类可应用 patch 误判为非法输出。
- 2026-04-02 又暴露出一个更贴近 `/jobs` 真链路的约束：模型虽然能产出可编译 UI，但仍可能使用 `withOpacity()` 这类当前稳定 Flutter 工具链里会触发 `deprecated_member_use` 的旧 API，导致 `flutter analyze` 直接失败。当前已在正式 builder-runtime prompt 里追加“优先 `withValues()`、禁止 `withOpacity()`，并把 analyze 视作必须消灭 deprecation 级问题的硬门槛”这一约束；修复后 `/jobs` regression 新样本已重新通过，说明这类失败更适合在 prompt contract 层先收掉，而不是继续留给人工重跑。
- 2026-04-01 同日又补做了 `qwen2.5-coder:32b` 的同口径真实 Flutter 验收，四类场景 4/4 通过，单题耗时约 25 到 81 秒，其中 `real-analyze-repair` 为 80 秒、`real-test-repair` 为 25 秒、`real-dual-file-wiring` 为 81 秒、`real-closure-repair` 为 50 秒。
- 在脚本归一化层对齐后，14B 的真实 Flutter 验收已经恢复为四类场景 4/4 通过：`real-analyze-repair` 为 4 秒、`real-test-repair` 为 35 秒、`real-dual-file-wiring` 为 36 秒、`real-closure-repair` 为 67 秒。这说明此前挡住真实验收的主要问题确实是验证入口与 runtime 归一化能力脱节，而不全是模型本身不可用。
- 同步复跑后，32B 的真实 Flutter 验收仍为四类场景 4/4 通过，最新一轮耗时约 34 到 78 秒，其中 `real-analyze-repair` 为 70 秒、`real-test-repair` 为 34 秒、`real-dual-file-wiring` 为 78 秒、`real-closure-repair` 为 55 秒。
- 在提示 contract 收紧后，本轮又补做了一次真实 Flutter 验收回归并验证了按模型归档：14B 仍为 4/4 通过，最新耗时为 `real-analyze-repair` 7 秒、`real-test-repair` 40 秒、`real-dual-file-wiring` 4 秒、`real-closure-repair` 74 秒；32B 仍为 4/4 通过，最新耗时为 `real-analyze-repair` 90 秒、`real-test-repair` 38 秒、`real-dual-file-wiring` 63 秒、`real-closure-repair` 48 秒。这说明这轮针对 JSON 闭合和受支持字段的提示收紧，以及 latest + per-model 归档改造，都没有把正式修复链路打坏。
- 为了把真实样本级漂移率真正落盘，本轮又用新结果格式复跑了 real validation。最新汇总显示：14B 当前 real validation 仍为 4/4 通过，但四个 case 都发生了 schema drift 归一化；32B 当前 real validation 仍为 4/4 通过，但只有 `real-closure-repair` 这一个 case 出现 schema drift。也就是说，14B 当前更快，但在真实样本的 schema 表达上更松；32B 当前更慢，但真实样本上的 schema 漂移反而更少。
- 从当前真实 Flutter 验收看，32B 的质量和稳定性更适合作为升级层；14B 更适合作为默认层，失败后再升级，而不是强行单模型打到底。

### 3.6 脚本与 runtime 对齐情况

- 2026-04-01 同日已把三条本地验证入口的脚本与 runtime 归一化规则对齐：`run-builder-runtime-model-samples.sh`、`run-builder-runtime-patch-sandbox.sh`、`run-builder-runtime-real-validation.sh` 现在都能兼容 fenced JSON、`action`/`operation`/`file_path`/`replacement` 这类别名，以及 `start/end` 范围替换。
- 其中 `run-builder-runtime-model-samples.sh` 现在还会在失败时自动把 prompt、raw response 和错误上下文归档到 `.runtime/builder-model-validation/failures/`，避免后续再手工复现坏样本。
- 当前还新增了 `bash scripts/summarize-builder-runtime-validation.sh` 与 `make summarize-builder-runtime-validation`，用于统一汇总 leaf samples、real validation、sandbox 三条入口的按模型通过率、耗时分布、平均 attempts、平均 repair rounds、平均无关改动率、schema drift case 数、parse failure 总数与 scope violation 总数，并把汇总结果写入 `.runtime/builder-model-validation/builder-runtime-validation-summary.latest.json`。
- 2026-04-01 最新汇总已经确认 `repair_rounds_available`，并且 `missing_metrics` 已收敛为 none。当前按模型结果为：叶子样本 14B `5/5`、平均 31.6 秒、P95 70 秒、平均 attempts 1、平均 repair rounds 1、schema drift case 0；32B `5/5`、平均 39.4 秒、P95 70 秒、平均 attempts 1、平均 repair rounds 1、schema drift case 2。real validation 14B `4/4`、平均 31 秒、P95 64 秒、平均 attempts 1、平均 repair rounds 1、schema drift case 4；32B `4/4`、平均 57.75 秒、P95 75 秒、平均 attempts 1、平均 repair rounds 1、schema drift case 1。sandbox 14B `1/1`、平均 72 秒、平均 repair rounds 1；32B `1/1`、平均 89 秒、平均 repair rounds 1。
- 当前这三条入口的平均无关改动率都为 0，且 parse failure / scope violation 继续保持 0。这说明本轮不仅修复了 32B 双文件叶子样本的坏 JSON 问题，也已经把 repair rounds、schema drift、无关改动率这三类长期指标真正沉淀到本地验证产物和 summary 中。

### 3.7 当前判断

- 14B 仍然是当前最适合的默认 builder-runtime 高频窄任务模型候选，因为它在叶子样本上更快，也更贴合当前受限 patch schema。
- 32B 当前更适合作为升级层：它在真实 Flutter 验收上 4/4 通过，叶子任务在提示收紧后也恢复为 5/5 通过，说明模型质量已经足够；但延迟仍明显更高，不值得直接替换 14B 默认层。
- 当前最合理的系统结论不是“二选一”，而是落地 `14b default + 32b upgrade`：14B 继续承担默认高频窄任务层，32B 作为复杂修复和失败升级层；这一结论现在已经同时有通过率、延迟、repair rounds、schema drift 和无关改动率支撑，而不再只是经验判断。
- 2026-04-01 同日已把这一路由真正接到 runner 的 edit step 执行链：当 `appfactory.builder_runtime` 启用且模型配置可解析时，runner 会按 `task_type` 选默认模型或升级模型请求 patch，并继续复用现有 `WorkspacePatch` apply 与 acceptance checks 链路。
- 当前 runtime 已补的归一化与统计包括：code fence 清洗、`action`/`file_path`/`file_content` 等别名归一化、`start/end` 区间替换支持、升级重试、无关改动计数与比例、schema 漂移次数、parse failure 次数、scope violation 次数，以及模型尝试序列。

## 4. 下一步

- 已在正式配置层补入 `appfactory.builder_runtime` 最小骨架，当前字段包含 `default_model`、`upgrade_model`、`task_routes` 与 `upgrade_threshold`；当前代码已开始在 runner 链路读取这套配置，并把解析后的 task_type 路由快照写入 round input，后续再把它继续接到真实模型执行层。
- `14b default + 32b upgrade` 的执行接线和本地长期指标已经落地，下一步重点不再是“repair rounds 能否落盘”，而是“阈值是否合理”：继续复测更大真实任务集，收窄默认层到升级层的触发条件。
- 32B 双文件叶子样本的原始坏 JSON 已经留档并完成定位；后续不再围绕这个 case 重复猜测，而是把它作为“提示 contract 需要写硬”的先验证据保留下来。
- 基于 `bash scripts/summarize-builder-runtime-validation.sh` 持续沉淀 leaf/real/sandbox 的按模型汇总结果，继续观察更大样本上的延迟分布、schema drift 分布和升级触发阈值，确认当前 `14b default + 32b upgrade` 是否需要进一步细化 task_type 边界。
- 单独修复 14B 在真实 Flutter 验收脚本后续 case 上的 schema 不稳定，明确它适合停留在哪些 task_type，不再把它默认外推到所有 repair 场景。
- 为每类样本补充“raw output -> normalized JSON -> WorkspacePatch -> apply -> validate”整链路记录，而不只停留在模型回复结构正确。
- 当前 14B / 32B 首轮 A/B、脚本归一化、repair loop 和长期汇总都已形成，下一步不再等待模型可用性，而是进入路由阈值和持续回归收口。