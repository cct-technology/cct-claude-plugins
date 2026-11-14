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

- **The command that names the path only reads, and so does every command it is piped with.** These do: `cat`, `head`, `tail`, `grep`, `rg`, `ls`, `find`, `wc`, `diff`, `jq`, `test`, `echo`, and in PowerShell `Get-Content`, `Select-String`, `Test-Path`, `Get-Item` and `Get-ChildItem`. So do `git diff`, `log`, `show`, `status` and `blame`. `git add` and `git commit` record a file without changing it, so they count too.
- **No output goes to the path**, or to a target the hook cannot read off the text, such as a pattern.
- **The line is plain**: commands, quotes, pipes, redirects, `;`, `&&` and `||`. A line with braces, backticks, `#` or `<` is asked about, and so is one with a backslash in Bash or an `@` in PowerShell.
- **The command that names the path uses no `$` and no brackets**, and neither does a command it is piped with. In Bash, another command on the line may: `cat quality.yaml; git diff "$(git merge-base main HEAD)" --stat` is a plain read. In PowerShell the whole line may not.
- **No other command on the line changes what a command name means**: `alias`, `source`, `eval`, `PATH=...`, and in PowerShell `Set-Alias` or `Import-Module`.

Other commands on the line are left alone, as long as they do not name the path.

So these are not asked about:

```sh
cat quality.yaml
test -f quality.yaml && qk init --maturity silver
git diff main -- quality.yaml
cat .quality/waivers.yaml | grep rule | head -20
cat quality.yaml && make docs
```

And these are:

```sh
sed -i 's/silver/bronze/' quality.yaml    # changes it
cat new.yaml > quality.yaml               # output goes to it
echo quality.yaml | xargs rm              # piped into a command that is no reader
cat "$HOME/repo/quality.yaml"             # a read, but the line is not plain
sed -n '1,5p' quality.yaml                # a read, but sed can also write
```

The last two are reads, and you are asked all the same. The hook reads the text of the command; it does not run a shell. It lets a narrow, certain shape through and asks about the rest, because a guess that is wrong lets a change through without you.

It can still miss a write when the name is not in the text of the command that writes: a file name in a variable from an earlier command, a script that does the writing, or a file that carries the name to a later command. Review of `.quality/` and `quality.yaml` at merge is the backstop: see [who approves](waive-a-finding.md#who-approves).

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
