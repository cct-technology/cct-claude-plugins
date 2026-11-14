package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/deps"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

func runQK(t *testing.T, reg []check.Rule, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(reg, args, strings.NewReader(""), &out, &errOut)
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

func TestCheckFormatSARIF(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")

	code, out, _ := runQK(t, []check.Rule{failing}, "check", "--format", "sarif")

	if code != 1 {
		t.Errorf("exit code = %d, want 1: SARIF output does not change what blocks", code)
	}
	var log struct {
		Version string
		Runs    []struct {
			Tool    struct{ Driver struct{ Version string } }
			Results []struct{ RuleID, Level string }
		}
	}
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if log.Version != "2.1.0" || len(log.Runs) != 1 || log.Runs[0].Tool.Driver.Version != "dev" {
		t.Errorf("want a SARIF 2.1.0 log from qk dev, got %+v", log)
	}
	if res := log.Runs[0].Results; len(res) != 1 || res[0].RuleID != "QK010" || res[0].Level != "error" {
		t.Errorf("results = %+v, want the QK010 failure as an error", res)
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
	if code, _, errOut := runQK(t, []check.Rule{failing}, "waiver", "add", "QK010", "--path", "*.go",
		"--reason", "being rewritten", "--owner", "rk", "--expires", "2099-12-31", "--approved-by", "a person"); code != 0 {
		t.Fatalf("waiver add: exit %d, stderr = %q", code, errOut)
	}

	code, out, _ := runQK(t, []check.Rule{failing}, "check")

	if code != 0 {
		t.Errorf("exit code = %d, want 0 with the only finding waived", code)
	}
	if !strings.Contains(out, "PASS  QK010") || !strings.Contains(out, "waived: a.go: docs must change (rk until 2099-12-31: being rewritten)") {
		t.Errorf("stdout = %q", out)
	}
}

// A waiver written by hand, with no approval in the log, must not unblock anything.
func TestCheckIgnoresAWaiverNobodyApproved(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	dir, _ := os.Getwd()
	writeWaiver(t, dir, "waivers:\n  - rule: QK010\n    reason: wrote this myself\n    owner: rk\n    expires: 2099-12-31\n")

	code, out, _ := runQK(t, append(rules.All(), failing), "check")

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "FAIL  QK010") || !strings.Contains(out, "FAIL  QK100") || !strings.Contains(out, "no approval in .quality/exceptions-log.jsonl") {
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
		"--reason", "being fixed", "--owner", "rk", "--expires", "2099-12-31", "--path", "a/**", "--path", "b/**",
		"--approved-by", "a person")

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
	if len(l.Log) != 1 || l.Log[0].ApprovedBy != "a person" || l.Log[0].Identity != "Test <test@example.com>" {
		t.Errorf("log = %+v", l.Log)
	}
	if _, err := os.Stat(filepath.Join(sub, ".quality")); err == nil {
		t.Error("wrote .quality in the subfolder")
	}
}

func TestWaiverAddUsageErrors(t *testing.T) {
	good := []string{"--reason", "r", "--owner", "o", "--expires", "2099-12-31", "--approved-by", "a"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown rule", append([]string{"QK999"}, good...), "unknown rule QK999"},
		{"QK100", append([]string{"QK100"}, good...), "QK100 cannot be waived"},
		{"bad date", []string{"QK001", "--reason", "r", "--owner", "o", "--expires", "soon", "--approved-by", "a"}, "expires must be a date"},
		{"past date", []string{"QK001", "--reason", "r", "--owner", "o", "--expires", "2000-01-01", "--approved-by", "a"}, "expired on 2000-01-01"},
		{"missing reason", []string{"QK001", "--owner", "o", "--expires", "2099-12-31", "--approved-by", "a"}, "reason"},
		{"missing owner", []string{"QK001", "--reason", "r", "--expires", "2099-12-31", "--approved-by", "a"}, "owner"},
		{"missing expires", []string{"QK001", "--reason", "r", "--owner", "o", "--approved-by", "a"}, "expires"},
		{"blank owner", []string{"QK001", "--reason", "r", "--owner", " ", "--expires", "2099-12-31", "--approved-by", "a"}, "no owner"},
		{"missing approver", []string{"QK001", "--reason", "r", "--owner", "o", "--expires", "2099-12-31"}, "approved-by"},
		{"blank approver", []string{"QK001", "--reason", "r", "--owner", "o", "--expires", "2099-12-31", "--approved-by", " "}, "no approver"},
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

func runHook(t *testing.T, stdin string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(rules.All(), []string{"hook", "pre-tool-use"}, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestHookAsksAPersonBeforeAWaiver(t *testing.T) {
	code, out, _ := runHook(t, `{"tool_name":"Bash","tool_input":{"command":"qk waiver add QK010 --reason r"}}`)

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, `"permissionDecision":"ask"`) || !strings.Contains(out, `"hookEventName":"PreToolUse"`) {
		t.Errorf("stdout = %q", out)
	}
}

func TestHookStaysQuietOnOrdinaryCommands(t *testing.T) {
	code, out, errOut := runHook(t, `{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`)

	if code != 0 || out != "" || errOut != "" {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q, want silence", code, out, errOut)
	}
}

func TestHookFailsOpenOnBadInput(t *testing.T) {
	code, out, errOut := runHook(t, "not json")

	if code != 0 || out != "" || !strings.Contains(errOut, "warning") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q, want exit 0 and a warning", code, out, errOut)
	}
}

const impactManifest = "version: 1\nmaturity: silver\ndocs:\n  impact_map:\n    - paths: [\"internal/config/**\"]\n      docs: [\"docs/settings.md\"]\n"

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
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

// impactRepo is a silver repo on branch feature with a mapped code change and no doc change.
func impactRepo(t *testing.T) string {
	t.Helper()
	dir := gitRepo(t)
	put(t, dir, "quality.yaml", impactManifest)
	put(t, dir, "internal/config/load.go", "package config\n")
	return dir
}

func approveNote(t *testing.T, reason string) {
	t.Helper()
	if code, _, errOut := runQK(t, rules.All(), "docs-impact", "none", "--reason", reason, "--approved-by", "a person"); code != 0 {
		t.Fatalf("docs-impact: exit %d, stderr = %q", code, errOut)
	}
}

func TestQK010BlocksAMappedChangeWithoutDocs(t *testing.T) {
	impactRepo(t)

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 1 || !strings.Contains(out, "FAIL  QK010") || !strings.Contains(out, "docs/settings.md") {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestQK010PassesWhenTheDocChanges(t *testing.T) {
	dir := impactRepo(t)
	put(t, dir, "docs/settings.md", "# Settings\n")

	if code, out, _ := runQK(t, rules.All(), "check"); code != 0 {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestQK010PassesWithAnApprovedUncommittedNote(t *testing.T) {
	dir := impactRepo(t)

	approveNote(t, "internal refactor")

	if code, out, _ := runQK(t, rules.All(), "check"); code != 0 {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".quality", "docs-impact", "feature.md")); err != nil {
		t.Error(err)
	}
	if l := waivers.Load(dir); len(l.Log) != 1 || l.Log[0].ApprovedBy != "a person" || l.Log[0].Identity != "Test <test@example.com>" {
		t.Errorf("log = %+v", l.Log)
	}
}

func TestQK010RejectsAHandWrittenNote(t *testing.T) {
	dir := impactRepo(t)
	put(t, dir, ".quality/docs-impact/feature.md", "Docs-Impact: none — trust me\n")

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 1 || !strings.Contains(out, "no approval") {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestQK010RejectsANoteEditedAfterApproval(t *testing.T) {
	dir := impactRepo(t)
	approveNote(t, "internal refactor")
	put(t, dir, ".quality/docs-impact/feature.md", "Docs-Impact: none — something else\n")

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 1 || !strings.Contains(out, "no approval") {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestQK010IgnoresANoteAlreadyOnMain(t *testing.T) {
	dir := gitRepo(t)
	gitIn(t, dir, "checkout", "-q", "main")
	put(t, dir, "quality.yaml", impactManifest)
	approveNote(t, "from an earlier branch")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "earlier work")
	gitIn(t, dir, "checkout", "-q", "-B", "feature")
	put(t, dir, "internal/config/load.go", "package config\n")

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 1 || !strings.Contains(out, "FAIL  QK010") {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

// CI checks out a detached merge commit, so a note must count without matching the branch name.
func TestQK010AcceptsTheNoteOnADetachedHead(t *testing.T) {
	dir := impactRepo(t)
	approveNote(t, "internal refactor")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "work")
	gitIn(t, dir, "checkout", "-q", "--detach")

	if code, out, _ := runQK(t, rules.All(), "check", "--base", "main"); code != 0 {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestQK010FindingCanBeWaivedByPath(t *testing.T) {
	impactRepo(t)
	if code, _, errOut := runQK(t, rules.All(), "waiver", "add", "QK010", "--path", "internal/config/**",
		"--reason", "being rewritten", "--owner", "rk", "--expires", "2099-12-31", "--approved-by", "a person"); code != 0 {
		t.Fatalf("waiver add: exit %d, stderr = %q", code, errOut)
	}

	code, out, _ := runQK(t, rules.All(), "check")

	if code != 0 || !strings.Contains(out, "PASS  QK010") || !strings.Contains(out, "waived: internal/config/load.go") {
		t.Errorf("exit code = %d, stdout = %q", code, out)
	}
}

func TestDocsImpactUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing reason", []string{"--approved-by", "a"}, "reason"},
		{"missing approver", []string{"--reason", "r"}, "approved-by"},
		{"blank reason", []string{"--reason", " ", "--approved-by", "a"}, "no reason"},
		{"blank approver", []string{"--reason", "r", "--approved-by", " "}, "no approver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t)

			code, _, errOut := runQK(t, rules.All(), append([]string{"docs-impact", "none"}, tt.args...)...)

			if code != 2 || !strings.Contains(errOut, tt.want) {
				t.Errorf("exit code = %d, stderr = %q, want 2 and %q", code, errOut, tt.want)
			}
			if _, err := os.Stat(filepath.Join(dir, ".quality")); err == nil {
				t.Error("wrote a note despite the error")
			}
		})
	}
}

func TestDocsImpactNeedsGit(t *testing.T) {
	inRepo(t, "")
	dir, _ := os.Getwd()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	code, _, errOut := runQK(t, rules.All(), "docs-impact", "none", "--reason", "r", "--approved-by", "a")

	if code != 2 || !strings.Contains(errOut, "git") {
		t.Errorf("exit code = %d, stderr = %q", code, errOut)
	}
}

func TestContextWithoutManifest(t *testing.T) {
	inRepo(t, "")

	code, out, errOut := runQK(t, rules.All(), "context")

	if code != 0 || !strings.Contains(out, "no quality.yaml") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestContextReportsEnforcedFailureWithoutBlocking(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	reg := []check.Rule{testRule("QK010", "docs.impact", check.Result{Status: check.Fail, Findings: []check.Finding{{Message: "x"}}})}

	code, out, errOut := runQK(t, reg, "context")

	if code != 0 || !strings.Contains(out, "QK010 enforced, 1 open") {
		t.Errorf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// runHookEvent pipes a hook input recorded from Claude Code into qk hook <event>.
func runHookEvent(t *testing.T, reg []check.Rule, event, recording string) (code int, stdout, stderr string) {
	t.Helper()
	input, err := os.ReadFile(filepath.Join(hookRecordings, recording))
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code = run(reg, []string{"hook", event}, bytes.NewReader(input), &out, &errOut)
	return code, out.String(), errOut.String()
}

// hookRecordings is resolved before any test changes directory.
var hookRecordings, _ = filepath.Abs(filepath.Join("..", "..", "internal", "hook", "testdata"))

func hookJSON(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	return v
}

// CI needs the summary for people and SARIF for code scanning from the same
// run: a second run would repeat every generator and could see another tree.
func TestCheckSarifFlagWritesSARIFBesideTheChosenFormat(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	runs := 0
	counted := failing
	counted.Run = func(ctx check.Context) check.Result {
		runs++
		return failing.Run(ctx)
	}
	file := filepath.Join(t.TempDir(), "qk.sarif")

	code, out, errOut := runQK(t, []check.Rule{counted}, "check", "--format", "claude", "--sarif", file)

	if code != 1 || !strings.HasPrefix(out, "Quality Kit: 1 fail, 0 warn.\nQK010 fail:") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q, want exit 1 and the claude report", code, out, errOut)
	}
	if runs != 1 {
		t.Errorf("the rule ran %d times, want once", runs)
	}
	_, want, _ := runQK(t, []check.Rule{failing}, "check", "--format", "sarif")
	if got, err := os.ReadFile(file); err != nil || string(got) != want {
		t.Errorf("--sarif wrote %q (%v), want what --format sarif prints:\n%s", got, err, want)
	}
}

func TestCheckSarifFlagSaysWhenTheFileCannotBeWritten(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	file := filepath.Join(t.TempDir(), "no-such-folder", "qk.sarif")

	code, out, errOut := runQK(t, []check.Rule{failing}, "check", "--sarif", file)

	if code != 2 || out != "" || !strings.Contains(errOut, "no-such-folder") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q, want exit 2, no report and the path named", code, out, errOut)
	}
}

func failingRule(id, key string) check.Rule {
	return testRule(id, key, check.Result{Status: check.Fail, Findings: []check.Finding{{File: "internal/x.go", Message: "docs/x.md must change too"}}})
}

func TestSessionStartHookAddsTheSummary(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")

	code, out, errOut := runHookEvent(t, []check.Rule{failingRule("QK010", "docs.impact")}, "session-start", "session-start.json")

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	specific, _ := hookJSON(t, out)["hookSpecificOutput"].(map[string]any)
	context, _ := specific["additionalContext"].(string)
	if specific["hookEventName"] != "SessionStart" || !strings.Contains(context, "QK010 enforced, 1 open") {
		t.Errorf("stdout = %s", out)
	}
}

func TestStopHookAdvisoryShowsFindingsAndAllows(t *testing.T) {
	inRepo(t, "")
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())

	code, out, errOut := runHookEvent(t, []check.Rule{failingRule("QK010", "docs.impact")}, "stop", "stop.json")

	v := hookJSON(t, out)
	if code != 0 || v["decision"] != nil || !strings.Contains(fmt.Sprint(v["systemMessage"]), "docs/x.md must change too") {
		t.Errorf("exit code = %d, stdout = %s, stderr = %q", code, out, errOut)
	}
}

func TestStopHookBlocksEnforcedFailures(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())

	code, out, errOut := runHookEvent(t, []check.Rule{failingRule("QK010", "docs.impact")}, "stop", "stop.json")

	v := hookJSON(t, out)
	if code != 0 || v["decision"] != "block" || !strings.Contains(fmt.Sprint(v["reason"]), "internal/x.go: docs/x.md must change too") {
		t.Errorf("exit code = %d, stdout = %s, stderr = %q", code, out, errOut)
	}
}

func TestStopHookEnforcedCleanIsSilent(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())
	reg := []check.Rule{testRule("QK010", "docs.impact", check.Result{Status: check.Pass})}

	code, out, errOut := runHookEvent(t, reg, "stop", "stop.json")

	if code != 0 || out != "" || errOut != "" {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q, want silence", code, out, errOut)
	}
}

func TestStopHookAllowsTheThirdBlockInARow(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())
	reg := []check.Rule{failingRule("QK010", "docs.impact")}

	for i, recording := range []string{"stop.json", "stop-active.json"} {
		if _, out, _ := runHookEvent(t, reg, "stop", recording); hookJSON(t, out)["decision"] != "block" {
			t.Fatalf("stop %d: stdout = %s, want a block", i+1, out)
		}
	}
	code, out, errOut := runHookEvent(t, reg, "stop", "stop-active.json")

	v := hookJSON(t, out)
	if code != 0 || v["decision"] != nil || !strings.HasPrefix(fmt.Sprint(v["systemMessage"]), "Quality Kit: unresolved findings, handing over to a human") {
		t.Errorf("third stop: exit code = %d, stdout = %s, stderr = %q", code, out, errOut)
	}
}

func TestStopHookShowsChecksThatDidNotRun(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\n")
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())
	reg := []check.Rule{testRule("QK010", "docs.impact", check.Result{Status: check.Skipped, Reason: "rule crashed: boom"})}

	code, out, errOut := runHookEvent(t, reg, "stop", "stop.json")

	v := hookJSON(t, out)
	if code != 0 || v["decision"] != nil || !strings.Contains(fmt.Sprint(v["systemMessage"]), "Skipped: QK010 (rule crashed: boom)") {
		t.Errorf("exit code = %d, stdout = %s, stderr = %q", code, out, errOut)
	}
}

func TestStopHookIsQuietWithoutAManifest(t *testing.T) {
	// QK001 is always skipped without quality.yaml; saying so after every turn is noise.
	inRepo(t, "")
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())

	code, out, errOut := runHookEvent(t, rules.All(), "stop", "stop.json")

	if code != 0 || out != "" || errOut != "" {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q, want silence", code, out, errOut)
	}
}

func TestHookErrorsNeverExitNonZero(t *testing.T) {
	// Claude Code reads exit 2 as "block". A qk older than its hooks.json must not block.
	tests := []struct {
		name, stdin string
		args        []string
	}{
		{"unknown hook", "{}", []string{"hook", "no-such-event"}},
		{"unknown flag", "{}", []string{"hook", "stop", "--no-such-flag"}},
		{"malformed stop input", "not json", []string{"hook", "stop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inRepo(t, "version: 1\nmaturity: silver\n")
			t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())
			var out, errOut bytes.Buffer

			code := run([]check.Rule{failingRule("QK010", "docs.impact")}, tt.args, strings.NewReader(tt.stdin), &out, &errOut)

			if code != 0 || out.Len() != 0 || !strings.Contains(errOut.String(), "warning") {
				t.Errorf("exit code = %d, stdout = %q, stderr = %q, want exit 0, no output and a warning", code, out.String(), errOut.String())
			}
		})
	}
}

func TestCheckFullFlag(t *testing.T) {
	gitRepo(t)
	t.Setenv("CI", "")
	var got check.Context

	if code, _, errOut := runQK(t, []check.Rule{recorder(&got)}, "check"); code != 0 || got.Full {
		t.Errorf("without --full: exit code = %d, stderr = %q, Full = %v; want 0 and false", code, errOut, got.Full)
	}
	if code, _, errOut := runQK(t, []check.Rule{recorder(&got)}, "check", "--full"); code != 0 || !got.Full {
		t.Errorf("with --full: exit code = %d, stderr = %q, Full = %v; want 0 and true", code, errOut, got.Full)
	}
}

func TestCheckRunsSlowChecksInCI(t *testing.T) {
	gitRepo(t)
	t.Setenv("CI", "true")
	var got check.Context

	if code, _, errOut := runQK(t, []check.Rule{recorder(&got)}, "check"); code != 0 || !got.Full {
		t.Errorf("exit code = %d, stderr = %q, Full = %v; want 0 and true", code, errOut, got.Full)
	}
}

func TestCheckHelpExplainsFull(t *testing.T) {
	code, out, _ := runQK(t, rules.All(), "check", "--help")

	if code != 0 || !strings.Contains(out, "--full") || !strings.Contains(out, "QK020") || !strings.Contains(out, "CI") {
		t.Errorf("exit code = %d, help = %s", code, out)
	}
}

// Generators are too slow for every stop, even when Claude Code runs in a CI job.
func TestStopHookLeavesOutSlowChecks(t *testing.T) {
	inRepo(t, "version: 1\nmaturity: silver\ngenerated:\n  - file: docs/api.md\n    cmd: make api-docs\n")
	t.Setenv("CLAUDE_PLUGIN_DATA", t.TempDir())
	t.Setenv("CI", "true")

	code, out, errOut := runHookEvent(t, rules.All(), "stop", "stop.json")

	if code != 0 || out != "" || errOut != "" {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q, want silence", code, out, errOut)
	}
}

// qk init writes at the repository root, and what it writes is a repository
// qk check accepts: the manifest passes QK001 and nothing blocks.
func TestInitWritesARepositoryQkCheckAccepts(t *testing.T) {
	dir := gitRepo(t)
	sub := filepath.Join(dir, "internal", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	code, out, errOut := runQK(t, rules.All(), "init", "--data-class", "client", "--maturity", "silver", "--replaces", "Excel", "--replaces", "Old CRM")

	if code != 0 || errOut != "" {
		t.Fatalf("init: exit code = %d, stderr = %q", code, errOut)
	}
	if !strings.Contains(out, "wrote quality.yaml\n") {
		t.Errorf("init printed %q, want it to report quality.yaml", out)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "quality.yaml"))
	if err != nil {
		t.Fatalf("no quality.yaml at the repository root: %v", err)
	}
	for _, want := range []string{"data_class: client\n", "maturity: silver\n", `replaces: ["Excel","Old CRM"]`} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("quality.yaml lacks %q:\n%s", want, manifest)
		}
	}
	if _, err := os.Stat(filepath.Join(sub, "quality.yaml")); err == nil {
		t.Error("wrote quality.yaml in the subfolder")
	}

	code, out, errOut = runQK(t, rules.All(), "check", "--format", "json")
	if code != 0 {
		t.Fatalf("check: exit code = %d, stderr = %q\n%s", code, errOut, out)
	}
	var rep check.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Results) == 0 || rep.Results[0].Rule != "QK001" || rep.Results[0].Status != check.Pass {
		t.Errorf("check = %s, want QK001 to pass", out)
	}
}

