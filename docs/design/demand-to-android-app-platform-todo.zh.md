# PicoClaw Android App 平台推进清单（2026-03-31）

本文档用于维护当前平台推进清单。

## 1. 平台基线

当前平台基线如下：

- 默认执行路径为 `executor`。
- `prepare -> job -> run -> workspace patch -> validate` 这条最小主线已经可用。
- 默认 public-job Flutter 链路以真实工作区为输出，并以 `flutter pub get`、`flutter analyze`、`flutter test`、`flutter build apk --debug` 作为基础验证链。
- `verify-appfactory-public-job` / `verify-appfactory-public-job-fast` 是默认回归入口。
- public job、orchestrator、notifications、review / handoff / delivery 已经属于平台主对象模型。

## 2. 阶段总目标

把默认主链继续推进成：

- 可持续完善的平台执行面
- 可持续排障的运行时面
- 可持续回归的工程资产
- 可持续交付的最小产品化闭环

## 3. 执行原则

- 优先收敛语义、证据链和长期工程资产，避免继续堆功能但缺少稳定出口。
- 文档、运行时语义、前后端展示和回归入口必须同步更新，不允许各说各话。
- 当前清单按主优先级排序推进；如果实跑证明顺序不合理，先改清单，再改实现。

## 4. TODO 清单（按优先级推进）

1. [ ] 收敛 public-job 对外语义与运维视图。
目标：把 jobs、notifications、orchestrator、watcher 面向人的状态语义统一起来，避免“底层能跑、对外难懂”。
完成标准：
- `status`、`phase`、`failure_domain`、`failure_category`、`human_approvals`、notification 投影语义一致。
- 前后端对“运行中”“待人工处理”“失败可恢复”“终态已完成”的表达不再明显错配。
- jobs 运维页、public facade、orchestrator status 的关键状态能一一对应当前运行时模型。

2. [ ] 收敛默认 Flutter 生成结果与模板约束。
目标：把默认记账结果收敛到稳定模板实现，而不是停留在一次性样本。
完成标准：
- 默认结果继续稳定落在 `lib/main.dart`、`lib/views/*`、`lib/controllers/*`、`lib/repositories/*` 等约定位置。
- 持久化方案、依赖选择、目录结构与当前冻结模板约束不再明显背离。
- 生成结果持续通过结构检查与 `flutter analyze`、`flutter test`、`flutter build apk --debug`，并有回归锁住这些约束。

3. [ ] 补齐 PRD / 模板 / 审批 / job 的前置输入链语义。
目标：让“前置输入已批准、已冻结、可复用”这件事在对象层和接口层真正说清楚，而不是只靠零散快照勉强拼起来。
完成标准：
- `PRD.json`、模板匹配结果、审批快照、job 输入包之间的版本约束明确。
- prepare 产物、approval record、job detail、resume 入口对“当前批准输入版本”有统一口径。
- 输入链变化时，失效、重编译、重提审、重启 job 的边界清晰。

4. [ ] 收敛 orchestrator / resume / preserved workspace 的长期运行时模型。
目标：把当前 file-backed orchestrator、execution attempt、preserved workspace、resume context 从“能用”推进到“长期可维护”。
完成标准：
- execution record、attempts、lease、resume context、preserved workspace 的状态迁移规则稳定。
- `start`、`resume`、`cancel`、recovery pass 的冲突语义与审计事件保持一致。
- 前端、排障脚本和 API 不再需要自行猜 preserved snapshot 与恢复模式的含义。

5. [ ] 补齐设备侧最小验证闭环。
目标：在 analyze/test/build 之外，补上设备安装、启动、基础日志与截图证据，让“可运行 Android App”具备可追溯运行证据。
完成标准：
- 至少一条默认 public-job Flutter 链路能跑通 `adb install -r`、启动和最小 `logcat` 采集。
- 设备侧验证结果能进入 `smoke-test-report`、artifact manifest 或等价可追溯对象。
- 失败时能区分设备缺失、环境阻塞、应用启动失败和运行时崩溃。

6. [ ] 收敛 review / handoff / delivery 的最小交付闭环。
目标：把已经存在的 review bundle、handoff checklist、delivery record 从“能写入对象”推进到“能指导人工交付动作”的稳定闭环。
完成标准：
- review bundle、handoff checklist、delivery record 与 artifact manifest 的字段口径一致。
- public job detail、events、notifications 对交付阶段信号的投影一致。
- 人工完成 review、设备验证和交付决策后，能沿当前链路留下可追溯最终状态。

7. [ ] 把默认验证入口接入持续回归机制。
目标：让 `verify-appfactory-public-job` / `verify-appfactory-public-job-fast` 不只是一组手工命令，而是长期稳定的工程回归资产。
完成标准：
- full / fast 两条入口的触发场景、预期耗时和失败信号有固定说明。
- 至少一条持续执行路径会按变更或按周期触发默认 public-job Flutter 回归。
- 回归失败时，排障入口优先回到稳定 workspace、稳定日志、稳定状态对象和稳定 artifact 路径。

8. [ ] 补齐治理与内部试运行基线。
目标：在默认主链可持续运行后，把模板治理、风险提示、脱敏和审计链继续补齐，为内部长期试运行做准备。
完成标准：
- 模板健康、许可证、风险依赖提示能进入当前平台链路。
- review / handoff / delivery 相关报告具备最小脱敏与审计要求。
- 内部试运行所需的运行手册、回归入口、值班入口和交付对象不再依赖口头经验。

## 5. 当前不做的事

- iOS 生成
- 应用商店发布自动化
- 多租户账号体系与复杂组织权限
- 完整分布式多活协调协议
- 任意技术栈并行扩展
- 无人工门禁的全自动发布闭环
