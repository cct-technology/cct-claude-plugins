// Command qk decides what is missing or stale around the code: docs,
// contracts, threat models, test plans and operational readiness.
// It runs the same way in Claude Code hooks, pre-push and CI.
//
// Exit codes: 0 nothing blocks, 1 an enforced rule failed, 2 usage error.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cct-technology/cct-claude-plugins/internal/changes"
	"github.com/cct-technology/cct-claude-plugins/internal/check"
	"github.com/cct-technology/cct-claude-plugins/internal/deps"
	"github.com/cct-technology/cct-claude-plugins/internal/docsimpact"
	"github.com/cct-technology/cct-claude-plugins/internal/hook"
	"github.com/cct-technology/cct-claude-plugins/internal/manifest"
	"github.com/cct-technology/cct-claude-plugins/internal/rules"
	"github.com/cct-technology/cct-claude-plugins/internal/scaffold"
	"github.com/cct-technology/cct-claude-plugins/internal/waivers"
)

// version is set at release time with -ldflags "-X main.version=<tag>".
var version = "dev"

// registry is what `qk deps verify` asks. Tests point it at a fake.
var registry = deps.Default()

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
		sarifFile    string
		changedOnly  bool
		full         bool
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
			ctx.Full = full || inCI()
			if errors.Is(ctx.ChangesErr, changes.ErrBadBase) {
				return ctx.ChangesErr // the user named a base that does not exist: usage error
			}
			rep := check.Run(reg, check.ConfigFrom(ctx.Manifest), ctx)
			if sarifFile != "" {
				var sarif bytes.Buffer
				if err := check.WriteSARIF(&sarif, rep, reg, version); err != nil {
					return err
				}
				if err := os.WriteFile(sarifFile, sarif.Bytes(), 0o644); err != nil {
					return fmt.Errorf("--sarif: %w", err)
				}
			}
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
	checkCmd.Flags().StringVar(&sarifFile, "sarif", "", "also write the report as SARIF to this file, whatever --format prints")
	checkCmd.Flags().StringVar(&base, "base", "", "compare against the merge-base with this ref (default: the default branch, from origin/HEAD, main or master)")
	checkCmd.Flags().BoolVar(&changedOnly, "changed", false, "limit checks that scan the whole repo to the files changed against the base")
	checkCmd.Flags().BoolVar(&full, "full", false, "also run slow checks: QK020 runs every generator, QK060 asks the package registries (on by default when CI=true)")
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
	root.AddCommand(newInitCmd())
	root.AddCommand(newWaiverCmd(reg))
	root.AddCommand(newDocsImpactCmd())
	root.AddCommand(newDepsCmd())
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

// inCI reports whether the CI environment variable, which most CI services
// set, says this is a CI run.
func inCI() bool {
	ci, _ := strconv.ParseBool(os.Getenv("CI"))
	return ci
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

func newInitCmd() *cobra.Command {
	var o scaffold.Options
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Set a repository up for Quality Kit: quality.yaml, the folders it names, and what the git host needs",
		Long: "Writes quality.yaml, a README in each docs folder it names, and the line in .gitattributes that lets two branches each record an exception. " +
			"When the `origin` remote is on GitHub, it also writes a CODEOWNERS stub and the CI workflow.\n\n" +
			"A file that exists is never overwritten: it is reported as skipped, so a second run writes only what is still missing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := repoRoot()
			if err != nil {
				return err
			}
			o.Version = version
			return scaffold.Init(cmd.OutOrStdout(), root, o)
		},
	}
	cmd.Flags().StringVar(&o.DataClass, "data-class", "internal", "the most sensitive data the service handles: "+strings.Join(scaffold.DataClasses, ", "))
	cmd.Flags().StringVar(&o.Maturity, "maturity", "bronze", "sets which checks block: "+strings.Join(scaffold.Maturities, ", "))
	cmd.Flags().StringArrayVar(&o.Replaces, "replaces", nil, "a tool this system replaces; repeatable")
	return cmd
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
	add.Flags().StringArrayVar(&w.Packages, "package", nil, "a dependency the waiver covers, as name or name@version; repeatable (default: findings about any package)")
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
		out, err := hook.PreToolUse(input, guards(reg))
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
		rep := check.Run(check.Fast(reg), check.ConfigFrom(ctx.Manifest), ctx)
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

// lookupBudget is how long the PreToolUse hook may spend asking registries
// about one command. After it, the packages left are not checked: a hook must
// not hold the session, and QK060 checks them in CI.
const lookupBudget = 30 * time.Second

