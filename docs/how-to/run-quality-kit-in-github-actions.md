# Run Quality Kit in GitHub Actions

Pull requests get the same checks Claude gets when it tries to finish, and a comment that stays up to date. `quality.yaml` sets the modes, so the job fails only when an enforced check fails.

## Add the workflow

Add `.github/workflows/quality-kit.yml` to your repository:

```yaml
name: Quality Kit
on: pull_request
jobs:
  quality:
    uses: rkvalvaa/cct-claude-plugins/.github/workflows/quality.yml@v0.1.0
    permissions: { contents: read, pull-requests: write }
    secrets: { QK_RELEASE_TOKEN: "${{ secrets.QK_RELEASE_TOKEN }}" }
```

Use the tag of the plugin version your repository runs. The workflow installs the `qk` release with that version and verifies it against the release's `checksums.txt`.

## While the Quality Kit repository is private

1. In `rkvalvaa/cct-claude-plugins`, go to **Settings → Actions → General → Access** and choose **Accessible from repositories owned by 'rkvalvaa' user**. Only private repositories with the same owner can then call the workflow.
2. Create a fine-grained personal access token with read access to **Contents** of `rkvalvaa/cct-claude-plugins`.
3. In your repository, add it under **Settings → Secrets and variables → Actions** as `QK_RELEASE_TOKEN`.

Without the token, the job warns `could not download qk_...` and passes, and the comment says qk did not run.

## What you get

- **A comment on the pull request**, edited on every push: failures and warnings with how to fix them, waived findings, and checks that did not run. Pull requests from forks get no comment, because their token cannot write.
- **The same text in the job summary.**
- **A `quality-kit-sarif` artifact**: SARIF 2.1.0. Waived findings are results with a suppression that gives the waiver reason. Checks that did not run are notifications.
- **A failed job when an enforced check fails**, or when the downloaded `qk` does not match `checksums.txt`, which may mean a tampered release. When `qk` cannot be downloaded or `qk check` cannot run, the job warns and passes, and the comment says why qk did not run. Check the comment as well as the status.

## Show findings in code scanning

```yaml
    uses: rkvalvaa/cct-claude-plugins/.github/workflows/quality.yml@v0.1.0
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
