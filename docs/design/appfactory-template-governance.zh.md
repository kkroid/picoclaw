# AppFactory 模板治理基线

本文档用于给 item 10 补第一版模板治理基线。目标不是把模板注册表做成完整平台，而是先把内部试运行阶段“什么模板能进主线、进来之前要过哪些门、出了问题先看哪里”说清楚。

## 1. 目标

- 为当前 Flutter 模板主线提供最小可执行的治理规则。
- 避免模板继续以“能跑就先接”方式进入产品级整链。
- 把模板验证入口、日志路径、人工审核项和风险边界统一起来。

## 2. 当前适用范围

当前基线只覆盖 AppFactory 默认 Flutter 模板族，尤其是：

- `examples/appfactory/templates/flutter-finance-lite`

其他模板如果要进入内部试运行主线，至少应满足本文档中的最低门槛，再进入 PRD/template approval 与 product-flow 主链。

## 3. 模板进入主线前的最低门槛

至少满足以下条件：

1. 模板目录是受控源码目录，而不是运行期临时下载物。
2. 至少包含 `pubspec.yaml`、`lib/main.dart`、`test/widget_test.dart`、`android/app/build.gradle.kts` 这四类基础文件。
3. 能通过 `flutter pub get`、`flutter analyze`、`flutter test`。
4. 如果要进入产品级整链或设备回归主线，还必须通过 `flutter build apk --debug --no-pub`。
5. 不引入当前 MVP 明确排除的高风险能力：支付、广告、推送、地图、实时通信、高风险原生 SDK。
6. 模板审批证据必须冻结当前 `template-fit-report.md`，不能只冻结 `template_id + pinned_ref`。

## 4. 默认验证入口

完整验证入口：

```bash
make verify-appfactory-template
```

治理检查入口：

```bash
make check-appfactory-template-governance
```

快速验证入口：

```bash
make verify-appfactory-template-fast
```

常用调试参数：

```bash
APPFACTORY_TEMPLATE_VERIFY_ANDROID_VERBOSE=1 \
APPFACTORY_TEMPLATE_VERIFY_ANDROID_GRADLE_LOG_LEVEL=info \
make verify-appfactory-template
```

当前脚本行为要点：

- 默认复用 `workspace/appfactory/builder-cache/template-verify/` 下的持久缓存。
- 默认持久化 workdir，避免每次验证都从零冷启动。
- Android 详细日志会写到 `workspace/appfactory/builder-cache/template-verify/logs/template-verify-android.log`。
- 模板治理检查会把许可证证据路径、直接依赖、Manifest 权限和风险命中写到 `workspace/appfactory/template-governance/latest.{json,md}`，并在 `runs/` 下保留按轮归档结果。
- 当前治理检查除了关键字风险命中外，还会额外区分：`git/path` 直连依赖与许可证缺失属于失败项，`any/*/纯范围式` 这类过松 direct dependency constraint 属于 warning 项。
- 当前治理检查还会继续读取 `pubspec.lock` 里的 transitive packages，并优先从 `workspace/appfactory/builder-cache/template-verify/pub-cache/` 查找 hosted package 的许可证证据；如果 pub cache 未预热或某些 hosted package 缺少许可证文件，会进入 transitive license warning。

## 5. 人工准入检查清单

模板进入内部试运行主线前，建议至少完成以下人工检查：

1. 模板用途是否仍属于 MVP 的工具类 App 包络，而不是已经隐含复杂后台、账号体系或商业化能力。
2. 模板依赖是否包含高风险第三方 SDK、闭源二进制或来源不明的 Gradle/Maven 仓库。
3. 模板是否需要额外环境前提，例如私有服务端、专有密钥或外部文件挂载；若需要，当前不应进入默认主线。
4. 模板默认文案、资源和示例数据是否带有不适合对外演示或容易误导审批的占位内容。
5. 模板的 `template-fit-report.md` 是否已经把“能力匹配”和“风险缺口”写明，而不是只给一个口头结论。
6. 如果模板 direct dependency 使用 `git:` 或 `path:`，必须先完成 vendoring、镜像托管或治理豁免，不应直接进入默认内部试运行主线。

## 6. 当前风险边界

内部试运行阶段，出现以下任一情况时，模板不应直接进入主线：

- 许可证不清楚，或无法快速确认可内部使用与二次改造。
- direct dependency 仍依赖 `git:` 或 `path:`，导致模板来源不可稳定复现。
- 依赖需要人工接受额外法律/商业条款。
- 模板默认接入高风险原生能力，但当前 item 10 还没有对应审计和发布规范。
- 模板通过 fast verify，但 full verify 持续不稳定。
- 模板只能在特定开发者机器上跑通，无法在当前受控 builder 镜像内复现。

## 7. 模板问题排障入口

优先顺序：

1. 先跑 `make verify-appfactory-template-fast`，确认问题是否已在 analyze/test 层暴露。
2. 如果 fast 通过但 full 失败，再跑 `make verify-appfactory-template`。
3. 如果 transitive license evidence 不完整，先确认是否已经跑过至少一轮 template verify，让 `workspace/appfactory/builder-cache/template-verify/pub-cache/` 被正确预热。
4. Android 构建问题优先看 `workspace/appfactory/builder-cache/template-verify/logs/template-verify-android.log`。
5. 如果模板已经进入 product-flow 主线，再回看对应 Job 的 `template-fit-report.md`、approval snapshot 和 `builder-input.json`。

## 8. 当前边界

本文档当前只定义“模板进入主线前的最低门槛”，还没有自动完成：

- 完整 SPDX/许可证文本自动扫描
- 更细粒度的 transitive dependency 风险自动分级
- 模板健康分数
- 模板注册表的正式准入/下线流程

这些能力后续可以继续加，但当前内部试运行不再允许跳过最小准入检查。