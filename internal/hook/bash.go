package hook

import (
	"regexp"
	"strings"
	"unicode"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// bashMayChange reports whether a Bash line could change .quality/ or
// quality.yaml. A line that names neither is no concern of this hook. One
// that does is let through only when it is plain reading (see plainStmt);
// everything else asks.
//
// Earlier versions followed where the name could go: arguments,
// substitutions, pipes, variables. Reviews kept finding one more way, so this
// lists what is allowed, not what is dangerous.
//
// A line that cannot be read as Bash asks when its text names the path, and
// so does one the parser trips over: qk turns a panic in a hook into a
// warning, which would let the line through.
func bashMayChange(line string) (ask bool) {
	named := protectedName.MatchString(line)
	defer func() {
		if recover() != nil {
			ask = named
		}
	}()
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(line), "")
	if err != nil {
		return named
	}
	// The name may be spelled so that the text alone does not show it: `q""uality.yaml`.
	if !named && !names(file) {
		return false
	}
	for _, s := range file.Stmts {
		if !plainStmt(s) {
			return true
		}
	}
	return false
}

// plainStmt reports whether the statement is plain reading: commands from a
// short list, with words written out in full, joined by `;`, `&&`, `||`, `|`,
// `if`, `( )` and `{ }`. No variables, no substitutions, no assignments, no
// loops, no functions.
func plainStmt(s *syntax.Stmt) bool {
	if s.Coprocess || s.Disown {
		return false
	}
	for _, r := range s.Redirs {
		if !plainRedirect(r) {
			return false
		}
	}
	switch c := s.Cmd.(type) {
	case nil: // only redirects
		return true
	case *syntax.CallExpr:
		return plainCall(c)
	case *syntax.BinaryCmd: // && || | |&
		return plainStmt(c.X) && plainStmt(c.Y)
	case *syntax.Block:
		return plainStmts(c.Stmts)
	case *syntax.Subshell:
		return plainStmts(c.Stmts)
	case *syntax.IfClause:
		for ; c != nil; c = c.Else {
			if !plainStmts(c.Cond) || !plainStmts(c.Then) {
				return false
			}
		}
		return true
	case *syntax.TestClause:
		return plainTest(c.X)
	case *syntax.TimeClause:
		return c.Stmt == nil || plainStmt(c.Stmt)
	}
	return false
}

func plainStmts(stmts []*syntax.Stmt) bool {
	for _, s := range stmts {
		if !plainStmt(s) {
			return false
		}
	}
	return true
}

// bystanders may stand on a line beside a read: they change no file, and no
// name the shell looks up. They may not name the protected path themselves.
// qk's commands that record an exception are caught by exceptionCommand.
var bystanders = map[string]bool{"qk": true, "cd": true, "which": true, "type": true, "false": true, ":": true}

// testOperators are the options `test`, `[` and `[[` may be given: file and
// string tests. The others compare numbers or look at a variable, and for
// those Bash works their operand out as an expression, which can run a command.
var testOperators = map[string]bool{
	"-e": true, "-f": true, "-d": true, "-r": true, "-w": true, "-x": true, "-s": true, "-L": true, "-h": true,
	"-z": true, "-n": true, "-a": true, "-o": true,
}

// plainCall reports whether the simple command is a reader, or a bystander
// that does not name the path, with every word written out in full.
func plainCall(c *syntax.CallExpr) bool {
	if len(c.Assigns) > 0 || len(c.Args) == 0 {
		return false
	}
	words := make([]string, len(c.Args))
	for i, w := range c.Args {
		word, ok := plainWord(w)
		if !ok {
			return false
		}
		words[i] = word
	}
	name, args := words[0], words[1:]
	switch {
	case name == "test" || name == "[":
		for _, a := range args {
			if strings.HasPrefix(a, "-") && !testOperators[a] {
				return false
			}
		}
		return true
	case name == "printf":
		return false // -v and %n set a variable, and Bash works a subscript in its name out
	case readOnly(name, args, false):
		return true
	case name == "command": // `command -v qk` looks a name up; `command <name>` runs it
		return len(args) == 2 && (args[0] == "-v" || args[0] == "-V") && !protectedName.MatchString(args[1])
	}
	return bystanders[name] && !protectedName.MatchString(strings.Join(args, " "))
}

// plainTest judges the inside of `[[ ]]` as plainCall judges `test`.
func plainTest(x syntax.TestExpr) bool {
	switch x := x.(type) {
	case *syntax.Word:
		_, ok := plainWord(x)
		return ok
	case *syntax.ParenTest:
		return plainTest(x.X)
	case *syntax.UnaryTest:
		return (x.Op.String() == "!" || testOperators[x.Op.String()]) && plainTest(x.X)
	case *syntax.BinaryTest:
		switch x.Op.String() {
		case "==", "=", "!=", "&&", "||":
			return plainTest(x.X) && plainTest(x.Y)
		}
	}
	return false
}

// descriptor: the target of a redirect to another stream, as in `2>&1` and `>&-`.
var descriptor = regexp.MustCompile(`^(\d+|-)$`)

// plainRedirect reports whether the redirect is from a file written out in
// full, to another stream, or to such a file that is not a protected path.
func plainRedirect(r *syntax.Redirect) bool {
	if r.N != nil && !descriptor.MatchString(r.N.Value) {
		return false // {name}> stores a descriptor in a variable
	}
	target, ok := plainWord(r.Word)
	if !ok {
		return false
	}
	switch r.Op {
	case syntax.RdrIn:
		return true
	case syntax.DplIn, syntax.DplOut:
		if descriptor.MatchString(target) {
			return true
		}
		if r.Op == syntax.DplIn {
			return false
		}
	case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.RdrAll, syntax.AppAll:
	default: // here-documents and here-strings feed text to a command; `<>` opens for writing too
		return false
	}
	// `>! file` is zsh's forced overwrite; read as Bash, the target is `!` and the file an argument.
	return target != "!" && !strings.ContainsAny(target, "*?") && !protectedName.MatchString(target)
}

