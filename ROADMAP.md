# OneAppFactory Roadmap

OneAppFactory is being narrowed into an Android app factory. The roadmap follows that product boundary instead of the older general assistant direction.

## Near Term

- Finish documentation cleanup so public docs only describe the AppFactory product surface.
- Keep `/jobs` as the primary web experience and remove stale configuration/i18n keys from old assistant pages.
- Complete the R8 live regression matrix from the extraction plan:
  - generic boolean/summary
  - numeric/summary
  - list-detail-form
  - no-home/no-detail negative topology
  - schema-driven widget test coverage
- Make regression output easier to scan by keeping concise summaries, stable latest logs, and direct artifact paths.

## Builder Runtime

- Reduce unnecessary repair rounds and keep successful runs close to deterministic generation.
- Improve fallback behavior for empty or malformed model patches.
- Keep the runtime contract aligned with the JSON schemas in `docs/design/schemas`.
- Expand template governance checks as template complexity grows.

## Templates

- Maintain `flutter-open-lite` and `flutter-finance-lite` as governed starter templates.
- Grow generic app coverage through the policy contract and runtime rule inventory.
- Keep generated Flutter projects inspectable and easy to validate with standard Flutter commands.

## Web Launcher

- Keep the launcher focused on jobs, configuration, and logs.
- Avoid reintroducing legacy non-AppFactory product pages.
- Improve live job status, failure diagnosis, and workspace artifact navigation.

## Release And Operations

- Keep release artifacts named `oneappfactory` and `oneappfactory-launcher`.
- Keep Docker surfaces limited to the launcher service and optional AppFactory builder image.
- Keep configuration defaults under `~/.appfactory` and `ONEAPPFACTORY_*` environment variables.
