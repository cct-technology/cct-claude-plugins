---
type: regex
target: { source: file, path: docs/security/threat-models/reply-drafts.md }
pattern: '^- \[[xX]\]'
flags: m
match: not_contains
---

A ticked line says the work is done. Claude wrote the list; nobody has done the work.
