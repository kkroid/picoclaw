# Troubleshooting

## `model ... not found in model_list`

**Symptom:** A job or CLI command fails before model execution with a missing model alias.

**Cause:** `appfactory.planning_engine`, `appfactory.builder_runtime.default_model`, `appfactory.builder_runtime.upgrade_model`, or a `task_routes` entry references an alias that is not present in `model_list`.

**Fix:** Add a matching `model_name` entry or change the route to an existing alias.

```json
{
  "model_list": [
    {
      "model_name": "qwen2.5-coder-14b-local",
      "model": "ollama/qwen2.5-coder:14b",
      "api_base": "http://127.0.0.1:11434/v1"
    }
  ],
  "appfactory": {
    "builder_runtime": {
      "default_model": { "primary": "qwen2.5-coder-14b-local" }
    }
  }
}
```

For OpenRouter, use a full model id such as `openrouter/free`; do not use `free` by itself.

## Flutter APK build fails with AAPT2 `Permission denied`

**Symptom:** `flutter build apk` or an AppFactory build step fails with messages such as:

```text
AAPT2 ... Daemon startup failed
Cannot run program ".../aapt2": error=13, Permission denied
```

**Cause:** Gradle extracted `aapt2` into the AppFactory Gradle cache without the executable bit.

**Current behavior:** The runner repairs the executable bit for this known AAPT2 permission pattern and retries the same step.

**Manual workaround:**

```bash
find ~/.appfactory/workspace/appfactory/.runtime/gradle-user-home \
  -type f -path '*/transformed/aapt2-*-linux/aapt2' \
  -exec chmod 755 {} +
```

If the build still fails, inspect the log around `:app:processDebugResources` or `:app:processReleaseResources`.

## Regression output is hard to read

The regression scripts write stable latest artifacts under `workspace/appfactory`:

```text
workspace/appfactory/jobs-ui-regression/latest.json
workspace/appfactory/jobs-ui-regression/latest.md
workspace/appfactory/jobs-ui-regression/latest.log
```

Read `latest.md` first. If the cause is still unclear, open the `console_log`, `builder_log`, and `event_log` paths referenced by the summary.

## Builder runtime returns an empty patch body

**Symptom:** The builder log shows a patch attempt header but the body is blank, or parsing fails with `unexpected end of JSON input`.

**Current behavior:** Empty patch bodies are treated as soft failures for the current alias. If fallbacks exist, the runtime tries the next alias before escalating to schema repair.

**Fix:** Check whether every alias returned an empty body. If only the primary alias was empty, tune or replace that alias. If all aliases are empty, inspect provider latency, rate limits, and model output settings.

## Generated controllers drift into placeholder APIs

**Symptom:** A generated Flutter workspace contains obvious placeholder or unrelated APIs, such as `uuid`, duplicate controller shells, or fields from another domain.

**Cause:** The model patch drifted away from the runtime contract or the launcher binary is stale.

**Fix:**

1. Rebuild the launcher with `make build-launcher`.
2. Restart it with the intended config.
3. Re-run the job or regression.
4. Inspect the live workspace content rather than only the raw model output.

## `go test ./...` touches generated workspace data

If `go test ./...` fails while walking ignored `workspace/appfactory` caches, add an ignored nested module boundary:

```text
workspace/appfactory/go.mod
```

That keeps root module traversal focused on repository source packages instead of local generated data.
