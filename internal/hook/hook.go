// Package hook holds the decisions behind Quality Kit's Claude Code hooks.
package hook

import (
	"encoding/json"
	"fmt"
	"path"
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

var (
	// exceptionCommand: qk waiver add / qk docs-impact, however qk is invoked.
	exceptionCommand = regexp.MustCompile(`(^|[\s"'/\\&;|(])(?i:qk(\.exe)?)["']?\s+(waiver\s+add|docs-impact)\b`)
	// protectedInShell: shell text that names .quality/ or quality.yaml, after
	// and before anything that is not part of a file name, as in `>&quality.yaml`.
	protectedInShell = regexp.MustCompile(`(^|[^\w.-])(\.quality([^\w.-]|$)|quality\.yaml\b)`)
	// redirect: where a command sends its output: `> file`, `>> file`, `2> file`, `>& file`, `>| file`.
	redirect = regexp.MustCompile(`>+[|&]?\s*["']?([^\s"'<>|;&]*)`)
	// descriptor: a redirect to another stream, as in `2>&1` and `>&-`, not to a file.
	descriptor = regexp.MustCompile(`^(\d+|-)$`)
	// toNull: PowerShell's way to discard a stream, the one use of `$` that is let through.
	toNull = regexp.MustCompile(`(?i)>\s*\$null\b`)
	// assignment: `NAME=value` in front of a command.
	assignment = regexp.MustCompile(`^\w+=`)
)

// notPlain holds the characters that make a shell line more than plain
// commands, quotes, pipes and redirects: backticks, blocks, comments, input
// redirects and here-documents, and the typographic quotes PowerShell reads
// as quotes. What they do depends on shell state that reading text cannot
// follow, so a line that holds one and names a protected path asks.
const notPlain = "`{}<#" + "‘’‚‛“”„"

// expansion holds the characters of variables, substitutions and groups.
// They are ruled out in a pipeline that names a protected path. In Bash,
// another pipeline on the line may hold them: what a substitution there
// prints stays in that pipeline. PowerShell runs a group in the session
// itself, so there they are ruled out for the whole line.
const expansion = "$()"

// PreToolUse asks a person before Claude changes an exception: running the
// commands that record one, or changing .quality/ or quality.yaml (where modes
// can turn a check off). Reading them asks nobody. A nil Output means no
// opinion. This is best effort: it reads command text, and qk writes the log
// itself, so a missed match is caught only by review of .quality/ on the PR.
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
	if exceptionCommand.MatchString(t.Command) || mayChangeProtected(t.Command, in.ToolName == "PowerShell") ||
		protectedFile(t.FilePath) || protectedFile(t.NotebookPath) {
		return &Output{Decision{"PreToolUse", "ask", askReason}}, nil
	}
	return nil, nil
}

