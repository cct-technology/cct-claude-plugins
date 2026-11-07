package rules

import (
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
)

// Guards every rule added to the registry: stable QKnnn IDs, no duplicates,
// a check key, a default maturity and a Run func.
func TestRegistryIsValid(t *testing.T) {
	if err := check.ValidateRegistry(All()); err != nil {
		t.Fatal(err)
	}
}
