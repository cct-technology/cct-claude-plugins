// Command qk decides what is missing or stale around the code: docs,
// contracts, threat models, test plans and operational readiness.
// It runs the same way in Claude Code hooks, pre-push and CI.
//
// Exit codes: 0 nothing blocks, 1 an enforced rule failed, 2 usage error.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/docsimpact"
	"github.com/rkvalvaa/cct-claude-plugins/internal/hook"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

// version is set at release time with -ldflags "-X main.version=<tag>".
var version = "dev" // temporary: makes QK010 fail on this PR

// checkFormats are the --format values for qk check. SARIF needs the rule
// registry and the qk version, so it is written by check.WriteSARIF.
var checkFormats = append(slices.Clone(check.Formats), "sarif")

// errBlocking means an enforced rule failed: exit 1, and nothing on stderr.
var errBlocking = errors.New("blocking findings")

func main() {
	os.Exit(run(rules.All(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(reg []check.Rule, args []string, stdin io.Reader, stdout, stderr io.Writer) (code int) {
	// Hooks fail open. Claude Code reads exit 2 as "block", so anything that
	// goes wrong in `qk hook`, even a hook this qk does not have, is a warning.
	isHook := len(args) > 0 && args[0] == "hook"
	if isHook {
		defer func() {
			if p := recover(); p != nil {
				fmt.Fprintln(stderr, "qk: warning:", p)
				code = 0
			}
		}()
	}
	root := newRootCmd(reg)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	switch {
	case err == nil:
		return 0
	case isHook:
		fmt.Fprintln(stderr, "qk: warning:", err)
		return 0
	case errors.Is(err, errBlocking):
		return 1
	}
	fmt.Fprintln(stderr, "qk:", err)
	return 2
}

func newRootCmd(reg []check.Rule) *cobra.Command {
	root := &cobra.Command{
		Use:           "qk",
		Short:         "Quality Kit: keeps docs, contracts and readiness in step with the code",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the qk version and the manifest schema version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "qk %s (manifest schema %d)\n", version, manifest.SchemaVersion)
		},
	})

	var (
		format, base string
		changedOnly  bool
	)
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Run every check and report what is missing or stale",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !slices.Contains(checkFormats, format) {
				return fmt.Errorf("unknown --format %q (want one of %s)", format, strings.Join(checkFormats, ", "))
			}
			root, err := repoRoot()
			if err != nil {
				return err
			}
			ctx := newContext(root, base)
			ctx.ChangedOnly = changedOnly
			if errors.Is(ctx.ChangesErr, changes.ErrBadBase) {
				return ctx.ChangesErr // the user named a base that does not exist: usage error
			}
			rep := check.Run(reg, check.ConfigFrom(ctx.Manifest), ctx)
			if format == "sarif" {
				err = check.WriteSARIF(cmd.OutOrStdout(), rep, reg, version)
			} else {
				err = check.WriteReport(cmd.OutOrStdout(), rep, format)
			}
			if err != nil {
				return err
			}
			if rep.Blocking {
				return errBlocking
			}
			return nil
		},
	}
	checkCmd.Flags().StringVar(&format, "format", "text", "output format: "+strings.Join(checkFormats, ", "))
	checkCmd.Flags().StringVar(&base, "base", "", "compare against the merge-base with this ref (default: the default branch, from origin/HEAD, main or master)")
	checkCmd.Flags().BoolVar(&changedOnly, "changed", false, "limit checks that scan the whole repo to the files changed against the base")
	root.AddCommand(checkCmd)
	root.AddCommand(&cobra.Command{
		Use:   "context",
		Short: "Print a short summary for the start of a session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			return check.WriteSessionSummary(cmd.OutOrStdout(), reg, newContext(root, ""))
		},
	})
	root.AddCommand(newWaiverCmd(reg))
	root.AddCommand(newDocsImpactCmd())
	root.AddCommand(newHookCmd(reg))

	root.AddCommand(&cobra.Command{
		Use:   "explain <rule-id>",
		Short: "Explain what a rule checks, why, and how to fix it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, r := range reg {
				if strings.EqualFold(r.ID, args[0]) {
					return check.WriteRule(cmd.OutOrStdout(), r)
				}
			}
			return fmt.Errorf("unknown rule %s", strings.ToUpper(args[0]))
		},
	})

	return root
}

// newContext gathers what rules see for the repository at root.
func newContext(root, base string) check.Context {
	ctx := check.Context{Root: root, Manifest: manifest.Load(root), Waivers: waivers.Load(root), Today: time.Now()}
	ctx.Changes, ctx.ChangesErr = changes.Compute(root, base)
	return ctx
}

// repoRoot is the git top level, or the working directory outside git.
func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if root, err := changes.Root(cwd); err == nil {
		return root, nil
	}
	return cwd, nil
}

