package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cct-technology/cct-claude-plugins/internal/changes"
	"github.com/cct-technology/cct-claude-plugins/internal/check"
)

// oasdiffTimeout bounds one comparison: a `$ref` to a slow URL must not hold the Stop hook.
const oasdiffTimeout = time.Minute

const oasdiffHint = "oasdiff is not installed; install it from https://github.com/oasdiff/oasdiff, for example `go install github.com/oasdiff/oasdiff@latest`"

// oasdiffErr is the level oasdiff gives a change that certainly breaks clients; below it is a warning.
const oasdiffErr = 3

func qk030() check.Rule {
	return check.Rule{
		ID:           "QK030",
		Check:        "contracts.breaking",
		Title:        "No breaking OpenAPI changes",
		Phase:        1,
		EnforcedFrom: check.Silver,
		Why: "A breaking API change should be caught before it merges, not by the clients it breaks. For each changed file matching `contracts.openapi` in `quality.yaml`, QK030 runs `oasdiff breaking` on its version at the base and its version now. " +
			"Each change that breaks clients is a finding. A change oasdiff cannot confirm as breaking, such as a removed request parameter, is a warning marked `possibly breaking`. " +
			"A deleted contract fails. A new one passes. A renamed one is compared with what it held under its old name. " +
			"Only the listed file starts a comparison: a change in a file it reaches through `$ref` is seen when the listed file changes too. " +
			"When `oasdiff` is not installed, or cannot read a contract, the check is skipped and says why.",
		Fix: "Keep the API compatible: put back what was removed, or make the new requirement optional. If the break is intended, release it as a new API version, or ask a person to approve a waiver with `qk waiver add QK030`.",
		Run: runQK030,
	}
}

func runQK030(ctx check.Context) check.Result {
	globs := ctx.Manifest.Manifest.Contracts.OpenAPI
	if len(globs) == 0 {
		return check.Result{Status: check.Pass}
	}
	if ctx.Changes == nil {
		return noChangeSet(ctx)
	}

	oasdiff, lookErr := exec.LookPath("oasdiff")
	var findings []check.Finding
	var notRun []string
	breaking := false
	for _, f := range ctx.Changes.Files {
		if !matches(globs, f) {
			continue
		}
		switch {
		case f.Status == changes.Added:
			continue // nothing at the base to break
		case f.Status == changes.Deleted:
			findings = append(findings, check.Finding{File: f.Path, Message: "deleted: every client of this API breaks"})
			breaking = true
			continue
		case lookErr != nil:
			notRun = []string{oasdiffHint}
			continue
		}
		old := f.Path
		if f.OldPath != "" {
			old = f.OldPath
		}
		// ponytail: oasdiff reads the base from git itself, `$ref`s included. On Windows it
		// asks git for `$ref` targets with backslashes (seen in 1.35 and 1.37), so a
		// multi-file contract is skipped there; copy the base tree out if that matters.
		found, err := breakingChanges(ctx.Root, oasdiff, ctx.Changes.Base+":"+old, f.Path)
		if err != nil {
			notRun = append(notRun, fmt.Sprintf("%s: %v", f.Path, err)) // one unreadable contract must not hide the others
			continue
		}
		for _, c := range found {
			msg := c.String()
			if c.Level >= oasdiffErr {
				breaking = true
			} else {
				msg = "possibly breaking: " + msg
			}
			findings = append(findings, check.Finding{File: f.Path, Message: msg})
		}
	}
	switch {
	case breaking:
		// ponytail: possibly breaking changes fail along with the certain ones, and
		// contracts that were not compared go unmentioned (SARIF level is per rule).
		return check.Result{Status: check.Fail, Findings: findings}
	case len(findings) > 0:
		return check.Result{Status: check.Warn, Findings: findings}
	case len(notRun) > 0:
		return check.Result{Status: check.Skipped, Reason: strings.Join(notRun, "; ")}
	}
	return check.Result{Status: check.Pass}
}

// oasChange is one entry of `oasdiff breaking --format json`.
type oasChange struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	Level     int    `json:"level"`
	Operation string `json:"operation"`
	Path      string `json:"path"`
}

func (c oasChange) String() string {
	where := strings.TrimSpace(c.Operation + " " + c.Path)
	if where != "" {
		where += ": "
	}
	return fmt.Sprintf("%s%s (%s)", where, c.Text, c.ID)
}

// breakingChanges runs oasdiff in root. base is `<commit>:<path>`, which oasdiff
// reads from git; revision is a path in the working tree.
func breakingChanges(root, oasdiff, base, revision string) ([]oasChange, error) {
	ctx, cancel := context.WithTimeout(context.Background(), oasdiffTimeout)
	defer cancel()
	// `--` so a contract whose name starts with a dash is never read as a flag.
	cmd := exec.CommandContext(ctx, oasdiff, "breaking", "--format", "json", "--", base, revision)
	cmd.Dir = root
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("oasdiff did not finish within %v", oasdiffTimeout)
		}
		return nil, fmt.Errorf("oasdiff: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	if len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		return nil, nil
	}
	var found []oasChange
	if err := json.Unmarshal(stdout.Bytes(), &found); err != nil {
		return nil, fmt.Errorf("cannot read what oasdiff printed: %w", err)
	}
	return found, nil
}
