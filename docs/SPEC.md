# Quality Kit — Claude Code plugin

Status: ready to build · Owner: CTO · Version of this spec: 1.0 (2026-11-07)

## 1. Purpose

AI agents now write most of our code quickly and well. The weak spots are the things around the code that developers have always been poor at: documentation, training material, data contracts, security assessment, human test plans, operational readiness and user feedback. If agents inherit our habits, they repeat the gaps at much higher volume.

Quality Kit is a Claude Code plugin that makes those surrounding artifacts part of the normal flow of work:

- **Skills** make Claude good at writing each artifact.
- **Hooks** make Claude keep them current before it can finish a task.
- **A deterministic CLI (`qk`)** decides what is missing or stale, and runs identically in hooks, pre-push and CI.

### Design rules (non-negotiable)

1. **Standalone.** Works in any repository with zero configuration. No dependency on the governance repo, the engineering baseline, or any other CCT system. Those can adopt it later.
2. **Progressive.** Three modes: `off`, `advisory` (default: warns, never blocks), `enforced` (blocks). A repo opts in to `enforced` per check.
3. **Deterministic first, AI second.** `qk` decides *whether* something is missing or changed. Skills decide *what it means* and draft the fix. The model never decides pass/fail.
4. **Never self-grading.** Verification skills run in a fresh context (subagent) and never in the session that wrote the code.
5. **Generate, don't write, reference material.** Settings, API, event and schema references are generated from code. AI may write tutorials, how-tos and explanations, never reference facts.
6. **Short artifacts.** Every template has a length budget. A threat model nobody reads is worse than a short one people act on.
7. **Fail open on tooling problems.** If `qk` is missing or crashes, hooks warn and exit 0. Only real findings block.
8. **Host-neutral.** `qk` depends only on git. Anything specific to a git host (CI workflows, pull request comments, code scanning upload, merge gates) is an optional add-on for that host. Only the host can block a merge, so Quality Kit documents that setting per host and never promises it.

## 2. Repository layout

The repo is a plugin marketplace that initially contains one plugin. More plugins can be added later.

```
cct-claude-plugins/
  .claude-plugin/marketplace.json
  plugins/quality-kit/
    .claude-plugin/plugin.json
    skills/<skill-name>/SKILL.md (+ templates/, references/, scripts/)
    agents/quality-reviewer.md
    hooks/hooks.json
    bin/qk                     # shim: finds or fetches the qk binary
    templates/                 # shared artifact templates
    schemas/quality.schema.json
  cmd/qk/                      # Go CLI source
  internal/...                 # Go packages for qk
  .github/workflows/           # CI for the plugin + reusable workflow for consumer repos
  docs/                        # Diátaxis docs for the plugin itself (dogfood)
  evals/                       # skill evals
```

Before writing manifests, hooks or skills, Claude Code must read the current official docs (plugins reference, hooks reference, skills) and follow them where they differ from this spec. Use the `claude-code-guide` agent if available.

## 3. The manifest: `quality.yaml` (optional)

Without a manifest, `qk` auto-detects the stack and runs in `advisory` mode with defaults. With a manifest, the repo declares what it owes.

```yaml
version: 1
service: isms-operations
owners: { product: rk, tech: <github-handle> }
data_class: internal            # internal | client | public
maturity: bronze                # bronze | silver | gold (sets default modes)
modes:                          # per-check override: off | advisory | enforced
  docs.impact: enforced
  contracts.breaking: enforced
docs:
  root: docs
  diataxis: true
  roles: [end-user, admin, developer]
  impact_map:                   # changed paths -> docs that must change too
    - paths: ["internal/config/**"]
      docs: ["docs/reference/settings.md"]
    - paths: ["web/app/(admin)/**"]
      docs: ["docs/how-to/admin/**"]
generated:                      # files that must equal generator output
  - file: docs/reference/settings.md
    cmd: go run ./cmd/gen-settings-doc
contracts:
  openapi: [api/openapi.yaml]
  asyncapi: [api/asyncapi.yaml]
  odcs: [contracts/*.odcs.yaml]
security:
  threat_models: docs/security/threat-models
  specs_dir: docs/specs         # a new spec here requires a threat model
testing:
  uat_dir: docs/testing/uat
  replaces: [<old tool>]        # non-empty -> parity matrix required
ops:
  runbooks: docs/how-to/operations
privacy:
  inventory: docs/privacy/data-inventory.yaml
feedback:
  linear_team: <TEAM-KEY>
```

