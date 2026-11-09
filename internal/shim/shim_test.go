// Package shim tests plugins/quality-kit/bin/qk. The tests live here because
// everything under plugins/ ships to users.
//
// They run the real shim against real qk builds. Only curl is faked, so the
// shim needs no test-only way to change where it downloads from.
package shim

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	required = "1.2.3" // the version the shim under test is pinned to
	older    = "0.0.1"
	repoRoot = "../.."
)

var (
	requiredQK string // qk built as the required version
	olderQK    string // qk built as another version
	gitBash    string // on Windows, the shell hooks run the shim with
	gitTools   string // on Windows, the directory holding sed, tar and friends
)

func exe() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func TestMain(m *testing.M) {
	code, err := setup(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shim tests:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func setup(m *testing.M) (int, error) {
	dir, err := os.MkdirTemp("", "qk-shim")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)

	build := func(version string) (string, error) {
		out := filepath.Join(dir, version, "qk"+exe())
		cmd := exec.Command("go", "build", "-ldflags", "-X main.version="+version, "-o", out, "./cmd/qk")
		cmd.Dir = repoRoot
		if msg, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("go build: %v\n%s", err, msg)
		}
		return out, nil
	}
	if requiredQK, err = build(required); err != nil {
		return 0, err
	}
	if olderQK, err = build(older); err != nil {
		return 0, err
	}

	if runtime.GOOS == "windows" {
		if err := findGitBash(); err != nil {
			return 0, err
		}
	}
	return m.Run(), nil
}

// findGitBash locates usr\bin\bash.exe above git.exe, which sits in <Git>\cmd,
// <Git>\bin or <Git>\mingw64\bin. bash on PATH may be WSL. Running usr\bin\bash
// directly keeps PATH as the test sets it; the <Git>\bin launcher would put the
// real curl ahead of the fake one.
func findGitBash() error {
	git, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	for dir := filepath.Dir(git); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		tools := filepath.Join(dir, "usr", "bin")
		if _, err := os.Stat(filepath.Join(tools, "bash.exe")); err == nil {
			gitTools, gitBash = tools, filepath.Join(tools, "bash.exe")
			return nil
		}
	}
	return fmt.Errorf("Git Bash not found above %s", git)
}

