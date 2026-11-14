---
type: regex
target: { source: file, path: docs/testing/uat/1.1.0.md }
pattern: '## Steps for[\s\S]*## Steps for'
match: not_contains
---

The app has one kind of user. A second set of steps would be for a role nobody has.
