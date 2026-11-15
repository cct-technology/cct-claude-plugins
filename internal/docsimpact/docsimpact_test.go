package docsimpact

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cct-technology/cct-claude-plugins/internal/waivers"
)

var approval = waivers.Approval{By: "a person", Identity: "Test <test@example.com>", Time: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}

func TestNotePath(t *testing.T) {
	tests := []struct{ branch, want string }{
		{"feature", ".quality/docs-impact/feature.md"},
		{"rk/cct-3432-qk010", ".quality/docs-impact/rk-cct-3432-qk010.md"},
		{"a/b c\\d", ".quality/docs-impact/a-b-c-d.md"},
		{"abc1234", ".quality/docs-impact/abc1234.md"},
	}
	for _, tt := range tests {
		if got := NotePath(tt.branch); got != tt.want {
			t.Errorf("NotePath(%q) = %q, want %q", tt.branch, got, tt.want)
		}
	}
}

func TestReason(t *testing.T) {
	tests := []struct{ name, content, want string }{
		{"written by qk", "Docs-Impact: none — internal refactor\n", "internal refactor"},
		{"not the first line", "# note\nDocs-Impact: none — x\n", "x"},
		{"empty reason", "Docs-Impact: none — \n", ""},
		{"blank reason", "Docs-Impact: none —    \n", ""},
		{"something else", "Docs-Impact: yes\n", ""},
		{"empty file", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Reason([]byte(tt.content)); got != tt.want {
				t.Errorf("Reason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteCreatesTheNoteAndLogsTheApproval(t *testing.T) {
	root := t.TempDir()

	path, err := Write(root, "rk/x", "internal   refactor", approval)
	if err != nil {
		t.Fatal(err)
	}

	if path != ".quality/docs-impact/rk-x.md" {
		t.Errorf("path = %q", path)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if got := Reason(data); got != "internal refactor" {
		t.Errorf("Reason = %q, want whitespace collapsed", got)
	}
	l := waivers.Load(root)
	if !l.NoteApproved(path, "internal refactor") {
		t.Errorf("not approved in the log: %+v", l)
	}
	if e := l.Log[0]; e.ApprovedBy != "a person" || e.Identity != "Test <test@example.com>" || e.Time != "2026-11-08T12:00:00Z" {
		t.Errorf("log entry = %+v", e)
	}
}

func TestWriteAgainReplacesTheNoteAndAddsALogLine(t *testing.T) {
	root := t.TempDir()
	if _, err := Write(root, "feature", "first", approval); err != nil {
		t.Fatal(err)
	}

	path, err := Write(root, "feature", "second", approval)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if got := Reason(data); got != "second" {
		t.Errorf("Reason = %q", got)
	}
	if l := waivers.Load(root); len(l.Log) != 2 || !l.NoteApproved(path, "second") {
		t.Errorf("log = %+v", l.Log)
	}
}
