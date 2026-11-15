# Work with Quality Kit's hooks

With the plugin enabled, three hooks run `qk` through `bin/qk` in every session: when it starts, before a tool runs, and when Claude finishes. You do not call them yourself.

| When | What happens |
| --- | --- |
| A session starts, resumes, or is cleared or compacted | `qk context` is added to Claude's context: maturity, which checks are enforced, open findings, and waivers ending soon. |
| Claude runs a shell command or edits a file | If it records an exception (`qk waiver add`, `qk docs-impact`) or could change `.quality/` or `quality.yaml`, Claude Code asks a person first. Reading them asks nobody. |
| Claude edits a generated file | The edit is blocked, and Claude is told which command writes the file. See [when an edit is blocked](#when-an-edit-to-a-generated-file-is-blocked). |
| Claude installs a package | The install is blocked when the package does not exist, or its version is less than 14 days old. See [when an install is blocked](#when-an-install-is-blocked). |
| Claude tries to finish | `qk check --changed` runs. Enforced failures send Claude back with the findings. Advisory findings are shown to you, and Claude stops. |

## When you are asked to approve

The prompt means Claude is about to record an exception, or to change `.quality/` or `quality.yaml`, where a check can be turned off. Read what it wants to change before you approve.

| Claude does this | Asked? |
| --- | --- |
| Runs `qk waiver add` or `qk docs-impact` | Yes |
| Edits or writes a file under `.quality/`, or `quality.yaml` | Yes |
| Runs a shell command that names one of them, unless it is a plain read | Yes |
| Runs a plain read of one of them | No |

The prompt for a shell command that is not a plain read says so, and tells Claude that a read on its own line needs no approval. So when a read shared a line with other work and nobody approves, Claude runs the read again by itself and goes on.

### What a plain read is

A shell line that names `.quality/` or `quality.yaml` is a plain read when all of this holds. The hook lists what is allowed; anything else on such a line is asked about.

- **Every command on the line only reads.** These do: `cat`, `head`, `tail`, `grep`, `rg`, `ls`, `find`, `wc`, `diff`, `jq`, `test`, `echo`, and in PowerShell `Get-Content`, `Select-String`, `Test-Path`, `Get-Item` and `Get-ChildItem`. So do `git diff`, `log`, `show`, `status` and `blame`. `git add` and `git commit` record a file without changing it, and `git tag` and `git branch` name commits, so they count too.
- **A few other commands may stand beside a read**, when they do not name the path themselves: `qk`, `cd`, `which`, `type` and `command -v`.
- **Every word is written out in full.** No variables and no `$( )`, with one exception: `$(git merge-base <rev> <rev>)`. Outside quotes a word may hold letters, digits and `_ . / : , @ % + = * ? ~ -`, and `find` may be given `\(` and `\)`. Inside quotes anything goes.
- **The commands are joined plainly**: `;`, `&&`, `||`, `|`, `if`, `( )` and `{ }`. No assignments, loops, functions or here-documents.
- **No output goes to the path**, or to a target that is not written out.

So these are not asked about:

```sh
cat quality.yaml
test -f quality.yaml && qk init --maturity silver
if [ -f quality.yaml ]; then cat quality.yaml; fi
git diff "$(git merge-base main HEAD)" -- quality.yaml
grep -n "silver|gold" quality.yaml | head -20
ls -a; cat quality.yaml 2>/dev/null; which qk
```

And these are:

```sh
sed -i 's/silver/bronze/' quality.yaml    # changes it
cat new.yaml > quality.yaml               # output goes to it
echo quality.yaml | xargs rm              # xargs is no reader
cat quality.yaml; rm $_                   # a variable: $_ is the last word of the command before
cat "$HOME/repo/quality.yaml"             # a read, but with a variable
cat quality.yaml && make docs             # a read, but make is not on the list
sed -n '1,5p' quality.yaml                # a read, but sed can also write
```

The last three are reads, and you are asked all the same. A hook that reads text cannot follow everything a shell does, so it lets a narrow, certain shape through and asks about the rest. A guess that is wrong lets a change through without you.

### How the hook reads a line

- **Bash lines are parsed**, with the parser of [`shfmt`](https://github.com/mvdan/sh), so quotes, escapes and comments are read as Bash reads them. A line the parser cannot read is asked about. The Bash tool may run zsh, as on macOS: the line is still read as Bash, which is one reason words outside quotes are held to plain characters.
- **PowerShell lines are not parsed.** The same rules are applied to the text of the line.

The hook runs nothing. It can miss a write when the name is not written on the line: a file name in a variable from an earlier command, a pattern such as `*.yaml`, a script that does the writing, or another name for the same file, such as a link. The name counts in any case, so `QUALITY.YAML` is asked about too, and so is the usual Windows short name, `QUALIT~1.YAM`. Review of `.quality/` and `quality.yaml` at merge is the backstop: see [who approves](waive-a-finding.md#who-approves).

## When an edit to a generated file is blocked

A generated file is written by a command, so an edit by hand is lost the next time the command runs. Claude's edit is refused, and Claude is told what to do:

```text
Quality Kit: `docs/reference/settings.md` is generated, so an edit by hand is undone the next time its generator runs. Do not edit it: change what it is generated from, and have the generator write the file. The generator is named as: `go run ./cmd/gen-settings-doc`
```

A file counts as generated when it is listed under `generated` in `quality.yaml`, or when its first line says `generated by qk: <command>. Do not edit by hand.` Blank lines and a `#!` line may come before it. A file that only quotes those words further down is not generated.

The command in that line is whatever the file says, so Claude is shown it and not told to run it. A command under `generated` in `quality.yaml` has been through a person's review. See [keep generated docs current](keep-docs-in-step.md#keep-generated-docs-current).

Only the Edit and Write tools are stopped. A shell command that writes the file is not: that is how the generator itself writes it. If the file is changed by hand that way, QK020 finds it in CI.

## When an install is blocked

When Claude runs a command that installs a package, `qk` asks the public registry about each package first. The install is refused when a package does not exist, or when the version is less than 14 days old:

```text
Quality Kit: this command installs a package that should not be installed without a person looking:
- `reakt-domm` (npm) does not exist in the registry
Check the name: it may be invented or misspelt. Do not install it another way. If a person decides it is right, they can run the command themselves.
```

A name that does not exist is often one Claude made up, and such names get registered by attackers. A version that is a day old may be a hijacked release.

These commands are read, in the Bash and the PowerShell tool:

| Registry | Commands |
| --- | --- |
| npm | `npm install`, `npm i`, `npm add`, `pnpm add`, `yarn add`, `bun add`, `bun install` |
| Go | `go get`, `go install` |
| Packagist | `composer require` |
| crates.io | `cargo add`, `cargo install` |
| PyPI | `pip install`, `python -m pip install`, `uv add`, `uv pip install`, `poetry add` |

If you want the package all the same, run the install yourself: in Claude Code, start the line with `!`. The hook reads Claude's commands, not yours. [QK060](check-dependencies.md) then checks the dependency in CI, where a person can [waive that package](check-dependencies.md#when-the-dependency-is-right-all-the-same).

What the hook does not stop:

- **A deprecated package, or a licence that is not allowed.** Those are for QK060, where a person can waive them.
- **A package that is not written on the line**: one in a variable, or in a file given with `-r`.
- **A path, a URL or a git source.** They do not come from a registry.
- **An upgrade of what is already there**: `npm update`, `cargo update`, `go get -u ./...`, `composer update`. No package is named on the line.
- **A package that is run and not installed**: `npx`, `uvx`, `go run <package>@<version>`.
- **More than ten packages on one line**, or lookups that take more than 30 seconds together. The rest are not checked here.
- **An install when the registry cannot be reached.**

QK060 checks all of these in CI, from the manifest and the lockfile.

### Where a check is advisory

The two blocks follow their checks: generated files follow QK020 (`generated.current`), installs follow QK060 (`supply-chain.dependencies`). Both are enforced from `silver`. At `bronze`, and without a `quality.yaml`, nothing is blocked. Claude gets a note with the result of the edit or the install, so after it has happened: for a generated file, to undo the edit; for a package, to tell you what was installed and what Quality Kit found. You see nothing from the hook yourself in that mode. With the check `off`, the hook says nothing.

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

Hooks never block because of a problem with Quality Kit itself. If `qk` is missing, crashes or times out, the hook warns and Claude carries on: the edit or the install then goes through unchecked. Only real findings block.

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
