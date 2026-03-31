# 记账任务包样式

目标：为最小记账 App 提供可直接落入 `builder-input.json` 的任务拆分样式。

## 推荐任务顺序

1. `domain`：定义账单实体与概览实体。
2. `storage`：接上本地持久化与主仓储。
3. `screen`：搭首页概览、记一笔、账单列表三个页面。
4. `flow`：接通新增账单、加载列表、刷新概览。
5. `validation`：收口 analyze、test、build 与特定检查项。

## 推荐任务样例

### 1. domain

- `title`: `定义账单与概览模型`
- `category`: `domain`
- `objective`: 固定账单字段、金额类型、日期和概览汇总结构
- `target_paths`:
  - `lib/models/entry.dart`
  - `lib/models/summary.dart`
- `completion_criteria`:
  - `entry.dart` 中已定义账单核心字段，至少包含金额、类型、日期
  - `summary.dart` 中已定义概览所需聚合字段

### 2. storage

- `title`: `接线本地账单仓储`
- `category`: `storage`
- `objective`: 让账单数据可从本地存储读写并供页面复用
- `target_paths`:
  - `lib/repositories/entry_repository.dart`
  - `lib/services/`
- `completion_criteria`:
  - 已存在单一主仓储负责账单读写
  - 仓储已接到本地持久化实现，而不是只操作内存列表

### 3. screen

- `title`: `搭建记账核心页面`
- `category`: `screen`
- `objective`: 固定首页概览、记一笔、账单列表的页面骨架与导航入口
- `target_paths`:
  - `lib/views/home_page.dart`
  - `lib/views/entry_form_page.dart`
  - `lib/views/entry_list_page.dart`
  - `lib/main.dart`
- `completion_criteria`:
  - 默认 Flutter counter demo 已移除
  - 三个核心页面都已有真实入口
  - `main.dart` 已接到新的页面结构

### 4. flow

- `title`: `接通新增账单与概览刷新流程`
- `category`: `flow`
- `objective`: 让新增账单后列表和概览都能看到持久化结果
- `target_paths`:
  - `lib/controllers/home_controller.dart`
  - `lib/controllers/entry_form_controller.dart`
  - `lib/controllers/entry_list_controller.dart`
  - `lib/repositories/entry_repository.dart`
- `completion_criteria`:
  - 提交表单后能写入仓储
  - 账单列表能加载已保存数据
  - 首页概览能反映最新账单聚合结果

### 5. validation

- `title`: `完成记账应用收口验证`
- `category`: `validation`
- `objective`: 让工程通过真实 Flutter 验收并暴露残留缺口
- `target_paths`:
  - `test/`
  - `lib/main.dart`
  - `lib/views/`
  - `lib/controllers/`
  - `lib/repositories/`
- `completion_criteria`:
  - `check-counter-demo-removed` 通过
  - `check-entry-form-wiring` 通过
  - `check-local-persistence-wiring` 通过
  - `flutter analyze`、`flutter test`、`flutter build apk --debug` 通过

## 任务拆分提醒

- 每个任务都要能指向具体文件，不要只给抽象功能名。
- 如果页面已经存在 seed 文件，优先把任务路径写到这些现有文件。
- 如果控制器或仓储还不存在，可以把目标路径写成将要创建的明确文件，而不是宽泛目录。