// Package check runs Quality Kit rules and decides what their results mean.
// Rules report facts; the mode for each rule (from maturity and overrides)
// decides whether a failure blocks.
package check

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

// Status is what a rule reports.
type Status string

const (
	Pass    Status = "pass"
	Warn    Status = "warn"
	Fail    Status = "fail"
	Skipped Status = "skipped" // never a failure: tooling problems fail open
)

// Mode decides what a rule's failure does.
type Mode string

const (
	Off      Mode = "off"      // the rule does not run
	Advisory Mode = "advisory" // a fail is reported as a warning
	Enforced Mode = "enforced" // a fail blocks
)

// Maturity is the level a repo declares in quality.yaml.
type Maturity int

const (
	NoManifest Maturity = iota // no quality.yaml: everything is advisory
	Bronze
	Silver
	Gold
)

func (m Maturity) String() string {
	return [...]string{"none", "bronze", "silver", "gold"}[m]
}

// Finding is one thing a rule found wrong.
type Finding struct {
	File    string `json:"file,omitempty"`
	Message string `json:"message"`
}

// Result is what a rule's Run returns.
type Result struct {
	Status   Status
	Reason   string // why the rule was skipped
	Findings []Finding
}

// Context is what a rule can see.
type Context struct {
	Root        string          // repository root
	Manifest    manifest.Loaded // quality.yaml, if any
	Changes     *changes.Set    // what changed against the base; nil when ChangesErr is set
	ChangesErr  error           // why there is no change set: rules that need one skip with it as reason
	ChangedOnly bool            // --changed: checks that scan the whole repo look only at changed files
	Waivers     waivers.Loaded  // .quality/waivers.yaml, if any
	Today       time.Time       // the day waivers are judged against
}

// Rule is one registered check. IDs are stable and never reused.
type Rule struct {
	ID           string   // QKnnn
	Check        string   // key used under `modes` in quality.yaml; may cover several rules
	Title        string   // short statement of what holds when the rule passes
	Phase        int      // spec phase that introduced the rule
	EnforcedFrom Maturity // lowest maturity at which the rule blocks by default
	Why          string   // what it checks and why
	Fix          string   // how to fix a finding
	Run          func(Context) Result
}

// Config is the part of quality.yaml that decides modes.
type Config struct {
	Maturity Maturity
	Modes    map[string]Mode // check key -> override
}

// ConfigFrom turns a loaded quality.yaml into a Config.
func ConfigFrom(l manifest.Loaded) Config {
	if !l.Present || l.Err != nil {
		return Config{} // no manifest, or unreadable (fail open): everything advisory
	}
	// A manifest exists, so the repo is at least bronze and QK001 blocks.
	// An invalid one cannot be trusted further: no maturity, no overrides.
	cfg := Config{Maturity: Bronze}
	if len(l.Problems) > 0 {
		return cfg
	}
	switch l.Manifest.Maturity {
	case "silver":
		cfg.Maturity = Silver
	case "gold":
		cfg.Maturity = Gold
	}
	for key, mode := range l.Manifest.Modes { // values are schema-checked: off, advisory or enforced
		if cfg.Modes == nil {
			cfg.Modes = map[string]Mode{}
		}
		cfg.Modes[key] = Mode(mode)
	}
	return cfg
}

// ModeFor returns the mode rule r runs in under c.
func (c Config) ModeFor(r Rule) Mode {
	if m, ok := c.Modes[r.Check]; ok {
		return m
	}
	if c.Maturity >= r.EnforcedFrom { // EnforcedFrom is at least Bronze, so NoManifest never enforces
		return Enforced
	}
	return Advisory
}

// RuleResult is one rule's outcome after its mode is applied.
// The JSON shape is a contract read by hooks, SARIF and CI: add fields, never rename.
type RuleResult struct {
	Rule     string          `json:"rule"`
	Check    string          `json:"check"`
	Title    string          `json:"title"`
	Mode     Mode            `json:"mode"`
	Status   Status          `json:"status"`
	Reason   string          `json:"reason,omitempty"`
	Findings []Finding       `json:"findings,omitempty"`
	Waived   []WaivedFinding `json:"waived,omitempty"`
	Fix      string          `json:"fix,omitempty"`
}

