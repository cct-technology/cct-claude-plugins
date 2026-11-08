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
var version = "dev"

// errBlocking means an enforced rule failed: exit 1, and nothing on stderr.
var errBlocking = errors.New("blocking findings")

func main() {
	os.Exit(run(rules.All(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(reg []check.Rule, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := newRootCmd(reg)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.Execute()
	switch {
	case err == nil:
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
			if !slices.Contains(check.Formats, format) {
				return fmt.Errorf("unknown --format %q (want one of %s)", format, strings.Join(check.Formats, ", "))
			}
			root, err := repoRoot()
			if err != nil {
				return err
			}
			ctx := check.Context{
				Root: root, Manifest: manifest.Load(root), ChangedOnly: changedOnly,
				Waivers: waivers.Load(root), Today: time.Now(),
			}
			ctx.Changes, ctx.ChangesErr = changes.Compute(root, base)
			if errors.Is(ctx.ChangesErr, changes.ErrBadBase) {
				return ctx.ChangesErr // the user named a base that does not exist: usage error
			}
			rep := check.Run(reg, check.ConfigFrom(ctx.Manifest), ctx)
			if err := check.WriteReport(cmd.OutOrStdout(), rep, format); err != nil {
				return err
			}
			if rep.Blocking {
				return errBlocking
			}
			return nil
		},
	}
	checkCmd.Flags().StringVar(&format, "format", "text", "output format: "+strings.Join(check.Formats, ", "))
	checkCmd.Flags().StringVar(&base, "base", "", "compare against the merge-base with this ref (default: the default branch, from origin/HEAD, main or master)")
	checkCmd.Flags().BoolVar(&changedOnly, "changed", false, "limit checks that scan the whole repo to the files changed against the base")
	root.AddCommand(checkCmd)
	root.AddCommand(newWaiverCmd(reg))
	root.AddCommand(newDocsImpactCmd())
	root.AddCommand(newHookCmd())

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

func newHookCmd() *cobra.Command {
	h := &cobra.Command{Use: "hook", Short: "Entry points for Claude Code hooks", Hidden: true}
	h.AddCommand(&cobra.Command{
		Use:   "pre-tool-use",
		Short: "Decide a PreToolUse hook from its JSON on stdin",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			input, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "qk: warning:", err)
				return nil // hooks fail open
			}
			out, err := hook.PreToolUse(input)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "qk: warning:", err)
				return nil
			}
			if out == nil {
				return nil
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		},
	})
	return h
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
