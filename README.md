# OneAppFactory

OneAppFactory is an Android app factory. Its current product surface is focused on one main workflow:

```text
requirement -> PRD / DomainModel / BuilderInput -> job -> builder runtime -> generated Flutter workspace -> analyze / test / build evidence
```

This repository no longer maintains legacy non-AppFactory surfaces. Documentation and code paths outside the Android app factory workflow have been removed or are being retired.

## What It Does

- Turns a short app requirement into structured AppFactory artifacts.
- Runs public `/jobs` through the builder runtime and persists build/run state.
- Generates Flutter app workspaces from maintained templates.
- Validates generated apps with deterministic checks, Flutter analysis/tests/builds, and regression scripts.
- Provides a local web launcher for jobs, configuration, and logs.

## Repository Layout

```text
cmd/oneappfactory/             CLI for prepare, run, executor, orchestrator, version
cmd/oneappfactory-launcher/    Local web launcher
pkg/appfactory/                AppFactory compiler, runner, emitter, and run storage
pkg/config/                    OneAppFactory config model
pkg/providers/                 Model provider adapters used by the builder runtime
web/backend/                   Launcher backend and AppFactory APIs
web/frontend/                  Jobs/config/logs web UI
examples/appfactory/           Regression inputs, samples, and templates
docs/design/                   Architecture notes, schemas, and template registry
scripts/                       AppFactory validation and regression scripts
docker/                        Launcher and builder images
```

## Build

Prerequisites:

- Go 1.25 or newer
- Node.js and `pnpm` for the frontend
- Docker when using the builder runtime image or live app builds

```bash
make build
make build-launcher
```

The main outputs are:

```text
build/oneappfactory
build/oneappfactory-launcher
```

## Configure

The default runtime directory is `~/.appfactory`.

```bash
mkdir -p ~/.appfactory
cp config/oneappfactory.example.json ~/.appfactory/config.json
```

Useful environment variables:

| Variable | Purpose |
| --- | --- |
| `ONEAPPFACTORY_HOME` | Runtime home directory, defaults to `~/.appfactory` |
| `ONEAPPFACTORY_CONFIG` | Config file path, defaults to `$ONEAPPFACTORY_HOME/config.json` |
| `ONEAPPFACTORY_BUILDER_IMAGE` | Builder image, defaults to `oneappfactory/builder:local` |

See [docs/configuration.md](docs/configuration.md) and [docs/providers.md](docs/providers.md).

## Run The Launcher

```bash
./build/oneappfactory-launcher -console -no-browser ~/.appfactory/config.json
```

Open `http://localhost:18800` for the jobs dashboard, configuration, and logs. Use `-public` only when you intentionally want the launcher to listen beyond localhost.

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

Build the optional Flutter/Android builder image with:

```bash
docker compose -f docker/docker-compose.oneappfactory.yml --profile builder build oneappfactory-builder
```

See [docs/docker.md](docs/docker.md).

## Validation

Common checks used while developing this repository:

```bash
GOPROXY=https://goproxy.cn,direct go test ./... -count=1 -timeout 600s
cd web/frontend && pnpm build:backend && pnpm test:run
bash scripts/check-appfactory-template-governance.sh
make build && make build-launcher
```

Live `/jobs` regressions are intentionally heavier and require a launcher plus builder/model environment. See [docs/debug.md](docs/debug.md) and [docs/troubleshooting.md](docs/troubleshooting.md).

## Design Docs

The active architecture docs live under [docs/design](docs/design). The most relevant cleanup/refactor records are:

- [docs/design/appfactory-extraction-implementation-plan.zh.md](docs/design/appfactory-extraction-implementation-plan.zh.md)
- [docs/design/oneappfactory-refactor-inventory.zh.md](docs/design/oneappfactory-refactor-inventory.zh.md)
- [docs/design/appfactory-generic-complexity-roadmap.zh.md](docs/design/appfactory-generic-complexity-roadmap.zh.md)
