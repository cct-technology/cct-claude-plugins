package hook

import (
	"regexp"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/cct-technology/cct-claude-plugins/internal/deps"
)

// installs returns the registry packages a shell line asks a package manager
// to install: `npm install x`, `pip install x`, `go get x` and their kin.
//
// It reads what is written. A package named through a variable, a file
// (`-r requirements.txt`) or another program's output is not seen, and a
// path, a URL or a git source is not a registry package. A line that only
// mentions an install, in a string or a here-document, installs nothing.
// What this misses, QK060 finds in CI, from the manifest and the lockfile.
func installs(line string, powershell bool) []deps.Package {
	commands := bashCommands
	if powershell {
		commands = powershellCommands
	}
	var pkgs []deps.Package
	for _, words := range commands(line) {
		pkgs = append(pkgs, packagesOf(words)...)
	}
	return pkgs
}

// unknown stands for a word that is not written out on the line.
const unknown = "\x00"

// bashCommands returns the words of every command on a Bash line, also those
// inside `if`, `( )`, `$( )` and the like. A line that cannot be read as Bash
// has no commands here: the guard fails open.
func bashCommands(line string) (cmds [][]string) {
	defer func() {
		if recover() != nil {
			cmds = nil
		}
	}()
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(line), "")
	if err != nil {
		return nil
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		if call, ok := n.(*syntax.CallExpr); ok && len(call.Args) > 0 {
			words := make([]string, len(call.Args))
			for i, w := range call.Args {
				words[i] = written(w)
			}
			cmds = append(cmds, words)
		}
		return true
	})
	return cmds
}

// written returns a word as the shell would pass it on, when every part of it
// is written on the line; otherwise unknown.
func written(w *syntax.Word) string {
	var b strings.Builder
	for _, p := range w.Parts {
		switch p := p.(type) {
		case *syntax.Lit:
			b.WriteString(unescape(p.Value, false))
		case *syntax.SglQuoted:
			if p.Dollar {
				return unknown
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				lit, ok := q.(*syntax.Lit)
				if !ok || p.Dollar {
					return unknown
				}
				b.WriteString(unescape(lit.Value, true))
			}
		default:
			return unknown
		}
	}
	return b.String()
}

