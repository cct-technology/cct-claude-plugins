# Your first run

In this tutorial you install Quality Kit in a small practice repository, let Claude set it up, see your first finding and fix it. Then you watch Quality Kit send Claude back to update a doc it forgot. It takes about 20 minutes.

You need:

- Claude Code, git and Go.
- On Windows, [Git Bash](https://git-scm.com/downloads/win). Run the commands below in it.
- `qk` on your `PATH`: follow [Get the qk binary](../how-to/get-the-qk-binary.md), then check it:

  ```sh
  qk version
  ```

  ```text
  qk 0.1.0 (manifest schema 1)
  ```

## 1. Make a practice repository

```sh
mkdir shop && cd shop
git init -b main
git remote add origin https://github.com/example/shop.git
printf 'module example.com/shop\n\ngo 1.27\n' > go.mod
mkdir -p internal/config
printf 'package config\n\n// Port is the port the shop listens on.\nconst Port = 8080\n' > internal/config/config.go
printf '# Shop\n' > README.md
git add -A && git commit -m "Start"
```

Nothing is pushed in this tutorial. The remote is there so Quality Kit knows the repository is meant for GitHub.

## 2. Install the plugin

```sh
claude plugin marketplace add cct-technology/cct-claude-plugins#v0.1.0 --scope project
claude plugin install quality-kit@cct-claude-plugins --scope project
```

The last line you see is:

```text
✔ Successfully installed plugin: quality-kit@cct-claude-plugins (scope: project)
```

Both commands wrote to `.claude/settings.json`. In a real repository you commit that file, so your teammates get the plugin too.

`#v0.1.0` keeps the repository on a version you chose. `#main` would follow the newest Quality Kit.

## 3. Let Claude set the repository up

Start Claude Code in the folder:

```sh
claude
```

Trust the folder when asked. If a message appears that ends with `qk did not run`, the plugin has not found `qk`: go back to [Get the qk binary](../how-to/get-the-qk-binary.md), then start Claude Code again.

Then say:

```text
Set up Quality Kit here. Internal data, silver, and it replaces nothing.
```

Claude runs `qk init`, which reports what it wrote:

```text
Stack: Go
Host: GitHub
wrote docs/explanation/README.md
wrote docs/how-to/README.md
wrote docs/reference/README.md
wrote docs/security/threat-models/README.md
wrote docs/testing/uat/README.md
wrote docs/tutorials/README.md
wrote quality.yaml
wrote .gitattributes
wrote .github/CODEOWNERS
wrote .github/workflows/quality-kit.yml
```

`quality.yaml` is the manifest: it says what this repository owes. `silver` means the checks on docs, contracts, generated files, dependencies, lockfiles and waivers block.

If Claude wants to change `quality.yaml` afterwards, Claude Code asks you first. That file decides which checks block, so a person approves every change to it.

## 4. See your first finding

Claude ends with the result of `qk check`. When it stops, Claude Code also shows you a message that starts with `Quality Kit: 0 fail, 1 warn.`: that is Quality Kit checking Claude's work as it finishes. Run the check yourself too, in a second terminal in the same folder:

```sh
qk check
```

```text
PASS  QK001  quality.yaml is valid (enforced)
PASS  QK010  Docs change with the code they describe (enforced)
PASS  QK020  Generated files equal their generator's output (enforced)
PASS  QK030  No breaking OpenAPI changes (enforced)
PASS  QK040  A new spec has a threat model (advisory)
PASS  QK060  New and upgraded dependencies exist, are old enough, maintained and licensed (enforced)
PASS  QK061  Dependencies have a lockfile that changes with them (enforced)
PASS  QK100  Waivers are valid, owned and not expired (enforced)
WARN  QK110  Every top-level path has a code owner (advisory)
      - .gitattributes: no owner: no line in `.github/CODEOWNERS` matches this file
      - .github: no owner: no line in `.github/CODEOWNERS` covers this folder
      - README.md: no owner: no line in `.github/CODEOWNERS` matches this file
      - docs: no owner: no line in `.github/CODEOWNERS` covers this folder
      - go.mod: no owner: no line in `.github/CODEOWNERS` matches this file
      - internal: no owner: no line in `.github/CODEOWNERS` covers this folder
      fix: Add a line for the path to `CODEOWNERS`, such as `/docs/ @your-team`, or give everything a default owner with `* @your-team` on the first line. ...

0 fail, 1 warn, 8 pass, 0 skipped
```

Each line is one check. `(enforced)` means a failure blocks; `(advisory)` means it only warns. QK110 warns: most of the repository has no owner. At silver that does not block, but fix it anyway.

## 5. Fix it

Open `.github/CODEOWNERS` and add a default owner above the Quality Kit lines:

```text
*               @your-team

# Quality Kit: an exception to a check, and the file that sets which checks block, need a person's review.
/.quality/      @your-team
/quality.yaml   @your-team
```

In a real repository, `@your-team` is a person or team that can write to it.

```sh
qk check
```

```text
0 fail, 0 warn, 9 pass, 0 skipped
```

That is the whole loop: `qk` names what is missing and how to fix it, and you or Claude fix it.

## 6. Tell Quality Kit which docs go with which code

Write a short how-to, `docs/how-to/change-the-port.md`:

```markdown
# Change the port

The shop listens on port 8080. To change it, edit `Port` in `internal/config/config.go`.
```

In `quality.yaml`, replace the commented `impact_map` example under `docs:` with:

```yaml
  impact_map:
    - paths: ["internal/config/**"]
      docs: ["docs/how-to/change-the-port.md"]
```

This says: when a file under `internal/config/` changes, that how-to must change too. Commit everything:

```sh
git add -A && git commit -m "Set up Quality Kit"
```

## 7. Watch Claude get sent back

Back in Claude Code, say:

```text
On a new branch, change the port to 9090. Change only the code.
```

Claude edits `internal/config/config.go` and tries to finish. Quality Kit runs `qk check --changed` at that moment, and QK010 fails, because the how-to still says 8080. Claude Code shows "Stop hook error occurred": that is the block working, not a fault. Claude is given this:

```text
Quality Kit: 1 fail, 0 warn.
QK010 fail: Docs change with the code they describe
- internal/config/config.go: changed, but none of its docs did; expected a change in docs/how-to/change-the-port.md
Fix: Update the mapped docs. If this change needs none, ask a person to approve `qk docs-impact none --reason <why> --approved-by <person>`. More: qk explain QK010
```

Claude then updates `docs/how-to/change-the-port.md` to say 9090, and this time it may finish.

Claude may do this differently. If it updates the doc straight away, there is nothing to send it back for. If it asks you whether to update the doc, say yes. Either way, check the result:

```sh
qk check
```

```text
0 fail, 0 warn, 9 pass, 0 skipped
```

You asked for a code change only, and the doc was updated all the same. That is what Quality Kit is for.

## What you have seen

- `qk` decides what is missing. Claude does not grade its own work.
- At silver, a missing doc change blocks Claude from finishing. A missing code owner only warns.
- A change to `quality.yaml` needs your approval.

## Where next

- Set up a real repository: [Set up Quality Kit with Claude](../how-to/set-up-quality-kit-with-claude.md).
- Add the checks to pull requests: [Run Quality Kit in GitHub Actions](../how-to/run-quality-kit-in-github-actions.md).
- When a finding cannot be fixed yet: [Waive a finding](../how-to/waive-a-finding.md).
- Why it works this way: [`qk` decides, Claude writes](../explanation/deterministic-first.md).
- Everything else: [the how-to guides](../how-to/README.md).
