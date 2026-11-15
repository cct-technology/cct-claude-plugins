# Quality Kit

Quality Kit is a Claude Code plugin. It keeps the things around the code current while agents write the code: documentation, API contracts, threat models, test plans and ownership.

It has three parts:

- **Checks.** `qk`, a command-line tool, decides what is missing or stale. It gives the same answer in Claude Code, in your terminal and in CI.
- **Hooks.** Claude runs the checks before it can finish a task, and a person approves every exception.
- **Skills.** Claude writes each artifact well: the docs a change owes, a threat model, a UAT plan.

It works in any git repository with no configuration. Without a `quality.yaml`, every check warns and nothing blocks.

> [!NOTE]
> **Status: Phase 1, before the first release.** No `v0.1.0` tag exists yet, so there is no `qk` to download: see [Get qk](#2-get-qk). Everything on this page is built. The plan for later phases is in [`docs/SPEC.md`](docs/SPEC.md#10-phases-and-acceptance-criteria).

## What it does

### Checks

`qk check` runs every check. Each one passes, warns, fails or is skipped with a reason. A check blocks only when it is enforced, and `maturity` in `quality.yaml` decides that: see [modes and maturity](docs/explanation/modes-and-maturity.md).

| Rule | What holds when it passes | Blocks from | How-to |
| --- | --- | --- | --- |
| QK001 | `quality.yaml` is valid | bronze | [Configure quality.yaml](docs/how-to/configure-quality-yaml.md) |
| QK010 | Docs change with the code they describe | silver | [Keep docs in step](docs/how-to/keep-docs-in-step.md) |
| QK020 | Generated files equal their generator's output | silver | [Keep generated docs current](docs/how-to/keep-docs-in-step.md#keep-generated-docs-current) |
| QK030 | No breaking OpenAPI changes (needs `oasdiff`) | silver | [Catch breaking API changes](docs/how-to/catch-breaking-api-changes.md) |
| QK040 | A new spec has a threat model | gold | [Require a threat model](docs/how-to/require-a-threat-model-for-a-new-spec.md) |
| QK060 | New and upgraded dependencies exist in their registry, are at least 14 days old, are maintained and have an allowed licence | silver | [Check new and upgraded dependencies](docs/how-to/check-dependencies.md) |
| QK061 | Dependencies have a lockfile that changes with them | silver | [Keep lockfiles in step](docs/how-to/keep-lockfiles-in-step.md) |
| QK100 | Waivers are valid, owned and not expired | silver | [Waive a finding](docs/how-to/waive-a-finding.md) |
| QK110 | Every top-level path has a code owner | gold | [Give every path a code owner](docs/how-to/give-every-path-a-code-owner.md) |

Every rule in full: [rule reference](docs/reference/rules.md). Every key of `quality.yaml`: [manifest reference](docs/reference/manifest.md). Both are generated from the code.

### Skills

Claude starts a skill when the work calls for it, or you run it by name.

| Skill | Say | What Claude does | How-to |
| --- | --- | --- | --- |
| `quality-init` | "Set up Quality Kit in this repository." | Asks up to three questions, runs `qk init`, fills in `quality.yaml` from what the repository shows | [Set up Quality Kit with Claude](docs/how-to/set-up-quality-kit-with-claude.md) |
| `docs-sync` | "Update the docs for this change." | Runs generators, updates how-tos and explanations, marks tutorials that need a re-run | [Update the docs with Claude](docs/how-to/update-docs-with-claude.md) |
| `contract-guard` | "The order endpoint now returns the currency too. Bring the contract in line." | Updates the OpenAPI, AsyncAPI or ODCS file, has `qk` compare it with the base, writes the note for clients, and asks you when a change breaks them | [Keep contracts true with Claude](docs/how-to/keep-contracts-true-with-claude.md) |
| `threat-model` | "We have a new spec, docs/specs/checkout.md. Write its threat model." | Walks STRIDE at each trust boundary, names the ISO/IEC 27001 Annex A control per mitigation, lists what is still to do | [Write a threat model with Claude](docs/how-to/write-a-threat-model-with-claude.md) |
| `test-plan` | "Release 2.3.0 is ready for people to test. Write the test plan." | Writes the UAT plan people test from: steps per role, exploratory charters, security checks, evidence columns | [Write a UAT plan with Claude](docs/how-to/write-a-uat-plan-with-claude.md) |

By name, a skill is `/quality-kit:<skill>`, for example `/quality-kit:docs-sync`.

A skill never decides pass or fail, and never approves an exception. `qk` decides, and a person approves.

### Hooks

With the plugin enabled, the hooks run in every session. Details: [Work with Quality Kit's hooks](docs/how-to/work-with-the-hooks.md).

| When | What happens |
| --- | --- |
| A session starts | Claude gets a short summary: maturity, which checks are enforced, open findings, waivers ending soon. |
| Claude tries to finish | `qk check --changed` runs. An enforced failure sends Claude back with the findings, at most twice in a row; then a person takes over. Advisory findings are shown to you. |
| Claude records an exception, or could change `.quality/` or `quality.yaml` | Claude Code asks a person first. Reading those files asks nobody. |
| Claude edits a generated file | The edit is blocked, and Claude is told which generator to run. |
| Claude installs a package | The install is blocked when the package does not exist in its registry, or the version is less than 14 days old. |

The two blocks apply where their check is enforced. Where it is advisory, Claude is told and nothing is blocked. A problem with Quality Kit itself never blocks: see [why Quality Kit fails open](docs/explanation/fail-open.md).

### The `qk` command

| Command | What it does | How-to |
| --- | --- | --- |
| `qk init` | Writes `quality.yaml`, the docs folders and, on GitHub, a `CODEOWNERS` stub and the CI workflow. Never overwrites a file. | [Set up Quality Kit in a repository](docs/how-to/set-up-quality-kit-in-a-repository.md) |
| `qk check` | Runs every check. `--changed`, `--base <ref>`, `--full`, `--format text\|json\|claude\|sarif`, `--sarif <file>`. | [Run qk check](docs/how-to/run-qk-check.md) |
| `qk context` | Prints the summary Claude gets when a session starts. | [Run qk check](docs/how-to/run-qk-check.md#see-a-short-summary) |
| `qk explain <rule>` | Says what a rule checks and how to fix a finding. | [Rule reference](docs/reference/rules.md) |
| `qk waiver add <rule>` | Records an agreed, owned, time-limited exception that a person approved. | [Waive a finding](docs/how-to/waive-a-finding.md) |
| `qk docs-impact none` | Records a person's approval that a change needs no docs. | [Keep docs in step](docs/how-to/keep-docs-in-step.md#when-a-change-needs-no-docs) |
| `qk version` | Prints the `qk` version and the `quality.yaml` version it reads. | |
| `qk deps verify <ecosystem> <name>` | Runs the QK060 checks for one package, before you add or upgrade it. | [Check one package](docs/how-to/check-dependencies.md#check-one-package-before-you-add-it) |

### CI

`qk` needs only git, so it runs in any CI. For GitHub there is a reusable workflow: it runs `qk check` on each pull request, keeps one comment up to date, and fails the job only when an enforced check fails. [Run Quality Kit in GitHub Actions](docs/how-to/run-quality-kit-in-github-actions.md).

Only your git host can block a merge. Quality Kit gives the status check; requiring it is a setting in the host.

## Install

You need:

- Claude Code and git.
- On Windows, [Git Bash](https://git-scm.com/downloads/win). The hooks are shell scripts.
- Access to `rkvalvaa/cct-claude-plugins`, which is private, with git credentials Claude Code can use without asking: `gh auth login`, then `gh auth setup-git`.

### 1. Add the plugin to a repository

From the root of the repository:

```sh
claude plugin marketplace add rkvalvaa/cct-claude-plugins#v0.1.0 --scope project
claude plugin install quality-kit@cct-claude-plugins --scope project
```

Until `v0.1.0` is released, use `#main` instead of `#v0.1.0`.

The two commands write `.claude/settings.json`:

```json
{
  "extraKnownMarketplaces": {
    "cct-claude-plugins": {
      "source": {
        "source": "github",
        "repo": "rkvalvaa/cct-claude-plugins",
        "ref": "v0.1.0"
      }
    }
  },
  "enabledPlugins": {
    "quality-kit@cct-claude-plugins": true
  }
}
```

Commit that file. A teammate then trusts the folder when Claude Code asks, and runs the second command once in their clone:

```sh
claude plugin install quality-kit@cct-claude-plugins --scope project
```

### 2. Get qk

The hooks run `qk`. The plugin downloads the right version by itself from a public release. That does not work yet: there is no release, and a download from a private repository needs you to sign in. Until both change, put `qk` on your `PATH` yourself: [Get the qk binary](docs/how-to/get-the-qk-binary.md).

### 3. Set the repository up

Start Claude Code in the repository and say:

```text
Set up Quality Kit in this repository.
```

The [tutorial](docs/tutorials/first-run.md) walks through this on a small repository, up to the first finding Claude fixes.

To try the plugin from a clone without installing it: `claude --plugin-dir plugins/quality-kit`.

To update: `claude plugin update quality-kit@cct-claude-plugins`. To remove: `claude plugin uninstall quality-kit@cct-claude-plugins --scope project`.

## Docs

| | |
| --- | --- |
| **Tutorial** | [Your first run](docs/tutorials/first-run.md): install, set up, see a finding, fix it |
| **How-to** | [Every guide, by goal](docs/how-to/README.md) |
| **Reference** | [Rules](docs/reference/rules.md) · [`quality.yaml`](docs/reference/manifest.md) |
| **Explanation** | [`qk` decides, Claude writes](docs/explanation/deterministic-first.md) · [Why Quality Kit fails open](docs/explanation/fail-open.md) · [Modes and maturity](docs/explanation/modes-and-maturity.md) |

## Work on Quality Kit

The specification is [`docs/SPEC.md`](docs/SPEC.md), and the rules for working in this repository are in [`CLAUDE.md`](CLAUDE.md). `go test ./...` runs the tests. See also [Run the skill evals](docs/how-to/run-the-skill-evals.md) and [Release Quality Kit](docs/how-to/release-quality-kit.md). Changes are listed in [`CHANGELOG.md`](CHANGELOG.md).
