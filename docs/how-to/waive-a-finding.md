# Waive a finding

A waiver records that your team knows about a finding and accepts it for now. Every waiver has a reason, an owner and an end date, so it cannot quietly become permanent. A person has to approve it. Fix the finding when you can; waive it when you can't yet.

## Add a waiver

```sh
qk waiver add QK010 \
  --reason "Legacy module is being rewritten in CCT-1234" \
  --owner rkvalvaa \
  --expires 2026-12-31 \
  --path "internal/legacy/**" \
  --approved-by rkvalvaa
```

This adds an entry to `.quality/waivers.yaml` at the repository root and an approval line to `.quality/exceptions-log.jsonl`. Commit both with your change, so a reviewer sees the exception.

- `--expires` is the last day the waiver applies.
- `--path` limits the waiver to matching files and can be repeated. Without it, the waiver covers every finding of that rule.
- In a path, `*` matches within one folder level, `**` matches any number of levels, and `?` matches one character. Everything else is literal, including `[id]` and `(admin)` in route folders.

## Who approves

A person, not Claude:

- **When Claude asks to add a waiver**, Claude Code prompts you first. The same prompt appears if Claude changes anything under `.quality/` or edits `quality.yaml`, because a check can be turned off there. Read the reason before you approve. With no person to answer, as in `claude -p`, the request is denied.
- **At review**, make `.quality/` and `quality.yaml` CODEOWNERS paths, so a change to an exception needs a person's approval on the pull request.

The prompt is best effort: it reads the command text. The review is the backstop.

## The log

`.quality/exceptions-log.jsonl` has one line per approved exception: time, git identity, approver, rule, paths, reason, owner and end date. Git history shows every change to it.

A waiver counts only with a line matching all its fields. A waiver written by hand, or edited after approval (a later date, wider paths), does not apply, and QK100 reports it. The approver's name is what was typed; the prompt and the review are what make it an approval.

## See what is waived

A waived finding no longer blocks. `qk check` still lists it, with its owner and end date. A rule whose findings are all waived reports `pass`.

## When a waiver ends

On the day after `--expires`, the waiver stops applying: the finding is reported again, and QK100 reports the expired waiver. At silver and gold, QK100 blocks until you do one of these:

- Fix the finding and delete the waiver.
- Delete the waiver and have a person approve a new one with `qk waiver add`.

QK100 also reports a waiver with no reason or no owner, a bad date, or no approval in the log. A waiver that names a rule your `qk` doesn't have is only a warning. QK100 itself cannot be waived.
