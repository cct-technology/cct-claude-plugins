package rules

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/deps"
)

// registry is what QK060 asks about a package. Tests point it at a fake.
var registry = deps.Default()

func qk060() check.Rule {
	return check.Rule{
		ID:           "QK060",
		Check:        "supply-chain.dependencies",
		Title:        "New and upgraded dependencies exist, are old enough, maintained and licensed",
		Phase:        1,
		EnforcedFrom: check.Silver,
		Why: "A release that is a day old has been read by nobody. It may be a hijacked version of a package you have trusted for years, or a name an agent invented and an attacker registered. " +
			"QK060 looks at every dependency a change adds or moves to another version, in `package.json`, `go.mod`, `composer.json`, `Cargo.toml`, `pyproject.toml`, `Pipfile` and `requirements*.txt`. " +
			"The version is the one the lockfile holds (`package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `composer.lock`, `Cargo.lock`, `poetry.lock`, `uv.lock`, `Pipfile.lock`), so an upgrade that only changes the lockfile is found too. Without a lockfile, or for a name the lockfile does not hold, it is the version the manifest names, and the newest release when the registry has no version of exactly that number. Each finding names the version. " +
			"QK060 asks the public registry (npm, the Go module proxy, Packagist, crates.io, PyPI) four things. " +
			"The package exists. That version was published at least 14 days ago. It is not deprecated, abandoned or yanked. Its licence is one of MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, 0BSD and Unlicense; for a choice of licences one of them is enough. " +
			"Only what a manifest names is checked, not the dependencies of dependencies. Indirect Go requirements, paths, git URLs, workspace members and folders named `node_modules` or `vendor` are left out. " +
			"The Go module proxy holds no licences, so for Go the licence is not checked. " +
			"Only public registries are asked: a package from a private registry is reported as missing. " +
			"QK060 uses the network, so it runs only with `qk check --full` or when the `CI` environment variable is `true`; otherwise it is skipped and names what it did not check. " +
			"Answers are kept for 24 hours. A registry that cannot be reached is never a finding: the check is skipped and says which package it could not ask about.",
		Fix: "For a package that does not exist: check the name, it may be invented or misspelt. For a version that is too young: wait, or stay on an older version. For a deprecated one or a licence that is not allowed: pick another package. " +
			"If a person decides the dependency is right all the same, such as an urgent security patch or a package of your own in a private registry, ask them to approve a waiver with `qk waiver add QK060 --path <the manifest>`.",
		Run: runQK060,
	}
}

func runQK060(ctx check.Context) check.Result {
	if ctx.Changes == nil {
		return noChangeSet(ctx)
	}
	files, err := changes.Tree(ctx.Root)
	if err != nil {
		return check.Result{Status: check.Skipped, Reason: err.Error()}
	}
	present := map[string]bool{}
	for _, f := range files {
		present[f] = true
	}
	base := map[string]string{} // changed path -> its path at the base
	for _, f := range ctx.Changes.Files {
		base[f.Path] = f.Path
		if f.OldPath != "" {
			base[f.Path] = f.OldPath
		}
	}
	atBase := func(file string) []byte {
		from, changed := base[file]
		if !changed {
			from = file
		}
		content, _, _ := ctx.Changes.ContentAtBase(from) // nothing at the base: every dependency is new
		return content
	}
	read := func(file string) []byte {
		content, _ := os.ReadFile(filepath.Join(ctx.Root, filepath.FromSlash(file)))
		return content
	}

	type moved struct {
		file string
		pkg  deps.Package
	}
	var all []moved
	var named []string
	for _, file := range files {
		if installed(file) || !deps.IsManifest(file) {
			continue
		}
		// The manifest's lockfiles, nearest first. An upgrade may change only one of them.
		locks := lockRules[path.Base(file)].lockPaths(path.Dir(file))
		_, touched := base[file]
		for _, lock := range locks {
			_, lockTouched := base[lock]
			touched = touched || lockTouched
		}
		if !touched {
			continue
		}
		var oldLock, nowLock map[string][]string
		for _, lock := range locks {
			if nowLock == nil && present[lock] {
				nowLock = deps.Locked(lock, read(lock))
			}
			if oldLock == nil {
				if before := atBase(lock); before != nil {
					oldLock = deps.Locked(lock, before)
				}
			}
		}
		for _, pkg := range deps.Changed(file, atBase(file), read(file), oldLock, nowLock) {
			all = append(all, moved{file, pkg})
			named = append(named, strings.TrimSpace(pkg.String()+" "+pkg.Version))
		}
	}
	if len(all) == 0 {
		return check.Result{Status: check.Pass}
	}
	if !ctx.Full {
		return check.Result{Status: check.Skipped, Reason: fmt.Sprintf(
			"slow: asks the package registries about %s; runs with `qk check --full` or in CI", strings.Join(named, ", "))}
	}

	var findings []check.Finding
	var notAsked []string
	// ponytail: one package after another; ask in parallel when a change moves dozens.
	for _, m := range all {
		v := registry.Verify(m.pkg, ctx.Today)
		if v.Err != nil {
			notAsked = append(notAsked, fmt.Sprintf("%s: %v", m.pkg, v.Err))
			continue
		}
		for _, p := range v.Problems {
			findings = append(findings, check.Finding{File: m.file, Message: p.Message})
		}
	}
	switch {
	case len(findings) > 0:
		// ponytail: packages that could not be asked about go unmentioned next to real findings.
		return check.Result{Status: check.Fail, Findings: findings}
	case len(notAsked) > 0:
		return check.Result{Status: check.Skipped, Reason: "could not ask the registry about " + strings.Join(notAsked, "; ")}
	}
	return check.Result{Status: check.Pass}
}
