---
type: regex
target: { source: file, path: docs/testing/uat/1.1.0.md }
pattern: '## Security acceptance checks[\s\S]*session'
flags: i
---

The download is behind the sign-in, so the expired-session check applies.