// WaivedFinding is a finding covered by a valid waiver.
type WaivedFinding struct {
	File    string `json:"file,omitempty"`
	Message string `json:"message"`
	Owner   string `json:"owner"`
	Expires string `json:"expires"`
	Reason  string `json:"reason"`
}

// Summary counts rule results by status.
type Summary struct {
	Pass    int `json:"pass"`
	Warn    int `json:"warn"`
	Fail    int `json:"fail"`
	Skipped int `json:"skipped"`
	Waived  int `json:"waived"` // findings, not rules
}

// Report is the outcome of a check run.
type Report struct {
	Results  []RuleResult `json:"results"`
	Summary  Summary      `json:"summary"`
	Blocking bool         `json:"blocking"` // an enforced rule failed
}

// Run runs every rule not turned off, in registry order.
func Run(rules []Rule, cfg Config, ctx Context) Report {
	rep := Report{Results: []RuleResult{}}
	for _, r := range rules {
		mode := cfg.ModeFor(r)
		if mode == Off {
			continue
		}
		res := runSafely(r, ctx)
		if res.Status == Fail && mode == Advisory {
			res.Status = Warn
		}
		rr := RuleResult{
			Rule: r.ID, Check: r.Check, Title: r.Title, Mode: mode,
			Status: res.Status, Reason: res.Reason,
		}
		rr.Findings, rr.Waived = applyWaivers(r.ID, res.Findings, ctx)
		if len(rr.Waived) > 0 && len(rr.Findings) == 0 {
			rr.Status = Pass // everything this rule found is covered
		}
		rep.Summary.Waived += len(rr.Waived)
		switch rr.Status {
		case Pass:
			rep.Summary.Pass++
		case Warn:
			rep.Summary.Warn++
			rr.Fix = r.Fix
		case Fail:
			rep.Summary.Fail++
			rr.Fix = r.Fix
			rep.Blocking = true
		case Skipped:
			rep.Summary.Skipped++
		}
		rep.Results = append(rep.Results, rr)
	}
	return rep
}

// applyWaivers splits findings into those still open and those a valid waiver covers.
func applyWaivers(rule string, findings []Finding, ctx Context) (open []Finding, waived []WaivedFinding) {
	for _, f := range findings {
		w := ctx.Waivers.Find(rule, f.File, ctx.Today)
		if w == nil {
			open = append(open, f)
			continue
		}
		waived = append(waived, WaivedFinding{File: f.File, Message: f.Message, Owner: w.Owner, Expires: w.Expires, Reason: w.Reason})
	}
	return open, waived
}

// runSafely turns a crashing rule into a skip: tooling problems fail open.
func runSafely(r Rule, ctx Context) (res Result) {
	defer func() {
		if p := recover(); p != nil {
			res = Result{Status: Skipped, Reason: fmt.Sprintf("rule crashed: %v", p)}
		}
	}()
	return r.Run(ctx)
}

var ruleID = regexp.MustCompile(`^QK\d{3}$`)

// ValidateRegistry checks the invariants every registered rule must hold.
func ValidateRegistry(rules []Rule) error {
	var errs []error
	seen := map[string]bool{}
	for _, r := range rules {
		switch {
		case !ruleID.MatchString(r.ID):
			errs = append(errs, fmt.Errorf("rule ID %q is not QKnnn", r.ID))
		case seen[r.ID]:
			errs = append(errs, fmt.Errorf("duplicate rule ID %s", r.ID))
		}
		seen[r.ID] = true
		if r.Check == "" {
			errs = append(errs, fmt.Errorf("%s: no check key", r.ID))
		}
		if r.EnforcedFrom < Bronze || r.EnforcedFrom > Gold {
			errs = append(errs, fmt.Errorf("%s: EnforcedFrom must be bronze, silver or gold", r.ID))
		}
		if r.Run == nil {
			errs = append(errs, fmt.Errorf("%s: no Run func", r.ID))
		}
	}
	return errors.Join(errs...)
}
