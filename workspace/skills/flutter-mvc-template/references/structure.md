# 目录与依赖方向

标准目录：

- `lib/core/`：应用级常量、主题、路由入口、通用错误与格式化工具。
- `lib/models/`：纯数据实体、枚举、序列化对象，不放 UI 逻辑。
- `lib/repositories/`：数据访问接口与实现，负责本地存储和外部数据源适配。
- `lib/controllers/`：页面动作、表单提交、导航编排、状态同步。
- `lib/views/`：页面和可复用 UI 组件，尽量不直接触达底层存储。
- `lib/services/`：时间、导入导出、权限、通知等横切能力。

依赖方向：

- `views -> controllers -> repositories/services -> models`
- `controllers -> models`
- `repositories -> models`
- `services` 不反向依赖具体页面。
- `models` 不依赖 Flutter UI 层。

页面落地规则：

- 一个核心页面对应一个主 view 文件。
- 一个复杂表单页面允许额外拆出本页局部 widget，但仍归入 `views/`。
- 一个核心用户流程至少有一个 controller 作为动作入口。
- 与单一业务实体强绑定的数据读写逻辑优先进入对应 repository。
- 如果首页同时承担概览与最近记录展示，仍只保留一个主页面入口，不再拆出第二个平行首页层。
- 表单页面的字段校验、提交和保存动作优先归入对应 controller，不把保存逻辑散落到 widget 层。
- 列表页的筛选、排序和分组可以拆局部 widget，但主状态与数据装载仍归 controller 和 repository。

命名规则：

- 页面使用 `xxx_page.dart`。
- 控制器使用 `xxx_controller.dart`。
- 仓储使用 `xxx_repository.dart`。
- 数据实体使用业务名词单数，如 `entry.dart`、`summary.dart`。

禁止事项：

- 不在 `views/` 中直接拼接存储层实现。
- 不在 `models/` 中混入页面状态或控制器动作。
- 不为了一个页面临时再造 `manager`、`helper`、`provider` 等平行层。
- 不为首页概览、记一笔、账单列表各自再发明一套不同的数据访问方式。