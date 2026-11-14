package hook

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// stateChangers are shell builtins that can change what a later command on
// the line runs or which file it gets: they define names, load or run code,
// change where commands are looked up, or set variables. The last three are
// zsh's: the Bash tool runs the user's shell, and the line is read as Bash.
var stateChangers = map[string]bool{
	"alias": true, "source": true, ".": true, "eval": true, "exec": true, "hash": true, "enable": true,
	"trap": true, "read": true, "mapfile": true, "readarray": true, "getopts": true,
	"autoload": true, "zmodload": true, "emulate": true,
}

// descriptor: the target of a redirect to another stream, as in `2>&1` and `>&-`.
var descriptor = regexp.MustCompile(`^(\d+|-)$`)

// bashMayChange reports whether a Bash line could change .quality/ or
// quality.yaml. It parses the line and asks nothing only when:
//
//   - no word names a protected path, or
//   - every command that gets the name, as an argument, through a
//     substitution or down a pipe, is one known to only read; no output is
//     sent to the path or to a target that is not known word for word; and
//     nothing on the line can change what a command means or carry the name
//     to another command: no assignment, declaration, function, `eval`,
//     `source`, `alias` and the like, and no variable in a command that is
//     not a reader.
//
// A line that cannot be read as Bash asks when its text names the path, and
// so does one the parser trips over: qk turns a panic in a hook into a
// warning, which would let the line through.
func bashMayChange(line string) (ask bool) {
	defer func() {
		if recover() != nil {
			ask = protectedName.MatchString(line)
		}
	}()
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(line), "")
	if err != nil {
		return protectedName.MatchString(line)
	}
	if !names(file) {
		return false
	}

	// Down a pipe, the name can reach every command of the pipeline.
	fed := map[*syntax.CallExpr]bool{}
	syntax.Walk(file, func(n syntax.Node) bool {
		if b, ok := n.(*syntax.BinaryCmd); ok && (b.Op == syntax.Pipe || b.Op == syntax.PipeAll) && names(b) {
			syntax.Walk(b, func(n syntax.Node) bool {
				if c, ok := n.(*syntax.CallExpr); ok {
					fed[c] = true
				}
				return true
			})
		}
		return true
	})

	syntax.Walk(file, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.FuncDecl, *syntax.DeclClause:
			ask = true
		case *syntax.ForClause:
			if loop, ok := n.Loop.(*syntax.WordIter); ok {
				for _, item := range loop.Items {
					ask = ask || names(item) // the loop variable would carry the name
				}
			}
		case *syntax.CaseClause:
			ask = ask || names(n.Word)
		case *syntax.Stmt:
			for _, r := range n.Redirs {
				ask = ask || writesProtected(r)
			}
			if call, ok := n.Cmd.(*syntax.CallExpr); ok {
				ask = ask || !plainRead(call, names(n) || fed[call])
			}
		}
		return !ask
	})
	return ask
}

// plainRead judges one simple command. gets is whether a protected name
// reaches it: in its own words, its redirects, a substitution, or down a pipe.
func plainRead(call *syntax.CallExpr, gets bool) bool {
	if len(call.Assigns) > 0 || len(call.Args) == 0 {
		return false // `NAME=value`: alone it sets a variable, in front of a command it can change what the command runs
	}
	name, ok := literal(call.Args[0])
	if !ok || stateChangers[name] {
		return false // a name worked out when the line runs may be anything
	}
	args, certain := make([]string, 0, len(call.Args)-1), true
	for _, w := range call.Args[1:] {
		arg, ok := literal(w)
		args, certain = append(args, arg), certain && ok
	}
	if name == "printf" && len(call.Args) > 1 {
		if first, ok := literal(call.Args[1]); !ok || strings.HasPrefix(first, "-v") {
			return false // printf -v sets a variable
		}
	}
	if readOnly(name, args, certain, false) {
		return true
	}
	// Not a reader. It may stand on the line when the name cannot reach it:
	// not in its words, not down a pipe, and not through a variable such as
	// $_, which holds the last word of the command before.
	return !gets && !hasVariable(call)
}

// writesProtected reports whether the redirect sends output to a protected
// path, or to a target that is not known word for word.
func writesProtected(r *syntax.Redirect) bool {
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrInOut, syntax.DplOut, syntax.RdrClob, syntax.RdrAll, syntax.AppAll:
	default:
		return false // input, and here-documents
	}
	target, ok := literal(r.Word)
	if r.Op == syntax.DplOut && ok && descriptor.MatchString(target) {
		return false
	}
	// `>! file` is zsh's forced overwrite; read as Bash, the target is `!` and the file an argument.
	return !ok || target == "!" || strings.ContainsAny(target, "*?[") || protectedName.MatchString(target)
}

// names reports whether any word in the node names a protected path, also
// when braces spell it, as in `quality.{yaml,bak}`.
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
			found = found || protectedName.MatchString(text(w))
			return !found
		}
		for expanded, err := range expand.BracesSeq(nil, braced) {
			if err != nil || protectedName.MatchString(text(expanded)) {
				found = true // too many to work out counts as naming it
				break
			}
		}
		return !found
	})
	return found
}

// hasVariable reports whether the command uses a variable, in its own words
// or in a substitution inside them.
func hasVariable(call *syntax.CallExpr) bool {
	found := false
	syntax.Walk(call, func(n syntax.Node) bool {
		if _, ok := n.(*syntax.ParamExp); ok {
			found = true
		}
		return !found
	})
	return found
}

// literal returns the word as Bash will see it, when that is known from the
// text alone: plain text and quotes, with the quotes and escapes taken off.
func literal(w *syntax.Word) (string, bool) {
	s := text(w)
	return s, !strings.Contains(s, unknown)
}

// unknown stands in text() for a part whose value is known only when the
// line runs. It is no part of a file name, so `$DIR/quality.yaml` still names
// the path.
const unknown = "\x00"

// text returns the word with quotes and escapes taken off, and unknown in
// place of each expansion.
func text(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	var add func(parts []syntax.WordPart, quoted bool)
	add = func(parts []syntax.WordPart, quoted bool) {
		for _, p := range parts {
			switch p := p.(type) {
			case *syntax.Lit:
				b.WriteString(unescape(p.Value, quoted))
			case *syntax.SglQuoted:
				if p.Dollar {
					b.WriteString(unknown) // $'...' has escapes of its own
				} else {
					b.WriteString(p.Value)
				}
			case *syntax.DblQuoted:
				add(p.Parts, true)
			default:
				b.WriteString(unknown)
			}
		}
	}
	add(w.Parts, false)
	return b.String()
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
