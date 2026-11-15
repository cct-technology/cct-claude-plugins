package rules

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cct-technology/cct-claude-plugins/internal/check"
	"github.com/cct-technology/cct-claude-plugins/internal/manifest"
)

// The test binary is also the fake generator: `rules.test qkgen file=content ...`
// writes each file, and `rules.test qkgen fail` fails with a message on stderr.
// It needs no shell or toolchain, so it runs the same on every OS.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "qkgen" {
		os.Exit(fakeGenerator(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func fakeGenerator(args []string) int {
	for _, a := range args {
		if a == "fail" {
			fmt.Println("rendering templates")
			fmt.Fprintln(os.Stderr, "template.tmpl: no such file")
			return 3
		}
		file, content, _ := strings.Cut(a, "=")
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(file, []byte(content+"\n"), 0o644); err != nil {
			panic(err)
		}
	}
	return 0
}

// generator returns the command that runs the fake generator with args, and
// puts the test binary on PATH so the command finds it.
func generator(t *testing.T, args string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return strings.TrimSuffix(filepath.Base(exe), ".exe") + " qkgen " + args
}

// qk020Repo commits files in a fresh repository and returns a context that
// runs slow rules with the given generated entries.
func qk020Repo(t *testing.T, files map[string]string, entries ...manifest.Generated) check.Context {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	for path, content := range files {
		put(t, dir, path, content)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "base")
	return check.Context{
		Root:     dir,
		Manifest: manifest.Loaded{Present: true, Manifest: manifest.Manifest{Version: 1, Generated: entries}},
		Full:     true,
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func put(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func content(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func status(t *testing.T, dir string) string {
	t.Helper()
	return gitIn(t, dir, "status", "--porcelain", "--untracked-files=all")
}

func runRegisteredQK020(t *testing.T, ctx check.Context) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK020" {
			if r.Check != "generated.current" || r.EnforcedFrom != check.Silver {
				t.Errorf("QK020 = check %q from %v, want generated.current from silver", r.Check, r.EnforcedFrom)
			}
			return r.Run(ctx)
		}
	}
	t.Fatal("QK020 is not registered")
	return check.Result{}
}

func TestQK020PassesWhenTheFileMatches(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "fresh\n"},
		manifest.Generated{File: "docs/api.md", Cmd: generator(t, "docs/api.md=fresh")})

	if res := runRegisteredQK020(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// Windows checkouts often have CRLF where the generator writes LF.
func TestQK020IgnoresLineEndings(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "fresh\r\n"},
		manifest.Generated{File: "docs/api.md", Cmd: generator(t, "docs/api.md=fresh")})

	if res := runRegisteredQK020(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK020FailsWhenTheFileDiffersAndPutsItBack(t *testing.T) {
	cmd := generator(t, "docs/api.md=fresh")
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "stale\n"}, manifest.Generated{File: "docs/api.md", Cmd: cmd})

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 {
		t.Fatalf("result = %+v, want fail with one finding", res)
	}
	if f := res.Findings[0]; f.File != "docs/api.md" || !strings.Contains(f.Message, "`"+cmd+"`") {
		t.Errorf("finding = %+v, want the file, naming the command", f)
	}
	if got := content(t, ctx.Root, "docs/api.md"); got != "stale\n" {
		t.Errorf("docs/api.md = %q after the check, want it as committed", got)
	}
	if s := status(t, ctx.Root); s != "" {
		t.Errorf("git status after the check:\n%s", s)
	}
}

func TestQK020FailsWhenTheGeneratorDoesNotWriteAMissingFile(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"README.md": "x\n"},
		manifest.Generated{File: "docs/api.md", Cmd: generator(t, "")})

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "docs/api.md" {
		t.Errorf("result = %+v, want fail on docs/api.md", res)
	}
}

func TestQK020ReportsAFailingGeneratorWithItsOutput(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "fresh\n"},
		manifest.Generated{File: "docs/api.md", Cmd: generator(t, "docs/api.md=half-written fail")})

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 {
		t.Fatalf("result = %+v, want fail with one finding", res)
	}
	msg := res.Findings[0].Message
	for _, want := range []string{"exit status 3", "template.tmpl: no such file", "rendering templates"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not contain %q", msg, want)
		}
	}
	if got := content(t, ctx.Root, "docs/api.md"); got != "fresh\n" {
		t.Errorf("docs/api.md = %q after the check, want it as committed", got)
	}
}

func TestQK020RunsEachCommandOnceForAllItsFiles(t *testing.T) {
	cmd := generator(t, "docs/a.md=fresh docs/b.md=fresh")
	ctx := qk020Repo(t, map[string]string{"docs/a.md": "fresh\n", "docs/b.md": "stale\n"},
		manifest.Generated{File: "docs/a.md", Cmd: cmd}, manifest.Generated{File: "docs/b.md", Cmd: cmd})

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "docs/b.md" {
		t.Errorf("result = %+v, want fail on docs/b.md only", res)
	}
}

