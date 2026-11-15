// Package hook holds the decisions behind Quality Kit's Claude Code hooks.
package hook

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/rkvalvaa/cct-claude-plugins/internal/deps"
)

// Output is a hook's JSON answer.
type Output struct {
	HookSpecificOutput Decision `json:"hookSpecificOutput"`
}

// Decision is what a PreToolUse hook says about a tool call: `deny` stops it,
// `ask` has a person decide. Without a decision the call goes on to the normal
// permission flow, and AdditionalContext is a note Claude reads with the
// tool's result. `allow` is never returned: it would skip the person's own
// permission prompt.
type Decision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

// Guards is what the two blocking guards need to know about the repository.
// The zero value turns both off.
type Guards struct {
	// GeneratedMode and DepsMode are the modes of QK020 and QK060: a guard
	// blocks when its check is "enforced", tells Claude when it is
	// "advisory", and does nothing otherwise. Hooks block only what a check
	// in enforced mode would.
	GeneratedMode, DepsMode string
	// Generator returns the command that writes the file at path, when the
	// file is generated.
	Generator func(path string) (cmd string, ok bool)
	// Verify asks the registry about a package.
	Verify func(deps.Package) deps.Verdict
}

// maxVerified is how many packages of one line are looked up. Each lookup can
// take seconds, and a hook that runs long holds the session.
// ponytail: the rest are not checked here; QK060 checks them in CI.
const maxVerified = 10

// guard runs the two blocking guards. block is the reason to stop the call;
// note is what to tell Claude when the check is advisory.
func (g Guards) guard(tool, command string, paths ...string) (block, note string) {
	// before is the reason a call is refused. after is the note for an
	// advisory check: Claude reads it with the tool's result, so the edit or
	// the install has happened by then.
	say := func(mode, before, after string) {
		switch mode {
		case "enforced":
			block = strings.TrimSpace(block + "\n\n" + before)
		case "advisory":
			note = strings.TrimSpace(note + "\n\n" + after)
		}
	}
	if g.Generator != nil && (g.GeneratedMode == "enforced" || g.GeneratedMode == "advisory") {
		for _, path := range paths {
			cmd, ok := g.Generator(path)
			if !ok || path == "" {
				continue
			}
			// The command may come from the file's own header, which anyone who
			// writes the file can set. It is shown as what the file says, on
			// one short line, and Claude is not told to run it.
			what := fmt.Sprintf("Quality Kit: `%s` is generated, so an edit by hand is undone the next time its generator runs. ", deps.Clean(path, 300))
			how := fmt.Sprintf("change what it is generated from, and have the generator write the file. The generator is named as: `%s`", deps.Clean(cmd, 120))
			say(g.GeneratedMode, what+"Do not edit it: "+how, what+"If you edited it, undo that: "+how)
		}
	}
	if g.Verify != nil && command != "" && (g.DepsMode == "enforced" || g.DepsMode == "advisory") {
		var problems []string
		advice := map[string]bool{}
		pkgs := installs(command, tool == "PowerShell")
		for _, pkg := range pkgs[:min(len(pkgs), maxVerified)] {
			// Only these two stop an install. A deprecated package or a licence
			// is for QK060, where a person can waive it. A registry that
			// cannot be asked stops nothing.
			for _, p := range g.Verify(pkg).Problems {
				switch p.Kind {
				case deps.Missing:
					advice["Check the name: it may be invented or misspelt."] = true
				case deps.TooYoung:
					advice["For a version that is too young, install an older version, by its number."] = true
				default:
					continue
				}
				problems = append(problems, "- "+p.Message)
			}
		}
		if len(problems) > 0 {
			tips := make([]string, 0, len(advice))
			for tip := range advice {
				tips = append(tips, tip)
			}
			slices.Sort(tips)
			found := "Quality Kit: this command installs a package that should not be installed without a person looking:\n" +
				strings.Join(problems, "\n") + "\n" + strings.Join(tips, " ")
			say(g.DepsMode,
				found+" Do not install it another way. If a person decides it is right, they can run the command themselves.",
				found+" If it was installed, tell the user what you installed and what Quality Kit found, and let them decide whether it stays.")
		}
	}
	return block, note
}

