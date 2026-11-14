package hook

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// psRedirect: where a command sends its output: `> file`, `>> file`, `2> file`. The first group is an opening quote.
	psRedirect = regexp.MustCompile(`>+&?\s*(["']?)([^\s"'<>|;&]*)`)
	// toNull: PowerShell's way to discard a stream, the one use of `$` that is let through.
	toNull = regexp.MustCompile(`(?i)>\s*\$null\b`)
	// newLines: PowerShell ends a statement at a carriage return as at a new line.
	newLines = strings.NewReplacer("\r\n", "\n", "\r", "\n")
)

// psBystanders may stand on a line beside a read: they change no file and no
// name PowerShell looks up. They may not name the protected path themselves.
var psBystanders = map[string]bool{"qk": true, "cd": true, "set-location": true, "sl": true, "get-command": true, "gcm": true}

// powershellMayChange reports whether a PowerShell line that names .quality/
// or quality.yaml could change it. There is no parser for PowerShell here, so
// only a narrow shape of text is let through:
//
//   - outside quotes the line holds only letters, digits, spaces and
//     `_ . / \ : , % + = * ? ~ - > & | ;`: no variables, groups, blocks,
//     comments or escapes;
//   - every command on the line is one known to read, or a bystander that
//     does not name the path;
//   - no output goes to the path, or to a target that cannot be read off the
//     text.
//
// Everything else asks, so some reads ask: `Get-Content $path`, a read beside
// another command.
func powershellMayChange(line string) bool {
	if !protectedName.MatchString(line) {
		return false
	}
	line = newLines.Replace(toNull.ReplaceAllString(line, "> nul"))
	pipelines, plain := psCommands(line)
	if !plain {
		return true
	}
	for _, pipeline := range pipelines {
		for _, cmd := range pipeline {
			words := strings.Fields(cmd)
			args := make([]string, 0, len(words))
			for _, w := range words[1:] {
				args = append(args, strings.Trim(w, `"'`)) // an option is an option in quotes too
			}
			if psWritesTo(cmd) {
				return true
			}
			if readOnly(words[0], args, true) {
				continue
			}
			if !psBystanders[strings.ToLower(words[0])] || protectedName.MatchString(cmd) {
				return true
			}
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
		if quote != "" || target == "" || strings.ContainsAny(target, "*?") || protectedFile(target) {
			return true
		}
	}
	return false
}

// psCommands splits a PowerShell line into pipelines, at `;`, a new line,
// `&`, `&&` and `||`, and each pipeline into its commands, at `|`. Text in
// quotes stays together and is taken as it is.
//
// plain is false where this does not vouch for how PowerShell reads the line:
// a character outside quotes that is not on the short list, a quote that
// opens in the middle of a word or is left open, a typographic quote (which
// PowerShell reads as a quote), and a `$` or a backtick inside double quotes.
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
		case strings.ContainsRune("‘’‚‛“”„", r):
			return nil, false
		case quote != 0:
			if r == quote {
				quote = 0
			} else if quote == '"' && (r == '$' || r == '`') {
				return nil, false
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
		case unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" \t_./\\:,%+=*?~->&", r):
			cur.WriteRune(r)
		default:
			return nil, false
		}
	}
	end()
	return pipelines, quote == 0
}
