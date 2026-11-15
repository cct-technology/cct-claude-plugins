// Command gen-docs regenerates the reference docs from the rule registry and
// the manifest schema. Run it from the repo root; CI fails when its output
// differs from what is committed.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	qualitykit "github.com/cct-technology/cct-claude-plugins"
	"github.com/cct-technology/cct-claude-plugins/internal/check"
	"github.com/cct-technology/cct-claude-plugins/internal/manifest"
	"github.com/cct-technology/cct-claude-plugins/internal/rules"
)

func main() {
	docs := map[string]func(io.Writer) error{
		"docs/reference/rules.md": func(w io.Writer) error { return check.WriteReference(w, rules.All()) },
		"docs/reference/manifest.md": func(w io.Writer) error {
			return manifest.WriteReference(w, qualitykit.ManifestSchema)
		},
	}
	for path, write := range docs {
		var buf bytes.Buffer
		if err := write(&buf); err != nil {
			fail(fmt.Errorf("%s: %w", path, err))
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-docs:", err)
	os.Exit(1)
}
