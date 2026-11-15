package qualitykit_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
)

var (
	// docLink matches a Markdown link that stays in the repository: a path, an anchor, or both.
	docLink = regexp.MustCompile(`\]\(([^)#:\s]*)(?:#([^)\s]*))?\)`)
	heading = regexp.MustCompile(`(?m)^#{1,6} +(.+)$`)
	// notInSlug is what GitHub drops from a heading to make its anchor.
	notInSlug = regexp.MustCompile(`[^\p{L}\p{N} _-]`)
)

// anchors returns the anchors GitHub gives the headings of a Markdown page.
// ponytail: no -1 suffix for a repeated heading; add it when a page links to one.
func anchors(page string) map[string]bool {
	found := map[string]bool{}
	for _, h := range heading.FindAllStringSubmatch(page, -1) {
		slug := notInSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(h[1])), "")
		found[strings.ReplaceAll(slug, " ", "-")] = true
	}
	return found
}

// The README and the docs are what a new team reads first, and no check reads
// them yet (QK013 is phase 2). So: every link leads to a file and a heading,
// the how-to index lists every how-to, and the README names every rule, with
// the level it blocks from, and every skill.
func TestDocsHoldTogether(t *testing.T) {
	pages := map[string]string{}
	read := func(path string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		pages[filepath.ToSlash(path)] = string(raw)
	}
	read("README.md")
	err := filepath.WalkDir("docs", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
			read(path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	for page, text := range pages {
		for _, link := range docLink.FindAllStringSubmatch(text, -1) {
			target := page
			if link[1] != "" {
				target = filepath.ToSlash(filepath.Join(filepath.Dir(page), link[1]))
			}
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s: link to %q leads nowhere", page, link[1])
				continue
			}
			if link[2] == "" {
				continue
			}
			to, isPage := pages[target]
			if isPage && !anchors(to)[link[2]] {
				t.Errorf("%s: %s has no heading for #%s", page, target, link[2])
			}
		}
	}

	index := pages["docs/how-to/README.md"]
	for page := range pages {
		name, isHowTo := strings.CutPrefix(page, "docs/how-to/")
		if isHowTo && name != "README.md" && !strings.Contains(index, "("+name+")") {
			t.Errorf("docs/how-to/README.md does not link to %s", name)
		}
	}

	readme := pages["README.md"]
	for _, r := range rules.All() {
		row := regexp.MustCompile(`(?m)^\| ` + r.ID + ` \|.*$`).FindString(readme)
		if !strings.Contains(row, "| "+r.EnforcedFrom.String()+" |") {
			t.Errorf("README.md: no row for %s that says it blocks from %s", r.ID, r.EnforcedFrom)
		}
	}
	skills, _ := filepath.Glob("plugins/quality-kit/skills/*/SKILL.md")
	for _, file := range skills {
		if name := filepath.Base(filepath.Dir(file)); !strings.Contains(readme, "| `"+name+"` |") {
			t.Errorf("README.md: no row for the skill %s", name)
		}
	}
}

// The PreToolUse hook refuses an edit to a generated file. A file that only
// talks about the header, such as the code that writes it or a how-to that
// quotes it, must stay editable: in this repository only the two reference
// pages and their golden copies are generated.
func TestOnlyTheReferencePagesCountAsGenerated(t *testing.T) {
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		t.Skip("not a git checkout:", err)
	}
	m := manifest.Load(".").Manifest
	var generated []string
	for _, file := range strings.Fields(string(out)) {
		if _, ok := m.Generator(".", file); ok {
			generated = append(generated, file)
		}
	}
	// The golden files are copies of generated pages, header and all, so they count too.
	want := []string{
		"docs/reference/manifest.md", "docs/reference/rules.md",
		"internal/check/testdata/reference-empty.golden", "internal/check/testdata/reference.golden",
		"internal/manifest/testdata/reference/reference.golden",
	}
	if !slices.Equal(generated, want) {
		t.Errorf("generated files = %v, want %v", generated, want)
	}
}
