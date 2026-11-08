package check

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files from current output")

// sample is a hand-built report, independent of Run, so the format tests
// only exercise the writers.
var sample = Report{
	Results: []RuleResult{
		{
			Rule: "QK010", Check: "docs.impact", Title: "Docs change with mapped code",
			Mode: Enforced, Status: Fail,
			Findings: []Finding{
				{File: "internal/config/load.go", Message: "docs/reference/settings.md must change too"},
				{Message: "no Docs-Impact trailer in the range"},
			},
			Fix: "Update the mapped docs, or add a `Docs-Impact: none — <reason>` commit trailer.",
		},
		{
			Rule: "QK110", Check: "ownership.codeowners", Title: "CODEOWNERS covers every top-level path",
			Mode: Advisory, Status: Warn,
			Findings: []Finding{{File: "scripts", Message: "no owner"}},
			Fix:      "Add a CODEOWNERS line for the path.",
		},
		{Rule: "QK001", Check: "manifest.valid", Title: "Manifest is valid", Mode: Enforced, Status: Pass},
		{
			Rule: "QK030", Check: "contracts.breaking", Title: "No breaking OpenAPI changes",
			Mode: Enforced, Status: Skipped, Reason: "oasdiff not installed",
		},
	},
	Summary:  Summary{Pass: 1, Warn: 1, Fail: 1, Skipped: 1},
	Blocking: true,
}

var clean = Report{
	Results: []RuleResult{{Rule: "QK001", Check: "manifest.valid", Title: "Manifest is valid", Mode: Enforced, Status: Pass}},
	Summary: Summary{Pass: 1},
}

// onlySkipped has nothing to act on, but two checks did not run: the agent must still see that.
var onlySkipped = Report{
	Results: []RuleResult{
		{Rule: "QK001", Check: "manifest.valid", Title: "quality.yaml is valid", Mode: Advisory, Status: Skipped,
			Reason: "no quality.yaml; every check runs advisory with defaults"},
		{Rule: "QK030", Check: "contracts.breaking", Title: "No breaking OpenAPI changes", Mode: Advisory, Status: Skipped,
			Reason: "oasdiff not installed"},
	},
	Summary: Summary{Skipped: 2},
}

var refRules = []Rule{
	{
		ID: "QK001", Check: "manifest.valid", Title: "Manifest is valid", Phase: 1, EnforcedFrom: Bronze,
		Why: "`quality.yaml` matches the schema.", Fix: "Correct the key named in the finding.",
		Run: func(Context) Result { return Result{} },
	},
	{
		ID: "QK010", Check: "docs.impact", Title: "Docs change with mapped code", Phase: 1, EnforcedFrom: Silver,
		Why: "Changed paths in `impact_map` need their docs changed too.", Fix: "Update the mapped docs.",
		Run: func(Context) Result { return Result{} },
	},
	{
		ID: "QK110", Check: "ownership.codeowners", Title: "CODEOWNERS covers every top-level path", Phase: 1, EnforcedFrom: Gold,
		Why: "Every finding needs an owner.", Fix: "Add a CODEOWNERS line.",
		Run: func(Context) Result { return Result{} },
	},
}

func TestWriteReportFormats(t *testing.T) {
	tests := []struct {
		format string
		report Report
		golden string
	}{
		{"text", sample, "report.text.golden"},
		{"json", sample, "report.json.golden"},
		{"claude", sample, "report.claude.golden"},
		{"text", clean, "clean.text.golden"},
		{"claude", clean, "clean.claude.golden"},
		{"claude", onlySkipped, "skipped.claude.golden"},
		{"json", Report{}, "empty.json.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteReport(&buf, tt.report, tt.format); err != nil {
				t.Fatal(err)
			}
			assertGolden(t, tt.golden, buf.Bytes())
		})
	}
}

func TestWriteReportRejectsUnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, sample, "sarif-ish"); err == nil {
		t.Fatal("want an error for an unknown format")
	}
	if buf.Len() != 0 {
		t.Errorf("wrote %q for an unknown format", buf.String())
	}
}

func TestWriteReference(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReference(&buf, refRules); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "reference.golden", buf.Bytes())
}

func TestWriteReferenceWithNoRules(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReference(&buf, nil); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "reference-empty.golden", buf.Bytes())
}

func TestWriteRuleIsTheReferenceSection(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteRule(&buf, refRules[1]); err != nil {
		t.Fatal(err)
	}
	want := "## QK010 — Docs change with mapped code\n" +
		"\n" +
		"- Check key: `docs.impact`\n" +
		"- Phase: 1\n" +
		"- Enforced by default at: silver, gold\n" +
		"\n" +
		"**What it checks.** Changed paths in `impact_map` need their docs changed too.\n" +
		"\n" +
		"**How to fix.** Update the mapped docs.\n"
	if got := buf.String(); got != want {
		t.Errorf("WriteRule =\n%s\nwant\n%s", got, want)
	}
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
