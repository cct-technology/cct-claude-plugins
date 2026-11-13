# Run the skill evals

Each skill has eval cases under `plugins/quality-kit/evals/`: a prompt, a script that builds the repository Claude starts in, and graders that check what Claude did. `claude plugin eval` runs them with real model calls, so run them when you change a skill, not on every push.

On 2026-11-13 the whole suite, three runs of each case on Sonnet, cost about $0.80.

## What you need

- **Linux, macOS or WSL2.** The cases let Claude use Bash, and `claude plugin eval` allows that only where it can sandbox the shell. On native Windows it refuses every run.
- **Claude Code 2.1.269 or later**, signed in: `claude auth login`.
- **On Linux and WSL2: `bubblewrap` and `socat`.**
- **Go**, to build `qk`.

These steps were checked on WSL2 with Ubuntu 24.04. They have not been run on macOS.

## Run them

From the repository root:

```sh
version=$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' plugins/quality-kit/.claude-plugin/plugin.json)
go build -ldflags "-X main.version=$version" -o dist/eval/qk ./cmd/qk

export PATH="$PWD/dist/eval:$PWD/plugins/quality-kit/bin:$PATH"
mkdir -p "$HOME/.cache/qk-eval-tmp"
export TMPDIR="$HOME/.cache/qk-eval-tmp"

claude plugin eval plugins/quality-kit \
  --scaffold --allow-tools Bash Edit Write \
  --ablation none --model claude-sonnet-5-5 --no-publish --max-cost-usd 5
```

Every case should score 1.00. The report is `plugins/quality-kit/evals/results/<time>/report.html`; git ignores that folder.

Why each part is there:

- **`-X main.version=$version`**: the plugin's `bin/qk` runs only a `qk` with the version in `plugin.json`.
- **`PATH`, in that order**: Claude's shell runs in a sandbox that sees the run's own home folder and the plugin, not your folders. So `qk` there is the plugin's `bin/qk`, and each case's script first copies your built `qk` to where `bin/qk` looks for it. The script takes the first `qk` on `PATH`, which must be the built one.
- **`TMPDIR`**: the runner keeps each run in the temp folder and forbids shell writes under `/tmp`. With the default temp folder on Linux, that makes the run's own repository read-only and `qk init` fails.
- **`--scaffold`**: runs each case's `scaffold.sh` as you. Read a script before you run one you did not write.
- **`--allow-tools Bash Edit Write`**: the tools the skills need beyond reading.
- **`--ablation none`**: leaves out the second run of each case without the plugin, which halves the cost.
- **`--no-publish`**: keeps the report on your machine.

To run less, add `--case 'quality-init-*'` or `--runs 1`.

## When a run fails before Claude does anything

| The run says | Cause | Fix |
| --- | --- | --- |
| `A shell tool (Bash or PowerShell) was granted but this machine cannot confine it` | Native Windows, or `bubblewrap` is missing | Run in WSL2; install `bubblewrap` and `socat` |
| `Failed to create bridge sockets` | `socat` is missing or does not start | Install `socat`; check that `socat -V` runs |
| `the Docker (~/.docker, DOCKER_CONFIG) credential store on this machine holds a symbolic link` | Docker Desktop's WSL integration links `contexts` and `features.json` in `~/.docker` to Windows | Move those two links out of `~/.docker` for the run and put them back afterwards. Setting `DOCKER_CONFIG` does not help |
| `qk: command not found` | The plugin's `bin` folder is not on `PATH` | Set `PATH` as above |
| `read-only file system` from `qk init` | The run is under `/tmp` | Set `TMPDIR` as above |
| `install-qk: ...` | No built `qk` on `PATH`, the wrong version, or the plugin's `bin` comes first | Build `qk` and set `PATH` as above |

## Add a case

1. Make a folder `plugins/quality-kit/evals/<skill>-<what-it-covers>/`. A test requires at least three cases for each skill, found by that name.
2. Write `prompt.md`: what a person would type, with the tools the case needs in `allowed_tools`.
3. Write `case.yaml` and `scaffold.sh`, as in the existing cases. Start the script with `. "$(dirname "$0")/../install-qk.sh"`, and commit what it creates, so the Stop hook sees no change that Claude did not make.
4. Write one grader per file under `graders/`. Prefer `regex`, `tool_used` and `file_exists`, which cost nothing and give the same answer every time.

The plugin's hooks run in every case. The session summary names open findings, so a grader on the final message cannot tell whether Claude ran `qk check`. To require that, grade the trace: `target: trace` with a line only the command prints.
