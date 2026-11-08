package rules

import (
	"fmt"
	"slices"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

func qk100() check.Rule {
	return check.Rule{
		ID:           waivers.NeverWaived,
		Check:        "waivers.valid",
		Title:        "Waivers are valid, owned and not expired",
		Phase:        1,
		EnforcedFrom: check.Silver,
		Why: "A waiver in `.quality/waivers.yaml` needs a reason, an owner and an end date, so an exception cannot quietly become permanent. " +
			"An expired or incomplete waiver never applies: the finding it covered is reported again. " +
			"A waiver naming a rule this `qk` does not have is a warning, not a failure, because it hides nothing and may come from a newer `qk`. " +
			"QK100 itself cannot be waived.",
		Fix: "Fix the finding and delete the waiver, or correct the field named here. To extend a waiver, give it a new date and a current reason.",
		Run: runQK100,
	}
}

func runQK100(ctx check.Context) check.Result {
	l := ctx.Waivers
	if l.Err != nil {
		return check.Result{Status: check.Skipped, Reason: "could not read " + waivers.FileName + ": " + l.Err.Error()}
	}

	var fails, warns []check.Finding
	add := func(list *[]check.Finding, msg string) {
		*list = append(*list, check.Finding{File: waivers.FileName, Message: msg})
	}
	for _, p := range l.Problems {
		add(&fails, p)
	}
	known := ruleIDs()
	for i, w := range l.Waivers {
		for _, p := range w.Problems(ctx.Today) {
			add(&fails, fmt.Sprintf("waivers[%d] (%s): %s", i, w.Rule, p))
		}
		if w.Rule != "" && !slices.Contains(known, w.Rule) {
			add(&warns, fmt.Sprintf("waivers[%d] (%s): no rule with this ID in this qk", i, w.Rule))
		}
	}

	switch {
	case len(fails) > 0:
		return check.Result{Status: check.Fail, Findings: append(fails, warns...)}
	case len(warns) > 0:
		return check.Result{Status: check.Warn, Findings: warns}
	}
	return check.Result{Status: check.Pass}
}

func ruleIDs() []string {
	var ids []string
	for _, r := range All() {
		ids = append(ids, r.ID)
	}
	return ids
}
