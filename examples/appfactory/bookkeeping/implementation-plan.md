# 实施计划

- Job ID: job-bookkeeping-lite
- PRD ID: prd-bookkeeping-lite

## 阶段拆分

- 阶段 1：冻结记账 App 最小范围，确认首页、录入页、列表页和数据模型。
- 阶段 2：把范围拆成 task bundle，生成 Builder 可执行的 acceptance checks。
- 阶段 3：通过当前 P0 adapter 跑通最小链路，为后续接入真实 Flutter Builder runtime 留接口。

## 任务包

### 定义账单与汇总数据模型

- 目标：明确记账 App 最小数据结构和本地持久化边界。
- 分类：data
- 目标路径：lib/models/**, lib/data/**
- 完成标准：账单记录包含金额、分类、日期和备注；支持本地读写账单列表

### 定义记账录入页

- 目标：让 Builder 明确首页到记账录入页的核心 UI 结构。
- 分类：ui
- 目标路径：lib/screens/entry/**, lib/widgets/**
- 完成标准：录入页字段齐全；保存后能回到首页或列表页

### 定义首页和账单列表页

- 目标：覆盖首页概览、最近账单和完整账单列表的导航关系。
- 分类：navigation
- 目标路径：lib/screens/home/**, lib/screens/ledger/**, lib/navigation/**
- 完成标准：首页能看到汇总信息；列表页按时间倒序呈现账单

## 验收检查

- 准备上下文文件：stage=baseline，commands=echo context-ready
- 确认记账 MVP 范围：stage=baseline，commands=echo bookkeeping-scope-home-entry-ledger
- 确认 Builder 输入包可执行：stage=cheap，commands=echo builder-input-ready
