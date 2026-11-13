package rules

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
)

const (
	onGitHub = "https://github.com/acme/app.git"
	onGitLab = "git@gitlab.com:acme/app.git"
)

func runRegisteredQK110(t *testing.T, ctx check.Context) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK110" {
			if r.Check != "ownership.codeowners" || r.EnforcedFrom != check.Gold {
				t.Errorf("QK110 = check %q from %v, want ownership.codeowners from gold", r.Check, r.EnforcedFrom)
			}
			return r.Run(ctx)
		}
	}
	t.Fatal("QK110 is not registered")
	return check.Result{}
}

// qk110Repo commits files in a fresh repository whose `origin` is remote; "" adds no remote.
func qk110Repo(t *testing.T, remote string, files map[string]string) check.Context {
	t.Helper()
	ctx := qk020Repo(t, files)
	if remote != "" {
		gitIn(t, ctx.Root, "remote", "add", "origin", remote)
	}
	return ctx
}

// unowned returns the files of the findings, or fails the test when the result is not a fail.
func unowned(t *testing.T, res check.Result) string {
	t.Helper()
	if res.Status != check.Fail {
		t.Fatalf("result = %+v, want fail", res)
	}
	var files []string
	for _, f := range res.Findings {
		files = append(files, f.File)
	}
	return strings.Join(files, " ")
}

func TestQK110FailsWithoutCodeowners(t *testing.T) {
	ctx := qk110Repo(t, onGitHub, map[string]string{"README.md": "x\n", "src/a.go": "package a\n"})

	res := runRegisteredQK110(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 {
		t.Fatalf("result = %+v, want fail with one finding", res)
	}
	if f := res.Findings[0]; f.File != ".github/CODEOWNERS" || !strings.Contains(f.Message, "`docs/CODEOWNERS`") {
		t.Errorf("finding = %+v, want it on .github/CODEOWNERS, naming where GitHub reads the file", f)
	}
}

func TestQK110PassesWhenEveryTopLevelPathHasAnOwner(t *testing.T) {
	for name, owners := range map[string]string{
		"a default owner": "* @acme/everyone\n",
		"a line per path": "# Owners\n/README.md @ann\n/src/ @acme/backend ben@example.com\n.github @ann\n",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := qk110Repo(t, onGitHub, map[string]string{"README.md": "x\n", "src/a.go": "package a\n", ".github/CODEOWNERS": owners})

			if res := runRegisteredQK110(t, ctx); res.Status != check.Pass {
				t.Errorf("result = %+v, want pass", res)
			}
		})
	}
}

// A folder has an owner when a new file in it would get one. `*.md` owns the
// Markdown files in docs/, not docs/.
func TestQK110FailsOnAGap(t *testing.T) {
	ctx := qk110Repo(t, onGitHub, map[string]string{
		"README.md":          "x\n",
		"LICENSE":            "x\n",
		"src/a.go":           "package a\n",
		"docs/guide.md":      "x\n",
		".github/CODEOWNERS": "/src/ @ann\n/.github/ @ann\n*.md @ann\n",
	})

	res := runRegisteredQK110(t, ctx)

	if got := unowned(t, res); got != "LICENSE docs" {
		t.Fatalf("findings on %q, want LICENSE and docs; result = %+v", got, res)
	}
	if msg := res.Findings[1].Message; !strings.Contains(msg, "`.github/CODEOWNERS`") {
		t.Errorf("message = %q, want it to name the CODEOWNERS file", msg)
	}
}

func TestQK110FailsWhenALaterLineRemovesOwnership(t *testing.T) {
	ctx := qk110Repo(t, onGitHub, map[string]string{
		"README.md":     "x\n",
		"docs/guide.md": "x\n",
		"CODEOWNERS":    "* @ann\n\n/docs/\n",
	})

	res := runRegisteredQK110(t, ctx)

	if got := unowned(t, res); got != "docs" {
		t.Fatalf("findings on %q, want docs; result = %+v", got, res)
	}
	if msg := res.Findings[0].Message; !strings.Contains(msg, "line 3") || !strings.Contains(msg, "`/docs/`") {
		t.Errorf("message = %q, want it to name line 3 and its pattern", msg)
	}
}