// powershellCommands splits a PowerShell line into commands and words. There
// is no parser for PowerShell here: a command ends at `;`, a line end, `|`,
// `&`, a bracket or a brace, and a word at a space. Text in quotes stays
// together. A word with a `$` or a backtick is not written out: unknown.
func powershellCommands(line string) (cmds [][]string) {
	var (
		words   []string
		cur     strings.Builder
		inWord  bool
		lost    bool // the word holds something only PowerShell can work out
		quote   rune
		endWord = func() {
			if inWord {
				if lost {
					words = append(words, unknown)
				} else {
					words = append(words, cur.String())
				}
			}
			cur.Reset()
			inWord, lost = false, false
		}
		endCommand = func() {
			endWord()
			if len(words) > 0 {
				cmds = append(cmds, words)
			}
			words = nil
		}
	)
	for _, r := range line {
		switch {
		case quote != 0:
			switch {
			case r == quote:
				quote = 0
			case quote == '"' && (r == '$' || r == '`'):
				lost = true
			default:
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case strings.ContainsRune(";\n\r|&(){}", r):
			endCommand()
		case r == ' ' || r == '\t':
			endWord()
		case r == '$' || r == '`':
			inWord, lost = true, true
		default:
			inWord = true
			cur.WriteRune(r)
		}
	}
	endCommand()
	return cmds
}

// manager says how one package manager's install command is written.
type manager struct {
	ecosystem string
	installs  []string // the subcommands that add packages
	values    []string // options followed by a value, which is not a package
	elsewhere []string // options that say the package does not come from the registry
}

var pip = manager{
	ecosystem: "pypi", installs: []string{"install"},
	values: []string{"-r", "--requirement", "-c", "--constraint", "-e", "--editable", "-i", "--index-url", "--extra-index-url",
		"-t", "--target", "--prefix", "--root", "-f", "--find-links", "--python", "--platform", "--only-binary", "--no-binary",
		"-C", "--config-settings", "--progress-bar", "--upgrade-strategy", "--src", "--proxy", "--cert", "--client-cert",
		"--trusted-host", "--retries", "--timeout", "--log", "--cache-dir", "--implementation", "--abi", "--python-version",
		"--report", "--group", "--global-option", "--exists-action", "--use-feature", "--use-deprecated", "--build-constraint"},
}

var managers = map[string]manager{
	"npm":  {"npm", []string{"install", "i", "add"}, []string{"--prefix", "-C", "--registry", "--tag", "-w", "--workspace", "--cache", "--userconfig", "--loglevel"}, nil},
	"pnpm": {"npm", []string{"add"}, []string{"--filter", "-F", "--dir", "-C", "--registry", "--loglevel"}, nil},
	"yarn": {"npm", []string{"add"}, []string{"--cwd", "--registry", "--modules-folder"}, nil},
	"bun":  {"npm", []string{"add", "install", "i"}, []string{"--cwd", "--registry"}, nil},
	"go": {"go", []string{"get", "install"},
		[]string{"-C", "-modfile", "-mod", "-tags", "-ldflags", "-gcflags", "-asmflags", "-o", "-p", "-overlay", "-pkgdir", "-toolexec", "-exec"}, nil},
	"composer": {"composer", []string{"require"}, []string{"--working-dir", "-d"}, nil},
	"cargo": {"cargo", []string{"add", "install"},
		[]string{"--features", "-F", "--registry", "--rename", "--package", "-p", "--manifest-path", "--target", "--version", "--vers",
			"--root", "--index", "--profile", "-j", "--jobs", "--branch", "--tag", "--rev", "--bin", "--example", "--color", "--config", "-Z"},
		[]string{"--path", "--git"}},
	"pip":  pip,
	"pip3": pip,
	"uv": {"pypi", []string{"add"},
		[]string{"--group", "--optional", "--extra", "--index", "--default-index", "--python", "-p", "--directory", "--project", "--tag", "--branch", "--rev",
			"--marker", "-m", "--constraint", "-c", "--index-url", "-i", "--extra-index-url", "--script", "--package", "--bounds", "-r", "--requirements"},
		[]string{"--editable"}},
	"poetry": {"pypi", []string{"add"},
		[]string{"--group", "-G", "--extras", "-E", "--source", "--python", "--platform", "--markers", "-C", "--directory", "-P", "--project"},
		[]string{"--editable", "-e"}},
}

// wrappers run the command that follows them.
var wrappers = []string{"sudo", "command", "time", "nice", "env", "nohup"}

// packagesOf returns the registry packages one command installs, or nil for a
// command that is not an install.
func packagesOf(words []string) []deps.Package {
	for len(words) > 0 && slices.Contains(wrappers, program(words[0])) {
		words = words[1:]
		for len(words) > 0 && (strings.HasPrefix(words[0], "-") || strings.Contains(words[0], "=")) {
			words = words[1:] // the wrapper's options, and `env NAME=value`
		}
	}
	if len(words) == 0 {
		return nil
	}
	name, args := program(words[0]), words[1:]
	switch {
	case (name == "python" || name == "python3" || name == "py") && len(args) >= 2 && args[0] == "-m" && args[1] == "pip":
		name, args = "pip", args[2:]
	case name == "uv" && len(args) >= 1 && args[0] == "pip":
		name, args = "pip", args[1:]
	}
	m, ok := managers[name]
	if !ok {
		return nil
	}
	// The subcommand is the first word that is neither an option nor an option's value.
	var named []string
	sub := ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case slices.Contains(m.elsewhere, a):
			return nil
		case slices.Contains(m.values, a):
			i++
		case strings.HasPrefix(a, "-"):
		case sub == "":
			sub = a
		default:
			named = append(named, a)
		}
	}
	if !slices.Contains(m.installs, sub) {
		return nil
	}
	var pkgs []deps.Package
	for i := 0; i < len(named); i++ {
		next := ""
		if i+1 < len(named) {
			next = named[i+1]
		}
		pkg, usedNext, ok := packageIn(m.ecosystem, named[i], next)
		if usedNext {
			i++
		}
		if ok {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

// program is the name a command is run by: without its folder, in lower case,
// and without the ending Windows gives a program.
func program(word string) string {
	if at := strings.LastIndexAny(word, `/\`); at >= 0 {
		word = word[at+1:]
	}
	word = strings.ToLower(word)
	for _, ending := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		word = strings.TrimSuffix(word, ending)
	}
	return word
}

var (
	npmName      = regexp.MustCompile(`(?i)^(@[a-z0-9~-][a-z0-9._~-]*/)?[a-z0-9~-][a-z0-9._~-]*$`)
	goPath       = regexp.MustCompile(`^[a-z0-9.-]+\.[a-z]{2,}(/[A-Za-z0-9._~-]+)*$`)
	composerName = regexp.MustCompile(`^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9]([_.-]*[a-z0-9]+)*$`)
	crateName    = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	constraint   = regexp.MustCompile(`^([0-9^~<>=*!]|v[0-9]|dev-)`)
	archive      = regexp.MustCompile(`(?i)\.(tgz|tar\.gz|zip|whl)$`)
)

// packageIn reads one word of an install command as a registry package of the
// ecosystem. ok is false for a path, a URL, a git source, and anything that
// is not shaped like a package name: such a word is never sent to a registry.
// usedNext says the next word was this package's version, as composer writes it.
func packageIn(ecosystem, word, next string) (pkg deps.Package, usedNext, ok bool) {
	if word == unknown || word == "" || archive.MatchString(word) ||
		strings.HasPrefix(word, ".") || strings.HasPrefix(word, "/") || strings.HasPrefix(word, "~") || strings.Contains(word, `\`) {
		return deps.Package{}, false, false
	}
	// name@version, with the first @ of an npm scope left alone.
	cut := func(s string) (name, version string) {
		if at := strings.LastIndex(s, "@"); at > 0 {
			return s[:at], s[at+1:]
		}
		return s, ""
	}
	switch ecosystem {
	case "npm":
		// `alias@npm:real@1` installs `real`.
		if _, real, isAlias := strings.Cut(word, "@npm:"); isAlias {
			word = real
		}
		name, version := cut(word)
		if strings.Contains(version, ":") || strings.Contains(version, "/") || !npmName.MatchString(name) {
			return deps.Package{}, false, false // git+ssh://..., github:x/y, user/repo
		}
		return deps.Package{Ecosystem: "npm", Name: name, Version: version}, false, true
	case "go":
		name, version := cut(word)
		name = strings.TrimSuffix(name, "/...")
		return deps.Package{Ecosystem: "go", Name: name, Version: version}, false, goPath.MatchString(name)
	case "composer":
		name, version, _ := strings.Cut(strings.Replace(word, "=", ":", 1), ":")
		if !composerName.MatchString(strings.ToLower(name)) {
			return deps.Package{}, false, false // php, ext-json, or the version of the package before
		}
		if version == "" && next != unknown && constraint.MatchString(next) {
			version, usedNext = next, true
		}
		return deps.Package{Ecosystem: "composer", Name: strings.ToLower(name), Version: version}, usedNext, true
	case "cargo":
		name, version := cut(word)
		return deps.Package{Ecosystem: "cargo", Name: name, Version: version}, false, crateName.MatchString(name)
	case "pypi":
		if strings.Contains(word, "/") {
			return deps.Package{}, false, false
		}
		name, version := word, ""
		if !strings.Contains(word, " @") {
			name, version = cut(word) // poetry's name@^1.2
		}
		p, ok := deps.Requirement(name)
		if version != "" {
			p.Version = version
		}
		return p, false, ok
	}
	return deps.Package{}, false, false
}
