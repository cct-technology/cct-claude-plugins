package rules

import (
	"errors"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
)

func runRegisteredQK001(t *testing.T, l manifest.Loaded) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK001" {
			return r.Run(check.Context{Manifest: l})
		}
	}
	t.Fatal("QK001 is not registered")
	return check.Result{}
}

func TestQK001EnforcedFromBronze(t *testing.T) {
	for _, r := range All() {
		if r.ID == "QK001" && (r.EnforcedFrom != check.Bronze || r.Check != "manifest.valid") {
			t.Errorf("QK001 = check %q from %v, want manifest.valid from bronze", r.Check, r.EnforcedFrom)
		}
	}
}

func TestQK001SkipsWithoutManifest(t *testing.T) {
	res := runRegisteredQK001(t, manifest.Loaded{})

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "no quality.yaml") {
		t.Errorf("result = %+v", res)
	}
}

func TestQK001SkipsUnreadableManifest(t *testing.T) {
	res := runRegisteredQK001(t, manifest.Loaded{Present: true, Err: errors.New("permission denied")})

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "permission denied") {
		t.Errorf("result = %+v", res)
	}
}

func TestQK001PassesValidManifest(t *testing.T) {
	res := runRegisteredQK001(t, manifest.Loaded{Present: true, Manifest: manifest.Manifest{
		Version: 1, Modes: map[string]string{"manifest.valid": "enforced"},
	}})

	if res.Status != check.Pass {
		t.Errorf("result = %+v", res)
	}
}

func TestQK001FailsWithEachProblemAsAFinding(t *testing.T) {
	res := runRegisteredQK001(t, manifest.Loaded{Present: true, Problems: []manifest.Problem{
		{Path: "maturity", Message: "value must be one of 'bronze', 'silver', 'gold'"},
		{Path: "docs", Message: "additional properties 'rot' not allowed"},
	}})

	if res.Status != check.Fail || len(res.Findings) != 2 {
		t.Fatalf("result = %+v", res)
	}
	want := check.Finding{File: "quality.yaml", Message: "maturity: value must be one of 'bronze', 'silver', 'gold'"}
	if res.Findings[0] != want {
		t.Errorf("finding = %+v, want %+v", res.Findings[0], want)
	}
}

// An unknown check key warns rather than fails: a manifest written for a newer qk
// (or using a check this qk does not have yet) must not block an older qk.
func TestQK001WarnsOnUnknownCheckKey(t *testing.T) {
	res := runRegisteredQK001(t, manifest.Loaded{Present: true, Manifest: manifest.Manifest{
		Version: 1, Modes: map[string]string{"docs.impcat": "enforced", "manifest.valid": "advisory"},
	}})

	if res.Status != check.Warn || len(res.Findings) != 1 {
		t.Fatalf("result = %+v", res)
	}
	msg := res.Findings[0].Message
	if !strings.Contains(msg, "modes.docs.impcat") || !strings.Contains(msg, "manifest.valid") {
		t.Errorf("message = %q, want the unknown key and the known keys", msg)
	}
}
