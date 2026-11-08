package changes

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// isolate keeps the user's and the system's git config (signing, hooks,
// autocrlf, default branch) and any enclosing repo out of the test.
func isolate(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, v := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"} {
		t.Setenv(v, "")
		os.Unsetenv(v)
	}
}

func newRepo(t *testing.T, branch string) string {
	t.Helper()
	isolate(t)
	dir := t.TempDir()
	run(t, dir, "init", "-q", "-b", branch)
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Test")
	return dir
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, msg string) string {
	t.Helper()
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", msg)
	return run(t, dir, "rev-parse", "HEAD")
}

func paths(s *Set) map[string]File {
	m := map[string]File{}
	for _, f := range s.Files {
		m[f.Path] = f
	}
	return m
}

func TestRootFromSubfolder(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "a/b/c.txt", "x")
	commit(t, dir, "init")

	got, err := Root(filepath.Join(dir, "a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	gotInfo, err1 := os.Stat(got)
	wantInfo, err2 := os.Stat(dir)
	if err1 != nil || err2 != nil || !os.SameFile(gotInfo, wantInfo) {
		t.Errorf("Root = %q, want the repo at %q", got, dir)
	}
}

func TestNotARepo(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if _, err := Root(dir); !errors.Is(err, ErrNotRepo) {
		t.Errorf("Root error = %v, want ErrNotRepo", err)
	}
	if _, err := Compute(dir, ""); !errors.Is(err, ErrNotRepo) {
		t.Errorf("Compute error = %v, want ErrNotRepo", err)
	}
}

func TestNoCommitsYet(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "a.txt", "x")

	if _, err := Compute(dir, ""); !errors.Is(err, ErrNoCommits) {
		t.Errorf("Compute error = %v, want ErrNoCommits", err)
	}
}

// main moves on after the branch was cut; those commits are not this branch's changes.
func TestDefaultBaseIsMergeBaseWithMain(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "a.txt", "1")
	base := commit(t, dir, "A")
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "a.txt", "2")
	write(t, dir, "b.txt", "new")
	commit(t, dir, "B")
	run(t, dir, "checkout", "-q", "main")
	write(t, dir, "c.txt", "later on main")
	commit(t, dir, "C")
	run(t, dir, "checkout", "-q", "feature")

	s, err := Compute(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	if s.Base != base {
		t.Errorf("base = %s, want merge-base %s", s.Base, base)
	}
	want := map[string]File{
		"a.txt": {Path: "a.txt", Status: Modified},
		"b.txt": {Path: "b.txt", Status: Added},
	}
	if got := paths(s); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %+v, want %+v", got, want)
	}
}

func TestDefaultBranchFallsBackToMaster(t *testing.T) {
	dir := newRepo(t, "master")
	write(t, dir, "a.txt", "1")
	base := commit(t, dir, "A")
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "a.txt", "2")
	commit(t, dir, "B")

	s, err := Compute(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Base != base {
		t.Errorf("base = %s, want %s", s.Base, base)
	}
}

// On the default branch itself, origin/HEAD makes unpushed commits count.
func TestDefaultBranchPrefersOriginHEAD(t *testing.T) {
	dir := newRepo(t, "trunk")
	write(t, dir, "a.txt", "1")
	pushed := commit(t, dir, "A")
	run(t, dir, "update-ref", "refs/remotes/origin/trunk", pushed)
	run(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	write(t, dir, "a.txt", "2")
	commit(t, dir, "unpushed")

	s, err := Compute(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if s.Base != pushed {
		t.Errorf("base = %s, want origin/trunk %s", s.Base, pushed)
	}
	if got := paths(s); got["a.txt"].Status != Modified {
		t.Errorf("files = %+v, want a.txt modified by the unpushed commit", got)
	}
}

func TestNoDefaultBranch(t *testing.T) {
	dir := newRepo(t, "trunk")
	write(t, dir, "a.txt", "1")
	commit(t, dir, "A")

	if _, err := Compute(dir, ""); !errors.Is(err, ErrNoDefaultBranch) {
		t.Errorf("Compute error = %v, want ErrNoDefaultBranch", err)
	}
}

// An explicit base is also a merge-base: a three-dot diff, as in a pull request.
func TestExplicitBase(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "a.txt", "1")
	commit(t, dir, "A")
	run(t, dir, "checkout", "-q", "-b", "release")
	write(t, dir, "r.txt", "release only")
	commit(t, dir, "R")
	run(t, dir, "checkout", "-q", "main")
	write(t, dir, "b.txt", "2")
	b := commit(t, dir, "B")
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "c.txt", "3")
	commit(t, dir, "C")

	s, err := Compute(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Base != b || len(s.Files) != 1 || s.Files[0].Path != "c.txt" {
		t.Errorf("base main: %+v, want base %s and only c.txt", s, b)
	}

	s, err = Compute(dir, "release")
	if err != nil {
		t.Fatal(err)
	}
	got := paths(s)
	if _, ok := got["r.txt"]; ok {
		t.Errorf("files = %+v: r.txt is only on release and must not appear", got)
	}
	if _, ok := got["b.txt"]; !ok {
		t.Errorf("files = %+v, want b.txt (after the merge-base with release)", got)
	}
}

func TestExplicitBaseThatDoesNotResolve(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "a.txt", "1")
	commit(t, dir, "A")

	_, err := Compute(dir, "no-such-ref")
	if !errors.Is(err, ErrBadBase) || !strings.Contains(err.Error(), "no-such-ref") {
		t.Errorf("Compute error = %v, want ErrBadBase naming the ref", err)
	}
}

