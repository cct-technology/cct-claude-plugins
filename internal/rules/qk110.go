package rules

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/glob"
)

func qk110() check.Rule {
	return check.Rule{
		ID:           "QK110",
		Check:        "ownership.codeowners",
		Title:        "Every top-level path has a code owner",
		Phase:        1,
		EnforcedFrom: check.Gold,
		Why: "Every finding needs someone to act on it, and every change someone to review it. QK110 checks that each file and folder at the top of the repository has an owner in `CODEOWNERS`. " +
			"It reads the file where the host of the `origin` remote reads it. GitHub: `.github/CODEOWNERS`, `CODEOWNERS`, then `docs/CODEOWNERS`. GitLab: `CODEOWNERS`, `docs/CODEOWNERS`, then `.gitlab/CODEOWNERS`. The first one found counts. " +
			"Patterns are matched as the host matches them, and the last matching line wins, so a later line without owners takes ownership away. " +
			"A top-level file has an owner when it has one itself; a top-level folder has one when a new file in it would get one. " +
			"On another host, or without an `origin` remote, the check is skipped and says so. " +
			"With `--changed`, only top-level paths the change adds are checked, unless the change touches `CODEOWNERS`.",
		Fix: "Add a line for the path to `CODEOWNERS`, such as `/docs/ @your-team`, or give everything a default owner with `* @your-team` on the first line. If a line without owners comes later in the file, give it an owner or remove it. If a path should have no owner, ask a person to approve a waiver with `qk waiver add QK110`.",
		Run: runQK110,
	}
}

// ownerHost is a git host that reads CODEOWNERS.
type ownerHost struct {
	name, mark string   // mark is what the remote's host name contains
	locations  []string // where it reads CODEOWNERS, first found first
}

