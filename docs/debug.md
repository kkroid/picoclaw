# Debugging OneAppFactory

Debugging usually starts from a job id. A job connects the public `/jobs` API, builder runtime events, generated Flutter workspace, and validation artifacts.

## Start With Console Logs

Run the launcher with console output enabled:

```bash
./build/oneappfactory-launcher -console -no-browser ~/.appfactory/config.json
```

The web UI also includes a logs page for recent service output.

## Inspect Job Artifacts

By default, runtime data lives under `~/.appfactory`. In repository-local development and regression scripts, generated data may also be under `workspace/appfactory`.

Useful locations:

```text
~/.appfactory/workspace/appfactory/
workspace/appfactory/jobs-ui-regression/latest.json
workspace/appfactory/jobs-ui-regression/latest.md
workspace/appfactory/jobs-ui-regression/latest.log
workspace/appfactory/template-governance/latest.json
workspace/appfactory/template-governance/latest.md
```

When a regression fails, read the summary first, then open the referenced builder log and event log.

## Run Focused Checks

```bash
GOPROXY=https://goproxy.cn,direct go test ./pkg/appfactory/... -count=1 -timeout 300s
GOPROXY=https://goproxy.cn,direct go test ./web/backend/... -count=1 -timeout 300s
cd web/frontend && pnpm test:run
bash scripts/check-appfactory-template-governance.sh
```

For full repository verification:

```bash
GOPROXY=https://goproxy.cn,direct go test ./... -count=1 -timeout 600s
cd web/frontend && pnpm build:backend && pnpm test:run
make build && make build-launcher
```

## Read Builder Runtime Failures

Common frontier fields in regression summaries:

| Field | Meaning |
| --- | --- |
| `frontier` | The most specific failure boundary detected by the script |
| `focus_path` | File or path cluster to inspect first |
| `failure` | Short raw failure summary |
| `explanation` | Normalized explanation for known signatures |
| `last_success` | Last concrete success before the failure |
| `builder_log` | Main builder runtime transcript |
| `event_log` | Structured event stream |
| `console_log` | Full script-side transcript |

Start with `frontier` and `focus_path`. Open `builder_log` only after reading the summary fields.

## Environment Gotcha

In this repository, ignored `workspace/appfactory` data can contain root-owned Gradle caches from container runs. If `go test ./...` unexpectedly tries to traverse that generated directory, keep it isolated as a nested module with an ignored `workspace/appfactory/go.mod` boundary.