func TestQK110TheLastMatchingLineWins(t *testing.T) {
	ctx := qk110Repo(t, onGitHub, map[string]string{
		"docs/guide.md": "x\n",
		"CODEOWNERS":    "/docs/\n/CODEOWNERS\n* @ann #a default, after the lines without owners\n",
	})

	if res := runRegisteredQK110(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// The examples on GitHub's "About code owners" page, and where GitLab differs.
func TestCodeownersPatterns(t *testing.T) {
	tests := []struct {
		pattern, path string
		gitlab, want  bool
	}{
		{"*", "a/b/c.txt", false, true},
		{"*.js", "web/app/main.js", false, true},
		{"*.js", "web/app/main.jsx", false, false},
		{"/build/logs/", "build/logs/2026/a.log", false, true},
		{"/build/logs/", "old/build/logs/a.log", false, false},
		{"docs/*", "docs/getting-started.md", false, true},
		{"docs/*", "docs/build-app/troubleshooting.md", false, false},
		{"apps/", "apps/web/main.go", false, true},
		{"apps/", "services/apps/web/main.go", false, true},
		{"apps/", "apps", false, false}, // a file named apps is no folder
		{"/docs/", "docs/a/b.md", false, true},
		{"**/logs", "build/logs/a.log", false, true},
		{"**/logs", "deeply/nested/logs/a.log", false, true},
		{"**/logs", "logs/a.log", false, true},
		{"/apps/github", "apps/github/api/main.go", false, true},
		{"/apps/github", "apps/gitlab/main.go", false, false},
		{"/*", "README.md", false, true},
		{"/*", "src/a.go", false, false},
		{"/src/*/", "src/api/v1/a.go", false, true},
		{"/src/*/", "src/a.go", false, false},
		{"/docs/**", "docs/a/b.md", false, true},
		{"/My\\ Docs/", "My Docs/a.md", false, true},
		{"README.md", "docs/README.md", false, true},
		{"/README.md", "docs/README.md", false, false},
		{"/app/[id]/", "app/[id]/page.tsx", false, true}, // brackets are no character range
		{"/app/[id]/", "app/i/page.tsx", false, false},
		{"readme.md", "README.md", false, false}, // case matters
		{"/", "README.md", false, false},
		// A path with a slash in the middle starts at the root on GitHub, anywhere on GitLab.
		{"docs/README.md", "web/docs/README.md", false, false},
		{"docs/README.md", "web/docs/README.md", true, true},
		{"docs/README.md", "docs/README.md", true, true},
	}
	for _, tt := range tests {
		lines := parseCodeowners([]byte(tt.pattern+" @ann\n"), tt.gitlab)
		if got := ownerLineFor(lines, tt.path) != nil; got != tt.want {
			t.Errorf("pattern %q, path %q, gitlab %v: matches = %v, want %v", tt.pattern, tt.path, tt.gitlab, got, tt.want)
		}
	}
}

// GitHub skips a line it cannot read, so the line takes no ownership away.
func TestQK110SkipsALineWithoutAValidOwner(t *testing.T) {
	ctx := qk110Repo(t, onGitHub, map[string]string{
		"docs/guide.md": "x\n",
		"CODEOWNERS":    "* @ann\n/docs/ someone\n!/CODEOWNERS\n",
	})

	if res := runRegisteredQK110(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK110ReadsTheFirstCodeownersTheHostFinds(t *testing.T) {
	everything, nothing := "* @ann\n", "/nothing @ann\n"
	tests := []struct {
		name, remote string
		files        map[string]string
		want         check.Status
	}{
		{"GitHub: .github before the root", onGitHub, map[string]string{".github/CODEOWNERS": everything, "CODEOWNERS": nothing}, check.Pass},
		{"GitHub: the root before docs", onGitHub, map[string]string{"CODEOWNERS": nothing, "docs/CODEOWNERS": everything}, check.Fail},
		{"GitHub: docs", "ssh://git@github.example.com:2222/acme/app.git", map[string]string{"docs/CODEOWNERS": everything}, check.Pass},
		{"GitHub does not read .gitlab", onGitHub, map[string]string{".gitlab/CODEOWNERS": everything}, check.Fail},
		{"GitLab: the root before .gitlab", onGitLab, map[string]string{"CODEOWNERS": everything, ".gitlab/CODEOWNERS": nothing}, check.Pass},
		{"GitLab: docs before .gitlab", "https://gitlab.example.com/acme/app.git", map[string]string{"docs/CODEOWNERS": nothing, ".gitlab/CODEOWNERS": everything}, check.Fail},
		{"GitLab does not read .github", onGitLab, map[string]string{".github/CODEOWNERS": everything}, check.Fail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if res := runRegisteredQK110(t, qk110Repo(t, tt.remote, tt.files)); res.Status != tt.want {
				t.Errorf("result = %+v, want %s", res, tt.want)
			}
		})
	}
}

// On GitLab a section header can name default owners for its lines.
func TestQK110ReadsGitLabSections(t *testing.T) {
	ctx := qk110Repo(t, onGitLab, map[string]string{
		"README.md":     "x\n",
		"src/a.go":      "package a\n",
		"docs/guide.md": "x\n",
		"ops/deploy.sh": "x\n",
		"CODEOWNERS": "[Team docs] @acme/docs\n/docs/\n/README.md\n" +
			"^[Backend][2] @acme/backend @ann\n/src/\n/CODEOWNERS @ann\n" +
			"[Ops]\n/ops/\n",
	})

	if got := unowned(t, runRegisteredQK110(t, ctx)); got != "ops" {
		t.Errorf("findings on %q, want ops", got)
	}
}

func TestQK110SkipsOnAHostItDoesNotKnow(t *testing.T) {
	files := map[string]string{"README.md": "x\n"}

	res := runRegisteredQK110(t, qk110Repo(t, "https://ann:s3cret@bitbucket.org/acme/app.git", files))
	if res.Status != check.Skipped || !strings.Contains(res.Reason, "bitbucket.org") || strings.Contains(res.Reason, "s3cret") {
		t.Errorf("result = %+v, want skipped, naming the host and not the password", res)
	}

	res = runRegisteredQK110(t, qk110Repo(t, "", files))
	if res.Status != check.Skipped || !strings.Contains(res.Reason, "`origin`") {
		t.Errorf("without a remote: result = %+v, want skipped, naming origin", res)
	}
}

// With --changed, the Stop hook reports what this change did: a new top-level
// path without an owner, or a change to CODEOWNERS. An old gap is left alone.
func TestQK110WithChangedOnlyLooksAtNewTopLevelPaths(t *testing.T) {
	ctx := qk110Repo(t, onGitHub, map[string]string{
		"scripts/build.sh": "x\n",
		"src/a.go":         "package a\n",
		"CODEOWNERS":       "/src/ @ann\n/CODEOWNERS @ann\n",
	})
	ctx.ChangedOnly = true
	put(t, ctx.Root, "scripts/build.sh", "y\n")
	put(t, ctx.Root, "src/b.go", "package a\n")

	if res := runRegisteredQK110(t, changed(t, ctx)); res.Status != check.Pass {
		t.Fatalf("old gap: result = %+v, want pass", res)
	}

	put(t, ctx.Root, "tools/gen.go", "package main\n")
	if err := os.Mkdir(filepath.Join(ctx.Root, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitIn(t, ctx.Root, "mv", "src/a.go", "lib/a.go")
	put(t, ctx.Root, "NOTES.md", "x\n")
	if got := unowned(t, runRegisteredQK110(t, changed(t, ctx))); got != "NOTES.md lib tools" {
		t.Errorf("new paths: findings on %q, want NOTES.md, lib and tools", got)
	}
}

func TestQK110WithChangedOnlyChecksEverythingWhenCodeownersChanges(t *testing.T) {
	ctx := qk110Repo(t, onGitHub, map[string]string{
		"scripts/build.sh": "x\n",
		"CODEOWNERS":       "* @ann\n",
	})
	ctx.ChangedOnly = true
	put(t, ctx.Root, "CODEOWNERS", "/CODEOWNERS @ann\n")

	if got := unowned(t, runRegisteredQK110(t, changed(t, ctx))); got != "scripts" {
		t.Errorf("CODEOWNERS edited: findings on %q, want scripts", got)
	}

	if err := os.Remove(filepath.Join(ctx.Root, "CODEOWNERS")); err != nil {
		t.Fatal(err)
	}
	if got := unowned(t, runRegisteredQK110(t, changed(t, ctx))); got != ".github/CODEOWNERS" {
		t.Errorf("CODEOWNERS deleted: findings on %q, want .github/CODEOWNERS", got)
	}
}

// A repository that never had a CODEOWNERS file did not lose it in this change,
// and without a change set nothing can be put down to this change.
func TestQK110WithChangedOnlyIsQuietWhenTheChangeDidNothing(t *testing.T) {
	ctx := qk110Repo(t, "", map[string]string{"README.md": "x\n"})
	ctx.ChangedOnly = true
	put(t, ctx.Root, "tools/gen.go", "package main\n")

	if res := runRegisteredQK110(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("no CODEOWNERS before or after: result = %+v, want pass", res)
	}

	ctx.ChangesErr = errors.New("the repository has no commits yet")
	if res := runRegisteredQK110(t, ctx); res.Status != check.Pass {
		t.Errorf("without a change set: result = %+v, want pass", res)
	}
}

func TestQK110PassesOutsideAGitRepository(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	ctx := check.Context{Root: t.TempDir(), ChangesErr: errors.New("not a git repository")}
	put(t, ctx.Root, "README.md", "x\n")

	if res := runRegisteredQK110(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}
