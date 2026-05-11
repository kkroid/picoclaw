---
name: flutter-build-closure
description: "在 Flutter 工作区接近完成时做低风险收口。Use when a workspace in the flutter-android-p0 profile is close to green and needs constrained analyze/test/build closure with deterministic fixes. This skill only serves the Flutter profile knowledge pack, not the generic executor core."
---

# Flutter Build Closure

用于在 Flutter 工作区已经基本成型后，执行低风险、可回放的收口动作，避免开放式“继续自由修到好”为系统带来不稳定性。

这个 skill 属于 `flutter-android-p0` profile 的知识包，只负责 Flutter 首个 profile 的收口阶段，不属于通用执行器内核。

## 何时使用

- `flutter analyze`、`flutter test`、`flutter build apk --release` 至少已有部分可运行基础。
- 问题集中在兼容性、命名、测试对齐、导入缺失、轻量配置修补。
- 需要一个严格受限的最终收敛步骤。

## 工作方式

1. 先读取 `references/closure-checklist.md`。
2. 再运行 `dart format .`、`flutter analyze`、`flutter test`。
3. 只修复当前失败直接指向的问题，不顺手重构无关代码。
4. 在 analyze 与 test 通过后，再跑 `flutter build apk --release`。
5. 记录每次修复对应的失败签名，避免重复开放式尝试。

## 允许处理的问题

- 导入缺失或错误。
- Flutter API 兼容性小改动。
- 测试断言与页面实际行为的小范围对齐。
- 轻量配置补齐和命名修复。

## 禁止事项

- 不重做页面结构。
- 不替换整套状态管理方案。
- 不引入新的大依赖。
- 不把一个失败信号扩展成大面积自由重构。
- 不借“为了过 analyze/test/build”之名，把首页概览、记一笔、账单列表重新改成另一套信息架构。
- 不为了解一个失败临时回退到默认 demo 或空页面占位。

## 输出要求

- 明确列出收口前失败点和收口后通过的检查项。
- 如果仍存在设备或环境阻塞，要把阻塞归因为环境问题，而不是继续盲修代码。
- 如果发现当前问题本质上需要重做功能实现，而不是低风险修补，必须明确停止收口并把问题交回前一阶段。
