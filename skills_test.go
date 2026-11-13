package qualitykit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// relativeLink matches the target of a Markdown link that stays inside the skill folder.
var relativeLink = regexp.MustCompile(`\]\(([^)#:]+)\)`)

// Every skill ships with what CLAUDE.md asks of one: a SKILL.md named after
// its folder, with a description that fits Claude Code's skill listing, links
// that lead to a file, and at least three eval cases with graders.
func TestEverySkillHasItsParts(t *testing.T) {
	const plugin = "plugins/quality-kit"
	// Claude Code cuts the description off here in the listing it chooses skills from.
	const listingLimit = 1536

	skills, err := filepath.Glob(plugin + "/skills/*/SKILL.md")
	if err != nil || len(skills) == 0 {
		t.Fatalf("no skills under %s/skills (err = %v)", plugin, err)
	}
	for _, file := range skills {
		dir := filepath.Dir(file)
		name := filepath.Base(dir)
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		// Claude Code reads frontmatter only when it opens the file.
		rest, ok := strings.CutPrefix(string(raw), "---\n")
		front, body, closed := strings.Cut(rest, "\n---\n")
		if !ok || !closed {
			t.Errorf("%s: no frontmatter on the first line", file)
			continue
		}
		var fm struct{ Name, Description string }
		if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
			t.Errorf("%s: frontmatter: %v", file, err)
			continue
		}
		if fm.Name != name {
			t.Errorf("%s: name is %q, want the folder name %q", file, fm.Name, name)
		}
		if n := len(fm.Description); n == 0 || n > listingLimit {
			t.Errorf("%s: description has %d characters, want 1 to %d", file, n, listingLimit)
		}
		for _, link := range relativeLink.FindAllStringSubmatch(body, -1) {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(link[1]))); err != nil {
				t.Errorf("%s: link to %q leads nowhere", file, link[1])
			}
		}

		cases, _ := filepath.Glob(plugin + "/evals/" + name + "-*/prompt.md")
		if len(cases) < 3 {
			t.Errorf("%s: %d eval cases named %s-*, want at least 3", file, len(cases), name)
		}
		for _, prompt := range cases {
			if graders, _ := filepath.Glob(filepath.Join(filepath.Dir(prompt), "graders", "*.md")); len(graders) == 0 {
				t.Errorf("%s: no graders", filepath.Dir(prompt))
			}
		}
	}
}