var ownerHosts = []ownerHost{
	{"GitHub", "github", []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"}},
	{"GitLab", "gitlab", []string{"CODEOWNERS", "docs/CODEOWNERS", ".gitlab/CODEOWNERS"}},
}

func isCodeowners(file string) bool {
	return slices.ContainsFunc(ownerHosts, func(h ownerHost) bool { return slices.Contains(h.locations, file) })
}

func runQK110(ctx check.Context) check.Result {
	pass := check.Result{Status: check.Pass}
	tree, err := changes.Tree(ctx.Root)
	if err != nil {
		return pass // outside git there are no tracked paths to own, and a skip would show after every turn
	}
	isFolder := map[string]bool{} // top-level path -> it is a folder
	for _, file := range tree {
		top, _, nested := strings.Cut(file, "/")
		isFolder[top] = isFolder[top] || nested
	}
	tops := slices.Sorted(maps.Keys(isFolder))

	// With --changed, report what this change did: a top-level path it added,
	// or everything when it touched CODEOWNERS. An old gap is left alone.
	everything := !ctx.ChangedOnly
	if ctx.ChangedOnly {
		if ctx.Changes == nil {
			return pass // nothing can be put down to this change
		}
		added := map[string]bool{}
		for _, f := range ctx.Changes.Files {
			if isCodeowners(f.Path) || isCodeowners(f.OldPath) {
				everything = true
			}
			if f.Status == changes.Added || f.Status == changes.Renamed && topOf(f.OldPath) != topOf(f.Path) {
				added[f.Path] = true
			}
		}
		if !everything {
			if !slices.ContainsFunc(tree, isCodeowners) {
				return pass // no CODEOWNERS before or after
			}
			old := map[string]bool{} // top-level paths that hold a file from before the change
			for _, file := range tree {
				if !added[file] {
					old[topOf(file)] = true
				}
			}
			tops = slices.DeleteFunc(tops, func(top string) bool { return old[top] })
		}
	}
	if len(tops) == 0 {
		return pass
	}

	url, ok := changes.OriginURL(ctx.Root)
	if !ok {
		return check.Result{Status: check.Skipped, Reason: "no remote named `origin`, so QK110 cannot tell which host reads CODEOWNERS"}
	}
	host := remoteHost(url)
	at := slices.IndexFunc(ownerHosts, func(h ownerHost) bool { return strings.Contains(host, h.mark) })
	if at < 0 {
		return check.Result{Status: check.Skipped, Reason: fmt.Sprintf(
			"remote `origin` is on `%s`, not GitHub or GitLab: QK110 does not know where that host reads CODEOWNERS", host)}
	}
	known := ownerHosts[at]

	at = slices.IndexFunc(known.locations, func(l string) bool { return slices.Contains(tree, l) })
	if at < 0 {
		if !everything {
			return pass // the only CODEOWNERS is where this host does not read it, as before the change
		}
		return check.Result{Status: check.Fail, Findings: []check.Finding{{File: known.locations[0], Message: fmt.Sprintf(
			"no CODEOWNERS file: %s reads %s", known.name, oneOf(known.locations))}}}
	}
	file := known.locations[at]
	data, err := os.ReadFile(filepath.Join(ctx.Root, filepath.FromSlash(file)))
	if err != nil {
		return check.Result{Status: check.Skipped, Reason: err.Error()}
	}
	lines := parseCodeowners(data, known.name == "GitLab")

	var findings []check.Finding
	for _, top := range tops {
		probe, what := top, "matches this file"
		if isFolder[top] {
			// A folder has an owner when a new file in it would get one: a name no pattern spells out.
			probe, what = top+"/\x00", "covers this folder"
		}
		switch line := ownerLineFor(lines, probe); {
		case line == nil:
			findings = append(findings, check.Finding{File: top, Message: fmt.Sprintf("no owner: no line in `%s` %s", file, what)})
		case !line.owned:
			findings = append(findings, check.Finding{File: top, Message: fmt.Sprintf(
				"no owner: line %d of `%s` (`%s`) names none", line.number, file, line.pattern)})
		}
	}
	if len(findings) > 0 {
		return check.Result{Status: check.Fail, Findings: findings}
	}
	return pass
}

func topOf(file string) string {
	top, _, _ := strings.Cut(file, "/")
	return top
}

// remoteHost returns the lower-case host name of a git remote URL, in the
// `https://host/path`, `ssh://user@host:port/path` and `user@host:path` forms.
func remoteHost(url string) string {
	if _, rest, ok := strings.Cut(url, "://"); ok {
		url = rest
	}
	host, _, _ := strings.Cut(url, "/")
	host = host[strings.LastIndex(host, "@")+1:] // drop the user, and a password if the URL holds one
	host, _, _ = strings.Cut(host, ":")
	return strings.ToLower(host)
}

// ownerLine is one pattern line of a CODEOWNERS file.
type ownerLine struct {
	number  int
	pattern string   // as written
	globs   []string // the internal/glob patterns it stands for
	owned   bool     // it names an owner
}

// gitlabSection matches a GitLab section header: `[Name]`, `^[Name]` or `[Name][2]`.
var gitlabSection = regexp.MustCompile(`^\^?\[[^\]]+\](\[\d+\])?`)

// parseCodeowners reads the pattern lines of a CODEOWNERS file.
// ponytail: GitLab sections are read as one list, so a `!` exclusion or a line
// without owners in one section takes away an owner another section gives.
// Keep the sections apart if that shows up.
func parseCodeowners(data []byte, gitlab bool) []ownerLine {
	var lines []ownerLine
	sectionOwned := false // GitLab: the section header names default owners
	for i, text := range strings.Split(string(data), "\n") {
		text = strings.TrimSpace(text)
		header := ""
		if gitlab {
			header = gitlabSection.FindString(text)
		}
		// `\ ` is a space inside a path, not the end of the pattern.
		fields := strings.Fields(strings.ReplaceAll(strings.TrimPrefix(text, header), `\ `, "\x00"))
		if at := slices.IndexFunc(fields, func(f string) bool { return strings.HasPrefix(f, "#") }); at >= 0 {
			fields = fields[:at] // a comment
		}
		notOwner := func(f string) bool { return !strings.Contains(f, "@") }
		if header != "" {
			sectionOwned = len(fields) > 0 && !slices.ContainsFunc(fields, notOwner)
			continue
		}
		if len(fields) == 0 || slices.ContainsFunc(fields[1:], notOwner) {
			continue // empty, or a line the host skips because an owner is not valid
		}
		line := ownerLine{number: i + 1, pattern: strings.ReplaceAll(fields[0], "\x00", " "), owned: len(fields) > 1 || sectionOwned}
		match := line.pattern
		if gitlab && strings.HasPrefix(match, "!") {
			match, line.owned = match[1:], false // an exclusion
		}
		line.globs = ownerGlobs(match, gitlab)
		lines = append(lines, line)
	}
	return lines
}

// ownerGlobs turns a CODEOWNERS pattern into internal/glob patterns. The rules
// are gitignore's, with the differences GitHub documents: no `!`, no `[ ]`
// ranges, and `docs/*` does not reach below `docs/`.
func ownerGlobs(pattern string, gitlab bool) []string {
	p := strings.TrimSuffix(pattern, "/")
	folderOnly := p != pattern
	rooted := strings.HasPrefix(p, "/")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	// A name without a slash matches at any depth. On GitLab so does every path without a leading slash.
	if !rooted && (gitlab || !strings.Contains(p, "/")) {
		p = "**/" + p
	}
	var globs []string
	if !folderOnly {
		globs = append(globs, p) // the path itself
	}
	if folderOnly || path.Base(p) != "*" {
		globs = append(globs, p+"/**/*") // everything below a folder it matches
	}
	return globs
}

// ownerLineFor returns the last line that matches file, or nil.
func ownerLineFor(lines []ownerLine, file string) *ownerLine {
	for i := len(lines) - 1; i >= 0; i-- {
		if slices.ContainsFunc(lines[i].globs, func(g string) bool { return glob.Match(g, file) }) {
			return &lines[i]
		}
	}
	return nil
}
