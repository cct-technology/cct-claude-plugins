// Command gen-docs regenerates the reference docs from the rule registry.
// Run it from the repo root; CI fails when its output differs from what is committed.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
)

const rulesDoc = "docs/reference/rules.md"

func main() {
	var buf bytes.Buffer
	if err := check.WriteReference(&buf, rules.All()); err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(rulesDoc), 0o755); err != nil {
		fail(err)
	}
	if err := os.WriteFile(rulesDoc, buf.Bytes(), 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-docs:", err)
	os.Exit(1)
}
