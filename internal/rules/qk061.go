package rules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
)

func qk061() check.Rule {
	return check.Rule{
		ID:           "QK061",
		Check:        "supply-chain.lockfile",
		Title:        "Dependencies have a lockfile that changes with them",
		Phase:        1,
		EnforcedFrom: check.Silver,
		Why: "Without a lockfile, a build installs whatever versions are newest that day, not the ones that were reviewed and tested. " +
			"QK061 reads every dependency manifest git tracks or would add: `package.json` (npm, pnpm, Yarn, Bun), `go.mod`, `composer.json`, `Cargo.toml`, `Pipfile`, and `pyproject.toml` when Poetry or uv is in use. " +
			"A manifest that declares dependencies needs its lockfile in git. In a workspace, a lockfile in a parent folder counts. " +
			"When a manifest's dependencies change, its lockfile must change in the same change set. Other edits to a manifest, such as a script or a version number, need nothing. " +
			"QK061 only reads files; it never runs a package manager. Folders named `node_modules` or `vendor` are left out. " +
			"Python with only `requirements.txt`, or a `pyproject.toml` without Poetry or uv, has no lockfile to check, so the check is skipped and says so. " +
			"When there is no base to compare with, QK061 only checks that each lockfile is there. Outside a git repository it checks nothing.",
		Fix: "Run the package manager so it writes the lockfile (`npm install`, `pnpm install`, `yarn install`, `go mod tidy`, `composer update --lock`, `cargo generate-lockfile`, `poetry lock`, `uv lock`, `pipenv lock`), and commit the lockfile with the manifest. If the lockfile exists but is reported as missing, git ignores it: take it out of `.gitignore`. If a manifest should have no lockfile, ask a person to approve a waiver with `qk waiver add QK061`.",
		Run: runQK061,
	}
}

// lockRule says which lockfile a dependency manifest needs.
type lockRule struct {
	locks  []string            // lockfile names; one of them is enough
	shared bool                // a workspace keeps one lockfile in a parent folder
	deps   func([]byte) string // the dependencies a manifest declares, in a form that compares; "" for none
	tools  []string            // when set, a missing lockfile counts only if the manifest has one of these
}

// lockRules is keyed by manifest file name.
var lockRules = map[string]lockRule{
	"package.json": {
		locks:  []string{"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb"},
		shared: true,
		deps:   jsonKeys("dependencies", "devDependencies", "optionalDependencies", "peerDependencies", "overrides", "resolutions"),
	},
	"go.mod":        {locks: []string{"go.sum"}, deps: goRequires},
	"composer.json": {locks: []string{"composer.lock"}, deps: jsonKeys("require", "require-dev")},
	"Cargo.toml":    {locks: []string{"Cargo.lock"}, shared: true, deps: tomlParts("dependenc")},
	"Pipfile":       {locks: []string{"Pipfile.lock"}, deps: tomlParts("packages")},
	// A pyproject.toml alone does not say a lock tool is in use: pip and setuptools read it too.
	"pyproject.toml": {
		locks:  []string{"poetry.lock", "uv.lock"},
		shared: true,
		deps:   tomlParts("dependenc"),
		tools:  []string{"[tool.poetry", "[tool.uv"},
	},
}

func runQK061(ctx check.Context) check.Result {
	files, err := changes.Tree(ctx.Root)
	if err != nil {
		// Outside git nothing is tracked, so nothing is owed. A skip here would
		// show after every turn in a folder that is not a repository.
		return check.Result{Status: check.Pass}
	}
	present := map[string]bool{}
	for _, f := range files {
		present[f] = true
	}
	// ponytail: without a change set (no commits yet, no default branch) every
	// manifest is looked at and only "is there a lockfile" is checked; the
	// result does not say the comparison was left out.
	base := map[string]string{} // changed path -> its path at the base
	if ctx.Changes != nil {
		for _, f := range ctx.Changes.Files {
			base[f.Path] = f.Path
			if f.OldPath != "" {
				base[f.Path], base[f.OldPath] = f.OldPath, f.OldPath
			}
		}
	}
	changedOnly := ctx.ChangedOnly && ctx.Changes != nil

	var findings []check.Finding
	var unlocked []string // Python manifests no lock tool covers
	checked := 0
	for _, file := range files {
		if installed(file) {
			continue
		}
		name := path.Base(file)
		from, changed := base[file]
		if strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt") {
			if changed || !changedOnly {
				unlocked = append(unlocked, file)
			}
			continue
		}
		rule, ok := lockRules[name]
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ctx.Root, filepath.FromSlash(file)))
		if err != nil {
			continue
		}
		deps := rule.deps(data)
		if deps == "" {
			continue // nothing to lock
		}
		lock, lockChanged := "", false
		for _, p := range rule.lockPaths(path.Dir(file)) {
			if lock == "" && present[p] {
				lock = p
			}
			if _, ok := base[p]; ok {
				lockChanged = true // changed, added or deleted
			}
		}
		if changedOnly && !changed && !lockChanged {
			continue
		}
		switch {
		case lock == "" && len(rule.tools) > 0 && !containsAny(data, rule.tools):
			unlocked = append(unlocked, file)
			continue
		case lock == "":
			findings = append(findings, check.Finding{File: file, Message: "dependencies without a lockfile: commit " + oneOf(rule.locks)})
		case changed && !lockChanged:
			old := ""
			if was, ok, _ := ctx.Changes.ContentAtBase(from); ok {
				old = rule.deps(was)
			}
			if old != deps {
				findings = append(findings, check.Finding{File: file, Message: fmt.Sprintf(
					"dependencies changed, but `%s` did not: update the lockfile and commit it", lock)})
			}
		}
		checked++
	}

	switch {
	case len(findings) > 0:
		return check.Result{Status: check.Fail, Findings: findings}
	case checked == 0 && len(unlocked) > 0:
		return check.Result{Status: check.Skipped, Reason: fmt.Sprintf(
			"Python dependencies without a lock tool (%s), so there is no lockfile to check; uv, Poetry and Pipenv write one", strings.Join(unlocked, ", "))}
	}
	return check.Result{Status: check.Pass}
}