// ponytail: matches by name at any depth, so fixtures named quality.yaml ask too; compare against cwd if that gets noisy.
func protectedFile(p string) bool {
	if p == "" {
		return false
	}
	p = strings.ReplaceAll(p, `\`, "/")
	return path.Base(p) == "quality.yaml" || slices.Contains(strings.Split(path.Dir(p), "/"), ".quality")
}

// mayChangeProtected reports whether a shell line that names .quality/ or
// quality.yaml could change it. Only a plain read is left alone:
//
//   - the line is nothing but commands, quotes, pipes and redirects;
//   - in each pipeline that names the path, every command is one known to
//     read, and none sends its output to the path or to a target that cannot
//     be read off the text;
//   - no other command on the line changes what a command name means.
//
// Everything else asks.
//
// ponytail: this does not parse a shell, it lets a narrow shape through and
// asks about the rest, so some reads ask (`cat "$f"`, a `#` comment). It
// cannot see a name that is not in the text of the command that writes: a
// variable set by an earlier tool call, a script, or a file that carries the
// name to a later command. Use a shell parser (mvdan.cc/sh for Bash) if more
// reads should pass.
func mayChangeProtected(line string, powershell bool) bool {
	if !protectedInShell.MatchString(line) {
		return false
	}
	if powershell {
		line = toNull.ReplaceAllString(line, "> nul")
	}
	// A backslash escapes in Bash and is a path separator in PowerShell, where
	// @ starts a here-string or splats arguments.
	alsoNotPlain := `\`
	if powershell {
		alsoNotPlain = "@" + expansion
	}
	if strings.ContainsAny(line, notPlain+alsoNotPlain) {
		return true
	}
	pipelines, plain := commands(line, powershell)
	if !plain {
		return true
	}
	named := false
	for _, pipeline := range pipelines {
		text := strings.Join(pipeline, " | ")
		names := protectedInShell.MatchString(text)
		named = named || names
		if names && strings.ContainsAny(text, expansion) {
			return true
		}
		for _, cmd := range pipeline {
			if names && (writesTo(cmd) || !readOnly(cmd, powershell)) || renames(cmd, powershell) {
				return true
			}
		}
	}
	return !named // the line names the path, yet no command does: it cannot be told apart
}

// renamers are the Bash builtins that can change what a later command on the
// line runs: they define names, load code, or change where names are looked up.
var renamers = map[string]bool{
	"alias": true, "unalias": true, "function": true, "source": true, ".": true, "eval": true, "exec": true,
	"hash": true, "enable": true, "builtin": true, "command": true, "shopt": true, "set": true, "trap": true,
	"export": true, "declare": true, "typeset": true, "local": true, "readonly": true, "unset": true,
}

// psRenamers are their PowerShell counterparts. Aliases can also be set
// through the alias: drive with general cmdlets, so any command there that
// says "alias" counts as well.
var psRenamers = map[string]bool{
	"import-module": true, "ipmo": true, "invoke-expression": true, "iex": true, ".": true,
	"set-variable": true, "sv": true, "new-variable": true, "nv": true,
	"set-item": true, "si": true, "new-item": true, "ni": true, "function": true, "filter": true,
}

// renames reports whether the command can change what a later command's name
// means: `alias cat=rm`, `PATH=/tmp/x`, `. ./helpers.sh`, `Set-Alias cat Remove-Item`.
func renames(cmd string, powershell bool) bool {
	words := strings.Fields(cmd)
	if len(words) == 0 {
		return false
	}
	name := strings.ToLower(words[0])
	if powershell {
		return psRenamers[name] || strings.Contains(strings.ToLower(cmd), "alias")
	}
	// A name with `$` in it is worked out when it runs, and may be any of these.
	// A bracket after the name defines a function: `cat() (git checkout .)`.
	// A bracket in front opens a subshell, which changes nothing for the line.
	return renamers[name] || assignment.MatchString(words[0]) ||
		strings.Contains(words[0], "$") || strings.Contains(words[0][1:], "(")
}

// writesTo reports whether the command redirects its output to a protected
// path, or to a target that cannot be read off the text, such as a pattern.
func writesTo(cmd string) bool {
	for _, m := range redirect.FindAllStringSubmatch(cmd, -1) {
		target := m[1]
		if descriptor.MatchString(target) {
			continue
		}
		if target == "" || strings.ContainsAny(target, "*?[%") || protectedFile(target) {
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
// add and commit record a file; they do not change it.
var gitReads = map[string]bool{
	"diff": true, "log": true, "show": true, "status": true, "blame": true, "grep": true, "describe": true,
	"ls-files": true, "ls-tree": true, "cat-file": true, "rev-parse": true, "merge-base": true,
	"check-attr": true, "check-ignore": true, "shortlog": true, "add": true, "commit": true,
}

// readOnly reports whether one simple command is known to only read.
func readOnly(cmd string, powershell bool) bool {
	words := strings.Fields(cmd)
	// `NAME=value` in front: alone, the value may be used by a later command;
	// before a command, it can make a reader run a program (GIT_EXTERNAL_DIFF, PAGER).
	if len(words) == 0 || assignment.MatchString(words[0]) {
		return false
	}
	// A bare name only: `./cat` and `C:\tools\cat.exe` are whatever sits at that path.
	if strings.ContainsAny(words[0], `/\`) {
		return false
	}
	name := strings.TrimSuffix(strings.ToLower(words[0]), ".exe")
	args := words[1:]
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
	case "test", "[", "[[":
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

// commands splits a plain shell line into pipelines, at `;`, a new line, `&`,
// `&&` and `||`, and each pipeline into its commands, at `|`. Text in quotes
// stays together, and so does a group in brackets, `( )` or `$( )`, as one
// piece of the command it is in. With the characters in notPlain and the
// shell's escape character ruled out beforehand, a quote ends at the next
// quote of its kind, in Bash and in PowerShell alike.
//
// plain is false where this does not vouch for how the shell reads the line:
// a quote inside a group, `$'` and `$"`, a bracket or quote left open, a
// bracket that closes nothing, and in PowerShell a quote that opens in the
// middle of a word.
func commands(line string, powershell bool) (pipelines [][]string, plain bool) {
	var (
		pipeline []string
		cur      strings.Builder
		quote    rune // the quote the text is in: ', " or 0
		depth    int  // brackets open
		inQuotes bool // the outermost bracket opened inside double quotes
	)
	endCommand := func() {
		if cmd := strings.TrimSpace(cur.String()); cmd != "" {
			pipeline = append(pipeline, cmd)
		}
		cur.Reset()
	}
	end := func() {
		endCommand()
		if len(pipeline) > 0 {
			pipelines = append(pipelines, pipeline)
		}
		pipeline = nil
	}
	rs := []rune(line)
	at := func(i int) rune {
		if i < 0 || i >= len(rs) {
			return 0
		}
		return rs[i]
	}
	for i, r := range rs {
		prev, next := at(i-1), at(i+1)
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			}
			cur.WriteRune(r)
		case depth > 0:
			switch r {
			case '\'', '"':
				return nil, false
			case '(':
				depth++
			case ')':
				if depth--; depth == 0 && inQuotes {
					quote, inQuotes = '"', false
				}
			}
			cur.WriteRune(r)
		case r == '(' && (quote == 0 || prev == '$'): // a group, or a substitution, which also runs inside double quotes
			depth, inQuotes, quote = 1, quote == '"', 0
			cur.WriteRune(r)
		case quote == '"':
			if r == '"' {
				quote = 0
			}
			cur.WriteRune(r)
		case r == ')', r == '$' && (next == '\'' || next == '"'):
			return nil, false
		case r == '\'' || r == '"':
			if powershell && prev != 0 && !strings.ContainsRune(" \t\n;|&=:,", prev) {
				return nil, false
			}
			quote = r
			cur.WriteRune(r)
		case r == ';' || r == '\n':
			end()
		case r == '&' && prev == '|': // `|&` pipes both streams: the pipeline goes on
		case r == '&' && prev != '>' && next != '>': // not the `&` of `2>&1` or `&>`
			end()
		case r == '|' && (next == '|' || prev == '|'): // `||`
			end()
		case r == '|' && prev != '>': // a pipe, not the `>|` redirect
			endCommand()
		default:
			cur.WriteRune(r)
		}
	}
	end()
	return pipelines, quote == 0 && depth == 0
}
