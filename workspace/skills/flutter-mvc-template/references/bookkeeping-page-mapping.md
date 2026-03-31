# 记账页面落点

目标：把记账类 App 的首页概览、记一笔、账单列表固定到稳定目录，减少 Builder 漂移。

## 推荐文件落点

- 首页概览：
  - `lib/views/home_page.dart`
  - `lib/controllers/home_controller.dart`
- 记一笔：
  - `lib/views/entry_form_page.dart`
  - `lib/controllers/entry_form_controller.dart`
- 账单列表：
  - `lib/views/entry_list_page.dart`
  - `lib/controllers/entry_list_controller.dart`
- 账单实体：
  - `lib/models/entry.dart`
- 概览实体：
  - `lib/models/summary.dart`
- 账单数据读写：
  - `lib/repositories/entry_repository.dart`

## 页面职责

### 首页概览

- 展示总收入、总支出、净额或等价概览信息。
- 可以展示最近若干条账单摘要，但不承担完整列表的全部交互。
- 只负责概览展示和跳转入口，不直接写存储。

### 记一笔

- 至少包含金额、类型、日期。
- 可以按最小实现增加备注或分类，但这些不是替代核心字段的理由。
- 提交动作必须接到 repository，而不是只更新页面局部状态。

### 账单列表

- 能展示已保存账单。
- 至少支持按时间顺序查看。
- 如需筛选或分组，优先在 controller 中组织，不在页面里直接操作原始存储结果。

## Repository 规则

- 记账类最小实现默认只保留一个主仓储：`entry_repository.dart`。
- 首页概览所需统计优先复用同一仓储聚合，而不是再造一个平行 summary repository。
- 如果后续新增导入导出、同步、分类管理，再评估是否拆更多 repository。

## 禁止事项

- 不把首页概览、记一笔、账单列表全部塞进一个超大页面文件。
- 不为每个页面各建一套独立存储实现。
- 不把持久化代码直接写进 `home_page.dart`、`entry_form_page.dart`、`entry_list_page.dart`。