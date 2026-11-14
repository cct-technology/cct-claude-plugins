---
type: regex
target: trace
pattern: 'FAIL\s+QK061|QK061 fail: Dependencies have a lockfile'
---

The session summary also names QK061, so the final message alone does not show that `qk check` ran and printed the finding. These lines are only in the output of `qk check`: the first in its text format, the second with `--format claude`.
