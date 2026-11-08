# Keep docs in step with the code

QK010 checks that when code changes, the docs that describe it change on the same branch. You say which docs go with which code.

## Map code to docs

In `quality.yaml`:

```yaml
version: 1
maturity: silver
docs:
  impact_map:
    - paths: ["internal/config/**"]
      docs: ["docs/reference/settings.md"]
    - paths: ["web/app/(admin)/**"]
      docs: ["docs/how-to/admin/**"]
```

If a changed file matches `paths`, at least one file matching `docs` must also change. Uncommitted work counts, so the check gives the same answer before and after you commit.

In a glob, `*` matches within one folder level, `**` matches any number of levels, and `?` matches one character. Everything else is literal, including `[id]` and `(admin)`.

From `silver` up, QK010 blocks. At `bronze`, it warns.

## When a change needs no docs

An internal refactor can touch mapped code without changing anything a reader would notice. A person can record that:

```sh
qk docs-impact none --reason "Internal refactor, behaviour unchanged" --approved-by rkvalvaa
```

This writes a note to `.quality/docs-impact/<branch>.md` and an approval line to `.quality/exceptions-log.jsonl`. Commit both, so a reviewer sees the claim.

- If Claude runs the command, Claude Code asks you first. See [who approves](waive-a-finding.md#who-approves).
- The note counts only while it is part of the current change, so a note from an earlier branch never applies.
- A note written by hand, or edited after approval, does not count. QK010 reports it.
- To change the reason, run the command again.

To cover only some files, or to accept a finding for a set time, use a [waiver](waive-a-finding.md) instead.
