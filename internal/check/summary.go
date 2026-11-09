package check

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

const (
	noManifestSummary = "Quality Kit: no quality.yaml, so checks run advisory with defaults. The quality-init skill sets it up.\n"

	// The summary is added to the context of every session, so it stays short.
	summaryMaxLines = 30
	expiringDays    = 14
)

// slowRules use the network or a long scan. Named here, not on the rule, so
// they stay out of the session-start summary from the day they are registered.
var slowRules = []string{"QK020", "QK060"}

// WriteSessionSummary writes what `qk context` prints: maturity and modes, open
// findings per rule, and waivers about to end. Skipped checks and slow rules
// left out are named, so a check that did not run never reads as a pass.
func WriteSessionSummary(w io.Writer, rules []Rule, ctx Context) error {
	if !ctx.Manifest.Present {
		_, err := io.WriteString(w, noManifestSummary)
		return err
	}

	cfg := ConfigFrom(ctx.Manifest)
	var fast []Rule
	var enforced, advisory, notRun []string
	for _, r := range rules {
		switch cfg.ModeFor(r) {
		case Off:
			continue
		case Enforced:
			enforced = append(enforced, r.ID)
		case Advisory:
			advisory = append(advisory, r.ID)
		}
		if slices.Contains(slowRules, r.ID) {
			notRun = append(notRun, r.ID)
			continue
		}
		fast = append(fast, r)
	}

	lines := []string{"Quality Kit: maturity " + cfg.Maturity.String() + "."}
	if len(enforced) > 0 {
		lines = append(lines, "Enforced: "+strings.Join(enforced, ", ")+".")
	}
	if len(advisory) > 0 {
		lines = append(lines, "Advisory: "+strings.Join(advisory, ", ")+".")
	}

	var open, skipped []string
	for _, r := range Run(fast, cfg, ctx).Results {
		switch r.Status {
		case Fail, Warn:
			count := ""
			if n := len(r.Findings); n > 0 {
				count = fmt.Sprintf(", %d open", n)
			}
			open = append(open, fmt.Sprintf("- %s %s%s: %s", r.Rule, r.Mode, count, r.Title))
		case Skipped:
			skipped = append(skipped, fmt.Sprintf("%s (%s)", r.Rule, r.Reason))
		}
	}
	var body []string
	if len(open) == 0 {
		body = append(body, "No open findings.")
	} else {
		body = append(body, "Open findings:")
		body = append(body, open...)
		body = append(body, "Details: qk check --format claude")
	}
	if expiring := ctx.Waivers.ExpiringWithin(expiringDays, ctx.Today); len(expiring) > 0 {
		body = append(body, fmt.Sprintf("Waivers expiring within %d days:", expiringDays))
		for _, e := range expiring {
			what := strings.TrimSpace(e.Rule + " " + strings.Join(e.Paths, ", "))
			body = append(body, fmt.Sprintf("- %s, ends %s (owner %s)", what, e.Expires, e.Owner))
		}
	}

	// Checks that did not run are always shown, so the cap cuts the body.
	var tail []string
	if len(skipped) > 0 {
		tail = append(tail, "Skipped: "+strings.Join(skipped, ", "))
	}
	if len(notRun) > 0 {
		tail = append(tail, "Not run at session start: "+strings.Join(notRun, ", ")+".")
	}

	if room := summaryMaxLines - len(lines) - len(tail) - 1; len(body) > room+1 {
		body = append(body[:room], fmt.Sprintf("... %d more lines. Run qk check.", len(body)-room))
	}
	lines = append(append(lines, body...), tail...)
	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}
