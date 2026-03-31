---
name: prd-to-task-bundle
description: "把已批准 PRD 编译成 Builder 可执行的 task bundle。Use when converting PRD screens, entities, user flows, and acceptance criteria into ordered Builder tasks for the flutter-android-p0 profile. This skill only serves the Flutter profile knowledge pack, not the generic executor core."
---

# PRD To Task Bundle

用于把结构化 PRD 转成低自由度的 `task_bundle` 和 acceptance checks，供 `builder-input.json` 直接消费。

这个 skill 属于 `flutter-android-p0` profile 的知识包，只负责 Flutter 首个 profile 的任务包翻译，不属于通用执行器内核。

## 何时使用

- 已经有 `PRD.json`、模板约束和页面清单。
- 需要生成 `implementation-plan.md` 或 `builder-input.json` 中的任务包。
- 需要把“功能需求”压成可排序、可验收、可追溯的执行单元。

## 工作方式

1. 读取 [任务包映射规则](references/task-bundle.md)。
2. 如目标是记账类 Flutter App，再读取 [记账任务包样式](./references/bookkeeping-task-bundle.md)。
3. 先按实体、页面、流程、验证四类信息抽取任务候选。
4. 再按依赖关系生成顺序，不按 M/V/C 教科书层次切碎。
5. 为每个任务补齐目标路径、完成标准和风险提示。
6. 生成 acceptance checks，确保至少覆盖 analyze、test、build。

## 强约束

- 一个任务必须能被单独判断完成或失败。
- 一个任务必须能映射回 PRD 条目或 acceptance criterion。
- 不要产出“继续完善 UI”“补齐逻辑”这类不可验收任务。
- 不要把所有页面和实体压成一个大任务。

## 结果要求

- 任务顺序可直接进入 `builder-input.json`。
- 任务标题和完成标准简洁、可验证、可追溯。
- 任务切分优先服务收敛与验证，不追求概念上的分层优雅。