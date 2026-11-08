package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
)

func qk001() check.Rule {
	return check.Rule{
		ID:           "QK001",
		Check:        "manifest.valid",
		Title:        "quality.yaml is valid",
		Phase:        1,
		EnforcedFrom: check.Bronze,
		Why: "`quality.yaml` matches the manifest schema, so every other check reads what the repo meant. " +
			"An unknown key or a mistyped mode would otherwise be ignored or misread. " +
			"A key under `modes` that names no known check is reported as a warning, not a failure, " +
			"so a manifest written for a newer `qk` does not block an older one.",
		Fix: "Correct the key or value named in the finding. Every allowed key and value is listed in `docs/reference/manifest.md`.",
		Run: runQK001,
	}
}

func runQK001(ctx check.Context) check.Result {
	m := ctx.Manifest
	switch {
	case !m.Present:
		return check.Result{Status: check.Skipped, Reason: "no quality.yaml; every check runs advisory with defaults"}
	case m.Err != nil:
		return check.Result{Status: check.Skipped, Reason: "could not read quality.yaml: " + m.Err.Error()}
	case len(m.Problems) > 0:
		res := check.Result{Status: check.Fail}
		for _, p := range m.Problems {
			res.Findings = append(res.Findings, check.Finding{File: manifest.FileName, Message: p.String()})
		}
		return res
	}

	known := checkKeys()
	var unknown []string
	for key := range m.Manifest.Modes {
		if !slices.Contains(known, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return check.Result{Status: check.Pass}
	}
	slices.Sort(unknown)
	res := check.Result{Status: check.Warn}
	for _, key := range unknown {
		res.Findings = append(res.Findings, check.Finding{File: manifest.FileName, Message: fmt.Sprintf(
			"modes.%s: no check with this key in this qk; known check keys: %s", key, strings.Join(known, ", "))})
	}
	return res
}

// checkKeys lists the check keys of every registered rule, sorted.
func checkKeys() []string {
	var keys []string
	for _, r := range All() {
		if !slices.Contains(keys, r.Check) {
			keys = append(keys, r.Check)
		}
	}
	slices.Sort(keys)
	return keys
}
