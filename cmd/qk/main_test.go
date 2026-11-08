package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

func runQK(t *testing.T, reg []check.Rule, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(reg, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// inRepo runs the test from a fresh directory holding quality.yaml with the
// given content, or no quality.yaml when content is empty.
func inRepo(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "quality.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
}

func testRule(id, key string, res check.Result) check.Rule {
	return check.Rule{
		ID: id, Check: key, Title: id + " title", Phase: 1, EnforcedFrom: check.Silver,
		Why: "Why it matters.", Fix: "How to fix it.",
		Run: func(check.Context) check.Result { return res },
	}
}

var failing = testRule("QK010", "docs.impact", check.Result{
	Status: check.Fail, Findings: []check.Finding{{File: "a.go", Message: "docs must change"}},
})

func TestVersionPrintsQKAndManifestSchemaVersion(t *testing.T) {
	code, out, _ := runQK(t, nil, "version")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if want := "qk dev (manifest schema 1)\n"; out != want {
		t.Errorf("qk version printed %q, want %q", out, want)
	}
}

func TestCheckExitsZeroWhenNothingBlocks(t *testing.T) {
	inRepo(t, "")

	code, out, errOut := runQK(t, []check.Rule{failing}, "check")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 for an advisory fail; stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "WARN  QK010") {
		t.Errorf("stdout = %q, want the advisory fail shown as WARN", out)
	}
}

func TestCheckExitsOneOnEnforcedFail(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")

	code, out, errOut := runQK(t, []check.Rule{failing}, "check")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "FAIL  QK010") {
		t.Errorf("stdout = %q, want the FAIL line", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing: a blocking finding is not an error", errOut)
	}
}

func TestCheckModesOverrideFromManifest(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\nmodes:\n  docs.impact: off\n")

	code, out, _ := runQK(t, []check.Rule{failing}, "check")

	if code != 0 || strings.Contains(out, "QK010") {
		t.Errorf("exit code = %d, stdout = %q, want QK010 turned off", code, out)
	}
}

func TestCheckWithoutManifestSkipsQK001(t *testing.T) {
	inRepo(t, "")

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 0 || !strings.Contains(out, "SKIP  QK001") {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestCheckInvalidManifestBlocks(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: platinum\n")

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "FAIL  QK001") || !strings.Contains(out, "quality.yaml: maturity: value must be one of") {
		t.Errorf("stdout = %q", out)
	}
}

func TestCheckUnreadableManifestFailsOpen(t *testing.T) {
	inRepo(t, "")
	if err := os.Mkdir("quality.yaml", 0o755); err != nil {
		t.Fatal(err)
	}

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 0 || !strings.Contains(out, "SKIP  QK001") {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestCheckFormatClaude(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")

	_, out, _ := runQK(t, []check.Rule{failing}, "check", "--format", "claude")

	if !strings.HasPrefix(out, "Quality Kit: 1 fail, 0 warn.\nQK010 fail:") {
		t.Errorf("stdout = %q", out)
	}
}

func TestCheckUnknownFormatIsAUsageErrorAndRunsNothing(t *testing.T) {
	inRepo(t, "")
	ran := false
	r := failing
	r.Run = func(check.Context) check.Result { ran = true; return check.Result{Status: check.Pass} }

	code, out, errOut := runQK(t, []check.Rule{r}, "check", "--format", "xml")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if ran {
		t.Error("rules ran despite an invalid --format")
	}
	if out != "" || !strings.Contains(errOut, "xml") {
		t.Errorf("stdout = %q, stderr = %q", out, errOut)
	}
}

func TestExplainKnownRule(t *testing.T) {
	code, out, _ := runQK(t, []check.Rule{failing}, "explain", "qk010")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(out, "## QK010 — QK010 title\n") || !strings.Contains(out, "How to fix it.") {
		t.Errorf("stdout = %q", out)
	}
}

func TestExplainUnknownRuleIsAUsageError(t *testing.T) {
	code, _, errOut := runQK(t, []check.Rule{failing}, "explain", "QK999")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknown rule QK999") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	if code, _, _ := runQK(t, nil, "bogus"); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

// gitRepo makes a fresh git repo on branch feature, cut from main, isolated
// from the user's git config, and runs the test from it.
func gitRepo(t *testing.T) string {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "A"},
		{"checkout", "-q", "-b", "feature"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Chdir(dir)
	return dir
}

// recorder is a rule that keeps the context it ran with.
func recorder(got *check.Context) check.Rule {
	r := testRule("QK901", "test.recorder", check.Result{Status: check.Pass})
	r.Run = func(ctx check.Context) check.Result { *got = ctx; return check.Result{Status: check.Pass} }
	return r
}

func TestCheckFindsManifestAtRepoRootFromSubfolder(t *testing.T) {
	dir := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "quality.yaml"), []byte("version: 1\nmaturity: platinum\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "internal", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 1 || !strings.Contains(out, "FAIL  QK001") {
		t.Errorf("exit code = %d, stdout = %q, want QK001 failing on the root quality.yaml", code, out)
	}
}

func TestCheckGivesRulesTheChangeSet(t *testing.T) {
	dir := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got check.Context

	code, _, errOut := runQK(t, []check.Rule{recorder(&got)}, "check")

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if got.ChangesErr != nil || got.Changes == nil || len(got.Changes.Files) != 1 || got.Changes.Files[0].Path != "new.txt" {
		t.Errorf("context = %+v, want a change set with new.txt", got)
	}
	if got.ChangedOnly {
		t.Error("ChangedOnly is set without --changed")
	}
}

func TestCheckChangedFlag(t *testing.T) {
	gitRepo(t)
	var got check.Context

	if code, _, errOut := runQK(t, []check.Rule{recorder(&got)}, "check", "--changed"); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if !got.ChangedOnly {
		t.Error("--changed did not reach the rules")
	}
}

// Outside a git repo there is no change set: rules that need one skip; qk does not fail.
func TestCheckOutsideGitGivesRulesAReasonNotAnError(t *testing.T) {
	inRepo(t, "")
	dir, _ := os.Getwd()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	var got check.Context

	code, _, errOut := runQK(t, []check.Rule{recorder(&got)}, "check")

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if got.Changes != nil || got.ChangesErr == nil {
		t.Errorf("context = %+v, want no change set and a reason", got)
	}
}

func TestCheckBadBaseIsAUsageError(t *testing.T) {
	gitRepo(t)

	code, _, errOut := runQK(t, rules.All(), "check", "--base", "no-such-ref")

	if code != 2 || !strings.Contains(errOut, "no-such-ref") {
		t.Errorf("exit code = %d, stderr = %q, want 2 naming the ref", code, errOut)
	}
}

func writeWaiver(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".quality"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".quality", "waivers.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAppliesAWaiver(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	dir, _ := os.Getwd()
	writeWaiver(t, dir, "waivers:\n  - rule: QK010\n    paths: [\"*.go\"]\n    reason: being rewritten\n    owner: rk\n    expires: 2099-12-31\n")

	code, out, _ := runQK(t, []check.Rule{failing}, "check")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 with the only finding waived", code)
	}
	if !strings.Contains(out, "PASS  QK010") || !strings.Contains(out, "waived: a.go: docs must change (rk until 2099-12-31: being rewritten)") {
		t.Errorf("stdout = %q", out)
	}
}

func TestCheckIgnoresAnExpiredWaiverAndQK100ReportsIt(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	dir, _ := os.Getwd()
	writeWaiver(t, dir, "waivers:\n  - rule: QK010\n    reason: too old\n    owner: rk\n    expires: 2000-01-01\n")

	code, out, _ := runQK(t, append(rules.All(), failing), "check")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "FAIL  QK010") || !strings.Contains(out, "FAIL  QK100") || !strings.Contains(out, "expired on 2000-01-01") {
		t.Errorf("stdout = %q", out)
	}
}

func TestWaiverAddWritesAtTheRepoRootFromASubfolder(t *testing.T) {
	dir := gitRepo(t)
	sub := filepath.Join(dir, "internal", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	code, out, errOut := runQK(t, rules.All(), "waiver", "add", "qk001",
		"--reason", "being fixed", "--owner", "rk", "--expires", "2099-12-31", "--path", "a/**", "--path", "b/**")

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "QK001") {
		t.Errorf("stdout = %q", out)
	}
	l := waivers.Load(dir)
	if len(l.Problems) != 0 || len(l.Waivers) != 1 {
		t.Fatalf("Load = %+v", l)
	}
	if w := l.Waivers[0]; w.Rule != "QK001" || len(w.Paths) != 2 || w.Owner != "rk" || w.Expires != "2099-12-31" {
		t.Errorf("waiver = %+v", w)
	}
	if _, err := os.Stat(filepath.Join(sub, ".quality")); err == nil {
		t.Error("wrote .quality in the subfolder")
	}
}

func TestWaiverAddUsageErrors(t *testing.T) {
	good := []string{"--reason", "r", "--owner", "o", "--expires", "2099-12-31"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown rule", append([]string{"QK999"}, good...), "unknown rule QK999"},
		{"QK100", append([]string{"QK100"}, good...), "QK100 cannot be waived"},
		{"bad date", []string{"QK001", "--reason", "r", "--owner", "o", "--expires", "soon"}, "expires must be a date"},
		{"past date", []string{"QK001", "--reason", "r", "--owner", "o", "--expires", "2000-01-01"}, "expired on 2000-01-01"},
		{"missing reason", []string{"QK001", "--owner", "o", "--expires", "2099-12-31"}, "reason"},
		{"missing owner", []string{"QK001", "--reason", "r", "--expires", "2099-12-31"}, "owner"},
		{"missing expires", []string{"QK001", "--reason", "r", "--owner", "o"}, "expires"},
		{"blank owner", []string{"QK001", "--reason", "r", "--owner", " ", "--expires", "2099-12-31"}, "no owner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inRepo(t, "")
			dir, _ := os.Getwd()

			code, _, errOut := runQK(t, rules.All(), append([]string{"waiver", "add"}, tt.args...)...)

			if code != 2 || !strings.Contains(errOut, tt.want) {
				t.Errorf("exit code = %d, stderr = %q, want 2 and %q", code, errOut, tt.want)
			}
			if _, err := os.Stat(filepath.Join(dir, ".quality")); err == nil {
				t.Error("wrote a waiver despite the error")
			}
		})
	}
}
