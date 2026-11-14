package deps

import (
	"encoding/json"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Changed returns the dependencies of the manifest `file` that are new or at
// a new version: the name was not there before, or the version it resolves to
// is not one it resolved to before. The version is the one the lockfile holds
// for the name, and what the manifest names when the lockfile does not have
// it. So an upgrade is found when the manifest changes and when only the
// lockfile does. old is empty for a new manifest; the locks come from Locked.
func Changed(file string, old, now []byte, oldLock, nowLock map[string][]string) []Package {
	resolved := func(p Package, lock map[string][]string) []string {
		if versions := lock[p.Name]; len(versions) > 0 {
			return versions
		}
		return []string{p.Version}
	}
	before := map[string][]string{}
	for _, p := range Declared(file, old) {
		before[p.Name] = append(before[p.Name], resolved(p, oldLock)...)
	}
	var changed []Package
	seen := map[string]bool{}
	for _, p := range Declared(file, now) {
		if seen[p.Name] { // named in two groups: once is enough
			continue
		}
		seen[p.Name] = true
		for _, version := range resolved(p, nowLock) {
			if !slices.Contains(before[p.Name], version) {
				changed = append(changed, Package{p.Ecosystem, p.Name, version})
			}
		}
	}
	return changed
}

// Locked returns the exact versions a lockfile holds, by package name, named
// as Declared names them. A name has several versions when the lockfile does.
// A file that is not a lockfile qk reads, or cannot be read as one, holds nothing.
// ponytail: bun.lock is not read; a Bun project is judged by its package.json.
func Locked(file string, data []byte) map[string][]string {
	held := map[string][]string{}
	add := func(name, version string) {
		if name != "" && version != "" && !slices.Contains(held[name], version) {
			held[name] = append(held[name], version)
		}
	}
	switch path.Base(file) {
	case "package-lock.json", "npm-shrinkwrap.json":
		npmLock(data, add)
	case "pnpm-lock.yaml":
		pnpmLock(data, add)
	case "yarn.lock":
		yarnLock(data, add)
	case "composer.lock":
		var doc struct {
			Packages    []struct{ Name, Version string } `json:"packages"`
			PackagesDev []struct{ Name, Version string } `json:"packages-dev"`
		}
		_ = json.Unmarshal(data, &doc)
		for _, p := range append(doc.Packages, doc.PackagesDev...) {
			add(strings.ToLower(p.Name), p.Version)
		}
	case "Cargo.lock":
		tomlPackages(data, func(name, version, source string) {
			if strings.HasPrefix(source, "registry") { // the workspace's own crates have no source
				add(name, version)
			}
		})
	case "poetry.lock", "uv.lock":
		tomlPackages(data, func(name, version, _ string) { add(pypiName(name), version) })
	case "Pipfile.lock":
		type group map[string]struct{ Version string }
		var doc struct {
			Default group `json:"default"`
			Develop group `json:"develop"`
		}
		_ = json.Unmarshal(data, &doc)
		for _, g := range []group{doc.Default, doc.Develop} {
			for name, p := range g {
				add(pypiName(name), strings.TrimPrefix(p.Version, "=="))
			}
		}
	}
	for _, versions := range held {
		slices.Sort(versions) // the same order on every run, whatever order the file was read in
	}
	return held
}

func npmLock(data []byte, add func(name, version string)) {
	type entry struct {
		Version string `json:"version"`
	}
	var doc struct {
		Packages     map[string]entry `json:"packages"`     // lockfile version 2 and 3
		Dependencies map[string]entry `json:"dependencies"` // version 1
	}
	_ = json.Unmarshal(data, &doc)
	const dir = "node_modules/"
	for key, p := range doc.Packages {
		// What a manifest names is installed at the top, or at the top of a
		// workspace. A copy under another package's node_modules is not.
		if strings.Count(key, dir) == 1 {
			add(key[strings.Index(key, dir)+len(dir):], p.Version)
		}
	}
	if len(doc.Packages) == 0 {
		for name, p := range doc.Dependencies {
			add(name, p.Version)
		}
	}
}

func pnpmLock(data []byte, add func(name, version string)) {
	var doc map[string]any
	if yaml.Unmarshal(data, &doc) != nil {
		return
	}
	groups := func(importer map[string]any) {
		for _, group := range []string{"dependencies", "devDependencies", "optionalDependencies"} {
			deps, _ := importer[group].(map[string]any)
			for name, held := range deps {
				version, _ := held.(string) // lockfile version 5: the version itself
				if fields, ok := held.(map[string]any); ok {
					version, _ = fields["version"].(string)
				}
				// 18.3.1(react@18.3.1) and 18.3.1_react@18.3.1 name the peers it was installed with.
				version, _, _ = strings.Cut(version, "(")
				version, _, _ = strings.Cut(version, "_")
				if !strings.Contains(version, ":") { // link:../x is a workspace member
					add(name, version)
				}
			}
		}
	}
	groups(doc) // one package, or lockfile version 5
	importers, _ := doc["importers"].(map[string]any)
	for _, importer := range importers {
		if fields, ok := importer.(map[string]any); ok {
			groups(fields)
		}
	}
}

// yarnLock reads both the classic format (`version "1.3.0"`) and Berry's
// (`version: 1.3.0`). An entry starts with its names at the left margin.
func yarnLock(data []byte, add func(name, version string)) {
	name := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case !strings.HasPrefix(line, " "):
			// `"@scope/pkg@npm:^2.0.0, @scope/pkg@npm:^2.1.0":`: the first name, up to its last @.
			first, _, _ := strings.Cut(strings.Trim(line, `":`), ",")
			first = strings.Trim(first, `" `)
			name = ""
			if at := strings.LastIndex(first, "@"); at > 0 {
				name = first[:at]
			}
		case name != "":
			if version, ok := strings.CutPrefix(strings.TrimSpace(line), "version"); ok {
				add(name, strings.Trim(version, `": `))
				name = ""
			}
		}
	}
}

// tomlPackages reads the [[package]] blocks of Cargo.lock, poetry.lock and
// uv.lock. Each block starts with its name.
func tomlPackages(data []byte, each func(name, version, source string)) {
	var name, version, source string
	flush := func() {
		if name != "" {
			each(name, version, source)
		}
		name, version, source = "", "", ""
	}
	for _, e := range tomlEntries(data) {
		if e.table != "package" {
			continue
		}
		value := strings.Trim(e.value, `"'`)
		switch e.key {
		case "name":
			flush()
			name = value
		case "version":
			version = value
		case "source":
			source = value
		}
	}
	flush()
}
