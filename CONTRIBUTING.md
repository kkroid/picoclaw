# Contributing to OneAppFactory

Thank you for helping OneAppFactory. The project is now focused on the AppFactory workflow: requirements become structured artifacts, jobs run through the builder runtime, and generated Flutter workspaces are validated with repeatable checks.

## Product Boundary

Before starting a change, check whether it supports the current product surface:

- AppFactory prepare/compiler/runtime/emitter logic
- `/jobs`, `/api/v1/jobs`, `/api/v1/prds`, `/api/v1/templates`, and internal builder/run APIs
- Web launcher pages for jobs, configuration, and logs
- Provider/config support required by the builder runtime
- Flutter template governance and regression scripts

Do not add legacy non-AppFactory product surfaces unless the product direction changes explicitly.

## Development Setup

Prerequisites:

- Go 1.25 or newer
- Node.js and `pnpm`
- Docker for builder image and live Flutter/Android validation

Common commands:

```bash
make build
make build-launcher
GOPROXY=https://goproxy.cn,direct go test ./... -count=1 -timeout 600s
cd web/frontend && pnpm build:backend && pnpm test:run
```

Template governance:

```bash
bash scripts/check-appfactory-template-governance.sh
```

## Making Changes

- Keep changes scoped to the AppFactory product boundary.
- Prefer the repository's existing patterns over new abstractions.
- Update tests or fixtures when behavior changes.
- Update docs when commands, config shape, or validation workflow changes.
- Avoid broad formatting-only churn.

## Pull Requests

Before opening a PR:

- Run the smallest relevant validation for your change.
- Run full Go/frontend checks for cross-cutting changes.
- Describe which AppFactory path was affected and how it was verified.
- Include live regression evidence when changing job orchestration, builder runtime, templates, or generated Flutter surfaces.

## AI-Assisted Contributions

AI assistance is welcome, but contributors remain responsible for the result.

- Read and understand generated changes.
- Validate behavior, not only syntax.
- Check for path traversal, secret exposure, unsafe command execution, and overly broad file writes.
- Mention AI involvement in the PR description when applicable.
