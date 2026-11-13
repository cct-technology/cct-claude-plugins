---
type: regex
target: trace
pattern: 'QK030 fail: No breaking OpenAPI changes'
---

The line `qk check` prints for the finding. The skill's own text is in the trace too, so the pattern is one only `qk` writes. It needs oasdiff where the sandbox can see it.
