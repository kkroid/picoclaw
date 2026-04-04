# PicoClaw 通用开放 App 模板创建总 TODO（唯一执行文档）

本文档是这件事的唯一执行文档。

从现在开始：

- 这条线只维护这一份文档
- 其他模板相关文档只保留历史背景或论证，不再作为执行清单
- 后续推进、勾选、补证据、改顺序，统一在这里完成

## 1. 固定结论

- [x] 内部模板 ID 固定为 `flutter-open-lite`
- [x] 技术栈固定为 Flutter
- [x] 第一阶段只做一个模板，不扩模板矩阵
- [x] 外部主参考固定为 `zubairehman/flutter_boilerplate_project`
- [x] 当前目标固定为“创建内部模板项目”，不再继续扩候选池

## 2. 硬限制

- [x] 不使用自建服务器作为默认前提
- [x] 不引入广告能力
- [x] 能省则省，可要可不要的能力默认不做
- [x] 功能专精，优先服务单一核心任务
- [x] 第一阶段默认本地优先，不把后端能力当基础依赖
- [x] 第一阶段不做登录、支付、推送、地图、实时通信、复杂原生桥接

## 3. 模板边界

### 3.1 必须具备

- [ ] 多页面导航
- [ ] 首页摘要区
- [ ] 列表页
- [ ] 表单页
- [ ] 详情页或编辑页
- [ ] 本地状态与本地持久化
- [ ] 基础主题与资源目录
- [ ] 基础测试入口

### 3.2 默认不具备

- [ ] 登录注册
- [ ] 自建服务器前提
- [ ] 默认远端 API 依赖
- [ ] 云同步
- [ ] 第三方登录
- [ ] 支付
- [ ] 广告
- [ ] 地图
- [ ] 推送
- [ ] 即时通信
- [ ] 复杂原生桥接

### 3.3 结构口径

- [ ] 固定分层边界：`view` / `presenter` 或 `state holder` / `repository` / `entity`
- [ ] 固定最小页面集：`home`、`list`、`editor`、`detail(optional)`
- [ ] 固定最小数据流：页面状态 -> 仓储接口 -> 本地存储适配器
- [ ] 固定产品口径：单一核心任务闭环，不做大而全功能壳

## 4. 执行顺序

### 4.1 创建模板骨架

- [ ] 在 `examples/appfactory/templates/` 下创建 `flutter-open-lite` 目录
- [ ] 冻结模板目录结构
- [ ] 冻结能力边界：`navigation`、`summary-card`、`list`、`form`、`detail`、`local-storage`、`theme`
- [ ] 冻结必须保留的测试入口：`widget_test.dart` + 基础 smoke test

### 4.2 冻结外部参考裁剪方案

- [ ] 冻结 `zubairehman/flutter_boilerplate_project` 的评估版本
- [ ] 记录许可证、最近维护情况、核心依赖、Flutter 兼容范围
- [ ] 补 AI 拆解适配度打分
- [ ] 输出保留 / 删除 / 替换清单
- [ ] 明确要删除的能力：login、network client、加密、通知、广告、服务端耦合、过重 DI、过时代码生成链
- [ ] 跑一轮本地 verify，确认不是作者环境专用样板

### 4.3 完成模板瘦身

- [ ] 去掉登录、通知、远端 API、演示品牌资产、无关页面和无关依赖
- [ ] 去掉广告位、服务端假设、商业化埋点和其他可选能力
- [ ] 改造成中性、可复用 seed，不保留明显业务语义
- [ ] 统一默认文案、资源名、包名占位规则
- [ ] 补模板自述文档，写清适用范围、已覆盖能力、未覆盖能力、风险边界

### 4.4 接入治理与注册表

- [ ] 接入模板治理文档和脚本
- [ ] 通过 `make verify-appfactory-template-fast`
- [ ] 通过 `make verify-appfactory-template`
- [ ] 更新 `template-fit-report.md` 主叙述口径
- [ ] 在模板注册表新增 `flutter-open-lite`
- [ ] 明确 `flutter-finance-lite` 与 `flutter-open-lite` 的边界

### 4.5 升级 generic compile

- [ ] 重写 `compileGenericSpec`，不再生成单屏 smoke 输入包
- [ ] 将 generic spec 提升为通用多页面工具类 App 规格
- [ ] 抽出固定槽位：概览页、列表页、表单页、记录实体、摘要卡
- [ ] 为 generic PRD 输出更稳定的页面、流程、实体和验收标准
- [ ] 让体重记录、习惯打卡、待办、库存记录映射到同一模板能力模型

### 4.6 升级 real-check

- [ ] 重新定义 `jobs-ui real_checks` 对 generic domain 的放行条件
- [ ] 放行前要求 generic compile 产物具备结构化页面与实体
- [ ] 为 `flutter-open-lite` 设计 structural checks：analyze、test、debug apk build
- [ ] 明确 generic real-check 失败后的 public-job 状态与提示文案

### 4.7 补回归样例与验证集

- [ ] 新增 3 个 generic domain 样例需求：体重记录、待办事项、习惯打卡
- [ ] 补 compile 测试，验证不会退回 `single-screen smoke-only`
- [ ] 补 jobs API 测试，验证 generic 模板能进入创建、审批、启动、结果查看链路
- [ ] 补模板漂移测试，验证模板版本变化会触发 template approval / prepare recompile
- [ ] 补失败回归，验证 generic real-check 失败时 UI 和 public-job 语义稳定

### 4.8 收尾产品口径

- [ ] 更新模板治理文档，补 `flutter-open-lite` 准入说明
- [ ] 更新 AppFactory 产品流文档，说明“记账专用模板 + 通用开放模板”的双轨
- [ ] 更新需求接口文档，明确 generic compile 与 generic real-check 输入门槛
- [ ] 更新 `/jobs` 新建任务失败文案，去掉只靠 bookkeeping/finance 举例的旧口径

## 5. 当前缺口

- [ ] 还没有冻结主参考仓库具体 commit/tag
- [ ] 还没有跑完整 `flutter pub get` / `flutter analyze` / `flutter test` / `flutter build apk --debug`
- [ ] 还没有把上游仓库实际瘦身成内部 seed
- [ ] 还没有把 generic compile 和 real-check 的代码实现接上

## 6. 完成标准

- [ ] 仓库内存在一份受控 Flutter 通用模板 seed
- [ ] 模板默认不依赖自建服务器、广告和可选商业化能力
- [ ] 模板默认服务单一核心任务，而不是大而全功能壳
- [ ] 模板已进入注册表和治理验证主线
- [ ] generic compile 不再只是单屏 smoke fallback
- [ ] 至少一个非记账类需求可以进入 real-check 主链
- [ ] `/jobs` 对 generic domain 的失败提示不再只依赖 bookkeeping 关键词

## 7. 当前不做

- [x] 第一阶段不接入多模板矩阵
- [x] 不引入复杂后端依赖的超级模板
- [x] 不为所有垂直领域分别做专用模板
- [x] 不为 iOS 或 Web 单独扩充模板矩阵
- [x] 不无约束接受任意 GitHub Flutter 仓库直接进入主线