// sandbox is one plugin install: a copy of the shim, its plugin.json, and a
// home directory, all on paths with a space and non-ASCII letters in them.
type sandbox struct {
	t       *testing.T
	root    string
	home    string
	data    string   // CLAUDE_PLUGIN_DATA; unset when empty
	onPath  []string // directories on PATH ahead of the system's
	tools   string
	curlLog string
	assets  string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	base := t.TempDir()
	s := &sandbox{
		t:       t,
		root:    filepath.Join(base, "plugin cachå", "quality-kit"),
		home:    filepath.Join(base, "the hømé"),
		tools:   filepath.Join(base, "tools"),
		curlLog: filepath.Join(base, "curl.log"),
		assets:  filepath.Join(base, "release"),
	}
	for _, d := range []string{filepath.Join(s.root, "bin"), filepath.Join(s.root, ".claude-plugin"), s.home, s.tools, s.assets} {
		mkdir(t, d)
	}

	source := filepath.Join(repoRoot, "plugins", "quality-kit")
	shim, err := os.ReadFile(filepath.Join(source, "bin", "qk"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(source, "bin", "qk"))
	if err != nil {
		t.Fatal(err)
	}
	// Keeps the mode from the checkout: a shim that lost its executable bit
	// fails on Linux and macOS, where it is run directly.
	writeFile(t, s.shim(), shim, info.Mode().Perm())

	manifest, err := os.ReadFile(filepath.Join(source, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Starts from the real manifest, so a format change that breaks how the
	// shim reads it fails here.
	version := regexp.MustCompile(`"version":\s*"[^"]*"`)
	if !version.Match(manifest) {
		t.Fatalf("no version in plugin.json:\n%s", manifest)
	}
	pinned := version.ReplaceAll(manifest, []byte(`"version": "`+required+`"`))
	writeFile(t, filepath.Join(s.root, ".claude-plugin", "plugin.json"), pinned, 0o644)

	s.fakeCurl(`[ -f "$assets/$name" ] || exit 22
cp "$assets/$name" "$out"`)
	return s
}

func (s *sandbox) shim() string { return filepath.Join(s.root, "bin", "qk") }

// fakeCurl installs a curl that logs each URL and then runs body, with $out,
// $name and $assets set.
func (s *sandbox) fakeCurl(body string) {
	script := fmt.Sprintf(`#!/bin/sh
assets='%s'
while [ $# -gt 0 ]; do
	case $1 in
		-o) out=$2; shift ;;
		https://*) url=$1 ;;
	esac
	shift
done
echo "$url" >> '%s'
name=${url##*/}
%s
`, filepath.ToSlash(s.assets), filepath.ToSlash(s.curlLog), body)
	writeFile(s.t, filepath.Join(s.tools, "curl"), []byte(script), 0o755)
}

// publish puts a release on the fake server: the given binary in an archive,
// and a checksums file holding sum, or the archive's real checksum when sum is
// empty.
func (s *sandbox) publish(binary, sum string) {
	s.t.Helper()
	name := archiveName()
	path := filepath.Join(s.assets, name)
	writeArchive(s.t, path, binary)
	if sum == "" {
		data, err := os.ReadFile(path)
		if err != nil {
			s.t.Fatal(err)
		}
		sum = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	lines := fmt.Sprintf("%064d  qk_%s_plan9_mips.tar.gz\n%s  %s\n", 0, required, sum, name)
	writeFile(s.t, filepath.Join(s.assets, "checksums.txt"), []byte(lines), 0o644)
}

func archiveName() string {
	return fmt.Sprintf("qk_%s_%s_%s.tar.gz", required, runtime.GOOS, runtime.GOARCH)
}

func (s *sandbox) run(stdin string, args ...string) (code int, stdout, stderr string) {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, gitBash, append([]string{s.shim()}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, s.shim(), args...)
	}

	path := append([]string{s.tools}, s.onPath...)
	if gitTools != "" {
		path = append(path, gitTools)
	}
	path = append(path, os.Getenv("PATH"))

	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CLAUDE_PLUGIN_DATA=") && !strings.HasPrefix(kv, "QK_SHIM_PROBE=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+strings.Join(path, string(os.PathListSeparator)), "HOME="+s.home)
	if s.data != "" {
		cmd.Env = append(cmd.Env, "CLAUDE_PLUGIN_DATA="+s.data)
	}
	cmd.Dir = s.home
	cmd.Stdin = strings.NewReader(stdin)

	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if ctx.Err() != nil {
		s.t.Fatalf("shim did not finish: %v\nstderr: %s", ctx.Err(), errOut.String())
	}
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		s.t.Fatal(err)
	}
	return code, out.String(), errOut.String()
}

func (s *sandbox) homeCache() string {
	return filepath.Join(s.home, ".cache", "quality-kit", required, "qk"+exe())
}

func (s *sandbox) dataCache() string {
	return filepath.Join(s.data, required, "qk"+exe())
}

func (s *sandbox) downloads() []string {
	s.t.Helper()
	data, err := os.ReadFile(s.curlLog)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		s.t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func (s *sandbox) wantRan(stdout, stderr string, code int) {
	s.t.Helper()
	if code != 0 || !strings.HasPrefix(stdout, "qk "+required+" ") {
		s.t.Fatalf("want qk %s to run; code=%d stdout=%q stderr=%q", required, code, stdout, stderr)
	}
}

func (s *sandbox) wantNoDownload() {
	s.t.Helper()
	if got := s.downloads(); len(got) != 0 {
		s.t.Errorf("want no download, curl fetched %v", got)
	}
}

// wantFailedOpen checks the contract hooks rely on: exit 0, nothing on stdout
// for a hook to parse, and a warning containing want.
func (s *sandbox) wantFailedOpen(code int, stdout, stderr, want string) {
	s.t.Helper()
	if code != 0 {
		s.t.Errorf("exit code = %d, want 0 (fail open)", code)
	}
	if stdout != "" {
		s.t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, want) {
		s.t.Errorf("stderr = %q, want it to contain %q", stderr, want)
	}
}

func (s *sandbox) wantNotCached(path string) {
	s.t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		s.t.Errorf("%s exists (err=%v), want nothing cached", path, err)
	}
}

func TestRunsQKOnPathAtRequiredVersion(t *testing.T) {
	s := newSandbox(t)
	s.onPath = []string{filepath.Dir(requiredQK)}

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	s.wantNoDownload()
}

func TestPrefersPathOverCache(t *testing.T) {
	s := newSandbox(t)
	s.onPath = []string{filepath.Dir(requiredQK)}
	writeFile(t, s.homeCache(), []byte("not a program"), 0o755) // would fail if run

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
}

func TestSkipsItselfOnPath(t *testing.T) {
	// Plugin bin/ is on the Bash tool's PATH, so the shim can find itself.
	s := newSandbox(t)
	s.onPath = []string{filepath.Join(s.root, "bin"), filepath.Dir(requiredQK)}

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	s.wantNoDownload()
}

func TestSkipsQKOnPathAtOtherVersion(t *testing.T) {
	s := newSandbox(t)
	s.onPath = []string{filepath.Dir(olderQK)}
	copyBinary(t, requiredQK, s.homeCache())

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	s.wantNoDownload()
}

func TestFindsRequiredVersionLaterOnPath(t *testing.T) {
	s := newSandbox(t)
	s.onPath = []string{filepath.Dir(olderQK), filepath.Dir(requiredQK)}

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	s.wantNoDownload()
}

func TestUsesCacheInHome(t *testing.T) {
	s := newSandbox(t)
	copyBinary(t, requiredQK, s.homeCache())

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	s.wantNoDownload()
}

func TestUsesCacheInPluginData(t *testing.T) {
	s := newSandbox(t)
	s.data = filepath.Join(s.home, "plugin data")
	copyBinary(t, requiredQK, s.dataCache())

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	s.wantNoDownload()
}

func TestDownloadsVerifiesAndCaches(t *testing.T) {
	s := newSandbox(t)
	s.data = filepath.Join(s.home, "plugin data") // Claude Code creates it on first use; the shim may run first
	s.publish(requiredQK, "")

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	base := "https://github.com/rkvalvaa/cct-claude-plugins/releases/download/v" + required + "/"
	got := strings.Join(s.downloads(), " ")
	for _, want := range []string{base + archiveName(), base + "checksums.txt"} {
		if !strings.Contains(got, want) {
			t.Errorf("curl fetched %q, want %q", got, want)
		}
	}
	if _, err := os.Stat(s.dataCache()); err != nil {
		t.Errorf("binary not cached: %v", err)
	}
	s.wantNotCached(s.homeCache())

	// The second run uses the cache.
	if err := os.Remove(s.curlLog); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = s.run("", "version")
	s.wantRan(stdout, stderr, code)
	s.wantNoDownload()
}

func TestDownloadCachesInHomeOutsideClaudeCode(t *testing.T) {
	s := newSandbox(t)
	s.publish(requiredQK, "")

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
	if _, err := os.Stat(s.homeCache()); err != nil {
		t.Errorf("binary not cached: %v", err)
	}
}

func TestReplacesBrokenCachedBinary(t *testing.T) {
	s := newSandbox(t)
	writeFile(t, s.homeCache(), []byte("not a program"), 0o755)
	s.publish(requiredQK, "")

	code, stdout, stderr := s.run("", "version")

	s.wantRan(stdout, stderr, code)
}

func TestRefusesBadChecksum(t *testing.T) {
	s := newSandbox(t)
	s.publish(requiredQK, strings.Repeat("ab", 32))

	code, stdout, stderr := s.run("", "version")

	s.wantFailedOpen(code, stdout, stderr, "checksum")
	s.wantNotCached(s.homeCache())
}

func TestRefusesAssetMissingFromChecksums(t *testing.T) {
	s := newSandbox(t)
	s.publish(requiredQK, "")
	writeFile(t, filepath.Join(s.assets, "checksums.txt"), []byte(strings.Repeat("ab", 32)+"  other.tar.gz\n"), 0o644)

	code, stdout, stderr := s.run("", "version")

	s.wantFailedOpen(code, stdout, stderr, "checksum")
	s.wantNotCached(s.homeCache())
}

func TestRefusesDownloadAtWrongVersion(t *testing.T) {
	s := newSandbox(t)
	s.publish(olderQK, "")

	code, stdout, stderr := s.run("", "version")

	s.wantFailedOpen(code, stdout, stderr, required)
	s.wantNotCached(s.homeCache())
}

func TestFailsOpenOffline(t *testing.T) {
	s := newSandbox(t)
	s.fakeCurl("exit 6")

	code, stdout, stderr := s.run("", "version")

	s.wantFailedOpen(code, stdout, stderr, "download")
	s.wantNotCached(s.homeCache())
}

func TestFailsOpenWithoutPluginVersion(t *testing.T) {
	s := newSandbox(t)
	if err := os.Remove(filepath.Join(s.root, ".claude-plugin", "plugin.json")); err != nil {
		t.Fatal(err)
	}
	s.onPath = []string{filepath.Dir(requiredQK)}

	code, stdout, stderr := s.run("", "version")

	s.wantFailedOpen(code, stdout, stderr, "version")
	s.wantNoDownload()
}

func TestPassesQKExitCodeThrough(t *testing.T) {
	// qk's non-zero exit is how enforced findings block: failing open must not
	// swallow it.
	s := newSandbox(t)
	s.onPath = []string{filepath.Dir(requiredQK)}

	code, _, stderr := s.run("", "no-such-command")

	if code != 2 {
		t.Errorf("exit code = %d, want 2; stderr=%q", code, stderr)
	}
}

func TestPassesStdinThrough(t *testing.T) {
	// Hooks hand qk their input on stdin.
	s := newSandbox(t)
	s.onPath = []string{filepath.Dir(requiredQK)}
	input := `{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"qk waiver add QK010"}}`

	code, stdout, stderr := s.run(input, "hook", "pre-tool-use")

	if code != 0 || !strings.Contains(stdout, `"permissionDecision"`) {
		t.Errorf("want a permission decision from the hook input; code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func copyBinary(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, to, data, 0o755)
}

func writeArchive(t *testing.T, path, binary string) {
	t.Helper()
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "qk" + exe(), Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}
