# Modes and maturity

Not every repository can meet every check on the first day. Quality Kit lets a repository take the checks on in steps. This page explains the two settings that do it.

## A mode says what a failure does

Each check runs in one of three modes:

| Mode | A failure |
| --- | --- |
| `off` | The check does not run. |
| `advisory` | Is a warning. You see it; nothing blocks. |
| `enforced` | Blocks: Claude is sent back before it can finish, `qk check` exits 1, and the CI job fails. |

The check is the same in each mode. Only the consequence changes.

## Maturity sets the modes for you

Choosing a mode per check is more than most teams want to do. So `quality.yaml` has one setting, `maturity`, that picks them:

| Maturity | Meant for | What blocks |
| --- | --- | --- |
| `bronze` | Repositories that existed before Quality Kit | Only a broken `quality.yaml`. Every other check warns. |
| `silver` | New services | Also docs, contracts, generated files, lockfiles and waivers. |
| `gold` | Client-facing or regulated services | Every check. |

Each check has a level from which it blocks. The [rule reference](../reference/rules.md) gives it for every rule, as "Enforced by default at".

Bronze exists so an old repository can install Quality Kit and learn what it is missing without being stopped. Move up when the warnings are gone.

## Without a quality.yaml

Quality Kit works with no configuration. Every check then runs `advisory` with defaults, and nothing blocks. That is below bronze: with no manifest there is nothing that can be invalid.

## Override one check

A level is a default. Under `modes` you set a single check to another mode, by its check key:

```yaml
version: 1
maturity: silver
modes:
  docs.impact: advisory
  ownership.codeowners: enforced
```

This repository is silver, but docs impact only warns and code owners block. Use it to take one check on early, or to hold one back while you fix what it finds.

How to: [Configure quality.yaml](../how-to/configure-quality-yaml.md).

## Who may change them

`maturity` and `modes` decide what blocks, so a change to them can turn a check off. When Claude wants to change `quality.yaml`, Claude Code asks a person first. At merge, require a code owner's review of the file: see [who approves](../how-to/waive-a-finding.md#who-approves).

## A mode is not a waiver

Both stop a finding from blocking. They answer different questions:

- **A mode** is about the check: "this repository is not ready for it yet". It has no end date.
- **A [waiver](../how-to/waive-a-finding.md)** is about one finding: "we know, and accept it until this date". It has a reason, an owner and an end date, and a person approves it.

Use a waiver when you can. A check set to `advisory` stays quiet about every new finding too.

## Slow checks

Some checks are slow, because they run your generators. They run with `qk check --full` and in CI, and not when Claude finishes. A slow check can be enforced and still not stop Claude in a session: CI is where it blocks. See [run the slow checks](../how-to/run-qk-check.md#run-the-slow-checks).
