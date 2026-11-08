# Waive a finding

A waiver records that your team knows about a finding and accepts it for now. Every waiver has a reason, an owner and an end date, so it cannot quietly become permanent. Fix the finding when you can; waive it when you can't yet.

## Add a waiver

```sh
qk waiver add QK010 \
  --reason "Legacy module is being rewritten in CCT-1234" \
  --owner rkvalvaa \
  --expires 2026-12-31 \
  --path "internal/legacy/**"
```

This adds an entry to `.quality/waivers.yaml` at the repository root. Commit that file with your change, so a reviewer sees the exception.

- `--expires` is the last day the waiver applies.
- `--path` limits the waiver to matching files and can be repeated. Without it, the waiver covers every finding of that rule.
- In a path, `*` matches within one folder level, `**` matches any number of levels, and `?` matches one character. Everything else is literal, including `[id]` and `(admin)` in route folders.

## See what is waived

A waived finding no longer blocks. `qk check` still lists it, with its owner and end date. A rule whose findings are all waived reports `pass`.

## When a waiver ends

On the day after `--expires`, the waiver stops applying: the finding is reported again, and QK100 reports the expired waiver. At silver and gold, QK100 blocks until you do one of these:

- Fix the finding and delete the waiver.
- Give the waiver a new date and a current reason.

QK100 also reports a waiver with no reason or no owner, or with a bad date. A waiver that names a rule your `qk` doesn't have is only a warning. QK100 itself cannot be waived.
