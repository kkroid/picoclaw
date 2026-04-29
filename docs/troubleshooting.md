# Troubleshooting

## "model ... not found in model_list" or OpenRouter "free is not a valid model ID"

**Symptom:** You see either:

- `Error creating provider: model "openrouter/free" not found in model_list`
- OpenRouter returns 400: `"free is not a valid model ID"`

**Cause:** The `model` field in your `model_list` entry is what gets sent to the API. For OpenRouter you must use the **full** model ID, not a shorthand.

- **Wrong:** `"model": "free"` → OpenRouter receives `free` and rejects it.
- **Right:** `"model": "openrouter/free"` → OpenRouter receives `openrouter/free` (auto free-tier routing).

**Fix:** In `~/.picoclaw/config.json` (or your config path):

1. **agents.defaults.model_name** must match a `model_name` in `model_list` (e.g. `"openrouter-free"`).
2. That entry’s **model** must be a valid OpenRouter model ID, for example:
   - `"openrouter/free"` – auto free-tier
   - `"google/gemini-2.0-flash-exp:free"`
   - `"meta-llama/llama-3.1-8b-instruct:free"`

Example snippet:

```json
{
  "agents": {
    "defaults": {
      "model_name": "openrouter-free"
    }
  },
  "model_list": [
    {
      "model_name": "openrouter-free",
      "model": "openrouter/free",
      "api_key": "sk-or-v1-YOUR_OPENROUTER_KEY",
      "api_base": "https://openrouter.ai/api/v1"
    }
  ]
}
```

Get your key at [OpenRouter Keys](https://openrouter.ai/keys).

## AppFactory Flutter APK build fails with AAPT2 `Permission denied`

**Symptom:** `check-flutter-build-apk` or a manual `flutter build apk --debug --no-pub` fails with lines such as:

- `AAPT2 ... Daemon startup failed`
- `Cannot run program ".../aapt2": error=13, Permission denied`

**Cause:** Gradle extracts `aapt2` into AppFactory's `.runtime/gradle-user-home` cache. In some environments that binary lands with mode `0644`, so the AAPT2 daemon cannot start. Flutter may then end with the generic message `Gradle does not have execution permission`, which hides the real cause.

**Current behavior:** When `check-flutter-build-apk` hits this exact AAPT2 permission pattern, the runner now restores the execute bit on the cached `aapt2` binary and retries the same step automatically.

**Manual workaround:** If you are reproducing outside the runner, repair the cached binary permissions first:

```sh
find ~/.picoclaw/workspace/appfactory/.runtime/gradle-user-home -type f -path '*/transformed/aapt2-*-linux/aapt2' -exec chmod 755 {} +
```

If it still fails after that, inspect the same build log around `:app:processDebugResources` or `:app:processReleaseResources` instead of relying on Flutter's final generic fix hint.

## AppFactory `/jobs` regression output is hard to read

**Symptom:** `scripts/run-appfactory-jobs-regression.sh` reaches a terminal failure, but the old poll line or raw event dump does not make it clear what actually failed.

**Current behavior:** The regression script now prints a final `== readable summary ==` block and writes the same structured fields into `workspace/appfactory/jobs-ui-regression/latest.json` and `workspace/appfactory/jobs-ui-regression/latest.md`. It also persists the full console transcript for each validation run into that run's `console.log`, and refreshes `workspace/appfactory/jobs-ui-regression/latest.log` as the stable entry point for the newest run.

Read the fields in this order:

- `result`: terminal status and total runtime, used to confirm whether the run really ended and how long it took.
- `frontier`: the most concrete failure boundary the script could identify, preferring run-level events such as patch apply or patch generation failures over generic orchestrator completion noise.
- `focus_path`: the file or path cluster you should inspect first.
- `failure`: the raw failure summary kept short enough to scan quickly.
- `explanation`: a normalized plain-language explanation for common failure signatures.
- `last_success`: the last concrete success event before the failure frontier.
- `builder_log` and `event_log`: the next artifacts to open when the summary alone is not enough.
- `console_log`: the full stdout/stderr transcript for this validation run, useful when you want the exact poll sequence and script-side output.
- `latest_console_log`: the stable log path for the newest validation run.

**Practical reading order:** Start with `frontier` and `focus_path`. If the root cause is still unclear, read `explanation`, then open `console_log` or `latest_console_log` for the exact transcript, and only then fall back to `builder_log` and the full event log.

## AppFactory builder runtime returns an empty patch body

**Symptom:** The builder log shows a model header for a patch attempt, but the body is blank, or the run fails immediately with parse errors such as `unexpected end of JSON input` after patch generation.

**Current behavior:** The builder runtime now treats an empty patch body as a soft failure for that specific model alias. If the route has fallback aliases, it automatically tries the next alias first. If every alias still returns an empty body, the runtime keeps forwarding that empty response into schema repair so single-model routes and exhausted fallback chains can still retry against the current workspace state instead of stopping at the raw parse failure.

**How to read the failure:** If a run still stops here, open the builder log and check whether only the first alias was empty or whether every alias returned an empty or truncated body. A single empty primary alias should now move the frontier forward to the fallback alias. If all aliases are empty, the next meaningful frontier should be schema repair rather than the initial patch attempt.

## AppFactory inventory relation-rich controllers drift into `uuid` or placeholder shells

**Symptom:** A fresh `/jobs` run for `examples/appfactory/relation-rich/inventory-sheet-line-item/requirement.md` gets past the model and repository slice, but the generated controllers contain obvious drift such as:

- `package:uuid/uuid.dart` or `const Uuid()` in `lib/controllers/record_form_controller.dart`
- `_waresides` or project-oriented APIs inside `lib/controllers/home_controller.dart`
- `_applyFilters()`, `_filterlyType`, `InventoryListControllerFixed`, or duplicate controller shells inside `lib/controllers/record_list_controller.dart`

**Current behavior:** The builder runtime now treats `inventory-sheet-line-item` as its own relation-rich profile and canonicalizes `lib/controllers/home_controller.dart`, `lib/controllers/record_form_controller.dart`, and `lib/controllers/record_list_controller.dart` to inventory-specific contracts aligned with `inventory_sheet.dart`, `line_item.dart`, `sku.dart`, `warehouse.dart`, `dashboard_summary.dart`, and `record_repository.dart`.

**How to read the failure:** If a controller-stage run still shows any of the drift above, check whether you are validating against a stale launcher binary. The reliable signal is the live workspace content, not the raw model output alone. Rebuild `build/picoclaw-launcher`, restart an isolated launcher, and confirm the landed controller files no longer contain `uuid`, project/tag APIs, `_waresides`, `_applyFilters()`, or duplicate fallback controller classes.
