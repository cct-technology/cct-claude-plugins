# CLAUDE.md — cct-claude-plugins (Quality Kit)

This repo is a Claude Code plugin marketplace. Its first plugin, **Quality Kit**, keeps documentation, training material, contracts, threat models, test plans and operational readiness current while AI agents build software. Full spec: `docs/SPEC.md`. Read it before planning any work.

## Non-negotiables
- The plugin must work standalone in any repo with zero configuration. Never add a dependency on other CCT repos or services.
- `qk` (Go CLI) decides pass/fail. Skills never decide pass/fail and never invent reference facts.
- Hooks fail open: tooling errors warn and exit 0. Only real findings block, and only in `enforced` mode.
- Before writing or changing `plugin.json`, `marketplace.json`, `hooks/hooks.json` or any `SKILL.md`, check the current official Claude Code docs (plugins reference, hooks reference, skills). The docs win over this file and the spec.

## Stack
- Go (latest stable), standard library first. No CGO. `cobra` is acceptable for the CLI.
- YAML: `gopkg.in/yaml.v3`. JSON Schema validation: `santhosh-tekuri/jsonschema`.
- Releases: GoReleaser, checksums published.
- Shell only for the thin `bin/qk` shim and hook wrappers; keep logic in Go.

## Workflow
- Work comes from Linear issues. Each issue references a section of `docs/SPEC.md`.
- TDD: write the fixture repo under `testdata/` and a failing test before implementing a rule.
- Rule IDs (`QKnnn`) are stable. Never renumber or reuse.
- Every new rule needs: implementation, tests, an entry in the rule registry, and generated reference docs (`go run ./cmd/gen-docs`).
- Every skill needs: `SKILL.md` with a precise `description`, templates in its folder, and at least 3 eval scenarios in `evals/`.
- Keep `SKILL.md` files short: procedure and pointers. Put long material in `references/` inside the skill.

## Commands
- `go test ./...` — all tests
- `go run ./cmd/qk check --format text` — run against this repo (dogfood)
- `go run ./cmd/gen-docs` — regenerate reference docs; CI fails if they differ
- `claude --plugin-dir plugins/quality-kit` — try the plugin locally

## Done means
- Tests pass, generated docs committed, `qk check` passes on this repo at `silver`.
- Docs updated: how-to for any user-visible change, reference regenerated.
- Changelog entry under `## Unreleased`.
