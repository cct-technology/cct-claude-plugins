---
type: regex
target: { source: file, path: api/openapi.yaml }
pattern: 'total'
match: not_contains
---

The contract follows the code: neither `properties` nor `required` names the field any more.
