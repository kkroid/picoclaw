# 参与贡献 OneAppFactory

感谢你帮助 OneAppFactory。当前项目已经收敛为 AppFactory 工作流：需求被编译成结构化制品，任务通过 builder runtime 执行，并用可复现检查验证生成的 Flutter 工作区。

## 产品边界

动手前请先确认改动是否服务于当前产品面：

- AppFactory prepare/compiler/runtime/emitter 主链
- `/jobs`、`/api/v1/jobs`、`/api/v1/prds`、`/api/v1/templates` 以及 internal builder/run API
- Web launcher 的任务、配置、日志页面
- builder runtime 所需的 provider/config 能力
- Flutter 模板治理和回归脚本

除非产品方向明确改变，不要新增非 AppFactory 的历史产品面。

## 开发环境

前置依赖：

- Go 1.25 或更新版本
- Node.js 和 `pnpm`
- 真实 Flutter/Android 构建或 builder 镜像验证需要 Docker

常用命令：

```bash
make build
make build-launcher
GOPROXY=https://goproxy.cn,direct go test ./... -count=1 -timeout 600s
cd web/frontend && pnpm build:backend && pnpm test:run
```

模板治理：

```bash
bash scripts/check-appfactory-template-governance.sh
```

## 修改原则

- 改动必须收敛在 AppFactory 产品边界内。
- 优先沿用仓库现有模式，不引入不必要抽象。
- 行为变化要同步更新测试或 fixture。
- 命令、配置结构、验证流程变化时同步更新文档。
- 避免大范围纯格式化改动。

## Pull Request

提交 PR 前：

- 运行与改动相关的最小验证。
- 跨模块改动要运行完整 Go/前端检查。
- 说明影响了哪条 AppFactory 路径，以及如何验证。
- 修改 job orchestrator、builder runtime、模板或生成 Flutter 表面时，提供 live regression 证据。

## AI 辅助贡献

可以使用 AI 辅助，但贡献者仍然对结果负责。

- 阅读并理解生成的改动。
- 验证行为，而不只是验证语法。
- 注意路径穿越、密钥泄露、不安全命令执行和过宽文件写入。
- PR 描述中按需说明 AI 参与情况。
