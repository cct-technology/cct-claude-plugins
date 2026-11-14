---
type: regex
target: { source: file, path: docs/security/threat-models/team-notes.md }
pattern: 'markdown|html|script|xss'
flags: i
---

The body is turned into HTML and put in the page: what one member writes runs in another member's browser.
