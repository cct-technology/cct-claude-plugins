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
	// protectedInShell: shell text that names .quality/ or quality.yaml.
	protectedInShell = regexp.MustCompile(`(^|[\s"'=/\\<>(:])(\.quality([/\\\s"';&|)]|$)|quality\.yaml\b)`)
	// redirect: where a command sends its output, as in `> file`, `>> file` and `2> file`.
	redirect = regexp.MustCompile(`>+\s*["']?([^\s"'<>|;&]*)`)
	// assignment: `NAME=value` in front of a command.
	assignment = regexp.MustCompile(`^\w+=`)
)

// PreToolUse asks a person before Claude changes an exception: running the
// commands that record one, or changing .quality/ or quality.yaml (where modes
// can turn a check off). Reading them asks nobody. A nil Output means no
// opinion. This is best effort: it reads command text, and qk writes the log
// itself, so a missed match is caught only by review of .quality/ on the PR.
func PreToolUse(input []byte) (*Output, error) {
	var in struct {
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
	if exceptionCommand.MatchString(t.Command) || mayChangeProtected(t.Command) ||
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
// quality.yaml could change it. A line that only reads is left alone: every
// command in each pipeline that names the path is one known to read, and none
// sends its output there. Anything else asks, so a command this does not know
// asks.
//
// ponytail: this reads text, it does not run a shell. Known gaps: a path held
// in a variable set by an earlier tool call, a program that git or a reader
// runs because the repository's own config says so, and here-documents. Use a
// shell parser (mvdan.cc/sh) and a list of allowed options per command if
// those turn up.
func mayChangeProtected(line string) bool {
	if !protectedInShell.MatchString(line) {
		return false
	}
	named := false
	for _, pipeline := range commands(line) {
		if !protectedInShell.MatchString(strings.Join(pipeline, " | ")) {
			continue
		}
		named = true
		for _, cmd := range pipeline {
			if writesTo(cmd) || !readOnly(cmd) {
				return true
			}
		}
	}
	return !named // the line names the path, yet no command does: it cannot be told apart
}

// writesTo reports whether the command redirects its output to a protected path.
func writesTo(cmd string) bool {
	for _, m := range redirect.FindAllStringSubmatch(cmd, -1) {
		if protectedFile(m[1]) {
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
	"ls": true, "dir": true, "du": true, "stat": true, "test": true, "[": true,
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
func readOnly(cmd string) bool {
	words := strings.Fields(cmd)
	// `NAME=value` in front: alone, the value may be used by a later command;
	// before a command, it can make a reader run a program (GIT_EXTERNAL_DIFF, PAGER).
	if len(words) == 0 || assignment.MatchString(words[0]) {
		return false
	}
	name := strings.ToLower(path.Base(strings.ReplaceAll(words[0], `\`, "/")))
	name = strings.TrimSuffix(name, ".exe")
	args := words[1:]
	has := func(prefixes ...string) bool {
		return slices.ContainsFunc(args, func(a string) bool {
			return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(a, p) })
		})
	}
	switch name {
	case "git":
		// Options that write a file, or name a program for git to run.
		return gitReads[gitSubcommand(args)] && !slices.Contains(args, "-c") &&
			!has("--output", "--ext-diff", "--config-env", "--exec-path", "-O", "--open-files-in-pager")
	case "find":
		return !has("-delete", "-exec", "-ok", "-fprint", "-fls")
	case "rg":
		return !has("--pre", "--hostname-bin")
	}
	return readers[name]
}

// gitSubcommand returns the first word after git's own options, such as `-C <dir>`.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-C":
			i++
		case !strings.HasPrefix(args[i], "-"):
			return args[i]
		}
	}
	return ""
}

// commands splits a shell line into pipelines, and each pipeline into its
// commands. Quotes keep their text together. A command substitution or a
// group in brackets is split on its own, and the command around it carries on
// without it.
func commands(line string) [][]string {
	type frame struct {
		pipeline []string
		cur      strings.Builder
		quote    rune // the quote the text is in: ', " or 0
		closer   rune // what ends this frame: ) or `, 0 for the line itself
	}
	var out [][]string
	stack := []*frame{{}}
	endCommand := func(f *frame) {
		f.pipeline = append(f.pipeline, f.cur.String())
		f.cur.Reset()
	}
	endPipeline := func(f *frame) {
		endCommand(f)
		out = append(out, f.pipeline)
		f.pipeline = nil
	}
	open := func(closer rune) { stack = append(stack, &frame{closer: closer}) }
	closeFrame := func() {
		endPipeline(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
		stack[len(stack)-1].cur.WriteByte(' ')
	}

	rs := []rune(line)
	at := func(i int) rune {
		if i < 0 || i >= len(rs) {
			return 0
		}
		return rs[i]
	}
	for i := 0; i < len(rs); i++ {
		f := stack[len(stack)-1]
		r, next := rs[i], at(i+1)
		switch {
		case f.quote == '\'':
			if r == '\'' {
				f.quote = 0
			}
			f.cur.WriteRune(r)
		case r == '`' && f.closer == '`':
			closeFrame()
		case r == '`':
			open('`')
		case r == '$' && next == '(': // a command runs here, inside double quotes too
			open(')')
			i++
		case f.quote == '"':
			if r == '"' {
				f.quote = 0
			}
			f.cur.WriteRune(r)
		case r == '\'' || r == '"':
			f.quote = r
			f.cur.WriteRune(r)
		case r == '\\' && next != 0: // an escaped character, or a Windows path
			f.cur.WriteRune(r)
			f.cur.WriteRune(next)
			i++
		case r == '(':
			open(')')
		case r == ')' && f.closer == ')':
			closeFrame()
		case r == ';' || r == '\n' || r == ')' || r == '{' || r == '}':
			endPipeline(f)
		case r == '&' && next == '&', r == '|' && next == '|':
			endPipeline(f)
			i++
		case r == '&' && at(i-1) != '>' && next != '>': // the background `&`, not `2>&1` or `&>`
			endPipeline(f)
		case r == '|':
			endCommand(f)
		default:
			f.cur.WriteRune(r)
		}
	}
	for len(stack) > 0 { // also what an unclosed bracket or backtick left open
		endPipeline(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
	}
	return out
}
