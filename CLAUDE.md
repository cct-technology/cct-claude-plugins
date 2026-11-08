# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

This repo (`cct-claude-plugins`) is a Claude Code plugin marketplace. Its first plugin, **Quality Kit**, keeps documentation, training material, contracts, threat models, test plans and operational readiness current while AI agents build software. Full spec: `docs/SPEC.md`. Read it before planning any work.

## Non-negotiables
- The plugin must work standalone in any repo with zero configuration. Never add a dependency on other CCT repos or services.
- Host-neutral: `qk` depends only on git. Anything specific to GitHub, GitLab or another host (CI, PR comments, merge gates) is an optional add-on.
- `qk` (Go CLI) decides pass/fail. Skills never decide pass/fail and never invent reference facts.
- Hooks fail open: tooling errors warn and exit 0. Only real findings block, and only in `enforced` mode.
- Exceptions need a person: waivers and docs-impact notes are approved by a human and logged, and changes to `quality.yaml` need a human's approval. Claude never grants one for itself.
- Before writing or changing `plugin.json`, `marketplace.json`, `hooks/hooks.json` or any `SKILL.md`, check the current official Claude Code docs (plugins reference, hooks reference, skills), via the `claude-code-guide` agent when available. The docs win over this file and the spec.
- Verification runs in a fresh context: the `quality-reviewer` agent and verification skills are read-only subagents, never the session that wrote the code.
- SPEC §11 lists open decisions (dependency age threshold, licence allowlist, Linear auto-filing of mitigations). Ask the user before settling any of them in code.

## Architecture
How the pieces connect (details in the SPEC section named):
- One binary, three callers: hooks (§5), skills (§6) and CI (§7) all run the same `qk`. Skills read `qk` output as their facts; CI uses `--format sarif`, hooks use `--format claude`.
- Hooks reach `qk` through the `bin/qk` shim (§4), which resolves PATH → cache → checksummed download, else warns and exits 0.
- `quality.yaml` (§3) is optional. `maturity` sets each check's default mode; `modes` overrides per check. Without a manifest, `qk` auto-detects and runs `advisory`.
- Each rule returns `pass | warn | fail | skipped(reason)`. Optional external tools (`oasdiff`, `datacontract`) are detected; when absent the rule is `skipped` with an install hint.
- The rule registry and `schemas/quality.schema.json` are the source for generated reference docs.
- Go source lives at the repo root (`cmd/`, `internal/`); everything shipped to users lives under `plugins/quality-kit/` (§2). `--plugin-dir` points at the latter.

## Stack
- Go (latest stable), standard library first. No CGO. `cobra` is acceptable for the CLI.
- YAML: `go.yaml.in/yaml/v3` (the maintained successor of the archived `gopkg.in/yaml.v3`). JSON Schema validation: `santhosh-tekuri/jsonschema`.
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
- `go test ./internal/<pkg> -run '^TestName$'` — single test
- `go test ./internal/check -update` — rewrite golden files after an intended output change; review the diff
- Hook scripts: pipe a recorded hook-input JSON file into the script (SPEC §9)
- `go run ./cmd/qk check --format text` — run against this repo (dogfood)
- `go run ./cmd/gen-docs` — regenerate reference docs; CI fails if they differ
- `claude --plugin-dir plugins/quality-kit` — try the plugin locally

## Done means
- Tests pass, generated docs committed, `qk check` passes on this repo at `silver`.
- Docs updated: how-to for any user-visible change, reference regenerated. At `silver`, QK010 is enforced: a commit touching a path in `impact_map` needs the mapped doc changed or a `Docs-Impact: none — <reason>` commit trailer.
- Changelog entry under `## Unreleased` in `CHANGELOG.md` (create it at the repo root if missing).
