package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func load(t *testing.T, fixture string) Loaded {
	t.Helper()
	return Load(filepath.Join("testdata", fixture))
}

func TestLoadWithoutManifest(t *testing.T) {
	l := Load(t.TempDir())

	if l.Present || l.Err != nil || len(l.Problems) != 0 {
		t.Errorf("Load = %+v, want not present, no error, no problems", l)
	}
}

func TestLoadValidManifestUsingEveryKey(t *testing.T) {
	l := load(t, "valid")

	if !l.Present || l.Err != nil || len(l.Problems) != 0 {
		t.Fatalf("Load = %+v, want present and valid", l)
	}
	if l.Manifest.Version != 1 || l.Manifest.Maturity != "silver" {
		t.Errorf("manifest = %+v", l.Manifest)
	}
	// Unquoted YAML `off` must stay the mode "off", not become boolean false.
	want := map[string]string{"docs.impact": "enforced", "contracts.breaking": "off"}
	if !reflect.DeepEqual(l.Manifest.Modes, want) {
		t.Errorf("modes = %v, want %v", l.Manifest.Modes, want)
	}
	wantMap := []ImpactEntry{
		{Paths: []string{"internal/config/**"}, Docs: []string{"docs/reference/settings.md"}},
		{Paths: []string{"web/app/(admin)/**"}, Docs: []string{"docs/how-to/admin/**"}},
	}
	if !reflect.DeepEqual(l.Manifest.Docs.ImpactMap, wantMap) {
		t.Errorf("impact map = %+v, want %+v", l.Manifest.Docs.ImpactMap, wantMap)
	}
}

func TestLoadMinimalManifest(t *testing.T) {
	l := load(t, "minimal")

	if len(l.Problems) != 0 || l.Manifest.Version != 1 || l.Manifest.Maturity != "" {
		t.Errorf("Load = %+v", l)
	}
}

func TestLoadReportsProblems(t *testing.T) {
	tests := []struct {
		fixture string
		want    []string // each problem as "path: message", checked by substring, in order
	}{
		{"unknown-key", []string{"(top level): additional properties 'servise' not allowed"}},
		{"bad-impact-map", []string{"docs.impact_map[0]: missing property 'docs'"}},
		{"nested-typo", []string{"docs: additional properties 'impact_mapp' not allowed"}},
		{"bad-enum", []string{"maturity: value must be one of 'bronze', 'silver', 'gold'"}},
		{"bad-mode", []string{"modes.docs.impact: value must be one of 'off', 'advisory', 'enforced'"}},
		{"bad-version", []string{"version: version 2 is not supported by this qk, which reads version 1; upgrade qk"}},
		{"old-version", []string{"version: value must be 1"}},
		{"missing-version", []string{"(top level): missing property 'version'"}},
		{"empty", []string{"quality.yaml is empty; it needs at least `version: 1`"}},
		{"bad-yaml", []string{"yaml: line 3: did not find expected key"}},
		{"duplicate-key", []string{`"maturity" already defined`}},
		{"non-string-key", []string{"yaml: invalid map key"}},
		{"several", []string{
			"(top level): additional properties 'extra' not allowed",
			"docs: additional properties 'rot' not allowed",
			"maturity: value must be one of",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			l := load(t, tt.fixture)
			if !l.Present || l.Err != nil {
				t.Fatalf("Load = %+v, want present and readable", l)
			}
			if len(l.Problems) != len(tt.want) {
				t.Fatalf("problems = %q, want %d", l.Problems, len(tt.want))
			}
			for i, w := range tt.want {
				if got := l.Problems[i].String(); !strings.Contains(got, w) {
					t.Errorf("problem %d = %q, want it to contain %q", i, got, w)
				}
			}
		})
	}
}

func TestLoadUnreadableManifestIsAnErrorNotAProblem(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, FileName), 0o755); err != nil {
		t.Fatal(err)
	}

	l := Load(root)

	if !l.Present || l.Err == nil || len(l.Problems) != 0 {
		t.Errorf("Load = %+v, want present with a read error and no problems", l)
	}
}

// The embedded schema must compile wherever qk runs, including from a folder
// whose path has non-ASCII letters (a relative schema URL resolved against the
// working directory broke there).
func TestSchemaCompilesFromAnyWorkingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kvalvågnes")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if _, err := compileSchema(); err != nil {
		t.Fatal(err)
	}
}
