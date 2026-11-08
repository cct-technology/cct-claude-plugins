package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
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
