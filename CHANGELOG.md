# Changelog

## Unreleased

### Added

- `quality.yaml` manifest, version 1, validated against `plugins/quality-kit/schemas/quality.schema.json`. `maturity` and `modes` now set each check's mode; with no manifest, every check stays advisory. An invalid manifest is treated as bronze, so QK001 blocks.
- QK001 (`manifest.valid`): `quality.yaml` is valid. Each problem names the key path and what is allowed. A `modes` key that matches no check is a warning, so a manifest written for a newer `qk` does not block an older one.
- Generated `docs/reference/manifest.md` listing every key, and a how-to for configuring `quality.yaml`.
- `qk check --format claude` ends with a `Skipped:` line naming each check that did not run and why, so a check that could not run never reads as "no findings".
- `qk check --format text|json|claude` runs every registered rule and reports `pass`, `warn`, `fail` or `skipped`. Exit code 1 means an enforced rule failed; 2 means a usage error.
- Modes: `maturity` sets each rule's default mode; `modes` overrides it per check key. Advisory failures are reported as warnings. A rule that crashes is reported as skipped (fail open).
- `qk explain <rule-id>` prints what a rule checks and how to fix it.
- Generated rule reference at `docs/reference/rules.md` (`go run ./cmd/gen-docs`); CI fails when it is out of date.
- `qk version` prints the qk version and the `quality.yaml` schema version it reads.
- Marketplace `cct-claude-plugins` with the `quality-kit` plugin (no components yet).
- CI runs `go vet` and `go test` on Linux, macOS and Windows.