const askReason = "Quality Kit: this records or changes an exception to a quality check " +
	"(a waiver, a docs-impact note, or a check's mode). A person must approve it. " +
	"Approve only if you agree with the reason given. " +
	"If this is denied or no one can approve it, stop and tell the user; do not try another way."

// notPlainReason is for a shell line that names a protected path and is not
// plainly a read. Claude reads it when nobody approves: without the advice it
// takes a refused `git tag; cat quality.yaml` for a ban on reading, and gives
// the task up.
const notPlainReason = "Quality Kit: this command names quality.yaml or .quality/ and is not plainly a read, " +
	"so it may change an exception to a quality check (a waiver, a docs-impact note, or a check's mode). " +
	"A person must approve it. Approve only if you agree with what it changes. " +
	"Claude: if you only meant to read, run the read as a command of its own, or use the Read tool. " +
	"If you meant to change it and this is denied or no one can approve it, stop and tell the user; do not try another way."

var (
	// exceptionCommand: qk waiver add / qk docs-impact, however qk is invoked.
	exceptionCommand = regexp.MustCompile(`(^|[\s"'/\\&;|(])(?i:qk(\.exe)?)["']?\s+(waiver\s+add|docs-impact)\b`)
	// protectedName: text that names .quality/ or quality.yaml, after and
	// before anything that is not part of a file name, as in `>&quality.yaml`.
	// In any case, and with dots after the folder's name: Windows and macOS
	// find the file as QUALITY.YAML, and Windows finds the folder as `.quality.`.
	// ponytail: of the short names Windows may give the two, only the usual
	// form (QUALIT~1.YAM, QUALIT~1) is known; the hashed form it uses after
	// four clashes is not, nor is a link. Resolve the path if that matters.
	protectedName = regexp.MustCompile(`(?i)(^|[^\w.-])(\.quality\.*([^\w.-]|$)|quality\.yaml\b|qualit~\d)`)
)

