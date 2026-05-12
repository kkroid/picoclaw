# flutter_open_lite

`flutter-open-lite` 是 AppFactory 的通用 Flutter 种子模板。

它只冻结最小可复用骨架，不直接等于某个最终业务应用。当前模板固定提供以下槽位：

- 首页摘要区
- 记录列表页
- 新建/编辑表单页
- 记录详情页
- 最小 CRUD 闭环
- 本地状态与本地持久化
- 基础 Widget 测试入口

模板边界：

- 本地优先，不依赖自建服务器
- 不包含登录、支付、广告、推送、地图、实时通信
- 目录分层固定为 `models / repositories / controllers / views`
- 默认使用 Hive 作为轻量本地持久化

验证方式：

```bash
make verify-appfactory-template-fast
make verify-appfactory-template
```

派生业务模板时，优先替换以下内容：

- 应用名称与默认文案
- 记录实体字段
- 摘要卡统计逻辑
- 列表和详情展示字段
- 分类枚举和表单默认项

当前模板已覆盖“新增 -> 查看详情 -> 编辑 -> 回到列表/首页”这条最小记录闭环，适合继续承接待办、习惯、库存、轻量追踪等单任务工具类需求的二开。

当前模板也已覆盖删除动作，因此默认能力边界已经达到最小 CRUD：创建、列表/详情读取、编辑、删除。
