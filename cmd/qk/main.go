// Command qk decides what is missing or stale around the code: docs,
// contracts, threat models, test plans and operational readiness.
// It runs the same way in Claude Code hooks, pre-push and CI.
//
// Exit codes: 0 nothing blocks, 1 an enforced rule failed, 2 usage error.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
)

// version is set at release time with -ldflags "-X main.version=<tag>".
var version = "dev"

// errBlocking means an enforced rule failed: exit 1, and nothing on stderr.
var errBlocking = errors.New("blocking findings")

func main() {
	os.Exit(run(rules.All(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(reg []check.Rule, args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(reg)
	root.SetArgs(args)
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
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			root, err := changes.Root(cwd)
			if err != nil {
				root = cwd // not a git repo: check the folder qk runs in
			}
			ctx := check.Context{Root: root, Manifest: manifest.Load(root), ChangedOnly: changedOnly}
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
