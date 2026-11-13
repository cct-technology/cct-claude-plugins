---
type: regex
target: { source: file, path: quality.yaml }
pattern: '^\s*replaces:'
flags: m
match: not_contains
---