func newWaiverCmd(reg []check.Rule) *cobra.Command {
	var (
		w          waivers.Waiver
		approvedBy string
	)
	add := &cobra.Command{
		Use:   "add <rule-id>",
		Short: "Record an agreed, owned, time-limited exception to a rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w.Rule = strings.ToUpper(args[0])
			known := w.Rule == waivers.NeverWaived // reported below as "cannot be waived"
			for _, r := range reg {
				known = known || r.ID == w.Rule
			}
			if !known {
				return fmt.Errorf("unknown rule %s", w.Rule)
			}
			now := time.Now()
			problems := w.Problems(now)
			if strings.TrimSpace(approvedBy) == "" {
				problems = append(problems, "no approver")
			}
			if len(problems) > 0 {
				return errors.New(strings.Join(problems, "; "))
			}
			root, err := repoRoot()
			if err != nil {
				return err
			}
			approval := waivers.Approval{By: approvedBy, Identity: changes.Identity(root), Time: now}
			if err := waivers.Add(root, w, approval); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Waived %s until %s, approved by %s. Recorded in %s and %s\n",
				w.Rule, w.Expires, approvedBy, waivers.FileName, waivers.LogName)
			return nil
		},
	}
	add.Flags().StringVar(&w.Reason, "reason", "", "why this exception is acceptable (required)")
	add.Flags().StringVar(&w.Owner, "owner", "", "who answers for this exception (required)")
	add.Flags().StringVar(&w.Expires, "expires", "", "last day the waiver applies, YYYY-MM-DD (required)")
	add.Flags().StringArrayVar(&w.Paths, "path", nil, "glob of files the waiver covers; repeatable (default: every finding of the rule)")
	add.Flags().StringVar(&approvedBy, "approved-by", "", "the person who approved this exception (required)")
	for _, f := range []string{"reason", "owner", "expires", "approved-by"} {
		_ = add.MarkFlagRequired(f)
	}

	waiver := &cobra.Command{Use: "waiver", Short: "Manage waivers in " + waivers.FileName}
	waiver.AddCommand(add)
	return waiver
}

func newHookCmd(reg []check.Rule) *cobra.Command {
	h := &cobra.Command{
		Use: "hook", Short: "Entry points for Claude Code hooks", Hidden: true,
		// Without RunE, cobra prints help to stdout for an unknown hook, and
		// SessionStart would add that help to Claude's context.
		RunE: func(_ *cobra.Command, args []string) error {
			return fmt.Errorf("this qk (%s) has no hook %q", version, strings.Join(args, " "))
		},
	}
	h.AddCommand(hookCmd("pre-tool-use", "Decide a PreToolUse hook from its JSON on stdin", func(input []byte) (any, error) {
		out, err := hook.PreToolUse(input)
		if out == nil {
			return nil, err
		}
		return out, err
	}))
	h.AddCommand(hookCmd("session-start", "Add the session summary to Claude's context", func([]byte) (any, error) {
		root, err := repoRoot()
		if err != nil {
			return nil, err
		}
		var summary strings.Builder
		if err := check.WriteSessionSummary(&summary, reg, newContext(root, "")); err != nil {
			return nil, err
		}
		return hook.SessionStart(summary.String()), nil
	}))
	h.AddCommand(hookCmd("stop", "Check what changed before Claude stops", func(input []byte) (any, error) {
		root, err := repoRoot()
		if err != nil {
			return nil, err
		}
		ctx := newContext(root, "")
		ctx.ChangedOnly = true
		rep := check.Run(reg, check.ConfigFrom(ctx.Manifest), ctx)
		var findings strings.Builder
		if err := check.WriteReport(&findings, rep, "claude"); err != nil {
			return nil, err
		}
		notify := rep.Summary.Warn > 0
		for _, r := range rep.Results {
			// Without quality.yaml, QK001 is always skipped: saying so after every turn is noise.
			if r.Status == check.Skipped && (ctx.Manifest.Present || r.Rule != "QK001") {
				notify = true
			}
		}
		out, err := hook.Stop(input, hook.StopCheck{Blocking: rep.Blocking, Findings: findings.String(), Notify: notify}, hookStateDir())
		if out == nil {
			return nil, err
		}
		return out, err
	}))
	return h
}

// hookCmd reads a hook's JSON on stdin and writes decide's answer as JSON. A
// nil answer writes nothing. Errors go to run, which makes them warnings.
func hookCmd(use, short string, decide func(input []byte) (any, error)) *cobra.Command {
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			input, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			out, err := decide(input)
			if err != nil || out == nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetEscapeHTML(false) // findings are read by people and Claude: keep <, > and & as written
			return enc.Encode(out)
		},
	}
}

// hookStateDir is where hooks keep state between calls: the plugin's data
// directory, which survives plugin updates, or the user cache outside Claude Code.
func hookStateDir() string {
	if dir := os.Getenv("CLAUDE_PLUGIN_DATA"); dir != "" {
		return dir
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "quality-kit")
	}
	return filepath.Join(dir, "quality-kit")
}

func newDocsImpactCmd() *cobra.Command {
	var reason, approvedBy string
	none := &cobra.Command{
		Use:   "none",
		Short: "Record a person's approval that this branch's change needs no docs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var problems []string
			if strings.TrimSpace(reason) == "" {
				problems = append(problems, "no reason")
			}
			if strings.TrimSpace(approvedBy) == "" {
				problems = append(problems, "no approver")
			}
			if len(problems) > 0 {
				return errors.New(strings.Join(problems, "; "))
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			root, err := changes.Root(cwd)
			if err != nil {
				return fmt.Errorf("a docs-impact note needs a git branch: %w", err)
			}
			approval := waivers.Approval{By: approvedBy, Identity: changes.Identity(root), Time: time.Now()}
			path, err := docsimpact.Write(root, changes.Branch(root), reason, approval)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "No docs needed, approved by %s. Recorded in %s and %s\n", approvedBy, path, waivers.LogName)
			return nil
		},
	}
	none.Flags().StringVar(&reason, "reason", "", "why this change needs no docs (required)")
	none.Flags().StringVar(&approvedBy, "approved-by", "", "the person who approved this (required)")
	for _, f := range []string{"reason", "approved-by"} {
		_ = none.MarkFlagRequired(f)
	}
	impact := &cobra.Command{Use: "docs-impact", Short: "Say what a change means for the docs"}
	impact.AddCommand(none)
	return impact
}
