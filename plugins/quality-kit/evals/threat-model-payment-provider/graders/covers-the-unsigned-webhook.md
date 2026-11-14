---
type: regex
target: { source: file, path: docs/security/threat-models/checkout.md }
pattern: 'webhook[\s\S]*signature|signature[\s\S]*webhook'
flags: i
---

The spec says the event is signed. The handler does not check it, so anyone can mark an order paid.