func TestInitWithoutAnswersWritesAnInternalBronzeManifest(t *testing.T) {
	inRepo(t, "")

	code, _, errOut := runQK(t, nil, "init")

	if code != 0 || errOut != "" {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut)
	}
	manifest, _ := os.ReadFile("quality.yaml")
	for _, want := range []string{"data_class: internal\n", "maturity: bronze\n"} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("quality.yaml lacks %q:\n%s", want, manifest)
		}
	}
}

func TestInitUnknownAnswerIsAUsageErrorAndWritesNothing(t *testing.T) {
	inRepo(t, "")

	code, out, errOut := runQK(t, nil, "init", "--maturity", "platinum")

	if code != 2 || out != "" {
		t.Errorf("exit code = %d, stdout = %q, want 2 and nothing on stdout", code, out)
	}
	if !strings.Contains(errOut, "--maturity") || !strings.Contains(errOut, "bronze, silver, gold") {
		t.Errorf("stderr = %q, want the flag and the allowed values", errOut)
	}
	if entries, _ := os.ReadDir("."); len(entries) > 0 {
		t.Errorf("wrote %d files before checking the answers", len(entries))
	}
}

// fakeRegistry points `qk deps verify` at a registry that knows three npm
// packages, and answers 404 for any other.
func fakeRegistry(t *testing.T) {
	t.Helper()
	doc := func(version, published, licence string) string {
		return `{"dist-tags": {"latest": "` + version + `"}, "time": {"` + version + `": "` + published + `"}, "versions": {"` + version + `": {"license": "` + licence + `"}}}`
	}
	pages := map[string]string{
		"/left-pad":     doc("1.3.0", "2018-04-09T01:10:45Z", "MIT"),
		"/@scope%2Fpkg": doc("2.0.0", "2020-01-01T00:00:00Z", "GPL-3.0-only"),

		"/example.com/lib/@latest":       `{"Version": "v1.2.0", "Time": "2024-03-01T00:00:00Z"}`,
		"/example.com/lib/@v/v1.2.0.mod": "module example.com/lib",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch body, ok := pages[r.URL.EscapedPath()]; {
		case r.URL.Path == "/flaky":
			http.Error(w, "down", http.StatusBadGateway)
		case !ok:
			http.NotFound(w, r)
		default:
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	real := registry
	t.Cleanup(func() { registry = real })
	registry = &deps.Client{NPM: srv.URL, GoProxy: srv.URL, HTTP: srv.Client(), Now: time.Now}
}

func TestDepsVerifyPassesAGoodPackage(t *testing.T) {
	fakeRegistry(t)

	code, out, errOut := runQK(t, rules.All(), "deps", "verify", "npm", "left-pad@1.3.0")

	if code != 0 || errOut != "" || !strings.Contains(out, "ok: `left-pad` (npm) 1.3.0, published 2018-04-09, licence MIT") {
		t.Errorf("exit code = %d, stdout = %q, stderr = %q", code, out, errOut)
	}
}

func TestDepsVerifyExitsOneForAFinding(t *testing.T) {
	fakeRegistry(t)

	for args, want := range map[string]string{
		"npm reakt-domm":       "`reakt-domm` (npm) does not exist in the registry",
		"npm @scope/pkg@2.0.0": "`@scope/pkg` (npm) 2.0.0 has the licence `GPL-3.0-only`, which is not on the allowed list",
	} {
		code, out, errOut := runQK(t, rules.All(), append([]string{"deps", "verify"}, strings.Fields(args)...)...)
		if code != 1 || errOut != "" || !strings.Contains(out, want) {
			t.Errorf("qk deps verify %s: exit code = %d, stdout = %q, stderr = %q; want 1 and %q", args, code, out, errOut, want)
		}
	}
}

// A registry that cannot be asked is not a finding, so it must not block an install.
func TestDepsVerifyExitsZeroWhenTheRegistryCannotBeAsked(t *testing.T) {
	fakeRegistry(t)

	code, out, _ := runQK(t, rules.All(), "deps", "verify", "npm", "flaky")

	if code != 0 || !strings.Contains(out, "skipped: could not ask the registry about `flaky` (npm)") {
		t.Errorf("exit code = %d, stdout = %q; want 0 and a skipped line", code, out)
	}
}

func TestDepsVerifySaysWhatItCouldNotCheck(t *testing.T) {
	fakeRegistry(t)

	code, out, _ := runQK(t, rules.All(), "deps", "verify", "go", "example.com/lib")

	if code != 0 || !strings.Contains(out, "ok: `example.com/lib` (go) v1.2.0, published 2024-03-01") ||
		!strings.Contains(out, "licence not checked: this registry holds no licence data") {
		t.Errorf("exit code = %d, stdout = %q; want 0, ok and the note about the licence", code, out)
	}
}

func TestDepsVerifyUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"deps", "verify", "left-pad"},
		{"deps", "verify", "gems", "rails"},
		{"deps", "verify", "npm", "@1.0.0"},
	} {
		if code, _, errOut := runQK(t, rules.All(), args...); code != 2 || errOut == "" {
			t.Errorf("qk %v: exit code = %d, stderr = %q; want 2 and a message", args, code, errOut)
		}
	}
}
