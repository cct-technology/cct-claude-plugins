# Work with Quality Kit's hooks

With the plugin enabled, three hooks run `qk` through `bin/qk` in every session. You do not call them yourself.

| When | What happens |
| --- | --- |
| A session starts, resumes, or is cleared or compacted | `qk context` is added to Claude's context: maturity, which checks are enforced, open findings, and waivers ending soon. |
| Claude runs a shell command or edits a file | If it records an exception (`qk waiver add`, `qk docs-impact`) or could change `.quality/` or `quality.yaml`, Claude Code asks a person first. Reading them asks nobody. |
| Claude tries to finish | `qk check --changed` runs. Enforced failures send Claude back with the findings. Advisory findings are shown to you, and Claude stops. |

## When you are asked to approve

The prompt means Claude is about to record an exception, or to change `.quality/` or `quality.yaml`, where a check can be turned off. Read what it wants to change before you approve.

| Claude does this | Asked? |
| --- | --- |
| Runs `qk waiver add` or `qk docs-impact` | Yes |
| Edits or writes a file under `.quality/`, or `quality.yaml` | Yes |
| Runs a shell command that can change one of them: a redirect into it, `sed -i`, `rm`, `mv`, `cp`, `tee`, `git checkout`, `Set-Content` | Yes |
| Runs a shell command the hook does not know, with one of them named in it: a script, `python -c`, `sh -c`, `xargs` | Yes |
| Runs a command that can write or start a program through an option or its script, with one of them named in it: `sed`, `awk`, `sort`, `yq`, `less`, `git -c ...`, a variable set in front as in `PAGER=x git log` | Yes |
| Reads one of them with a command that can only read: `cat`, `head`, `grep`, `ls`, `test -f`, `diff`, `git diff`, `git log`, `Get-Content` | No |
| Records one in git: `git add`, `git commit` | No |

A line with several commands is asked about when any command in it could change a protected path. So `test -f quality.yaml && qk init` is not asked about, and `cat quality.yaml; rm quality.yaml` is.

The hook reads the text of the command; it does not run a shell. When it cannot tell a read from a write, it asks. It can still miss a write, for example when a file name sits in a variable from an earlier command. Review of `.quality/` and `quality.yaml` at merge is the backstop: see [who approves](waive-a-finding.md#who-approves).

## When Claude is sent back

Claude gets the findings and tries to fix them. Claude Code labels this "Stop hook error occurred"; that is the block working, not a fault. If an enforced check still fails after Claude has been sent back twice in a row, the next stop is allowed and you see:

```
Quality Kit: unresolved findings, handing over to a human.
```

followed by the findings. Then either fix them yourself, or record an exception you agree with:

- docs that need no change: [keep docs in step](keep-docs-in-step.md)
- any other finding: [waive a finding](waive-a-finding.md)

Claude must not approve its own exception. When it tries, Claude Code asks you.

## When a check could not run

Stop shows a `Skipped:` line for a check that did not run, for example `rule crashed: ...`, so a broken check never looks like a pass. In a repository without `quality.yaml` it leaves out the expected `QK001 (no quality.yaml ...)` line, which would otherwise appear after every turn.

Stop never runs the slow checks (QK020, QK060), and says nothing about them. Run `qk check --full` before you push; CI runs them too.

## When a hook goes wrong

Hooks never block because of a problem with Quality Kit itself. If `qk` is missing, crashes or times out, the hook warns and Claude carries on. Only real findings block.

Claude Code hides warnings from a hook that succeeds, so when `qk` cannot be found or downloaded, the session-start hook also shows you one message, for example:

```
quality-kit: could not download qk_0.1.0_windows_amd64.tar.gz from https://github.com/.... qk did not run.
```

No check runs in that session until `qk` is installed. See [get the qk binary](get-the-qk-binary.md).

On Windows the hooks need [Git Bash](https://git-scm.com/downloads/win). Without it, Claude Code runs hooks in PowerShell, each hook fails with `The term 'sh' is not recognized`, and no check runs.

## Use a qk you installed yourself

A `qk` on `PATH` is used only when `qk version` matches the plugin's `version`. Builds between releases share that version, so build from a commit that has every hook the plugin calls:

```sh
qk hook session-start </dev/null
```

prints a line of JSON. An older `qk` prints its help or a warning instead.