// PreToolUse asks a person before Claude changes an exception: running the
// commands that record one, or changing .quality/ or quality.yaml (where modes
// can turn a check off). Reading them asks nobody. A nil Output means no
// opinion.
//
// A shell line is let through only when it is plainly a read; everything else
// that names a protected path asks. Bash lines are parsed (bash.go);
// PowerShell lines are held to a narrow shape of text (powershell.go). Neither
// runs anything, so neither sees a name that is not written on the line: a
// variable set by an earlier tool call, a pattern that matches the file, a
// script, or a file that carries the name to a later command. qk writes the
// exceptions log itself, and review of .quality/ on the PR catches the rest.
func PreToolUse(input []byte, g Guards) (*Output, error) {
	var in struct {
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command      string `json:"command"`
			FilePath     string `json:"file_path"`
			NotebookPath string `json:"notebook_path"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("reading hook input: %w", err)
	}
	t := in.ToolInput
	mayChange := bashMayChange
	if in.ToolName == "PowerShell" {
		mayChange = powershellMayChange
	}
	// A block comes first: across hooks Claude Code ranks deny above ask too.
	block, note := g.guard(in.ToolName, t.Command, t.FilePath, t.NotebookPath)
	switch {
	case block != "":
		return &Output{Decision{HookEventName: "PreToolUse", PermissionDecision: "deny", PermissionDecisionReason: block}}, nil
	case exceptionCommand.MatchString(t.Command), protectedFile(t.FilePath), protectedFile(t.NotebookPath):
		return &Output{Decision{"PreToolUse", "ask", askReason, note}}, nil
	case mayChange(t.Command):
		return &Output{Decision{"PreToolUse", "ask", notPlainReason, note}}, nil
	case note != "":
		return &Output{Decision{HookEventName: "PreToolUse", AdditionalContext: note}}, nil
	}
	return nil, nil
}

// ponytail: matches by name at any depth, so fixtures named quality.yaml ask too; compare against cwd if that gets noisy.
//
// The path is read as Windows reads it, the widest reading: in any case, with
// dots and spaces after a name dropped, and with `C:name` and `name:stream`
// taken for the file `name`.
func protectedFile(p string) bool {
	for _, name := range strings.FieldsFunc(strings.ToLower(p), func(r rune) bool { return strings.ContainsRune(`/\:`, r) }) {
		name = strings.TrimRight(name, ". ")
		if name == "quality.yaml" || name == ".quality" || strings.HasPrefix(name, "qualit~") {
			return true
		}
	}
	return false
}

// readers are commands that only read the files they are given, in Bash and
// PowerShell, whatever options they get. A command that can write a file or
// run a program through an option or a script is left out, so it asks: sed,
// awk, sort, yq, less, xxd and tree are examples.
var readers = map[string]bool{
	"cat": true, "head": true, "tail": true, "nl": true, "od": true,
	"grep": true, "egrep": true, "fgrep": true, "jq": true, "cut": true, "tr": true, "wc": true,
	"ls": true, "dir": true, "du": true, "stat": true,
	"diff": true, "cmp": true, "basename": true, "dirname": true, "realpath": true, "readlink": true,
	"echo": true, "printf": true, "true": true, "pwd": true,
	"get-content": true, "gc": true, "type": true, "select-string": true, "sls": true, "test-path": true,
	"get-item": true, "gi": true, "get-childitem": true, "gci": true, "get-filehash": true,
	"write-output": true, "write-host": true,
}

// gitReads are git subcommands that leave the files in the working tree as they are.
// add and commit record a file; they do not change it. tag and branch name commits.
var gitReads = map[string]bool{
	"tag": true, "branch": true,
	"diff": true, "log": true, "show": true, "status": true, "blame": true, "grep": true, "describe": true,
	"ls-files": true, "ls-tree": true, "cat-file": true, "rev-parse": true, "merge-base": true,
	"check-attr": true, "check-ignore": true, "shortlog": true, "add": true, "commit": true,
}

// readOnly reports whether a command with this name and these arguments is
// known to only read. The name is as written: `./cat` and `C:\tools\cat.exe`
// are whatever sits at that path, so only a bare name counts. The arguments
// are known word for word: git, find and rg are judged by their options.
func readOnly(name string, args []string, powershell bool) bool {
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	name = strings.TrimSuffix(strings.ToLower(name), ".exe")
	has := func(prefixes ...string) bool {
		return slices.ContainsFunc(args, func(a string) bool {
			return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(a, p) })
		})
	}
	switch name {
	case "git":
		// Options that write a file or name a program for git to run: --output,
		// --ext-diff, -O and --open-files-in-pager. git takes a long option
		// shortened, so the shortest forms that cannot mean another are matched.
		return gitReads[gitSubcommand(args)] && !slices.Contains(args, "-c") &&
			!has("--ou", "--ext", "-O", "--op")
	case "find":
		return !has("-delete", "-exec", "-ok", "-fprint", "-fls")
	case "rg":
		return !has("--pre", "--hostname-bin")
	case "test", "[":
		return !powershell // in PowerShell `[` opens a type, as in [IO.File]
	}
	return readers[name]
}

// gitSubcommand returns git's subcommand. It returns "" when an option stands
// before it other than the two known here: such an option may take the next
// word as its value (`--git-dir diff checkout`), or name a program to run.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-C":
			i++
		case a == "--no-pager":
		case strings.HasPrefix(a, "-"):
			return ""
		default:
			return a
		}
	}
	return ""
}
