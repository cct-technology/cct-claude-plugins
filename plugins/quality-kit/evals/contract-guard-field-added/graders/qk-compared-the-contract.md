---
type: regex
target: trace
pattern: 'QK030 \('
match: not_contains
---

`qk check` writes a rule followed by a reason in brackets only on its `Skipped:` line. With oasdiff where the sandbox can see it, and a contract it can read, QK030 runs and is not there.
