---
name: flutter-mvc-template
description: "在 Flutter 工具类 Android App 中套用 PicoClaw 固定 MVC 模板。Use when translating an approved PRD into the flutter-android-p0 profile with fixed folders, dependency rules, and naming conventions. This skill only serves the Flutter profile knowledge pack, not the generic executor core."
---

# Flutter MVC Template

用于把“已批准 PRD + 已选模板”收敛到固定 Flutter MVC 结构，减少 Builder 在项目结构、依赖和命名上的自由发挥。

这个 skill 属于 `flutter-android-p0` profile 的知识包，只负责 Flutter 模板和目录约束，不属于通用执行器内核。

## 何时使用

- 需要把工具类 App 放进 PicoClaw 标准 Flutter MVC 模板。
- 需要确定页面、控制器、模型、仓储和服务各自的落点。
- 需要限制第三方库选择，避免同类能力重复引入。

## 工作方式

1. 先读取 [目录与依赖方向](references/structure.md)。
2. 再读取 [依赖白名单](references/dependencies.md)。
3. 如目标是记账类工具 App，再读取 [记账页面落点](./references/bookkeeping-page-mapping.md)。
4. 只在既定目录下新增或修改文件，不临时发明新的架构层。
5. 对每个页面确认对应的 controller、view、repository 归属。
6. 输出前自查是否违反目录规则、依赖方向和库白名单。

## 强约束

- 这是固定模板 skill，不负责自由设计新架构。
- 优先按功能闭环落文件，再映射到 MVC 目录，而不是先按教科书概念拆碎任务。
- 不同时引入两个同类状态管理库或两个同类本地存储库。
- 任何超出白名单的依赖，都必须在结果中明确说明原因。

## 结果要求

- 页面、控制器、模型、仓储的文件位置稳定。
- 页面导航、表单校验、数据读写能从目录结构直接追溯。
- 对记账类 App，首页概览、记一笔、账单列表必须有稳定落点，而不是临时拼装。
- 生成结果能直接被后续 `prd-to-task-bundle` 和 `flutter-build-closure` 使用。