// plainWord returns the word as the shell will see it, when it is written
// out in full: text and quotes only. One substitution is let through, the
// form the skills use: `$(git merge-base <rev> <rev>)`, which prints a commit
// id or nothing.
//
// Text outside quotes may hold only what no shell reads as more than text or
// a file pattern: letters, digits and `_ . / : , @ % + = * ? ~ -`. Bash and
// zsh differ on the rest (`=cmd`, `(…)` after a pattern, `^`, `#`, `!`).
// Text inside quotes is taken as it is.
func plainWord(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", true
	}
	if isMergeBase(w) {
		return "MERGE_BASE", true
	}
	switch lit := w.Lit(); lit {
	case "[", "]": // the command `[` and its closing word
		return lit, true
	case `\(`, `\)`: // find's grouping
		return lit[1:], true
	}
	var b strings.Builder
	for i, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			if i == 0 && strings.HasPrefix(p.Value, "=") {
				return "", false // zsh: `=name` is the path of the command `name`
			}
			for _, r := range p.Value {
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_./:,@%+=*?~-", r) {
					return "", false
				}
			}
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return "", false // $'...' has escapes of its own
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				return "", false
			}
			for _, q := range p.Parts {
				lit, ok := q.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(unescape(lit.Value, true))
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

// isMergeBase reports whether the word is `$(git merge-base <rev>...)`, bare
// or in double quotes, with each revision a plain word that is no option.
func isMergeBase(w *syntax.Word) bool {
	parts := w.Parts
	if len(parts) == 1 {
		if q, ok := parts[0].(*syntax.DblQuoted); ok && !q.Dollar {
			parts = q.Parts
		}
	}
	if len(parts) != 1 {
		return false
	}
	sub, ok := parts[0].(*syntax.CmdSubst)
	if !ok || len(sub.Stmts) != 1 || len(sub.Stmts[0].Redirs) > 0 || sub.Stmts[0].Negated || sub.Stmts[0].Background {
		return false
	}
	call, ok := sub.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) > 0 || len(call.Args) < 3 {
		return false
	}
	for i, arg := range call.Args {
		word, ok := plainWord(arg)
		switch {
		case !ok || isMergeBase(arg):
			return false
		case i == 0 && word != "git", i == 1 && word != "merge-base", i > 1 && strings.HasPrefix(word, "-"):
			return false
		}
	}
	return true
}

// names reports whether a word in the tree names a protected path once
// quotes, escapes and braces are worked out, as in `q""uality.yaml` and
// `quality.{yaml,bak}`. It finds what the text of the line does not show; a
// name built when the line runs is beyond it.
func names(node syntax.Node) bool {
	found := false
	syntax.Walk(node, func(n syntax.Node) bool {
		w, ok := n.(*syntax.Word)
		if !ok {
			return !found
		}
		// SplitBraces rewrites the word it is given, into a form Walk does not
		// take: work on a copy.
		braced := &syntax.Word{Parts: make([]syntax.WordPart, len(w.Parts))}
		for i, p := range w.Parts {
			if lit, ok := p.(*syntax.Lit); ok {
				p = &syntax.Lit{Value: lit.Value}
			}
			braced.Parts[i] = p
		}
		if !syntax.SplitBraces(braced) {
			found = found || namesProtected(w)
			return !found
		}
		for expanded, err := range expand.BracesSeq(nil, braced) {
			if err != nil || namesProtected(expanded) {
				found = true // too many to work out counts as naming it
				break
			}
		}
		return !found
	})
	return found
}

// In $'...' a backslash starts an escape. Those that spell a character by its
// number (`\x2e`, `\056`, `.`, `\cA`) are not worked out here: a word
// that holds one counts as naming the path. The others stand for characters
// no protected name holds.
var (
	numberedEscape = regexp.MustCompile(`\\[xuUc0-7]`)
	escape         = regexp.MustCompile(`(?s)\\.`)
)

// namesProtected reports whether the word names a protected path, with each
// part known only when the line runs taken as empty, and as a gap.
func namesProtected(w *syntax.Word) bool {
	const unknown = "\x00"
	var b strings.Builder
	hidden := false // a numbered escape in $'...' may spell the name
	var add func(parts []syntax.WordPart, quoted bool)
	add = func(parts []syntax.WordPart, quoted bool) {
		for _, p := range parts {
			switch p := p.(type) {
			case *syntax.Lit:
				b.WriteString(unescape(p.Value, quoted))
			case *syntax.SglQuoted:
				switch {
				case !p.Dollar:
					b.WriteString(p.Value)
				case numberedEscape.MatchString(p.Value):
					hidden = true
				default:
					b.WriteString(escape.ReplaceAllString(p.Value, unknown))
				}
			case *syntax.DblQuoted:
				add(p.Parts, true)
			case *syntax.ExtGlob:
				b.WriteString(p.Pattern.Value)
			default:
				b.WriteString(unknown)
			}
		}
	}
	add(w.Parts, false)
	s := b.String()
	return hidden || protectedName.MatchString(s) || protectedName.MatchString(strings.ReplaceAll(s, unknown, ""))
}

// unescape takes the backslashes off as Bash does: before any character
// outside quotes, and before $ ` " \ and a new line inside double quotes.
func unescape(s string, quoted bool) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			next := s[i+1]
			if next == '\n' {
				i++
				continue
			}
			if !quoted || strings.IndexByte("$`\"\\", next) >= 0 {
				i++
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
