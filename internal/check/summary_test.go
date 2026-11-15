package check

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cct-technology/cct-claude-plugins/internal/manifest"
	"github.com/cct-technology/cct-claude-plugins/internal/waivers"
)

var summaryDay = time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)

func silverManifest() manifest.Loaded {
	return manifest.Loaded{Present: true, Manifest: manifest.Manifest{Maturity: "silver"}}
}

func summaryRule(id, title string, from Maturity, res Result) Rule {
	return Rule{ID: id, Check: id, Title: title, EnforcedFrom: from, Run: func(Context) Result { return res }}
}

func passing(id string, from Maturity) Rule {
	return summaryRule(id, id+" title", from, Result{Status: Pass})
}

func approvedWaivers(list ...waivers.Waiver) waivers.Loaded {
	l := waivers.Loaded{Present: true, Waivers: list}
	for _, w := range list {
		l.Log = append(l.Log, waivers.NewLogEntry(w, waivers.Approval{By: "a person", Time: summaryDay}))
	}
	return l
}

func summary(t *testing.T, rules []Rule, ctx Context) string {
	t.Helper()
	var b strings.Builder
	if err := WriteSessionSummary(&b, rules, ctx); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSessionSummaryGolden(t *testing.T) {
	waiver := func(rule, expires string, paths ...string) waivers.Waiver {
		return waivers.Waiver{Rule: rule, Paths: paths, Reason: "agreed", Owner: "rk", Expires: expires}
	}
	tests := []struct {
		golden string
		rules  []Rule
		ctx    Context
	}{
		{
			golden: "summary-no-manifest.golden",
			rules:  []Rule{summaryRule("QK010", "Docs change", Silver, Result{Status: Fail, Findings: []Finding{{Message: "x"}}})},
			ctx:    Context{Today: summaryDay},
		},
		{
			golden: "summary-clean.golden",
			rules:  []Rule{passing("QK001", Bronze), passing("QK010", Silver), passing("QK110", Gold)},
			ctx:    Context{Manifest: silverManifest(), Today: summaryDay},
		},
		{
			golden: "summary-findings.golden",
			rules: []Rule{
				passing("QK001", Bronze),
				summaryRule("QK010", "Docs change with the code", Silver, Result{Status: Fail, Findings: []Finding{{Message: "a"}, {Message: "b"}}}),
				summaryRule("QK020", "Generated files are current", Silver, Result{Status: Fail, Findings: []Finding{{Message: "slow rule ran"}}}),
				summaryRule("QK030", "API contract is current", Silver, Result{Status: Skipped, Reason: "oasdiff not installed"}),
				summaryRule("QK110", "CODEOWNERS covers every path", Gold, Result{Status: Fail, Findings: []Finding{{Message: "c"}}}),
			},
			ctx: Context{Manifest: silverManifest(), Today: summaryDay},
		},
		{
			golden: "summary-expiring.golden",
			rules:  []Rule{passing("QK001", Bronze), passing("QK010", Silver)},
			ctx: Context{
				Manifest: silverManifest(), Today: summaryDay,
				Waivers: approvedWaivers(
					waiver("QK010", "2026-11-20", "docs/**", "api/**"),
					waiver("QK030", "2026-11-23"),
					waiver("QK040", "2026-11-24"), // 15 days away
				),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			assertGolden(t, tt.golden, []byte(summary(t, tt.rules, tt.ctx)))
		})
	}
}

func TestSessionSummarySkipsSlowRules(t *testing.T) {
	for _, id := range []string{"QK020", "QK060"} {
		t.Run(id, func(t *testing.T) {
			r := passing(id, Silver)
			r.Run = func(Context) Result {
				t.Errorf("%s ran at session start", id)
				return Result{Status: Pass}
			}

			got := summary(t, []Rule{r}, Context{Manifest: silverManifest(), Today: summaryDay})

			if !strings.Contains(got, "Not run at session start: "+id+".") {
				t.Errorf("summary does not say %s did not run:\n%s", id, got)
			}
		})
	}
}

func TestSessionSummaryLeavesOutRulesTurnedOff(t *testing.T) {
	m := silverManifest()
	m.Manifest.Modes = map[string]string{"QK010": "off"}

	got := summary(t, []Rule{passing("QK001", Bronze), passing("QK010", Silver)}, Context{Manifest: m, Today: summaryDay})

	if strings.Contains(got, "QK010") {
		t.Errorf("rule turned off is listed:\n%s", got)
	}
}

func TestSessionSummaryUnreadableManifestIsNotNoManifest(t *testing.T) {
	m := manifest.Loaded{Present: true, Err: errors.New("permission denied")}

	got := summary(t, []Rule{summaryRule("QK001", "t", Bronze, Result{Status: Skipped, Reason: "could not read quality.yaml"})}, Context{Manifest: m, Today: summaryDay})

	if strings.Contains(got, "no quality.yaml") {
		t.Errorf("an unreadable manifest reads as a missing one:\n%s", got)
	}
	if !strings.Contains(got, "Skipped: QK001 (could not read quality.yaml)") {
		t.Errorf("the tooling problem is hidden:\n%s", got)
	}
}

func TestSessionSummaryNeverExceedsThirtyLines(t *testing.T) {
	var rules []Rule
	for i := range 40 {
		rules = append(rules, summaryRule(fmt.Sprintf("QK%03d", i+200), "t", Silver, Result{Status: Fail, Findings: []Finding{{Message: "x"}}}))
	}
	rules = append(rules, summaryRule("QK999", "t", Silver, Result{Status: Skipped, Reason: "tool missing"}))

	got := summary(t, rules, Context{Manifest: silverManifest(), Today: summaryDay})

	// Header and Enforced (2 lines), "Open findings:", 40 rules and Details
	// (42), and Skipped (1). The findings are cut so Skipped still shows.
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 30 {
		t.Fatalf("%d lines, want 30:\n%s", len(lines), got)
	}
	for i, want := range map[int]string{28: "... 16 more lines. Run qk check.", 29: "Skipped: QK999 (tool missing)"} {
		if lines[i] != want {
			t.Errorf("line %d = %q, want %q", i+1, lines[i], want)
		}
	}
}

func TestSessionSummaryCountsOnlyRealFindings(t *testing.T) {
	r := summaryRule("QK010", "Docs change", Silver, Result{Status: Fail})

	got := summary(t, []Rule{r}, Context{Manifest: silverManifest(), Today: summaryDay})

	if !strings.Contains(got, "- QK010 enforced: Docs change\n") {
		t.Errorf("a failure with no findings should carry no count:\n%s", got)
	}
}
