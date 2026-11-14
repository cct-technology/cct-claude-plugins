---
type: regex
target: { source: file, path: docs/testing/uat/2.3.0.md }
pattern: '\|\s*(pass|fail|blocked)\s*\|'
match: not_contains
---

Claude writes the plan. People run it: a status other than `todo` would be a result nobody saw.
