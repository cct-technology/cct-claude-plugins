---
type: regex
target: { source: file, path: docs/security/threat-models/team-notes.md }
pattern: '^## LLM threats'
flags: m
match: not_contains
---

Nothing in the feature calls a model.
