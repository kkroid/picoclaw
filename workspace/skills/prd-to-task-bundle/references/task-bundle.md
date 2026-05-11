# 任务包映射规则

推荐任务类别：

- `domain`：实体、字段、序列化、仓储接口。
- `storage`：本地存储、数据源接线、种子数据迁移。
- `screen`：页面骨架、导航入口、空态与列表容器。
- `flow`：表单提交、筛选、排序、详情跳转、状态同步。
- `validation`：格式化、静态检查、测试、构建与必要的低风险收口。

推荐切分顺序：

1. 先产出 `domain` 任务，固定实体和字段。
2. 再产出 `storage` 任务，固定数据流向。
3. 再产出 `screen` 任务，搭页面骨架和导航。
4. 最后产出 `flow` 任务，把交互闭环接上。
5. 把 `validation` 任务放在末端，但 acceptance checks 要从一开始就生成。

每个任务至少包含：

- `title`
- `category`
- `objective`
- `target_paths`
- `completion_criteria`

target paths 写法要求：

- 优先写到具体文件，至少写到明确目录。
- 不要只写 `lib/**` 这类过宽路径作为主要目标。
- 页面任务、存储任务、验证任务要能从目标路径直接看出归属。

completion criteria 写法要求：

- 使用可观察结果，不写抽象意图。
- 尽量引用页面、实体、命令结果或文件位置。
- 单条标准只判断一件事。

acceptance checks 最低要求：

- `flutter analyze`
- `flutter test`
- `flutter build apk --release`

对记账类 Flutter App，推荐附加验收信号：

- 默认 counter demo 已移除。
- 已有真实账单录入表单接线。
- 本地持久化已经接线，而不是只放在内存变量中。

如当前环境没有设备，不强制生成 `adb` 侧硬依赖任务，但要在任务或人工说明中明确记录。

禁止事项：

- 不把“实现记账功能”压成一个总任务。
- 不把任务目标路径写成无法定位责任边界的宽泛通配。
- 不省略 validation 任务，导致 builder 只做代码改动却没有收口要求。
