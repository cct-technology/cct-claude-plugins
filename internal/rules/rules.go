// Package rules is the registry of every rule qk runs, in report order.
// It is the single source for `qk check`, `qk explain` and the generated
// rule reference. Add a rule here; never renumber or reuse an ID.
package rules

import "github.com/rkvalvaa/cct-claude-plugins/internal/check"

// All returns the registered rules.
func All() []check.Rule {
	return nil
}
