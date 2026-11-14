package hook

import (
	"regexp"
	"strings"
)

// psNotPlain holds the characters that make a PowerShell line more than plain
// commands, quotes, pipes and redirects: variables, groups, blocks, splatting
// and here-strings, comments, the escape character, and the typographic
// quotes PowerShell reads as quotes. There is no parser for PowerShell here,
// so a line that holds one and names a protected path asks.
const psNotPlain = "`$(){}<#@" + "‘’‚‛“”„"

var (
	// psRedirect: where a command sends its output: `> file`, `>> file`, `2> file`. The first group is an opening quote.
	psRedirect = regexp.MustCompile(`>+&?\s*(["']?)([^\s"'<>|;&]*)`)
	// toNull: PowerShell's way to discard a stream, the one use of `$` that is let through.
	toNull = regexp.MustCompile(`(?i)>\s*\$null\b`)
)

// psStateChangers are commands that can change what a later command on the
// line runs: they load code or set names. Aliases, functions and variables
// can also be set through their drives with general commands, so a command
// that mentions one of those counts as well (psChangesState).
var psStateChangers = map[string]bool{
	"import-module": true, "ipmo": true, "invoke-expression": true, "iex": true, ".": true,
	"set-variable": true, "sv": true, "new-variable": true, "nv": true,
	"set-item": true, "si": true, "new-item": true, "ni": true, "function": true, "filter": true,
}

// powershellMayChange reports whether a PowerShell line that names .quality/
// or quality.yaml could change it. Only a plain read is left alone:
//
//   - the line is nothing but commands, quotes, pipes and redirects;
//   - in each pipeline that names the path, every command is one known to
//     read, and none sends its output to the path or to a target that cannot
//     be read off the text;
//   - no other command on the line changes what a command name means.
//
// Everything else asks, so some reads ask: `Get-Content $path`, a `#` comment.
func powershellMayChange(line string) bool {
	if !protectedName.MatchString(line) {
		return false
	}
	line = toNull.ReplaceAllString(line, "> nul")
	if strings.ContainsAny(line, psNotPlain) {
		return true
	}
	pipelines, plain := psCommands(line)
	if !plain {
		return true
	}
	named := false
	for _, pipeline := range pipelines {
		names := protectedName.MatchString(strings.Join(pipeline, " | "))
		named = named || names
		for _, cmd := range pipeline {
			words := strings.Fields(cmd)
			args := make([]string, 0, len(words))
			for _, w := range words[1:] {
				args = append(args, strings.Trim(w, `"'`)) // an option is an option in quotes too
			}
			if psChangesState(cmd, words[0]) || names && (psWritesTo(cmd) || !readOnly(words[0], args, true, true)) {
				return true
			}
		}
	}
	return !named // the line names the path, yet no command does: it cannot be told apart
}

// psChangesState reports whether the command can change what a later
// command's name means: `Set-Alias cat Remove-Item`, `Import-Module x`,
// `Copy-Item function:Remove-Item function:cat`.
func psChangesState(cmd, name string) bool {
	if psStateChangers[strings.ToLower(name)] {
		return true
	}
	lower := strings.ToLower(cmd)
	for _, drive := range []string{"alias", "function:", "variable:", "env:"} {
		if strings.Contains(lower, drive) {
			return true
		}
	}
	return false
}

// psWritesTo reports whether the command redirects its output to a protected
// path, or to a target that cannot be read off the text: one in quotes, where
// it may hold spaces, or a pattern.
func psWritesTo(cmd string) bool {
	for _, m := range psRedirect.FindAllStringSubmatch(cmd, -1) {
		quote, target := m[1], m[2]
		if quote == "" && descriptor.MatchString(target) {
			continue
		}
		if quote != "" || target == "" || strings.ContainsAny(target, "*?[%") || protectedFile(target) {
			return true
		}
	}
	return false
}

// psCommands splits a plain PowerShell line into pipelines, at `;`, a new
// line, `&`, `&&` and `||`, and each pipeline into its commands, at `|`. Text
// in quotes stays together. With the characters in psNotPlain ruled out
// beforehand, a quote ends at the next quote of its kind.
//
// plain is false where this does not vouch for how PowerShell reads the line:
// a quote that opens in the middle of a word, and a quote left open.
func psCommands(line string) (pipelines [][]string, plain bool) {
	var (
		pipeline []string
		cur      strings.Builder
		quote    rune // the quote the text is in: ', " or 0
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
		case quote != 0:
			if r == quote {
				quote = 0
			}
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			if prev != 0 && !strings.ContainsRune(" \t\n;|&=:,", prev) {
				return nil, false
			}
			quote = r
			cur.WriteRune(r)
		case r == ';' || r == '\n':
			end()
		case r == '&' && prev != '>' && next != '>': // not the `&` of `2>&1`
			end()
		case r == '|' && (next == '|' || prev == '|'): // `||`
			end()
		case r == '|':
			endCommand()
		default:
			cur.WriteRune(r)
		}
	}
	end()
	return pipelines, quote == 0
}