`schemas/quality.schema.json` validates it. The schema is versioned; `qk` reads older versions and suggests upgrades.

### Maturity levels

| Level | Meant for | Default mode |
| --- | --- | --- |
| bronze | Legacy and adopted repos | All checks advisory except manifest validity |
| silver | New services | Docs impact, contracts, waivers, generated files, supply chain enforced |
| gold | Client-facing or regulated services | All checks enforced |

## 4. The `qk` CLI (Go)

### Commands

| Command | Purpose |
| --- | --- |
| `qk init` | Non-interactive scaffold used by the `quality-init` skill: writes manifest, folders, templates |
| `qk check [--changed] [--base <ref>] [--format text|json|claude|sarif]` | Runs all checks. `--changed` limits to the diff against base (default: merge-base with default branch) |
| `qk context` | Short summary for session start: mode, maturity, open findings, active waivers expiring soon |
| `qk explain <rule-id>` | What the rule checks, why, and how to fix |
| `qk waiver add <rule-id> --reason --expires --owner --approved-by` | Writes `.quality/waivers.yaml` and appends the approval to `.quality/exceptions-log.jsonl` |
| `qk docs-impact none --reason <reason> --approved-by <person>` | Writes `.quality/docs-impact/<branch>.md`, a "no docs needed" note that QK010 accepts before anything is committed, and appends the approval to `.quality/exceptions-log.jsonl` |
| `qk version` | Version + manifest schema version |

### Checks (rule IDs are stable; never reuse an ID)

| ID | Check | Phase |
| --- | --- | --- |
| QK001 | Manifest valid against schema | 1 |
| QK010 | Docs impact: changed paths in `impact_map` require changes in mapped docs, or a waiver, or an approved docs-impact note (from `qk docs-impact none`) in the change | 1 |
| QK011 | Diátaxis structure present when `diataxis: true`; each doc has frontmatter `owner` and `review_by` | 2 |
| QK012 | Docs past `review_by` | 2 |
| QK013 | Broken internal links and missing images in docs | 2 |
| QK020 | Generated files equal generator output (CI and `--full` only; slow) | 1 |
| QK030 | OpenAPI breaking changes vs base (uses `oasdiff` if installed) | 1 |
| QK031 | AsyncAPI breaking changes vs base | 2 |
| QK032 | ODCS contracts lint (uses `datacontract` CLI if installed) | 2 |
| QK033 | Contract changed without a behavioural-change note in the PR or `CHANGES` entry | 2 |
| QK040 | New spec in `specs_dir` without a threat model | 1 |
| QK041 | Threat model older than the latest change to its declared boundaries (new endpoint, store, external call) | 3 |
| QK050 | Release has no UAT plan, or UAT plan has unexecuted steps at release tag | 2 |
| QK051 | `replaces` set but no parity matrix, or matrix has `missing`/`worse` rows without a decision | 2 |
| QK060 | Supply chain: newly added dependencies exist in the registry, are older than N days (default 14), not deprecated, licence on allowlist | 1 |
| QK061 | Lockfile present and consistent | 1 |
| QK070 | Privacy: new DB columns, log fields or API fields that look like personal data are listed in the data inventory | 3 |
| QK080 | Ops readiness: every alert rule references a runbook that exists | 3 |
| QK081 | DB migration safety: destructive operations (drop/rename column, type narrowing) without an expand/contract plan or waiver | 2 |
| QK090 | Release notes: user-visible change without a changelog entry | 2 |
| QK100 | Waivers valid, owned, not expired, and approved in the exceptions log | 1 |
| QK110 | Ownership: CODEOWNERS present, in the locations the repo's host reads, and covers all top-level paths. `skipped` on a host without CODEOWNERS | 1 |

Every check returns `pass | warn | fail | skipped(reason)`. `skipped` is never a failure (fail open).

### Implementation notes

- Go, single static binary, no CGO. Released with GoReleaser for darwin/linux/windows, amd64/arm64.
- External tools (`oasdiff`, `datacontract`, AsyncAPI diff) are optional: detect, run if present, `skipped` with install hint if not.
- Package registry lookups for QK060: npm, Go proxy, Packagist, crates.io, PyPI. Cache results for 24h.
- `--format claude` prints a compact, actionable list written for an agent: rule, file, what to do.
- `--format sarif` is a standard format; GitHub code scanning reads it.
- Unit tests per rule with fixture repos under `testdata/`.

