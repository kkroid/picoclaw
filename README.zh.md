# OneAppFactory

OneAppFactory 是一个面向 Android 应用生成的 AppFactory 产品。当前主链只聚焦一件事：

```text
一句话需求 -> PRD / DomainModel / BuilderInput -> job -> builder runtime -> Flutter 工作区 -> analyze / test / build 证据
```

这个仓库不再维护非 AppFactory 的历史产品面。Android 应用工厂工作流之外的文档和代码路径已经删除或正在退场。

## 当前能力

- 将简短应用需求编译成结构化 AppFactory 制品。
- 通过 `/jobs` 和 builder runtime 执行生成任务，并持久化 run 状态。
- 基于维护中的 Flutter 模板生成应用工作区。
- 通过确定性检查、Flutter analyze/test/build 和回归脚本验证生成结果。
- 提供本地 Web launcher，用于任务、配置和日志管理。

## 仓库结构

```text
cmd/oneappfactory/             CLI：prepare、run、executor、orchestrator、version
cmd/oneappfactory-launcher/    本地 Web launcher
pkg/appfactory/                AppFactory 编译、运行、emitter、run 存储
pkg/config/                    OneAppFactory 配置模型
pkg/providers/                 builder runtime 使用的模型 provider
web/backend/                   launcher 后端和 AppFactory API
web/frontend/                  jobs/config/logs Web UI
examples/appfactory/           回归输入、样例和模板
docs/design/                   架构方案、schema 和模板注册表
scripts/                       AppFactory 验证与回归脚本
docker/                        launcher 和 builder 镜像
```

## 构建

前置依赖：

- Go 1.25 或更新版本
- Node.js 和 `pnpm`
- 使用 builder runtime 镜像或真实 Flutter/Android 构建时需要 Docker

```bash
make build
make build-launcher
```

主要产物：

```text
build/oneappfactory
build/oneappfactory-launcher
```

## 配置

默认运行目录是 `~/.appfactory`。

```bash
mkdir -p ~/.appfactory
cp config/oneappfactory.example.json ~/.appfactory/config.json
```

常用环境变量：

| 变量 | 作用 |
| --- | --- |
| `ONEAPPFACTORY_HOME` | 运行目录，默认 `~/.appfactory` |
| `ONEAPPFACTORY_CONFIG` | 配置路径，默认 `$ONEAPPFACTORY_HOME/config.json` |
| `ONEAPPFACTORY_BUILDER_IMAGE` | builder 镜像，默认 `oneappfactory/builder:local` |

更多说明见 [docs/configuration.md](docs/configuration.md) 和 [docs/providers.md](docs/providers.md)。

## 启动 Launcher

```bash
./build/oneappfactory-launcher -console -no-browser ~/.appfactory/config.json
```

打开 `http://localhost:18800` 使用任务面板、配置页和日志页。只有明确需要从其他机器访问时才使用 `-public`。

## CLI

```bash
./build/oneappfactory version
./build/oneappfactory prepare --help
./build/oneappfactory run-requirement --help
./build/oneappfactory run-executor --help
./build/oneappfactory orchestrator --help
```

## Docker

```bash
docker compose -f docker/docker-compose.oneappfactory.yml up --build
```

可选 builder 镜像：

```bash
docker compose -f docker/docker-compose.oneappfactory.yml --profile builder build oneappfactory-builder
```

更多说明见 [docs/docker.md](docs/docker.md)。

## 验证

常用开发验证：

```bash
GOPROXY=https://goproxy.cn,direct go test ./... -count=1 -timeout 600s
cd web/frontend && pnpm build:backend && pnpm test:run
bash scripts/check-appfactory-template-governance.sh
make build && make build-launcher
```

真实 `/jobs` 回归更重，需要 launcher、builder 和模型环境。排查方式见 [docs/debug.md](docs/debug.md) 和 [docs/troubleshooting.md](docs/troubleshooting.md)。

## 设计文档

当前架构文档位于 [docs/design](docs/design)。重点文档：

- [docs/design/appfactory-extraction-implementation-plan.zh.md](docs/design/appfactory-extraction-implementation-plan.zh.md)
- [docs/design/oneappfactory-refactor-inventory.zh.md](docs/design/oneappfactory-refactor-inventory.zh.md)
- [docs/design/appfactory-generic-complexity-roadmap.zh.md](docs/design/appfactory-generic-complexity-roadmap.zh.md)
