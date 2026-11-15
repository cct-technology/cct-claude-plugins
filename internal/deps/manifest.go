package deps

import (
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strings"
)

// IsManifest reports whether file is a dependency manifest Declared reads.
func IsManifest(file string) bool {
	name := path.Base(file)
	switch name {
	case "package.json", "go.mod", "composer.json", "Cargo.toml", "pyproject.toml", "Pipfile":
		return true
	}
	return strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt")
}

// Declared returns the dependencies a manifest takes from a public registry.
// Paths, git URLs, workspace members and the platform itself are left out.
// A file that is not a manifest, or cannot be read as one, declares nothing.
func Declared(file string, data []byte) []Package {
	var pkgs []Package
	name := path.Base(file)
	switch {
	case name == "package.json":
		pkgs = npmDeps(data)
	case name == "go.mod":
		pkgs = goDeps(data)
	case name == "composer.json":
		pkgs = composerDeps(data)
	case name == "Cargo.toml":
		pkgs = cargoDeps(data)
	case name == "pyproject.toml":
		pkgs = pyprojectDeps(data)
	case name == "Pipfile":
		pkgs = pipfileDeps(data)
	case strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt"):
		pkgs = requirementsDeps(data)
	}
	// By name, so findings come in the same order on every run: a JSON
	// manifest is read into a map, which has no order.
	slices.SortStableFunc(pkgs, func(a, b Package) int { return strings.Compare(a.Name, b.Name) })
	return pkgs
}

// jsonGroups returns the name-to-spec pairs under keys of a JSON manifest.
func jsonGroups(data []byte, keys ...string) map[string]string {
	var doc map[string]json.RawMessage
	_ = json.Unmarshal(data, &doc)
	all := map[string]string{}
	for _, k := range keys {
		var group map[string]string
		_ = json.Unmarshal(doc[k], &group)
		for name, spec := range group {
			all[name] = spec
		}
	}
	return all
}

func npmDeps(data []byte) []Package {
	var pkgs []Package
	for name, spec := range jsonGroups(data, "dependencies", "devDependencies", "optionalDependencies") {
		// `npm:real@^1` installs `real` under another name.
		if alias, ok := strings.CutPrefix(spec, "npm:"); ok {
			at := strings.LastIndex(alias, "@")
			if at <= 0 {
				at = len(alias)
			}
			name, spec = alias[:at], strings.TrimPrefix(alias[at:], "@")
		}
		// Anything else with a scheme or a path is not the registry's: file:, link:, workspace:, git+..., github:, a URL, ../x.
		if strings.ContainsAny(spec, ":/") {
			continue
		}
		pkgs = append(pkgs, Package{"npm", name, spec})
	}
	return pkgs
}

func composerDeps(data []byte) []Package {
	var pkgs []Package
	for name, spec := range jsonGroups(data, "require", "require-dev") {
		if strings.Contains(name, "/") { // php, ext-json and the like have no vendor
			pkgs = append(pkgs, Package{"composer", strings.ToLower(name), spec})
		}
	}
	return pkgs
}

func goDeps(data []byte) []Package {
	var pkgs []Package
	replaced := map[string]bool{}
	block := "" // the directive whose ( ... ) block the line is in
	for _, line := range strings.Split(string(data), "\n") {
		line, comment, _ := strings.Cut(line, "//")
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		directive := block
		switch {
		case block != "" && f[0] == ")":
			block = ""
			continue
		case block == "" && f[len(f)-1] == "(":
			block = f[0]
			continue
		case block == "":
			directive, f = f[0], f[1:]
		}
		switch {
		case directive == "replace" && len(f) > 0:
			replaced[f[0]] = true
		case directive == "require" && len(f) == 2 && !strings.Contains(comment, "indirect"):
			pkgs = append(pkgs, Package{"go", f[0], f[1]})
		}
	}
	kept := pkgs[:0]
	for _, p := range pkgs {
		if !replaced[p.Name] { // replaced by a path or a fork: the proxy is not asked for it
			kept = append(kept, p)
		}
	}
	return kept
}

// tomlEntry is one `key = value` line of a TOML file, with the table it is in.
type tomlEntry struct{ table, key, value string }

// tomlEntries reads the tables and keys of a TOML manifest.
// ponytail: reads lines, not TOML, as QK061 does. It follows a value over
// several lines by counting brackets, and takes a # outside quotes for a
// comment. Multi-line strings are not understood, and a dotted key only in a
// dependency table. Use a TOML
// parser when a manifest in the wild needs one.
func tomlEntries(data []byte) []tomlEntry {
	var entries []tomlEntry
	table := ""
	lines := strings.Split(string(data), "\n")
	for i := 0; i < len(lines); i++ {
		line := stripComment(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			table = strings.Trim(line, "[] \t")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		for depth(value) > 0 && i+1 < len(lines) {
			i++
			value += " " + stripComment(lines[i])
		}
		entries = append(entries, tomlEntry{table, strings.TrimSpace(key), value})
	}
	return entries
}

func stripComment(line string) string {
	quote := rune(0)
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#':
			return strings.TrimSpace(line[:i])
		}
	}
	return strings.TrimSpace(line)
}

func depth(value string) int {
	return strings.Count(value, "[") + strings.Count(value, "{") - strings.Count(value, "]") - strings.Count(value, "}")
}

var (
	tomlField  = regexp.MustCompile(`\b(version|path|git|url|package|workspace)\s*=\s*(?:"([^"]*)"|'([^']*)'|(true))`)
	tomlString = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	// tomlInlineTable matches `{ ... }` inside a list.
	tomlInlineTable = regexp.MustCompile(`\{[^}]*\}`)
)

