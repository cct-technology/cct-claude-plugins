# Run qk check

`qk check` runs every check and reports what is missing or stale. Run it from anywhere inside the repository: it finds the repository root and reads `quality.yaml` there.

## Choose what to compare against

Checks that look at "what changed" compare the working tree with a base: the merge-base of `HEAD` and the default branch. Uncommitted and untracked files count; ignored files do not.

The default branch is `origin/HEAD` when it is set, otherwise `main`, otherwise `master`. On the default branch itself, `origin/HEAD` makes commits you have not pushed count as changes.

To compare against something else, name it. `qk` still uses the merge-base, as a pull request does:

```sh
qk check --base origin/release-2
```

A `--base` that names no commit is a usage error (exit 2). Outside a git repository, or with no default branch, checks that need a base are skipped with the reason.

In CI, fetch enough history for the merge-base. With `actions/checkout`, set `fetch-depth: 0`.

## Limit whole-repo checks to changed files

```sh
qk check --changed
```

Checks that scan the whole repository then look only at the files changed against the base. The Stop hook runs `qk check --changed`.

## Read the result

- `--format text` (default) lists every check.
- `--format json` is for tools.
- `--format claude` lists only what needs acting on, then a `Skipped:` line.
- `--format sarif` is SARIF 2.1.0, for GitHub code scanning and other SARIF readers. See [run Quality Kit in GitHub Actions](run-quality-kit-in-github-actions.md).

Exit code `0`: nothing blocks. `1`: an enforced check failed. `2`: usage error.

## See a short summary

```sh
qk context
```

This prints the maturity, which checks are enforced or advisory, open findings per check, and waivers ending within 14 days, in at most 30 lines. It is what Claude sees at the start of a session. It exits `0` even when an enforced check fails, and it leaves out slow checks (QK020, QK060) and names them, so run `qk check` before relying on them.

Without a `quality.yaml` it prints one line saying checks run advisory with defaults.
