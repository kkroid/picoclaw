---
name: builder-direct-edit
description: "强约束 Builder 先读工作区再直接改代码。Use when a builder for the flutter-android-p0 profile keeps returning plan-only replies, meta output, Tasks, Commands, Uses, patch drafts, or legacy finish markers instead of making real file edits in the existing workspace. This skill only serves the Flutter profile knowledge pack, not the generic executor core."
---

# Builder Direct Edit

用于把 Builder 从“输出计划和说明”拉回到“读取当前工作区并直接修改文件”。

这个 skill 属于 `flutter-android-p0` profile 的知识包，只负责 Flutter 工作区中的单轮直接改码约束，不属于通用执行器内核。

这个 skill 不负责调度、超时、重试和 stream 控制；它只负责约束 Builder 在一次编辑轮里的行为，减少 plan-only reply。

## 何时使用

- 当前工作区已经有 Flutter seed 工程，需要在现有文件上继续施工。
- Builder 倾向输出计划、说明、补丁草稿、`Tasks`、`Commands`、`Uses` 或历史 finish 标记。
- 目标是直接把工作区改成可运行的功能闭环，而不是先写一份“将要怎么做”的说明。

## 工作方式

1. 先读取 [直接改码规则](references/direct-edit-rules.md)。
2. 必须先查看当前工作区中的 `pubspec.yaml`、`lib/`、`test/` 和已有 seed 文件，再开始改动。
3. 以当前工作区为准做增量修改，不重写成另一套陌生工程。
4. 优先补齐最小功能闭环，再补测试和构建收口，不把一次编辑轮拆成抽象计划。
5. 输出前自查：这轮是否真的改了文件，而不是只输出元信息。

## 强约束

- 禁止输出计划、任务拆解、实施说明、补丁草稿、整段文件草稿或阅读性代码块来替代真实改动。
- 禁止输出 `Tasks`、`Commands`、`Uses`、历史 finish 标记、`_apply.sh` 说明或类似元信息作为主结果。
- 禁止忽略现有 seed 工程；必须在已有 `pubspec.yaml`、`lib/`、`test/` 基础上继续施工。
- 禁止只改测试、只改文档、只改配置而不落真实功能代码。
- 禁止把默认 Flutter counter demo 留在主入口里冒充已完成实现。

## 结果要求

- 至少有真实文件改动落到当前工作区。
- 改动能直接对应 PRD 中的页面、数据或流程，而不是抽象规划。
- 改动后的工程能继续交给 `flutter-mvc-template` 和 `flutter-build-closure` 收敛。
