---
type: regex
target: { source: file, path: docs/testing/uat/1.1.0.md }
pattern: '\|\s*(pass|fail|blocked)\s*\|'
match: not_contains
---

Claude writes the plan. People run it: a status other than `todo` would be a result nobody saw.
