# Configuration

OneAppFactory reads one JSON config file. By default the launcher and CLI use:

```text
~/.appfactory/config.json
```

The sample file is [../config/oneappfactory.example.json](../config/oneappfactory.example.json).

## Environment Variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `ONEAPPFACTORY_HOME` | `~/.appfactory` | Runtime home directory |
| `ONEAPPFACTORY_CONFIG` | `$ONEAPPFACTORY_HOME/config.json` | Config file path |
| `ONEAPPFACTORY_BINARY` | auto-detected | Override CLI binary used by launcher helpers |
| `ONEAPPFACTORY_BUILDER_IMAGE` | `oneappfactory/builder:local` | Docker image used for builder runtime validation |

## Top-Level Fields

```json
{
  "version": 1,
  "workspace": "~/.appfactory/workspace",
  "model_list": [],
  "appfactory": {}
}
```

| Field | Required | Description |
| --- | --- | --- |
| `version` | Yes | Config schema version. Current version is `1`. |
| `workspace` | Yes | Root workspace for generated AppFactory artifacts. |
| `model_list` | Yes | Named provider/model entries used by planning and builder runtime. |
| `appfactory` | Yes | AppFactory planning and builder runtime settings. |

Legacy non-AppFactory sections are not part of the current product surface.

## Model Entries

Each model entry needs a stable `model_name` and provider-specific `model` value:

```json
{
  "model_name": "qwen2.5-coder-14b-local",
  "model": "ollama/qwen2.5-coder:14b",
  "api_base": "http://127.0.0.1:11434/v1"
}
```

Common optional fields:

| Field | Description |
| --- | --- |
| `api_key` / `api_keys` | Provider credentials. Sensitive values may be stored through the credential layer. |
| `api_base` | Override endpoint for OpenAI-compatible or local providers. |
| `fallbacks` | Model aliases to try after the current entry fails. |
| `rpm` | Requests-per-minute guard. |
| `request_timeout` | Request timeout in seconds. |
| `thinking_level` | Provider-specific reasoning depth where supported. |
| `extra_body` | Additional provider-specific JSON body fields. |

See [providers.md](providers.md).

## AppFactory Settings

```json
{
  "appfactory": {
    "planning_engine": {
      "planning_model": { "primary": "qwen2.5-coder-14b-local" },
      "decision_model": { "primary": "qwen2.5-coder-14b-local" }
    },
    "builder_runtime": {
      "enabled": true,
      "default_model": {
        "primary": "qwen2.5-coder-14b-local",
        "fallbacks": ["qwen2.5-coder-32b-local"]
      },
      "upgrade_model": { "primary": "qwen2.5-coder-32b-local" },
      "task_routes": [
        { "task_type": "single_file_edit", "model": "qwen2.5-coder-14b-local" },
        { "task_type": "dual_file_wiring", "model": "qwen2.5-coder-14b-local" },
        { "task_type": "analyze_repair", "model": "qwen2.5-coder-14b-local" },
        { "task_type": "test_repair", "model": "qwen2.5-coder-14b-local" },
        { "task_type": "closure_repair", "model": "qwen2.5-coder-32b-local" }
      ]
    }
  }
}
```

`default_model` is used for normal builder runtime tasks. `upgrade_model` is used when the runtime detects repeated failures, large patches, schema drift, validation failures, patch parse failures, scope violations, or semantic conflicts according to `upgrade_threshold`.

## Local Development Example

```bash
mkdir -p ~/.appfactory
cp config/oneappfactory.example.json ~/.appfactory/config.json
./build/oneappfactory-launcher -console -no-browser ~/.appfactory/config.json
```

Open `http://localhost:18800` and use the config page to inspect or patch runtime settings.
