package check

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// sarifRules covers every rule in the sample reports, so each result has its rule's text.
var sarifRules = []Rule{
	{ID: "QK001", Check: "manifest.valid", Title: "Manifest is valid", EnforcedFrom: Bronze,
		Why: "`quality.yaml` matches the schema.", Fix: "Correct the key named in the finding."},
	{ID: "QK010", Check: "docs.impact", Title: "Docs change with mapped code", EnforcedFrom: Silver,
		Why: "Changed paths in `impact_map` need their docs changed too.", Fix: "Update the mapped docs."},
	{ID: "QK020", Check: "generated.current", Title: "Generated files are current", EnforcedFrom: Bronze,
		Why: "Generated files equal their generator's output.", Fix: "Run the generator."},
	{ID: "QK030", Check: "contracts.breaking", Title: "No breaking OpenAPI changes", EnforcedFrom: Silver,
		Why: "OpenAPI changes do not break clients.", Fix: "Version the API."},
	{ID: "QK110", Check: "ownership.codeowners", Title: "CODEOWNERS covers every top-level path", EnforcedFrom: Gold,
		Why: "Every finding needs an owner.", Fix: "Add a CODEOWNERS line."},
}

func TestWriteSARIF(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteSARIF(&buf, sample, sarifRules, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "report.sarif.golden", buf.Bytes())
}

func TestSARIFValidatesAgainstTheSchema(t *testing.T) {
	schema := sarifSchema(t)
	for name, rep := range map[string]Report{"sample": sample, "clean": clean, "only skipped": onlySkipped, "empty": {}, "paths": oddPaths} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteSARIF(&buf, rep, sarifRules, "0.1.0"); err != nil {
				t.Fatal(err)
			}
			doc, err := jsonschema.UnmarshalJSON(&buf)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(doc); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestSARIFResults(t *testing.T) {
	run := decodeSARIF(t, sample)
	if len(run.Results) != 4 {
		t.Fatalf("want 4 results (2 fail, 1 warn, 1 waived), got %d", len(run.Results))
	}
	byMessage := map[string]sarifTestResult{}
	for _, r := range run.Results {
		byMessage[r.Message.Text] = r
		if len(r.Locations) == 0 || r.Locations[0].PhysicalLocation.ArtifactLocation.URI == "" {
			t.Errorf("%q has no location; GitHub code scanning does not show it", r.Message.Text)
		}
		if run.Tool.Driver.Rules[r.RuleIndex].ID != r.RuleID {
			t.Errorf("%q: ruleIndex %d is not %s", r.Message.Text, r.RuleIndex, r.RuleID)
		}
	}
	if got := byMessage["docs/reference/settings.md must change too"].Level; got != "error" {
		t.Errorf("an enforced failure is level %q, want error", got)
	}
	if got := byMessage["no owner"].Level; got != "warning" {
		t.Errorf("an advisory finding is level %q, want warning", got)
	}
	if got := byMessage["no Docs-Impact trailer in the range"].Locations[0].PhysicalLocation.ArtifactLocation.URI; got != "quality.yaml" {
		t.Errorf("a finding without a file is placed at %q, want quality.yaml", got)
	}
	waived := byMessage["differs from generator output"]
	if len(waived.Suppressions) != 1 || waived.Suppressions[0].Kind != "external" || waived.Suppressions[0].Justification != "generator moves in CCT-1" {
		t.Errorf("a waived finding has suppressions %+v, want one external with the waiver reason", waived.Suppressions)
	}
	if len(byMessage["no owner"].Suppressions) != 0 {
		t.Error("an open finding is marked suppressed")
	}
}

func TestSARIFNamesChecksThatDidNotRun(t *testing.T) {
	run := decodeSARIF(t, sample)
	notes := run.Invocations[0].ToolExecutionNotifications
	if len(notes) != 1 || notes[0].AssociatedRule.ID != "QK030" || !strings.Contains(notes[0].Message.Text, "oasdiff not installed") {
		t.Errorf("want one notification naming QK030 and its reason, got %+v", notes)
	}
}

var oddPaths = Report{
	Results: []RuleResult{{Rule: "QK010", Check: "docs.impact", Title: "Docs change with mapped code", Mode: Enforced, Status: Fail,
		Findings: []Finding{{File: "docs/a b/ü.md", Message: "one"}, {File: "docs/a b/ü.md", Message: "two"}}}},
	Summary:  Summary{Fail: 1},
	Blocking: true,
}

func TestSARIFEscapesPathsAndKeepsFindingsApart(t *testing.T) {
	run := decodeSARIF(t, oddPaths)
	one, two := run.Results[0], run.Results[1]
	if got := one.Locations[0].PhysicalLocation.ArtifactLocation.URI; got != "docs/a%20b/%C3%BC.md" {
		t.Errorf("uri %q, want docs/a%%20b/%%C3%%BC.md", got)
	}
	// GitHub merges results with the same fingerprint into one alert.
	if one.PartialFingerprints["primaryLocationLineHash"] == two.PartialFingerprints["primaryLocationLineHash"] {
		t.Error("two findings in one file share a fingerprint")
	}
}

type sarifTestResult struct {
	RuleID    string
	RuleIndex int
	Level     string
	Message   struct{ Text string }
	Locations []struct {
		PhysicalLocation struct{ ArtifactLocation struct{ URI string } }
	}
	PartialFingerprints map[string]string
	Suppressions        []struct{ Kind, Status, Justification string }
}

type sarifTestRun struct {
	Tool struct {
		Driver struct {
			Rules []struct{ ID string }
		}
	}
	Invocations []struct {
		ToolExecutionNotifications []struct {
			Message        struct{ Text string }
			AssociatedRule struct{ ID string }
		}
	}
	Results []sarifTestResult
}

func decodeSARIF(t *testing.T, rep Report) sarifTestRun {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteSARIF(&buf, rep, sarifRules, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	var log struct{ Runs []sarifTestRun }
	if err := json.Unmarshal(buf.Bytes(), &log); err != nil {
		t.Fatal(err)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("want 1 run, got %d", len(log.Runs))
	}
	return log.Runs[0]
}

// sarifSchema is the OASIS SARIF 2.1.0 schema (errata 01), vendored unchanged from
// https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json
func sarifSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open("testdata/sarif-schema-2.1.0.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"
	c := jsonschema.NewCompiler()
	c.AssertFormat() // uri and uri-reference: paths must be escaped
	if err := c.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
