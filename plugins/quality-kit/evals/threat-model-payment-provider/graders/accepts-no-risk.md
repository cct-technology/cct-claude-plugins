---
type: regex
target: { source: file, path: docs/security/threat-models/checkout.md }
pattern: '\|\s*risk accepted\s*\|'
flags: i
match: not_contains
---

Living with a risk is a person's decision. Claude marks a threat `in place` or `to do`.
