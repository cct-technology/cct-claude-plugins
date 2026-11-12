package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/docsimpact"
	"github.com/rkvalvaa/cct-claude-plugins/internal/glob"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

func qk010() check.Rule {
	return check.Rule{
		ID:           "QK010",
		Check:        "docs.impact",
		Title:        "Docs change with the code they describe",
		Phase:        1,
		EnforcedFrom: check.Silver,
		Why: "When a changed file matches `paths` in an `impact_map` entry, at least one file matching that entry's `docs` must change on the same branch. " +
			"A change that truly needs no docs can carry a note written by `qk docs-impact none`. The note counts only while it is part of the change and matches a person's approval in `" + waivers.LogName + "`, so a hand-written or later-edited note does not count.",
		Fix: "Update the mapped docs. If this change needs none, ask a person to approve `qk docs-impact none --reason <why> --approved-by <person>`.",
		Run: runQK010,
	}
}

func runQK010(ctx check.Context) check.Result {
	entries := ctx.Manifest.Manifest.Docs.ImpactMap
	if len(entries) == 0 {
		return check.Result{Status: check.Pass}
	}
	if ctx.Changes == nil {
		return noChangeSet(ctx)
	}

	findings := missingDocs(entries, ctx.Changes.Files)
	if len(findings) == 0 {
		return check.Result{Status: check.Pass}
	}
	approved, noteProblems := readNotes(ctx)
	if approved {
		return check.Result{Status: check.Pass}
	}
	return check.Result{Status: check.Fail, Findings: append(findings, noteProblems...)}
}

// missingDocs reports each changed mapped file, one finding per file so a
// waiver can cover some of them by path.
func missingDocs(entries []manifest.ImpactEntry, files []changes.File) []check.Finding {
	var out []check.Finding
	for _, e := range entries {
		if anyFileMatches(e.Docs, files) {
			continue
		}
		for _, f := range files {
			// The note and the log are part of the change set, but they are not code.
			if !strings.HasPrefix(f.Path, ".quality/") && matches(e.Paths, f) {
				out = append(out, check.Finding{File: f.Path, Message: "changed, but none of its docs did; expected a change in " + strings.Join(e.Docs, ", ")})
			}
		}
	}
	return out
}

func anyFileMatches(globs []string, files []changes.File) bool {
	for _, f := range files {
		if matches(globs, f) {
			return true
		}
	}
	return false
}

func matches(globs []string, f changes.File) bool {
	for _, g := range globs {
		if glob.Match(g, f.Path) || (f.OldPath != "" && glob.Match(g, f.OldPath)) {
			return true
		}
	}
	return false
}

// readNotes looks only at notes in the change set, found by folder rather than
// branch name: CI checks out a detached commit, where the branch name is unknown.
func readNotes(ctx check.Context) (approved bool, problems []check.Finding) {
	for _, f := range ctx.Changes.Files {
		if !strings.HasPrefix(f.Path, docsimpact.Dir) || f.Status == changes.Deleted {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ctx.Root, filepath.FromSlash(f.Path)))
		if err != nil {
			problems = append(problems, check.Finding{File: f.Path, Message: err.Error()})
			continue
		}
		reason := docsimpact.Reason(data)
		switch {
		case reason == "":
			problems = append(problems, check.Finding{File: f.Path, Message: "no reason in this note"})
		case !ctx.Waivers.NoteApproved(f.Path, reason):
			problems = append(problems, check.Finding{File: f.Path, Message: fmt.Sprintf("no approval in %s for this note", waivers.LogName)})
		default:
			return true, nil
		}
	}
	return false, problems
}