func TestRenameKeepsOldAndNewPath(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "docs/old.md", "same content that is long enough to be detected as a rename\n")
	commit(t, dir, "A")
	run(t, dir, "checkout", "-q", "-b", "feature")
	run(t, dir, "mv", "docs/old.md", "docs/new.md")
	commit(t, dir, "rename")

	s, err := Compute(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []File{{Path: "docs/new.md", OldPath: "docs/old.md", Status: Renamed}}
	if !reflect.DeepEqual(s.Files, want) {
		t.Errorf("files = %+v, want %+v", s.Files, want)
	}
}

// The Stop hook runs before Claude commits, so work in the tree counts.
func TestUncommittedAndUntrackedWorkCounts(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, ".gitignore", "*.log\n")
	write(t, dir, "keep.txt", "1")
	write(t, dir, "gone.txt", "1")
	write(t, dir, "edit.txt", "1")
	commit(t, dir, "A")
	run(t, dir, "checkout", "-q", "-b", "feature")

	write(t, dir, "edit.txt", "2")     // unstaged modify
	write(t, dir, "staged.txt", "new") // staged add
	run(t, dir, "add", "staged.txt")
	os.Remove(filepath.Join(dir, "gone.txt"))                     // delete
	write(t, dir, "docs/how-to/kvalvågnes notes.md", "untracked") // untracked, nested, space, non-ASCII
	write(t, dir, "debug.log", "ignored")                         // ignored

	s, err := Compute(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]File{
		"edit.txt":                        {Path: "edit.txt", Status: Modified},
		"staged.txt":                      {Path: "staged.txt", Status: Added},
		"gone.txt":                        {Path: "gone.txt", Status: Deleted},
		"docs/how-to/kvalvågnes notes.md": {Path: "docs/how-to/kvalvågnes notes.md", Status: Added},
	}
	if got := paths(s); !reflect.DeepEqual(got, want) {
		t.Errorf("files = %+v, want %+v", got, want)
	}
}

func TestTrailersComeOnlyFromCommitsAfterTheBase(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "a.txt", "1")
	commit(t, dir, "A\n\nDocs-Impact: none — before the base")
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "a.txt", "2")
	c := commit(t, dir, "refactor: move code\n\nBody text.\n\nDocs-Impact: none — internal refactor\nRefs: CCT-1")

	s, err := Compute(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []Trailer{
		{Commit: c, Key: "Docs-Impact", Value: "none — internal refactor"},
		{Commit: c, Key: "Refs", Value: "CCT-1"},
	}
	if !reflect.DeepEqual(s.Trailers, want) {
		t.Errorf("trailers = %+v, want %+v", s.Trailers, want)
	}
}

func TestContentAtBase(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "api/openapi.yaml", "openapi: 3.1.0\n")
	commit(t, dir, "A")
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "api/openapi.yaml", "openapi: 3.1.1\n")
	write(t, dir, "api/new.yaml", "new\n")
	commit(t, dir, "B")

	s, err := Compute(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.ContentAtBase("api/openapi.yaml")
	if err != nil || !ok || string(got) != "openapi: 3.1.0\n" {
		t.Errorf("ContentAtBase(openapi.yaml) = %q, %v, %v", got, ok, err)
	}
	if _, ok, err := s.ContentAtBase("api/new.yaml"); err != nil || ok {
		t.Errorf("ContentAtBase(new.yaml) = ok %v, err %v; want not found", ok, err)
	}
}
