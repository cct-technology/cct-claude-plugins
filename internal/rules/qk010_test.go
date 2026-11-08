package rules

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

var impactMap = []manifest.ImpactEntry{
	{Paths: []string{"internal/config/**"}, Docs: []string{"docs/reference/settings.md"}},
	{Paths: []string{"web/app/(admin)/**"}, Docs: []string{"docs/how-to/admin/**"}},
}

const notePath = ".quality/docs-impact/feature.md"

func runRegisteredQK010(t *testing.T, ctx check.Context) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK010" {
			if r.Check != "docs.impact" || r.EnforcedFrom != check.Silver {
				t.Errorf("QK010 = check %q from %v, want docs.impact from silver", r.Check, r.EnforcedFrom)
			}
			return r.Run(ctx)
		}
	}
	t.Fatal("QK010 is not registered")
	return check.Result{}
}

func qk010Ctx(t *testing.T, files ...changes.File) check.Context {
	t.Helper()
	return check.Context{
		Root:     t.TempDir(),
		Manifest: manifest.Loaded{Present: true, Manifest: manifest.Manifest{Version: 1, Docs: manifest.Docs{ImpactMap: impactMap}}},
		Changes:  &changes.Set{Files: files},
	}
}

func mod(path string) changes.File { return changes.File{Path: path, Status: changes.Modified} }

func writeNote(t *testing.T, ctx check.Context, path, content string) {
	t.Helper()
	full := filepath.Join(ctx.Root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func approve(ctx *check.Context, path, reason string) {
	ctx.Waivers.Log = append(ctx.Waivers.Log, waivers.NewNoteEntry(path, reason, waivers.Approval{By: "a person"}))
}

func TestQK010Passes(t *testing.T) {
	tests := []struct {
		name  string
		files []changes.File
	}{
		{"nothing changed", nil},
		{"unmapped change", []changes.File{mod("internal/other/x.go")}},
		{"mapped change with its doc", []changes.File{mod("internal/config/load.go"), mod("docs/reference/settings.md")}},
		{"admin route with a matching how-to", []changes.File{mod("web/app/(admin)/users/page.tsx"), {Path: "docs/how-to/admin/users.md", Status: changes.Added}}},
		{"doc renamed counts as changed", []changes.File{mod("internal/config/load.go"), {Path: "docs/new.md", OldPath: "docs/reference/settings.md", Status: changes.Renamed}}},
		{"similar path is not mapped", []changes.File{mod("internal/configs/load.go")}},
		{"route folder without the parentheses is not mapped", []changes.File{mod("web/app/admin/users/page.tsx")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if res := runRegisteredQK010(t, qk010Ctx(t, tt.files...)); res.Status != check.Pass {
				t.Errorf("result = %+v, want pass", res)
			}
		})
	}
}

func TestQK010FailsWithOneFindingPerChangedFile(t *testing.T) {
	ctx := qk010Ctx(t, mod("internal/config/load.go"), mod("internal/config/sub/env.go"), mod("README.md"))

	res := runRegisteredQK010(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 2 {
		t.Fatalf("result = %+v, want fail with two findings", res)
	}
	f := res.Findings[0]
	if f.File != "internal/config/load.go" || !strings.Contains(f.Message, "docs/reference/settings.md") {
		t.Errorf("finding = %+v, want the code path as File and the expected doc named", f)
	}
}

func TestQK010CountsDeletedAndRenamedCode(t *testing.T) {
	tests := []struct {
		name string
		file changes.File
		want string
	}{
		{"deleted", changes.File{Path: "internal/config/old.go", Status: changes.Deleted}, "internal/config/old.go"},
		{"moved out of the mapped folder", changes.File{Path: "internal/other/x.go", OldPath: "internal/config/x.go", Status: changes.Renamed}, "internal/other/x.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runRegisteredQK010(t, qk010Ctx(t, tt.file))
			if res.Status != check.Fail || res.Findings[0].File != tt.want {
				t.Errorf("result = %+v", res)
			}
		})
	}
}

// The note and the log live in the change set; they must not count as code changes.
func TestQK010IgnoresQualityFolderAsCode(t *testing.T) {
	ctx := qk010Ctx(t, changes.File{Path: ".quality/exceptions-log.jsonl", Status: changes.Added})
	ctx.Manifest.Manifest.Docs.ImpactMap = []manifest.ImpactEntry{{Paths: []string{"**"}, Docs: []string{"docs/**"}}}

	if res := runRegisteredQK010(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v", res)
	}
}

func TestQK010PassesWithAnApprovedNote(t *testing.T) {
	ctx := qk010Ctx(t, mod("internal/config/load.go"), changes.File{Path: notePath, Status: changes.Added})
	writeNote(t, ctx, notePath, "Docs-Impact: none — internal refactor\n")
	approve(&ctx, notePath, "internal refactor")

	if res := runRegisteredQK010(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v", res)
	}
}

func TestQK010RejectsNotes(t *testing.T) {
	note := changes.File{Path: notePath, Status: changes.Added}
	tests := []struct {
		name    string
		content string
		logged  string // reason in the log; empty: no log line
		want    string
	}{
		{"no reason", "Docs-Impact: none — \n", "x", "no reason"},
		{"not a note at all", "hello\n", "x", "no reason"},
		{"written by hand, no approval", "Docs-Impact: none — trust me\n", "", "no approval"},
		{"edited after approval", "Docs-Impact: none — a different reason\n", "internal refactor", "no approval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := qk010Ctx(t, mod("internal/config/load.go"), note)
			writeNote(t, ctx, notePath, tt.content)
			if tt.logged != "" {
				approve(&ctx, notePath, tt.logged)
			}

			res := runRegisteredQK010(t, ctx)

			if res.Status != check.Fail {
				t.Fatalf("result = %+v, want fail", res)
			}
			last := res.Findings[len(res.Findings)-1]
			if last.File != notePath || !strings.Contains(last.Message, tt.want) {
				t.Errorf("finding = %+v, want %q on the note", last, tt.want)
			}
		})
	}
}

func TestQK010IgnoresNotesOutsideTheChangeSet(t *testing.T) {
	ctx := qk010Ctx(t, mod("internal/config/load.go"))
	writeNote(t, ctx, notePath, "Docs-Impact: none — from an earlier branch\n")
	approve(&ctx, notePath, "from an earlier branch")

	if res := runRegisteredQK010(t, ctx); res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("result = %+v, want fail with only the code finding", res)
	}
}

func TestQK010IgnoresADeletedNote(t *testing.T) {
	ctx := qk010Ctx(t, mod("internal/config/load.go"), changes.File{Path: notePath, Status: changes.Deleted})
	approve(&ctx, notePath, "was approved once")

	if res := runRegisteredQK010(t, ctx); res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("result = %+v, want fail with only the code finding", res)
	}
}

func TestQK010WithoutAnImpactMapPasses(t *testing.T) {
	ctx := check.Context{ChangesErr: errors.New("not a git repository")}

	if res := runRegisteredQK010(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v", res)
	}
}

func TestQK010SkipsWithoutAChangeSet(t *testing.T) {
	ctx := qk010Ctx(t)
	ctx.Changes, ctx.ChangesErr = nil, errors.New("not a git repository")

	res := runRegisteredQK010(t, ctx)

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "not a git repository") {
		t.Errorf("result = %+v", res)
	}
}
