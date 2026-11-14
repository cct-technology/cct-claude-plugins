---
type: regex
target: { source: file, path: docs/security/threat-models/team-notes.md }
pattern: '^spec: docs/specs/team-notes\.md$'
flags: m
---

QK040 finds the threat model of a spec by this line, or by the file name.
