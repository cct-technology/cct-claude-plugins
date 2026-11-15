# Why Quality Kit fails open

When Quality Kit itself has a problem, it warns and lets the work go on. Only a real finding blocks. This page says what that means and what it costs.

## The rule

A check that cannot run is not a failed check. If `qk` is missing, crashes or times out, nobody is blocked.

The reason is trust. A tool that stops your work because its own download failed gets turned off, and a tool that is turned off checks nothing. Quality Kit would sooner miss a finding for a day than teach a team to remove it.

## Where it applies

| What goes wrong | What happens |
| --- | --- |
| `qk` cannot be found or downloaded | The hook warns and exits 0. When a session starts, you also see one message saying `qk` did not run. |
| The hook that reads Claude's edits and installs fails, or runs out of time | The edit or the install goes through unchecked. QK020 and QK060 check the same things in CI. |
| A hook fails in any other way | The same: a warning, exit 0. A `qk hook` command never exits with an error, because Claude Code reads one as a block. |
| A check crashes | That check is `skipped` with `rule crashed: ...`. The other checks run. |
| A tool a check needs is not installed, such as `oasdiff` | The check is `skipped` and says how to install the tool. |
| A package registry cannot be reached | QK060 is `skipped` and names the package it could not ask about. |
| There is no git repository, or no base to compare with | Checks that need one are `skipped` with the reason. |
| `quality.yaml` is missing | Every check runs advisory with defaults. Nothing blocks. |
| In CI, `qk` cannot be downloaded or cannot run | The job warns and passes. The pull request comment says `qk` did not run. |

## Skipped is always shown

Failing open must not look like passing. A check that did not run is reported as `skipped` with its reason, in every output: the text report, the `Skipped:` line Claude gets, the pull request comment, and the SARIF file.

So read the comment, not only the green tick. A job that passed because `qk` did not run says so.

Two things are left unsaid when Claude finishes, so that the message after every turn stays short. The slow checks (QK020, QK060) do not run then and are not named: `qk check --full` and CI run them. And in a repository without `quality.yaml`, the line saying so is left out.

## Where it stops

Three things block even though they look like tooling problems, because each one is a finding:

- **A download of `qk` whose checksum does not match.** It may be a tampered release. `bin/qk` refuses it, and the CI job fails.
- **A generator that runs and fails.** QK020 reports it with its output. A generator that is not installed is skipped.
- **An invalid `quality.yaml`.** QK001 blocks. Otherwise a typing mistake in the file would turn checks off without anyone seeing. The repository is treated as bronze until the file is fixed.

In CI, a `setup` command that fails also fails the job. That is your command, not Quality Kit's.

## Claude is not held for ever

When an enforced check fails, Claude is sent back to fix it. After two blocks in a row, the next stop is allowed and you see:

```text
Quality Kit: unresolved findings, handing over to a human.
```

A finding Claude cannot fix must reach a person. Holding the session in a loop would hide it.

## What this costs

A repository where `qk` never installs gets no checks, and the only sign is a message at the start of each session. Two things cover that gap:

- CI runs the same checks, and its comment says when `qk` did not run.
- Requiring the `quality / qk check` status on the default branch is a setting in your git host. Only the host can block a merge.

See [Work with Quality Kit's hooks](../how-to/work-with-the-hooks.md#when-a-hook-goes-wrong) and [Get the qk binary](../how-to/get-the-qk-binary.md).
