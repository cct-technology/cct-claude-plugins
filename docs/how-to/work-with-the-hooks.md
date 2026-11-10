# Work with Quality Kit's hooks

With the plugin enabled, three hooks run `qk` through `bin/qk` in every session. You do not call them yourself.

| When | What happens |
| --- | --- |
| A session starts, resumes, or is cleared or compacted | `qk context` is added to Claude's context: maturity, which checks are enforced, open findings, and waivers ending soon. |
| Claude runs a shell command or edits a file | If it records an exception (`qk waiver add`, `qk docs-impact`) or touches `.quality/` or `quality.yaml`, Claude Code asks a person first. |
| Claude tries to finish | `qk check --changed` runs. Enforced failures send Claude back with the findings. Advisory findings are shown to you, and Claude stops. |

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
