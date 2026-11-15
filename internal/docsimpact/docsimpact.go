// Package docsimpact reads and writes "no docs needed" notes: a person's
// approved statement that a branch's change needs no doc change.
package docsimpact

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cct-technology/cct-claude-plugins/internal/waivers"
)

// Dir holds the notes, relative to the repo root.
const Dir = ".quality/docs-impact/"

const prefix = "Docs-Impact: none — "

var unsafeInName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// NotePath is the note's repo-relative path for a branch.
func NotePath(branch string) string {
	return Dir + unsafeInName.ReplaceAllString(branch, "-") + ".md"
}

// Reason returns the reason a note gives, or "" if it gives none.
func Reason(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		if r, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(r)
		}
	}
	return ""
}

// Write logs the approval and writes the note for branch, replacing any earlier
// note. It returns the note's repo-relative path.
func Write(root, branch, reason string, a waivers.Approval) (string, error) {
	reason = strings.Join(strings.Fields(reason), " ")
	note := NotePath(branch)
	// The log goes first: a stray log line grants nothing, while a note
	// written without its line simply does not count.
	if err := waivers.AppendLog(root, waivers.NewNoteEntry(note, reason, a)); err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(note))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	return note, os.WriteFile(full, []byte(prefix+reason+"\n"), 0o644)
}
