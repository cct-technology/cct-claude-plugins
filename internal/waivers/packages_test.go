package waivers

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A waiver for one package must not let another package through: an urgent
// patch of express is no reason to accept a day-old lodash.
func TestFindMatchesAWaiverByPackage(t *testing.T) {
	l := approved(
		Waiver{Rule: "QK060", Packages: []string{"express@4.19.3", "@acme/tools"}, Reason: "r", Owner: "o", Expires: "2026-12-31"},
		Waiver{Rule: "QK060", Paths: []string{"web/**"}, Packages: []string{"react"}, Reason: "r", Owner: "o", Expires: "2026-12-31"},
		Waiver{Rule: "QK010", Paths: []string{"docs/**"}, Reason: "r", Owner: "o", Expires: "2026-12-31"},
	)
	tests := []struct {
		name, rule, file, pkg string
		want                  bool
	}{
		{"the package at the version named", "QK060", "package.json", "express@4.19.3", true},
		{"the same package at another version", "QK060", "package.json", "express@4.19.4", false},
		{"a name without a version covers every version", "QK060", "package.json", "@acme/tools@1.0.0", true},
		{"a name without a version covers a finding without one", "QK060", "package.json", "@acme/tools", true},
		{"another package in the same manifest", "QK060", "package.json", "lodash@4.17.21", false},
		{"a name that only starts the same", "QK060", "package.json", "express-session@1.18.0", false},
		{"a finding that is about no package", "QK060", "package.json", "", false},
		{"package and path: both match", "QK060", "web/package.json", "react@19.0.0", true},
		{"package and path: the path does not", "QK060", "api/package.json", "react@19.0.0", false},
		{"a waiver without packages covers a finding about one", "QK010", "docs/a.md", "x@1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := l.Find(tt.rule, tt.file, tt.pkg, today); (got != nil) != tt.want {
				t.Errorf("Find = %+v, want found = %v", got, tt.want)
			}
		})
	}
}

// The approval names the packages. Adding one by hand afterwards widens the
// waiver, so it no longer counts.
func TestAWaiverWithOtherPackagesThanApprovedDoesNotApply(t *testing.T) {
	w := Waiver{Rule: "QK060", Packages: []string{"express@4.19.3"}, Reason: "r", Owner: "o", Expires: "2026-12-31"}
	l := approved(w)
	l.Waivers[0].Packages = []string{"express@4.19.3", "lodash"}

	if got := l.ProblemsOf(l.Waivers[0], today); !slices.ContainsFunc(got, func(p string) bool { return strings.Contains(p, "no approval") }) {
		t.Errorf("problems = %v, want one saying there is no approval", got)
	}
	if l.Find("QK060", "package.json", "lodash@4.17.21", today) != nil {
		t.Error("the widened waiver applies")
	}
}

func TestAddWritesThePackagesAndLogsThem(t *testing.T) {
	root := t.TempDir()
	w := Waiver{Rule: "QK060", Packages: []string{"express@4.19.3"}, Reason: "urgent fix", Owner: "o", Expires: "2026-12-31"}

	if err := Add(root, w, approval); err != nil {
		t.Fatal(err)
	}

	l := Load(root)
	if len(l.Problems) != 0 || len(l.Waivers) != 1 || !slices.Equal(l.Waivers[0].Packages, w.Packages) {
		t.Fatalf("Load = %+v, want the waiver back with its package", l)
	}
	if l.Find("QK060", "package.json", "express@4.19.3", today) == nil {
		t.Error("the waiver qk wrote does not apply")
	}
	log, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(LogName)))
	if err != nil || !strings.Contains(string(log), `"packages":["express@4.19.3"]`) {
		t.Errorf("log = %s (err %v), want the packages in it", log, err)
	}
}