// installed reports whether file belongs to an installed dependency, which
// has a manifest of its own and no lockfile.
func installed(file string) bool {
	return strings.Contains("/"+file, "/node_modules/") || strings.Contains("/"+file, "/vendor/")
}

// lockPaths lists where the lockfile of a manifest in dir may be, nearest first.
// ponytail: any lockfile further up counts for a workspace ecosystem, also when
// the manifest is not a member of that workspace. Read the workspace globs if
// stray manifests slip through.
func (r lockRule) lockPaths(dir string) []string {
	var paths []string
	for {
		for _, lock := range r.locks {
			paths = append(paths, path.Join(dir, lock))
		}
		if !r.shared || dir == "." {
			return paths
		}
		dir = path.Dir(dir)
	}
}

func containsAny(data []byte, marks []string) bool {
	return slices.ContainsFunc(marks, func(m string) bool { return bytes.Contains(data, []byte(m)) })
}

// oneOf writes file names as "`a`, `b` or `c`".
func oneOf(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "`" + n + "`"
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}

// jsonKeys returns the non-empty objects under keys of a JSON manifest.
// A manifest that is not JSON declares nothing qk can read.
func jsonKeys(keys ...string) func([]byte) string {
	return func(data []byte) string {
		var doc map[string]json.RawMessage
		if json.Unmarshal(data, &doc) != nil {
			return ""
		}
		var b strings.Builder
		for _, k := range keys {
			var deps map[string]any
			if json.Unmarshal(doc[k], &deps) == nil && len(deps) > 0 {
				sorted, _ := json.Marshal(deps) // map keys come out sorted
				b.WriteString(k)
				b.Write(sorted)
			}
		}
		return b.String()
	}
}

// goRequires returns the `require` lines of a go.mod, sorted, without comments.
func goRequires(data []byte) string {
	var reqs []string
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "//")
		f := strings.Fields(line)
		switch {
		case len(f) == 0:
		case inBlock && f[0] == ")":
			inBlock = false
		case inBlock:
			reqs = append(reqs, strings.Join(f, " "))
		case f[0] == "require" && f[len(f)-1] == "(":
			inBlock = true
		case f[0] == "require":
			reqs = append(reqs, strings.Join(f[1:], " "))
		}
	}
	slices.Sort(reqs)
	return strings.Join(reqs, "\n")
}

// tomlParts returns the lines of a TOML manifest that declare dependencies:
// every table whose name contains word, and every key containing word in
// another table, such as `dependencies = [...]` under `[project]`.
// ponytail: reads lines, not TOML. A string with an unmatched `[` or `]`
// outside those parts confuses it, and so does a changed comment at the end of
// a kept line. Use a TOML parser when a manifest in the wild needs one.
func tomlParts(word string) func([]byte) string {
	return func(data []byte) string {
		var kept []string
		inTable, keep, depth := false, false, 0
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			switch {
			case depth > 0: // inside a value that spans lines: as the line that opened it
			case strings.HasPrefix(line, "["):
				inTable = strings.Contains(line, word)
				keep = inTable
			case !inTable:
				key, _, _ := strings.Cut(line, "=")
				keep = strings.Contains(key, word)
			}
			depth += strings.Count(line, "[") - strings.Count(line, "]")
			if keep {
				kept = append(kept, line)
			}
		}
		return strings.Join(kept, "\n")
	}
}