// Uncommitted work survives the check.
func TestQK020LeavesTheWorkingTreeAsItWas(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "stale\n", "src/main.go": "package main\n"},
		manifest.Generated{File: "./docs/api.md", Cmd: generator(t, "docs/api.md=fresh")})
	put(t, ctx.Root, "docs/api.md", "fresh\n") // regenerated, not yet committed
	put(t, ctx.Root, "src/main.go", "package main // edited\n")
	put(t, ctx.Root, "scratch.txt", "mine\n")
	before := status(t, ctx.Root)

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Pass {
		t.Errorf("result = %+v, want pass: the uncommitted file matches", res)
	}
	if after := status(t, ctx.Root); after != before {
		t.Errorf("git status after the check:\n%s\nwant:\n%s", after, before)
	}
	for path, want := range map[string]string{"docs/api.md": "fresh\n", "src/main.go": "package main // edited\n", "scratch.txt": "mine\n"} {
		if got := content(t, ctx.Root, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

// A file outside `generated` that changed during the run may hold someone's
// edit, so QK020 leaves it and warns, keeping what it held before in git.
func TestQK020LeavesOtherFilesThatChangedAndNamesThem(t *testing.T) {
	cmd := generator(t, "docs/api.md=fresh notes.txt=junk build/out.txt=junk src/main.go=junk")
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "fresh\n", "src/main.go": "package main\n", "notes.txt": "committed\n"},
		manifest.Generated{File: "docs/api.md", Cmd: cmd})
	put(t, ctx.Root, "src/main.go", "package main // edited\n")

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Warn {
		t.Fatalf("result = %+v, want warn: nothing generated is stale", res)
	}
	var files []string
	for _, f := range res.Findings {
		files = append(files, f.File)
		if !strings.Contains(f.Message, "`"+cmd+"`") || !strings.Contains(f.Message, "`generated`") {
			t.Errorf("%s: message %q does not name the command and `generated`", f.File, f.Message)
		}
	}
	if want := "build/out.txt notes.txt src/main.go"; strings.Join(files, " ") != want {
		t.Fatalf("findings on %v, want %s", files, want)
	}
	blob := regexp.MustCompile("git cat-file -p ([0-9a-f]+)").FindStringSubmatch(res.Findings[2].Message)
	if blob == nil {
		t.Fatalf("src/main.go: message %q does not say where its uncommitted edit is", res.Findings[2].Message)
	}
	if got := gitIn(t, ctx.Root, "cat-file", "-p", blob[1]); got != "package main // edited\n" {
		t.Errorf("kept blob = %q, want the uncommitted edit", got)
	}
	for path, want := range map[string]string{"docs/api.md": "fresh\n", "notes.txt": "junk\n", "build/out.txt": "junk\n", "src/main.go": "junk\n"} {
		if got := content(t, ctx.Root, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestQK020SkipsWhenTheGeneratorIsNotInstalled(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "fresh\n"},
		manifest.Generated{File: "docs/api.md", Cmd: "qk-no-such-generator --out docs/api.md"})

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "qk-no-such-generator") {
		t.Errorf("result = %+v, want skipped naming the command", res)
	}
}

func TestQK020RunsOnlyWhenAskedForSlowChecks(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "stale\n"},
		manifest.Generated{File: "docs/api.md", Cmd: generator(t, "docs/api.md=fresh")})
	ctx.Full = false

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "--full") {
		t.Errorf("result = %+v, want skipped, naming --full", res)
	}
}

func TestQK020PassesWithoutGeneratedFiles(t *testing.T) {
	if res := runRegisteredQK020(t, check.Context{}); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// A file under `generated` is put back whichever command wrote it, and is never
// reported as missing from `generated`.
func TestQK020PutsBackAFileAnotherCommandWrote(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/a.md": "a\n", "docs/b.md": "b\n"},
		manifest.Generated{File: "docs/a.md", Cmd: generator(t, "docs/a.md=a")},
		manifest.Generated{File: "docs/b.md", Cmd: generator(t, "docs/b.md=b docs/a.md=overwritten")})

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
	if got := content(t, ctx.Root, "docs/a.md"); got != "a\n" {
		t.Errorf("docs/a.md = %q after the check, want it as committed", got)
	}
}

// One missing tool must not hide what the other generators found.
func TestQK020FailsOnOtherFilesWhenOneGeneratorIsNotInstalled(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"docs/api.md": "fresh\n", "docs/b.md": "stale\n"},
		manifest.Generated{File: "docs/api.md", Cmd: "qk-no-such-generator --out docs/api.md"},
		manifest.Generated{File: "docs/b.md", Cmd: generator(t, "docs/b.md=fresh")})

	res := runRegisteredQK020(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "docs/b.md" {
		t.Errorf("result = %+v, want fail on docs/b.md", res)
	}
}
