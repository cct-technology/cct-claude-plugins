# Changelog

## Unreleased

### Added

- This repository checks itself: `quality.yaml` at silver, CODEOWNERS, and `qk check` on every pull request.
- QK010 (`docs.impact`): when a changed file matches `paths` in `impact_map`, a file matching that entry's `docs` must change too. Enforced from silver.
- `qk docs-impact none --reason --approved-by` records a person's approval that a branch's change needs no docs, in a note and in the exceptions log. A hand-written or later-edited note does not count.
- Exceptions need a person's approval and are logged. `qk waiver add` requires `--approved-by` and appends to `.quality/exceptions-log.jsonl`; a waiver with no matching line does not apply, and QK100 fails it. `qk hook pre-tool-use` makes Claude Code ask a person before Claude records an exception or changes `.quality/` or `quality.yaml`.
- Waivers in `.quality/waivers.yaml`: `qk waiver add <rule-id> --reason --owner --expires --approved-by [--path]` records an agreed, owned, time-limited exception. A waived finding no longer blocks and is listed as waived in every output format; a rule with only waived findings reports `pass`. The JSON report gains `waived` on rule results and in the summary.
- QK100 (`waivers.valid`): every waiver has a reason, an owner, a valid end date and a matching approval in the log, and has not expired. An expired, incomplete or unapproved waiver never applies. A waiver for a rule this `qk` does not have is a warning. QK100 cannot be waived.
- `qk check` finds the repository root from any subfolder and computes what changed: files changed between the merge-base with the default branch (`origin/HEAD`, then `main`, then `master`) and the working tree, including uncommitted and untracked files. Outside git, checks that need this are skipped with the reason.
- `qk check --base <ref>` compares against the merge-base with another ref; a ref that names no commit exits 2. `qk check --changed` limits whole-repo checks to changed files.
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