### The `bin/qk` shim

Plugins are copied to a cache on install, so the shim resolves the binary in this order: `qk` on `PATH` at the required version → `~/.cache/quality-kit/<version>/qk` (`$CLAUDE_PLUGIN_DATA/<version>/qk` when set, because that survives plugin updates) → download the matching release asset with checksum verification → otherwise print a warning to stderr and exit 0.

The required version is `version` in `plugin.json`, the only place it is pinned. The release workflow fails when the tag does not match it. The shim is POSIX `sh`, so on Windows it needs Git Bash.

## 5. Hooks

| Event | Matcher | Behaviour |
| --- | --- | --- |
| `SessionStart` | — | Run `qk context`; inject its output as additional context. Keep under 30 lines. |
| `PreToolUse` | `Edit|Write` | Block edits to files listed under `generated` or carrying the `generated by qk` header; tell Claude which generator to run instead. |
| `PreToolUse` | `Bash\|PowerShell` | When the command installs a package (`npm i`, `pnpm add`, `go get`, `composer require`, `cargo add`, `pip install`), run `qk deps verify <name>`; block if the package does not exist or is younger than the threshold. This is the slopsquatting guard. Both shell tools are matched: on Windows Claude Code may run installs through PowerShell, and without Git Bash there is no Bash tool at all. |
| `PreToolUse` | `Bash\|PowerShell\|Edit\|Write\|NotebookEdit` | Run `qk hook pre-tool-use`. When Claude records an exception (`qk waiver add`, `qk docs-impact`) or touches `.quality/` or `quality.yaml`, return `permissionDecision: "ask"` so a person approves. Best effort; the exceptions log catches hand edits, and review of those paths at merge, where the host enforces it, is the backstop. |
| `Stop` | — | Run `qk check --changed --format claude`. In `enforced` mode with failures: return a block decision with the findings as the reason. In `advisory` mode: surface findings as a message and allow stop. |

Stop-hook loop protection: if the hook input says a stop hook is already active, allow at most 2 further blocks per session (track in a temp file keyed by session ID), then allow stop and print "Quality Kit: unresolved findings, handing over to a human".

## 6. Skills

Each skill: `SKILL.md` with frontmatter `name` and a precise `description` (when to use it, when not), a short procedure, and templates in the skill folder. Skills call `qk` for facts and never invent them.

### Phase 1 (MVP)

| Skill | When | What it does | Output |
| --- | --- | --- | --- |
| `quality-init` | New repo, or user asks to set up Quality Kit | Detects stack, asks at most 3 questions (data class, maturity, replaced tools), runs `qk init`, fills templates, adds CI workflow and CODEOWNERS stub | Manifest + skeleton + CI |
| `docs-sync` | Stop hook reports QK010/QK020, or user asks to update docs | Reads the diff and `impact_map`, updates how-tos and explanations, runs generators for reference docs, marks tutorials that need re-running | Doc changes in the same branch |
| `contract-guard` | API, event or dataset shape changed | Updates OpenAPI/AsyncAPI/ODCS, runs diffs, writes the behavioural-change note, drafts a waiver only if the user confirms a break is intended | Contract + note |
| `threat-model` | New spec, new boundary, or user asks | STRIDE per trust boundary; adds OWASP Top 10 for LLM apps when the feature uses an LLM; each accepted mitigation becomes a task (Linear issue if the Linear MCP is connected, otherwise a checklist); tags ISO 27001 Annex A controls | `docs/security/threat-models/<feature>.md`, max ~2 pages |
| `test-plan` | Epic or release is ready for human testing | UAT steps per role, exploratory charters, security acceptance checks (wrong role, other tenant, expired session), evidence columns | `docs/testing/uat/<release>.md` |

### Phase 2

