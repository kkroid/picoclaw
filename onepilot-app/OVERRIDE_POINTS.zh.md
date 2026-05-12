# flutter-open-lite 覆盖点说明

本模板仍然是中性 seed，但后续 Builder 必须优先通过以下覆盖点完成领域收口，而不是到处散改。

## 必须优先覆盖的点

- `lib/template/open_lite_copy.dart`
  - 应用标题
  - 首页摘要标题
  - 首页主按钮文案
  - 列表页标题、筛选标题、空态文案
  - 表单页标题、字段标签、按钮文案
  - 详情页标题、删除确认文案、字段标签
  - 状态标签
- `android/app/src/main/res/values/strings.xml`
  - Android 启动器应用名 `app_name`
- `android/app/build.gradle.kts`
  - `defaultOpenLiteApplicationId`
  - Android `namespace`
  - Android `applicationId`

## 可以保持通用骨架的点

- `lib/main.dart` 的应用启动流程
- `home/list/form/detail` 四页结构
- 控制器与仓储的连接方式
- Hive 本地持久化基线

## 当前约束

- 不允许非空领域需求继续保留 `Open Lite Seed` 作为应用标题或首页标题
- 不允许 Android 启动器名称继续停留在默认 seed 名称
- 不允许领域需求只改 PRD，不改模板文案入口
- 在没有新模板之前，优先复用这套骨架，不新增第二套平行页面结构