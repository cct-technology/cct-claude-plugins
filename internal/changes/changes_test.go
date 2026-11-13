package changes

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
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

func TestIdentity(t *testing.T) {
	dir := newRepo(t, "main")

	if got := Identity(dir); got != "Test <test@example.com>" {
		t.Errorf("Identity = %q", got)
	}
}

func TestIdentityOutsideGitIsEmpty(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if got := Identity(dir); got != "" {
		t.Errorf("Identity = %q, want empty", got)
	}
}

func TestBranch(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, "a.txt", "1")
	c := commit(t, dir, "A")
	run(t, dir, "checkout", "-q", "-b", "rk/feature")

	if got := Branch(dir); got != "rk/feature" {
		t.Errorf("Branch = %q", got)
	}

	run(t, dir, "checkout", "-q", "--detach")
	if got := Branch(dir); got == "" || !strings.HasPrefix(c, got) {
		t.Errorf("detached Branch = %q, want a short form of %s", got, c)
	}
}

func read(t *testing.T, dir, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func TestRestoreWorktreeUndoesWhatACommandDid(t *testing.T) {
	dir := newRepo(t, "main")
	for _, p := range []string{"clean.txt", "edited.txt", "removed-later.txt", "removed-before.txt"} {
		write(t, dir, p, "committed\n")
	}
	commit(t, dir, "base")
	write(t, dir, "edited.txt", "uncommitted edit\n")
	write(t, dir, "mine.txt", "untracked\n")
	if err := os.Remove(filepath.Join(dir, "removed-before.txt")); err != nil {
		t.Fatal(err)
	}
	before := run(t, dir, "status", "--porcelain", "--untracked-files=all")

	saved, err := SaveWorktree(dir)
	if err != nil {
		t.Fatal(err)
	}
	// What a generator might do.
	written := []string{"clean.txt", "edited.txt", "mine.txt", "removed-before.txt", "new/dir/new.txt"}
	for _, p := range written {
		write(t, dir, p, "generated\n")
	}
	if err := os.Remove(filepath.Join(dir, "removed-later.txt")); err != nil {
		t.Fatal(err)
	}

	left, err := saved.Restore(append(written, "removed-later.txt")...)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Errorf("left = %+v, want every file restored", left)
	}

	if after := run(t, dir, "status", "--porcelain", "--untracked-files=all"); after != before {
		t.Errorf("git status after restore:\n%s\nwant:\n%s", after, before)
	}
	for path, want := range map[string]string{
		"clean.txt":         "committed\n",
		"edited.txt":        "uncommitted edit\n",
		"mine.txt":          "untracked\n",
		"removed-later.txt": "committed\n",
	} {
		if got, _ := read(t, dir, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{"removed-before.txt", "new/dir/new.txt"} {
		if _, exists := read(t, dir, path); exists {
			t.Errorf("%s exists after restore", path)
		}
	}
}

// Restore cannot tell a generator's writes from an edit made meanwhile, so it
// puts back only the files it is given. Every other change stays and is named,
// and what such a file held before is kept in git.
func TestRestoreLeavesOtherChangesAndNamesThem(t *testing.T) {
	dir := newRepo(t, "main")
	for _, p := range []string{"listed.txt", "clean.txt", "edited.txt", "untouched.txt", "reverted.txt", "removed.txt"} {
		write(t, dir, p, "committed\n")
	}
	commit(t, dir, "base")
	write(t, dir, "edited.txt", "uncommitted edit\n")
	write(t, dir, "untouched.txt", "uncommitted edit\n")
	write(t, dir, "reverted.txt", "uncommitted edit\n")
	write(t, dir, "mine.txt", "untracked\n")

	saved, err := SaveWorktree(dir)
	if err != nil {
		t.Fatal(err)
	}
	// While the command runs, it or someone else changes these.
	for _, p := range []string{"listed.txt", "clean.txt", "edited.txt", "new.txt"} {
		write(t, dir, p, "meanwhile\n")
	}
	write(t, dir, "reverted.txt", "committed\n")
	for _, p := range []string{"mine.txt", "removed.txt"} {
		if err := os.Remove(filepath.Join(dir, p)); err != nil {
			t.Fatal(err)
		}
	}

	left, err := saved.Restore("./listed.txt")
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, l := range left {
		paths = append(paths, l.Path)
	}
	if want := []string{"clean.txt", "edited.txt", "mine.txt", "new.txt", "removed.txt", "reverted.txt"}; !slices.Equal(paths, want) {
		t.Fatalf("left = %v, want %v", paths, want)
	}
	for _, l := range left {
		want := map[string]string{"edited.txt": "uncommitted edit\n", "mine.txt": "untracked\n", "reverted.txt": "uncommitted edit\n"}[l.Path]
		if want == "" {
			if l.Before != "" {
				t.Errorf("%s: Before = %q, want none: git has what it held", l.Path, l.Before)
			}
			continue
		}
		if got := run(t, dir, "cat-file", "-p", l.Before); got != strings.TrimSpace(want) { // run trims
			t.Errorf("%s: blob %q holds %q, want %q", l.Path, l.Before, got, want)
		}
	}
	for path, want := range map[string]string{
		"listed.txt": "committed\n", "clean.txt": "meanwhile\n", "edited.txt": "meanwhile\n",
		"untouched.txt": "uncommitted edit\n", "reverted.txt": "committed\n", "new.txt": "meanwhile\n",
	} {
		if got, _ := read(t, dir, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
	for _, path := range []string{"mine.txt", "removed.txt"} {
		if _, exists := read(t, dir, path); exists {
			t.Errorf("%s exists after restore, want it left deleted", path)
		}
	}
}

func TestSaveWorktreeOutsideGitFails(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if _, err := SaveWorktree(dir); err == nil {
		t.Error("SaveWorktree outside git: no error")
	}
}

// Paths are not globs: restoring web/app/[id]/page.tsx must not touch web/app/i/page.tsx.
func TestRestoreWorktreeTakesPathsLiterally(t *testing.T) {
	dir := newRepo(t, "main")
	for _, p := range []string{"web/app/[id]/page.tsx", "web/app/i/page.tsx"} {
		write(t, dir, p, "committed\n")
	}
	commit(t, dir, "base")
	write(t, dir, "web/app/i/page.tsx", "uncommitted edit\n")

	saved, err := SaveWorktree(dir)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "web/app/[id]/page.tsx", "generated\n")
	if _, err := saved.Restore("web/app/[id]/page.tsx"); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{"web/app/[id]/page.tsx": "committed\n", "web/app/i/page.tsx": "uncommitted edit\n"} {
		if got, _ := read(t, dir, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestTreeListsFilesGitTracksOrWouldAdd(t *testing.T) {
	dir := newRepo(t, "main")
	write(t, dir, ".gitignore", "ignored.txt\n")
	write(t, dir, "kept.txt", "x\n")
	write(t, dir, "gone.txt", "x\n")
	write(t, dir, "old.txt", "a file long enough for git to follow through a rename\n")
	commit(t, dir, "base")
	write(t, dir, "new/untracked.txt", "x\n")
	write(t, dir, "ignored.txt", "x\n")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "mv", "old.txt", "moved.txt")

	got, err := Tree(dir)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	if want := []string{".gitignore", "kept.txt", "moved.txt", "new/untracked.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tree() = %v, want %v", got, want)
	}
}

func TestTreeOutsideARepositoryIsAnError(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if got, err := Tree(dir); !errors.Is(err, ErrNotRepo) {
		t.Errorf("Tree() = %v, %v, want ErrNotRepo", got, err)
	}
}
