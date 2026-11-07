# Changelog

## Unreleased

### Added

- `qk check --format text|json|claude` runs every registered rule and reports `pass`, `warn`, `fail` or `skipped`. Exit code 1 means an enforced rule failed; 2 means a usage error.
- Modes: `maturity` sets each rule's default mode; `modes` overrides it per check key. Advisory failures are reported as warnings. A rule that crashes is reported as skipped (fail open).
- `qk explain <rule-id>` prints what a rule checks and how to fix it.
- Generated rule reference at `docs/reference/rules.md` (`go run ./cmd/gen-docs`); CI fails when it is out of date.
- `qk version` prints the qk version and the `quality.yaml` schema version it reads.
- Marketplace `cct-claude-plugins` with the `quality-kit` plugin (no components yet).
- CI runs `go vet` and `go test` on Linux, macOS and Windows.