// tomlDep reads the value of a dependency: `"1.2"` or `{ version = "1.2", ... }`.
// fromRegistry is false for a path, a git URL, a file URL or a workspace member.
func tomlDep(value string) (version, rename string, fromRegistry bool) {
	if !strings.HasPrefix(value, "{") {
		return strings.Trim(value, `"'`), "", true
	}
	fromRegistry = true
	for _, m := range tomlField.FindAllStringSubmatch(value, -1) {
		text := m[2] + m[3]
		switch m[1] {
		case "version":
			version = text
		case "package":
			rename = text
		default:
			fromRegistry = false
		}
	}
	return version, rename, fromRegistry
}

// tableDeps reads dependency tables: `name = spec` lines in a table that
// isDeps accepts, and `[<such a table>.name]` tables that spell one out.
func tableDeps(data []byte, ecosystem string, isDeps func(table string) bool) []Package {
	var pkgs []Package
	spelled := map[string][]string{} // table -> its lines, for [dependencies.name]
	var order []string
	for _, e := range tomlEntries(data) {
		// `serde.workspace = true` under [dependencies] is `workspace = true` under [dependencies.serde].
		// A quoted key is one name, dots and all.
		if name, field, dotted := strings.Cut(e.key, "."); dotted && isDeps(e.table) && !strings.ContainsAny(e.key, `"'`) {
			e.table, e.key = e.table+"."+name, field
		}
		e.key = strings.Trim(e.key, `"'`)
		if isDeps(e.table) {
			version, rename, ok := tomlDep(e.value)
			if name := e.key; ok {
				if rename != "" {
					name = rename
				}
				pkgs = append(pkgs, Package{ecosystem, name, version})
			}
			continue
		}
		if dot := strings.LastIndex(e.table, "."); dot > 0 && isDeps(e.table[:dot]) {
			if _, seen := spelled[e.table]; !seen {
				order = append(order, e.table)
			}
			spelled[e.table] = append(spelled[e.table], e.key+" = "+e.value)
		}
	}
	for _, table := range order {
		version, rename, ok := tomlDep("{" + strings.Join(spelled[table], ", ") + "}")
		if name := table[strings.LastIndex(table, ".")+1:]; ok {
			if rename != "" {
				name = rename
			}
			pkgs = append(pkgs, Package{ecosystem, name, version})
		}
	}
	return pkgs
}

func cargoDeps(data []byte) []Package {
	pkgs := tableDeps(data, "cargo", func(table string) bool {
		// dependencies, dev-dependencies, build-dependencies, alone or under target.<cfg> or workspace.
		last := table[strings.LastIndex(table, ".")+1:]
		return last == "dependencies" || last == "dev-dependencies" || last == "build-dependencies"
	})
	// Cargo reads a bare `1.2` as `^1.2`: a range, not that version.
	for i, p := range pkgs {
		if p.Version != "" && p.Version[0] >= '0' && p.Version[0] <= '9' {
			pkgs[i].Version = "^" + p.Version
		}
	}
	return pkgs
}

func pipfileDeps(data []byte) []Package {
	return pypiNamed(tableDeps(data, "pypi", func(table string) bool {
		return table == "packages" || table == "dev-packages"
	}))
}

func pyprojectDeps(data []byte) []Package {
	// Poetry names each dependency as a key.
	pkgs := tableDeps(data, "pypi", func(table string) bool {
		return strings.HasPrefix(table, "tool.poetry.") && strings.HasSuffix(table, "dependencies")
	})
	kept := pkgs[:0]
	for _, p := range pkgs {
		if p.Name != "python" {
			kept = append(kept, p)
		}
	}
	pkgs = kept
	// The standard tables hold lists of requirement strings.
	for _, e := range tomlEntries(data) {
		if (e.table == "project" && e.key == "dependencies") || e.table == "project.optional-dependencies" || e.table == "dependency-groups" {
			// `{include-group = "test"}` names another group, not a package.
			for _, m := range tomlString.FindAllStringSubmatch(tomlInlineTable.ReplaceAllString(e.value, ""), -1) {
				if p, ok := Requirement(m[1] + m[2]); ok {
					pkgs = append(pkgs, p)
				}
			}
		}
	}
	return pypiNamed(pkgs)
}

func requirementsDeps(data []byte) []Package {
	var pkgs []Package
	for _, line := range strings.Split(string(data), "\n") {
		if at := strings.Index(line, " #"); at >= 0 {
			line = line[:at]
		}
		if p, ok := Requirement(line); ok {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs
}

var requirementLine = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._-]*)\s*(?:\[[^\]]*\])?\s*([^;]*)`)

// Requirement reads one requirement string, such as `uvicorn[standard]~=0.30 ; python_version >= "3.9"`.
// Options (-r, -e), comments, URLs and `name @ url` are not registry packages.
func Requirement(line string) (Package, bool) {
	line = strings.TrimSpace(line)
	m := requirementLine.FindStringSubmatch(line)
	if m == nil || strings.Contains(line, "://") {
		return Package{}, false
	}
	spec := strings.TrimSpace(m[2])
	if strings.HasPrefix(spec, "@") {
		return Package{}, false
	}
	return Package{"pypi", pypiName(m[1]), spec}, true
}

var pypiSeparators = regexp.MustCompile(`[-_.]+`)

// pypiName writes a project name as PyPI compares names.
func pypiName(name string) string {
	return pypiSeparators.ReplaceAllString(strings.ToLower(name), "-")
}

func pypiNamed(pkgs []Package) []Package {
	for i := range pkgs {
		pkgs[i].Name = pypiName(pkgs[i].Name)
	}
	return pkgs
}