// guards tells the PreToolUse hook what the repository's generated files are
// and how to ask about a package. Each guard takes its mode from its check:
// QK020 for generated files, QK060 for installs.
func guards(reg []check.Rule) hook.Guards {
	root, err := repoRoot()
	if err != nil {
		return hook.Guards{}
	}
	loaded := manifest.Load(root)
	cfg := check.ConfigFrom(loaded)
	mode := func(id string) string {
		for _, r := range reg {
			if r.ID == id {
				return string(cfg.ModeFor(r))
			}
		}
		return ""
	}
	deadline := time.Now().Add(lookupBudget)
	return hook.Guards{
		GeneratedMode: mode("QK020"),
		DepsMode:      mode("QK060"),
		Generator:     func(path string) (string, bool) { return loaded.Manifest.Generator(root, path) },
		Verify: func(p deps.Package) deps.Verdict {
			if time.Now().After(deadline) {
				return deps.Verdict{Package: p, Err: errors.New("out of time")}
			}
			first := registry.Verify(p, time.Now())
			// `go get` names a package, which may be a folder of its module.
			// The proxy knows modules, so ask about each shorter path too.
			for v := first; p.Ecosystem == "go" && len(v.Problems) == 1 && v.Problems[0].Kind == deps.Missing && strings.Count(p.Name, "/") > 1; {
				p.Name = p.Name[:strings.LastIndex(p.Name, "/")]
				if v = registry.Verify(p, time.Now()); v.Err != nil || len(v.Problems) != 1 || v.Problems[0].Kind != deps.Missing {
					return v
				}
			}
			return first
		},
	}
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

func newDepsCmd() *cobra.Command {
	verify := &cobra.Command{
		Use:   "verify <ecosystem> <name>[@version]",
		Short: "Check one package before it is added or upgraded: it exists, is old enough, maintained and licensed",
		Long: "Asks the public registry about one package, as QK060 does for every dependency a change adds or upgrades. " +
			"The ecosystem is one of " + strings.Join(deps.Ecosystems, ", ") + ". Without a version, the newest release is checked.\n\n" +
			"Exit code 1 means a finding. A registry that cannot be reached is not a finding: the command says so and exits 0.",
		Example: "  qk deps verify npm left-pad\n  qk deps verify npm @types/node@22.1.0\n  qk deps verify go github.com/spf13/cobra@v1.8.0",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg := deps.Package{Ecosystem: args[0], Name: args[1]}
			if !slices.Contains(deps.Ecosystems, pkg.Ecosystem) {
				return fmt.Errorf("unknown ecosystem %q (want one of %s)", pkg.Ecosystem, strings.Join(deps.Ecosystems, ", "))
			}
			// The last @, and not the first character: an npm scope starts with one.
			if at := strings.LastIndex(pkg.Name, "@"); at > 0 {
				pkg.Name, pkg.Version = pkg.Name[:at], pkg.Name[at+1:]
			}
			if pkg.Name == "" || strings.HasPrefix(pkg.Name, "@") && !strings.Contains(pkg.Name, "/") {
				return fmt.Errorf("no package name in %q", args[1])
			}
			return writeVerdict(cmd.OutOrStdout(), registry.Verify(pkg, time.Now()))
		},
	}
	d := &cobra.Command{Use: "deps", Short: "Check dependencies against their registry"}
	d.AddCommand(verify)
	return d
}

// writeVerdict prints what was found about one package. It returns
// errBlocking for a finding, and nil when the registry could not be asked.
func writeVerdict(w io.Writer, v deps.Verdict) error {
	if v.Err != nil {
		_, err := fmt.Fprintf(w, "skipped: could not ask the registry about %s: %v\n", v.Package, v.Err)
		return err
	}
	if len(v.Problems) == 0 {
		facts := fmt.Sprintf("ok: %s %s", v.Package, v.Info.Version)
		if !v.Info.Published.IsZero() {
			facts += ", published " + v.Info.Published.Format(time.DateOnly)
		}
		if v.Info.Licence != "" {
			facts += ", licence " + v.Info.Licence
		}
		fmt.Fprintln(w, facts)
	}
	for _, p := range v.Problems {
		fmt.Fprintln(w, "- "+p.Message)
	}
	for _, n := range v.Notes {
		fmt.Fprintln(w, n)
	}
	if len(v.Problems) > 0 {
		return errBlocking
	}
	return nil
}
