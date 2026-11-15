# Run Quality Kit in GitHub Actions

Pull requests get the same checks Claude gets when it tries to finish, and a comment that stays up to date. `quality.yaml` sets the modes, so the job fails only when an enforced check fails.

## Add the workflow

[`qk init`](set-up-quality-kit-in-a-repository.md) writes this workflow when the `origin` remote is on GitHub. To add it by hand, create `.github/workflows/quality-kit.yml`:

```yaml
name: Quality Kit
on: pull_request
jobs:
  quality:
    uses: cct-technology/cct-claude-plugins/.github/workflows/quality.yml@v0.1.0
    permissions: { contents: read, pull-requests: write }
```

Use the tag of the plugin version your repository runs. The workflow installs the `qk` release with that version and verifies it against the release's `checksums.txt`. The Quality Kit repository is public, so the job needs no token or secret of yours, and a repository of any owner can call the workflow.

## What you get

- **A comment on the pull request**, edited on every push: failures and warnings with how to fix them, waived findings, and checks that did not run. Pull requests from forks get no comment, because their token cannot write.
- **The same text in the job summary.**
- **A `quality-kit-sarif` artifact**: SARIF 2.1.0. Waived findings are results with a suppression that gives the waiver reason. Checks that did not run are notifications.
- **A failed job when an enforced check fails**, when a [`setup`](#set-up-what-the-checks-need) command fails, or when the downloaded `qk` does not match `checksums.txt`, which may mean a tampered release. When `qk` cannot be downloaded or `qk check` cannot run, the job warns and passes, and the comment says why qk did not run. Check the comment as well as the status.

The comment, the artifact and the job result come from one `qk check` run, so they agree, and each command under `generated` runs once.

## Set up what the checks need

The workflow installs only `qk`. Two checks run other tools: QK020 runs every command listed under `generated` in `quality.yaml`, and QK030 runs `oasdiff`. Give the job those tools with `setup`: shell commands that run in the root of your repository before `qk check`.

```yaml
    uses: cct-technology/cct-claude-plugins/.github/workflows/quality.yml@v0.1.0
    with:
      setup: |
        npm ci
```

- **The commands run with `bash -eo pipefail`.** The first one that fails stops the setup and fails the job. The comment then says qk did not run.
- **The job runs on `ubuntu-latest`**, with the toolchains GitHub installs there.
- **To put a tool on `PATH` for `qk check`**, add its folder to `$GITHUB_PATH`.
- **Keep new files out of the checkout**, unless git ignores them: qk counts a new file there as a change. Use `$RUNNER_TEMP`.
- **`setup` cannot use actions** such as `actions/setup-node`. If you need one, run `qk check` in a job of your own.

### Generated files

GitHub sets `CI=true`, so QK020 runs your generators. A generator whose tool is missing is skipped. One that runs and fails, for example because its dependencies are not installed, is a finding, and from `silver` that fails the job. Install what it needs in `setup`.

When `setup` cannot do that, set `generated.current: advisory` under `modes`, and run `qk check --full` in a job of your own that sets the generator up.

### OpenAPI contracts

Without `oasdiff`, QK030 is skipped and the comment says so. To have [breaking API changes](catch-breaking-api-changes.md) block a pull request, install it:

```yaml
    with:
      setup: |
        v=1.35.0
        release="https://github.com/oasdiff/oasdiff/releases/download/v$v"
        mkdir -p "$RUNNER_TEMP/oasdiff" && cd "$RUNNER_TEMP/oasdiff"
        curl -fsSL -O "$release/oasdiff_${v}_linux_amd64.tar.gz" -O "$release/checksums.txt"
        grep " oasdiff_${v}_linux_amd64.tar.gz$" checksums.txt | sha256sum -c -
        tar -xzf "oasdiff_${v}_linux_amd64.tar.gz" oasdiff
        echo "$PWD" >> "$GITHUB_PATH"
```

This repository's own CI does the same.

### New and upgraded dependencies

QK060 needs no setup. It asks the public package registries from the runner about each [dependency a pull request adds or upgrades](check-dependencies.md). If a registry cannot be reached, the check is skipped and the comment says so.

## Show findings in code scanning

```yaml
    uses: cct-technology/cct-claude-plugins/.github/workflows/quality.yml@v0.1.0
    with: { code-scanning: true }
    permissions: { contents: read, pull-requests: write, security-events: write, actions: read }
```

Public repositories have code scanning. Private repositories need GitHub Code Security. If the upload fails, the job still passes and the findings stay in the comment and the artifact.

Findings have no line numbers yet, so an alert points at line 1 of its file. A finding about the repository as a whole points at `quality.yaml`.

## Require a person's review of exceptions

Waivers, docs-impact approvals and the exceptions log live in `.quality/`, and `quality.yaml` decides what blocks. Name both in `CODEOWNERS`:

```
/.quality/      @your-team
/quality.yaml   @your-team
```

Then add a branch ruleset for the default branch (**Settings → Rules → Rulesets**) with:

- **Require a pull request before merging**, with **Require review from Code Owners**.
- **Require status checks to pass**, with `quality / qk check`.

Private repositories on GitHub Free cannot use rulesets or branch protection. There, review of `.quality/` and `quality.yaml` is an agreement, not a rule.
