package rules

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

var waiverToday = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func runRegisteredQK100(t *testing.T, l waivers.Loaded) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK100" {
			if r.Check != "waivers.valid" || r.EnforcedFrom != check.Silver {
				t.Errorf("QK100 = check %q from %v, want waivers.valid from silver", r.Check, r.EnforcedFrom)
			}
			return r.Run(check.Context{Waivers: l, Today: waiverToday})
		}
	}
	t.Fatal("QK100 is not registered")
	return check.Result{}
}

func fixture(t *testing.T, name string) waivers.Loaded {
	t.Helper()
	return waivers.Load(filepath.Join("..", "waivers", "testdata", name))
}

func TestQK100PassesWithoutAWaiversFile(t *testing.T) {
	if res := runRegisteredQK100(t, waivers.Loaded{}); res.Status != check.Pass {
		t.Errorf("result = %+v", res)
	}
}

func TestQK100SkipsAnUnreadableFile(t *testing.T) {
	res := runRegisteredQK100(t, waivers.Loaded{Present: true, Err: errors.New("permission denied")})

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "permission denied") {
		t.Errorf("result = %+v", res)
	}
}

func TestQK100PassesValidWaivers(t *testing.T) {
	// The fixture waives QK010 and QK110, which this registry may not hold yet:
	// those are warnings at most, never failures.
	res := runRegisteredQK100(t, fixture(t, "valid"))

	if res.Status == check.Fail {
		t.Errorf("result = %+v", res)
	}
}

func TestQK100Fails(t *testing.T) {
	tests := []struct{ fixture, want string }{
		{"expired", "waivers[0] (QK010): expired on 2020-01-31"},
		{"no-owner", "waivers[0] (QK010): no owner"},
		{"no-reason", "waivers[0] (QK010): no reason"},
		{"bad-date", "waivers[0] (QK010): expires must be a date like 2026-12-31"},
		{"waives-qk100", "waivers[0] (QK100): QK100 cannot be waived"},
		{"bad-yaml", "did not find expected"},
		{"unknown-key", "reson"},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			res := runRegisteredQK100(t, fixture(t, tt.fixture))
			if res.Status != check.Fail {
				t.Fatalf("result = %+v, want fail", res)
			}
			f := res.Findings[0]
			if f.File != ".quality/waivers.yaml" || !strings.Contains(f.Message, tt.want) {
				t.Errorf("finding = %+v, want %q", f, tt.want)
			}
		})
	}
}

// A waiver for a rule this qk does not know hides nothing, and may come from a
// newer qk: it warns, like an unknown key under modes in quality.yaml.
func TestQK100WarnsOnUnknownRule(t *testing.T) {
	res := runRegisteredQK100(t, fixture(t, "unknown-rule"))

	if res.Status != check.Warn || len(res.Findings) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if msg := res.Findings[0].Message; !strings.Contains(msg, "waivers[0] (QK999)") || !strings.Contains(msg, "no rule with this ID") {
		t.Errorf("message = %q", msg)
	}
}

func TestQK100FailureOutranksUnknownRuleWarning(t *testing.T) {
	res := runRegisteredQK100(t, waivers.Loaded{Present: true, Waivers: []waivers.Waiver{
		{Rule: "QK999", Reason: "r", Owner: "o", Expires: "2099-01-01"},
		{Rule: "QK001", Reason: "r", Owner: "", Expires: "2099-01-01"},
	}})

	if res.Status != check.Fail || len(res.Findings) != 2 {
		t.Errorf("result = %+v, want fail with both findings", res)
	}
}
