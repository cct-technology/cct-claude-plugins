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
| Runs a shell command that names one of them, unless it is a plain read | Yes |
| Runs a plain read of one of them | No |

### What a plain read is

A shell line that names `.quality/` or `quality.yaml` is a plain read when all of this holds:

- **Every command that gets the name only reads.** A command gets the name as an argument, from a `$( )` inside it, or down a pipe. These only read: `cat`, `head`, `tail`, `grep`, `rg`, `ls`, `find`, `wc`, `diff`, `jq`, `test`, `echo`, and in PowerShell `Get-Content`, `Select-String`, `Test-Path`, `Get-Item` and `Get-ChildItem`. So do `git diff`, `log`, `show`, `status` and `blame`. `git add` and `git commit` record a file without changing it, so they count too.
- **No output goes to the path**, or to a target that is not written out, such as `> $OUT` or a pattern.
- **Nothing on the line can change what a command means, or carry the name to another command**: no `NAME=value`, `alias`, `source`, `eval`, function or `read`, and no variable in a command that is not a reader. In PowerShell: no `Set-Alias`, `Import-Module` or the like.

Other commands on the line are left alone.

So these are not asked about:

```sh
cat quality.yaml
test -f quality.yaml && qk init --maturity silver
if [ -f quality.yaml ]; then cat quality.yaml; fi
git diff main -- quality.yaml
cat .quality/waivers.yaml | grep rule | head -20
cat "$HOME/repo/quality.yaml"
cat quality.yaml && make docs
```

And these are:

```sh
sed -i 's/silver/bronze/' quality.yaml    # changes it
cat new.yaml > quality.yaml               # output goes to it
echo quality.yaml | xargs rm              # the name goes down a pipe to a command that is no reader
cat quality.yaml; rm $_                   # $_ is the last word of the command before
sed -n '1,5p' quality.yaml                # a read, but sed can also write
```

The last one is a read, and you are asked all the same: the hook does not work out what a `sed` script does.

### How the hook reads a line

- **Bash lines are parsed**, with the parser of [`shfmt`](https://github.com/mvdan/sh), so quotes, escapes, comments, here-documents and substitutions are read as Bash reads them. A line the parser cannot read is asked about. The Bash tool may run zsh, as on macOS: the line is still read as Bash.
- **PowerShell lines are not parsed.** A PowerShell line is a plain read only when it also holds nothing but commands, quotes, pipes and redirects. One with `$`, brackets, braces, a backtick, `#`, `<` or `@` is asked about, so `Get-Content $path` asks.

The hook runs nothing. It can miss a write when the name is not written on the line: a file name in a variable from an earlier command, a pattern such as `*.yaml`, a script that does the writing, or a file that carries the name to a later command. Review of `.quality/` and `quality.yaml` at merge is the backstop: see [who approves](waive-a-finding.md#who-approves).

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