| Skill | What it does |
| --- | --- |
| `parity-matrix` | Builds the capability list from the replaced tool (its docs, exports, screenshots supplied by the user), then the parity and improvement matrix. Rows: capability, today, new, verdict (parity / improved / missing / dropped / worse), improvement target, evidence |
| `training-pack` | Role-based training from docs: learning path, exercises, short quiz, Doc Detective or Playwright specs so tutorials are executed in CI |
| `release-notes` | Writes user-facing release notes and an internal changelog from merged changes and Linear issues; separates "what users notice" from "what admins must do" |
| `migration-safety` | Reviews DB migrations for expand/contract, backfill, lock risk, rollback path |
| `ops-readiness` | Checks logs, metrics, traces, alerts and dashboards for a new feature; writes runbooks for each alert; defines rollback and feature-flag plan |

### Phase 3

| Skill | What it does |
| --- | --- |
| `privacy-review` | Maintains the personal-data inventory (what, why, legal basis, retention, where stored, sub-processors); flags new PII in logs; triggers a DPIA note when thresholds are met. Feeds the ISMS and the GDPR records of processing |
| `a11y-review` | Checks UI changes against WCAG 2.2 AA (keyboard, contrast, labels, focus, screen-reader names); adds axe-core checks to Playwright tests |
| `ai-feature-review` | For features that use an LLM: eval set, prompt/version record, model-change log, prompt-injection tests, data-residency check. Supports ISO 42001 evidence |
| `quality-adopt` | For legacy repos (PHP, Vue, Rust): inventories what exists, reverse-engineers reference docs and contracts, opens a gap backlog, sets maturity to bronze |
| `quality-retro` | Monthly: reads escaped defects and user feedback, finds which check would have caught each, proposes new rules or template changes as a PR to this repo |

### Agent

`agents/quality-reviewer.md`: a reviewer subagent with read-only tools. Given a branch, it checks the artifacts against the code with fresh context and returns findings. Used by `release-readiness` (phase 2) and on request. It never edits files.

## 7. CI

`qk check` is the same everywhere; CI is a thin add-on per host. GitHub comes first, then GitLab.

For GitHub, provide a reusable GitHub Actions workflow `quality.yml` that consumer repos call in one line. It installs `qk` (and optional tools), runs `qk check --base origin/<default> --format sarif`, uploads SARIF, and posts a short PR comment summarising findings. Mode and maturity come from the manifest.

## 8. Distribution

- Phase 1: teams add the marketplace per repo (`.claude/settings.json` with `extraKnownMarketplaces` and `enabledPlugins`), pinned to a release tag.
- Later: org-wide through managed settings with an allowlist. That step is independent of this repo.
- Semantic versioning. Breaking changes to `quality.yaml` bump the schema version and ship a `qk migrate` command.

## 9. Testing the plugin

- Go unit tests per rule with fixture repos (TDD: write the fixture and failing test first).
- Hook scripts tested by piping recorded hook input JSON into them.
- Skill evals in `evals/`: for each skill, 3–5 scenario repos with expected outputs; use `claude plugin eval` if available in the installed Claude Code version.
- Dogfood: this repo has its own `quality.yaml` at `silver`.

## 10. Phases and acceptance criteria

### Phase 1 — usable in one repo
- [ ] `qk` implements QK001, QK010, QK020, QK030, QK040, QK060, QK061, QK100, QK110 with tests.
- [ ] Shim resolves or downloads the binary and fails open.
- [ ] Hooks: SessionStart, both PreToolUse hooks, Stop with loop protection.
- [ ] Five MVP skills with templates.
- [ ] Reusable CI workflow with SARIF output.
- [ ] Installed and working in one pilot repo (ISMS-operations or `linear-intake`), advisory mode.
- [ ] Docs for the plugin itself: tutorial (install and first run), how-tos per skill, reference for every rule and manifest key (generated from the schema and rule registry).

### Phase 2 — enforce on new builds
- [ ] Remaining phase 2 rules and skills.
- [ ] `release-readiness` using the reviewer agent.
- [ ] Pilot repo moved to `silver` with no unwaived failures.

### Phase 3 — breadth
- [ ] Phase 3 rules and skills.
- [ ] `quality-adopt` run on one legacy repo.
- [ ] First `quality-retro` PR merged.

## 11. Open decisions

- Plugin name and repo name (working: Quality Kit, `cct-claude-plugins`).
- Default dependency age threshold (proposed 14 days) and licence allowlist.
- Whether threat-model mitigations go to Linear automatically or need confirmation (proposed: confirmation).
- Docs publishing (repo only vs a docs site).
