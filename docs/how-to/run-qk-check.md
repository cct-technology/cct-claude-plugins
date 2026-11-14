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

## Run the slow checks

```sh
qk check --full
```

This also runs the slow checks: QK020 runs every command listed under `generated`, and QK060 asks the package registries about [new and upgraded dependencies](check-dependencies.md). They run without `--full` when the `CI` environment variable is `true`, which most CI services set. Otherwise they are skipped with that reason. The hooks never run them.

> [!WARNING]
> Do not edit files while `qk check --full` runs, and do not let Claude edit them.
>
> - Afterwards QK020 puts every file listed under `generated` back as it was before the run, so an edit to one of those files during the run is lost.
> - Changes to any other file are kept. qk cannot tell them from a change the generator made, so QK020 leaves the file as it is and warns, naming it.
> - If such a file had uncommitted changes before the run, the warning gives a `git cat-file -p` command that prints what the file held then. Recover it soon: git may delete that copy after about two weeks.

## Read the result

- `--format text` (default) lists every check.
- `--format json` is for tools.
- `--format claude` lists only what needs acting on, then a `Skipped:` line.
- `--format sarif` is SARIF 2.1.0, for GitHub code scanning and other SARIF readers. See [run Quality Kit in GitHub Actions](run-quality-kit-in-github-actions.md).

To get SARIF and another format from one run, add `--sarif <file>`. The SARIF goes to the file, and `--format` still decides what is printed:

```sh
qk check --format claude --sarif qk.sarif
```

Write the file outside the repository, or ignore it in git: a new file in the repository counts as a change in the next run.

Exit code `0`: nothing blocks. `1`: an enforced check failed. `2`: usage error.

## See a short summary

```sh
qk context
```

This prints the maturity, which checks are enforced or advisory, open findings per check, and waivers ending within 14 days, in at most 30 lines. It is what Claude sees at the start of a session. It exits `0` even when an enforced check fails, and it leaves out slow checks (QK020, QK060) and names them, so run `qk check` before relying on them.

Without a `quality.yaml` it prints one line saying checks run advisory with defaults.
