// Package rules is the registry of every rule qk runs, in report order.
// It is the single source for `qk check`, `qk explain` and the generated
// rule reference. Add a rule here; never renumber or reuse an ID.
package rules

import "github.com/rkvalvaa/cct-claude-plugins/internal/check"

// All returns the registered rules. It builds the list on each call, so a
// rule's Run may call All without an initialisation cycle.
func All() []check.Rule {
	return []check.Rule{
		qk001(),
		qk010(),
		qk020(),
		qk030(),
		qk040(),
		qk061(),
		qk100(),
		qk110(),
	}
}

// noChangeSet is what a rule that needs the change set returns when there is none.
func noChangeSet(ctx check.Context) check.Result {
	reason := "no change set"
	if ctx.ChangesErr != nil {
		reason = ctx.ChangesErr.Error()
	}
	return check.Result{Status: check.Skipped, Reason: reason}
}
