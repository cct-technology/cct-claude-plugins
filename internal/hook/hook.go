// Package hook holds the decisions behind Quality Kit's Claude Code hooks.
package hook

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Output is a hook's JSON answer.
type Output struct {
	HookSpecificOutput Decision `json:"hookSpecificOutput"`
}

// Decision is a PreToolUse permission decision.
type Decision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
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
func PreToolUse(input []byte) (*Output, error) {
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
	switch {
	case exceptionCommand.MatchString(t.Command), protectedFile(t.FilePath), protectedFile(t.NotebookPath):
		return &Output{Decision{"PreToolUse", "ask", askReason}}, nil
	case mayChange(t.Command):
		return &Output{Decision{"PreToolUse", "ask", notPlainReason}}, nil
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
