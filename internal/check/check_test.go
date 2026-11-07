package check

import (
	"strings"
	"testing"
)

func rule(id, key string, from Maturity, res Result) Rule {
	return Rule{
		ID: id, Check: key, Title: id + " title", Phase: 1, EnforcedFrom: from,
		Why: "why", Fix: "fix",
		Run: func(Context) Result { return res },
	}
}

func TestModeFor(t *testing.T) {
	bronzeRule := rule("QK901", "manifest.valid", Bronze, Result{})
	silverRule := rule("QK902", "docs.impact", Silver, Result{})
	goldRule := rule("QK903", "ownership.codeowners", Gold, Result{})

	tests := []struct {
		name string
		cfg  Config
		r    Rule
		want Mode
	}{
		{"no manifest, bronze rule", Config{}, bronzeRule, Advisory},
		{"no manifest, silver rule", Config{}, silverRule, Advisory},
		{"no manifest, gold rule", Config{}, goldRule, Advisory},

		{"bronze, bronze rule", Config{Maturity: Bronze}, bronzeRule, Enforced},
		{"bronze, silver rule", Config{Maturity: Bronze}, silverRule, Advisory},
		{"bronze, gold rule", Config{Maturity: Bronze}, goldRule, Advisory},

		{"silver, bronze rule", Config{Maturity: Silver}, bronzeRule, Enforced},
		{"silver, silver rule", Config{Maturity: Silver}, silverRule, Enforced},
		{"silver, gold rule", Config{Maturity: Silver}, goldRule, Advisory},

		{"gold, bronze rule", Config{Maturity: Gold}, bronzeRule, Enforced},
		{"gold, silver rule", Config{Maturity: Gold}, silverRule, Enforced},
		{"gold, gold rule", Config{Maturity: Gold}, goldRule, Enforced},

		{"override turns a check off at gold",
			Config{Maturity: Gold, Modes: map[string]Mode{"docs.impact": Off}}, silverRule, Off},
		{"override enforces a check at bronze",
			Config{Maturity: Bronze, Modes: map[string]Mode{"docs.impact": Enforced}}, silverRule, Enforced},
		{"override downgrades to advisory at silver",
			Config{Maturity: Silver, Modes: map[string]Mode{"docs.impact": Advisory}}, silverRule, Advisory},
		{"override for another check has no effect",
			Config{Maturity: Bronze, Modes: map[string]Mode{"contracts.breaking": Enforced}}, silverRule, Advisory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.ModeFor(tt.r); got != tt.want {
				t.Errorf("ModeFor = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunAdvisoryFailIsReportedAsWarnAndDoesNotBlock(t *testing.T) {
	r := rule("QK902", "docs.impact", Silver, Result{Status: Fail, Findings: []Finding{{File: "a.go", Message: "m"}}})

	rep := Run([]Rule{r}, Config{Maturity: Bronze}, Context{})

	if got := rep.Results[0].Status; got != Warn {
		t.Errorf("status = %q, want %q", got, Warn)
	}
	if rep.Blocking {
		t.Error("advisory fail must not block")
	}
	if rep.Summary != (Summary{Warn: 1}) {
		t.Errorf("summary = %+v", rep.Summary)
	}
	if rep.Results[0].Fix != "fix" {
		t.Errorf("fix = %q, want the rule's fix text on a warning", rep.Results[0].Fix)
	}
}

func TestRunEnforcedFailBlocks(t *testing.T) {
	r := rule("QK902", "docs.impact", Silver, Result{Status: Fail, Findings: []Finding{{File: "a.go", Message: "m"}}})

	rep := Run([]Rule{r}, Config{Maturity: Silver}, Context{})

	if got := rep.Results[0].Status; got != Fail {
		t.Errorf("status = %q, want %q", got, Fail)
	}
	if !rep.Blocking {
		t.Error("enforced fail must block")
	}
	if rep.Results[0].Fix != "fix" {
		t.Errorf("fix = %q, want the rule's fix text on a failing result", rep.Results[0].Fix)
	}
}

func TestRunEnforcedWarnDoesNotBlock(t *testing.T) {
	r := rule("QK902", "docs.impact", Silver, Result{Status: Warn})

	rep := Run([]Rule{r}, Config{Maturity: Silver}, Context{})

	if rep.Blocking {
		t.Error("a warning must never block")
	}
}

func TestRunOffRuleIsNotRunAndNotReported(t *testing.T) {
	ran := false
	r := rule("QK902", "docs.impact", Silver, Result{})
	r.Run = func(Context) Result { ran = true; return Result{Status: Fail} }

	rep := Run([]Rule{r}, Config{Maturity: Gold, Modes: map[string]Mode{"docs.impact": Off}}, Context{})

	if ran {
		t.Error("a rule in mode off must not run")
	}
	if len(rep.Results) != 0 || rep.Blocking {
		t.Errorf("report = %+v, want empty and not blocking", rep)
	}
}

func TestRunSkippedNeverBlocks(t *testing.T) {
	r := rule("QK901", "manifest.valid", Bronze, Result{Status: Skipped, Reason: "oasdiff not installed"})

	rep := Run([]Rule{r}, Config{Maturity: Gold}, Context{})

	got := rep.Results[0]
	if got.Status != Skipped || got.Reason != "oasdiff not installed" {
		t.Errorf("result = %+v", got)
	}
	if rep.Blocking {
		t.Error("skipped must never block")
	}
}

func TestRunRulePanicFailsOpenAsSkipped(t *testing.T) {
	r := rule("QK901", "manifest.valid", Bronze, Result{})
	r.Run = func(Context) Result { panic("boom") }

	rep := Run([]Rule{r}, Config{Maturity: Gold}, Context{})

	got := rep.Results[0]
	if got.Status != Skipped || !strings.Contains(got.Reason, "boom") {
		t.Errorf("result = %+v, want skipped with the panic in the reason", got)
	}
	if rep.Blocking {
		t.Error("a crashing rule must not block")
	}
}

func TestRunKeepsRegistryOrderAndCounts(t *testing.T) {
	rules := []Rule{
		rule("QK903", "c", Gold, Result{Status: Pass}),
		rule("QK901", "a", Bronze, Result{Status: Skipped, Reason: "r"}),
		rule("QK902", "b", Silver, Result{Status: Fail}),
	}

	rep := Run(rules, Config{Maturity: Silver}, Context{})

	var ids []string
	for _, r := range rep.Results {
		ids = append(ids, r.Rule)
	}
	if strings.Join(ids, ",") != "QK903,QK901,QK902" {
		t.Errorf("order = %v", ids)
	}
	if rep.Summary != (Summary{Pass: 1, Fail: 1, Skipped: 1}) {
		t.Errorf("summary = %+v", rep.Summary)
	}
}

func TestValidateRegistry(t *testing.T) {
	good := rule("QK001", "manifest.valid", Bronze, Result{})

	tests := []struct {
		name    string
		rules   []Rule
		wantErr string
	}{
		{"valid", []Rule{good}, ""},
		{"duplicate ID", []Rule{good, good}, "duplicate"},
		{"bad ID", []Rule{rule("Q1", "k", Bronze, Result{})}, "Q1"},
		{"no check key", []Rule{rule("QK002", "", Bronze, Result{})}, "check key"},
		{"no maturity", []Rule{rule("QK002", "k", NoManifest, Result{})}, "EnforcedFrom"},
		{"no run func", []Rule{func() Rule { r := good; r.Run = nil; return r }()}, "Run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegistry(tt.rules)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}
