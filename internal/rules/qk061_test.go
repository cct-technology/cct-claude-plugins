package rules

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
)

func runRegisteredQK061(t *testing.T, ctx check.Context) check.Result {
	t.Helper()
	for _, r := range All() {
		if r.ID == "QK061" {
			if r.Check != "supply-chain.lockfile" || r.EnforcedFrom != check.Silver {
				t.Errorf("QK061 = check %q from %v, want supply-chain.lockfile from silver", r.Check, r.EnforcedFrom)
			}
			return r.Run(ctx)
		}
	}
	t.Fatal("QK061 is not registered")
	return check.Result{}
}

const (
	packageJSON = `{"name": "app", "dependencies": {"left-pad": "^1.3.0"}}`
	goMod       = "module example.com/app\n\ngo 1.22\n\nrequire github.com/google/uuid v1.6.0\n"
	cargoToml   = "[package]\nname = \"app\"\nversion = \"0.1.0\"\n\n[dependencies]\nserde = \"1\"\n"
	poetryToml  = "[tool.poetry]\nname = \"app\"\n\n[tool.poetry.dependencies]\nrequests = \"^2.32\"\n"
	uvToml      = "[project]\nname = \"app\"\ndependencies = [\n    \"requests>=2.32\",\n]\n\n[tool.uv]\npackage = false\n"
)

// One entry per ecosystem: its manifest with dependencies, with one more
// dependency, and with the same dependencies but another edit.
var lockCases = []struct {
	name, manifest, lock     string
	deps, moreDeps, sameDeps string
}{
	{
		"npm", "package.json", "package-lock.json", packageJSON,
		`{"name": "app", "dependencies": {"left-pad": "^1.3.0"}, "devDependencies": {"is-odd": "^3.0.1"}}`,
		`{"name": "app", "scripts": {"test": "node test.js"}, "dependencies": {"left-pad": "^1.3.0"}}`,
	},
	{
		"pnpm", "package.json", "pnpm-lock.yaml", packageJSON,
		`{"name": "app", "dependencies": {"left-pad": "^1.4.0"}}`,
		`{"dependencies": {"left-pad": "^1.3.0"}, "name": "app", "version": "2.0.0"}`,
	},
	{
		"yarn", "package.json", "yarn.lock", packageJSON,
		`{"name": "app", "dependencies": {"left-pad": "^1.3.0"}, "resolutions": {"minimist": "1.2.8"}}`,
		`{"name": "app", "private": true, "dependencies": {"left-pad": "^1.3.0"}}`,
	},
	{
		"go", "go.mod", "go.sum", goMod,
		"module example.com/app\n\ngo 1.22\n\nrequire (\n\tgithub.com/google/uuid v1.6.0\n\tgolang.org/x/text v0.14.0 // indirect\n)\n",
		"module example.com/app\r\n\r\ngo 1.23\r\n\r\nrequire (\r\n\tgithub.com/google/uuid v1.6.0 // indirect\r\n)\r\n",
	},
	{
		"composer", "composer.json", "composer.lock",
		`{"require": {"monolog/monolog": "^3.0"}}`,
		`{"require": {"monolog/monolog": "^3.0"}, "require-dev": {"phpunit/phpunit": "^11"}}`,
		`{"description": "An app", "require": {"monolog/monolog": "^3.0"}}`,
	},
	{
		"cargo", "Cargo.toml", "Cargo.lock", cargoToml,
		cargoToml + "\n[dev-dependencies]\ntempfile = \"3\"\n",
		"[package]\nname = \"app\"\nversion = \"0.1.0\"\ndescription = \"An app\"\n\n[dependencies]\nserde = \"1\"\n\n[features]\nfast = []\n",
	},
	{
		"poetry", "pyproject.toml", "poetry.lock", poetryToml,
		poetryToml + "\n[tool.poetry.group.dev.dependencies]\npytest = \"^8\"\n",
		poetryToml + "\n[tool.ruff]\nline-length = 100\nexclude = [\n    \"build\",\n]\n",
	},
	{
		"uv", "pyproject.toml", "uv.lock", uvToml,
		strings.Replace(uvToml, "\n]\n", "\n    \"uvicorn[standard]>=0.30\",\n]\n", 1),
		"# The app\n" + uvToml + "\n[tool.ruff]\nline-length = 100\n",
	},
	{
		"pipenv", "Pipfile", "Pipfile.lock",
		"[packages]\nrequests = \"*\"\n",
		"[packages]\nrequests = \"*\"\n\n[dev-packages]\npytest = \"*\"\n",
		"[packages]\nrequests = \"*\"\n\n[scripts]\ntest = \"pytest\"\n",
	},
}

func TestQK061PassesWithALockfile(t *testing.T) {
	for _, c := range lockCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := qk020Repo(t, map[string]string{c.manifest: c.deps, c.lock: "locked\n"})

			if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
				t.Errorf("result = %+v, want pass", res)
			}
		})
	}
}

func TestQK061FailsWithoutALockfile(t *testing.T) {
	for _, c := range lockCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := qk020Repo(t, map[string]string{c.manifest: c.deps})

			res := runRegisteredQK061(t, changed(t, ctx))

			if res.Status != check.Fail || len(res.Findings) != 1 {
				t.Fatalf("result = %+v, want fail with one finding", res)
			}
			if f := res.Findings[0]; f.File != c.manifest || !strings.Contains(f.Message, "`"+c.lock+"`") {
				t.Errorf("finding = %+v, want it on %s, naming %s", f, c.manifest, c.lock)
			}
		})
	}
}

func TestQK061FailsWhenDependenciesChangeButTheLockfileDoesNot(t *testing.T) {
	for _, c := range lockCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := qk020Repo(t, map[string]string{c.manifest: c.deps, c.lock: "locked\n"})
			put(t, ctx.Root, c.manifest, c.moreDeps)

			res := runRegisteredQK061(t, changed(t, ctx))

			if res.Status != check.Fail || len(res.Findings) != 1 {
				t.Fatalf("result = %+v, want fail with one finding", res)
			}
			if f := res.Findings[0]; f.File != c.manifest || !strings.Contains(f.Message, "`"+c.lock+"` did not") {
				t.Errorf("finding = %+v, want it on %s, saying %s did not change", f, c.manifest, c.lock)
			}

			put(t, ctx.Root, c.lock, "locked again\n")
			if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
				t.Errorf("with the lockfile updated: result = %+v, want pass", res)
			}
		})
	}
}

// A changed script, version, comment or line ending leaves the lockfile as it is.
func TestQK061PassesWhenAManifestChangesButItsDependenciesDoNot(t *testing.T) {
	for _, c := range lockCases {
		t.Run(c.name, func(t *testing.T) {
			ctx := qk020Repo(t, map[string]string{c.manifest: c.deps, c.lock: "locked\n"})
			put(t, ctx.Root, c.manifest, c.sameDeps)

			if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
				t.Errorf("result = %+v, want pass", res)
			}
		})
	}
}

func TestQK061SkipsPythonWithoutALockTool(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{
		"requirements.txt": "requests==2.32.3\n",
		"pyproject.toml":   "[project]\nname = \"app\"\ndependencies = [\"requests\"]\n",
	})

	res := runRegisteredQK061(t, changed(t, ctx))

	if res.Status != check.Skipped || !strings.Contains(res.Reason, "requirements.txt") {
		t.Errorf("result = %+v, want skipped, naming requirements.txt", res)
	}
}

// requirements.txt beside a checked ecosystem does not hide that one's result.
func TestQK061ChecksOtherEcosystemsBesideRequirementsTxt(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"requirements-dev.txt": "pytest\n", "go.mod": goMod, "go.sum": "locked\n"})
	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("with go.sum: result = %+v, want pass", res)
	}

	ctx = qk020Repo(t, map[string]string{"requirements.txt": "pytest\n", "go.mod": goMod})
	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("without go.sum: result = %+v, want fail with one finding", res)
	}
}

// uv needs no `[tool.uv]` table, so its lockfile shows it is in use.
func TestQK061ChecksAPlainPyprojectThatHasALockfile(t *testing.T) {
	plain := "[project]\nname = \"app\"\ndependencies = [\"requests\"]\n"
	ctx := qk020Repo(t, map[string]string{"pyproject.toml": plain, "uv.lock": "locked\n"})
	put(t, ctx.Root, "pyproject.toml", strings.Replace(plain, `"requests"`, `"requests", "httpx"`, 1))

	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("result = %+v, want fail with one finding", res)
	}
}

func TestQK061NeedsNoLockfileWithoutDependencies(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{
		"go.mod":            "module example.com/app\n\ngo 1.22\n",
		"web/package.json":  `{"type": "module", "scripts": {"build": "tsc"}}`,
		"php/composer.json": `{"name": "acme/app", "require": {}}`,
		"rs/Cargo.toml":     "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
		"py/pyproject.toml": "[tool.poetry]\nname = \"app\"\n",
		"bad/package.json":  "{not json",
		"README.md":         "x\n",
	})

	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// A workspace keeps one lockfile at its root. Go modules each have their own go.sum.
func TestQK061TakesAWorkspaceLockfileInAParentFolder(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{
		"package.json":              `{"workspaces": ["packages/*"], "devDependencies": {"typescript": "^5"}}`,
		"pnpm-lock.yaml":            "locked\n",
		"packages/api/package.json": packageJSON,
		"Cargo.toml":                "[workspace]\nmembers = [\"crates/*\"]\n",
		"Cargo.lock":                "locked\n",
		"crates/core/Cargo.toml":    cargoToml,
		"go.mod":                    goMod,
		"go.sum":                    "locked\n",
		"tools/go.mod":              goMod,
	})

	res := runRegisteredQK061(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "tools/go.mod" {
		t.Fatalf("result = %+v, want fail with one finding on tools/go.mod", res)
	}
}

func TestQK061FailsWhenANewWorkspacePackageLeavesTheLockfileAlone(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{
		"package.json":      `{"workspaces": ["packages/*"], "devDependencies": {"typescript": "^5"}}`,
		"package-lock.json": "locked\n",
	})
	put(t, ctx.Root, "packages/api/package.json", packageJSON)

	res := runRegisteredQK061(t, changed(t, ctx))

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "packages/api/package.json" {
		t.Errorf("result = %+v, want fail with one finding on packages/api/package.json", res)
	}
}

// Installed dependencies carry manifests of their own, also when they are committed.
func TestQK061IgnoresInstalledDependencies(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{
		"composer.json":                          `{"require": {"monolog/monolog": "^3.0"}}`,
		"composer.lock":                          "locked\n",
		"vendor/monolog/monolog/composer.json":   `{"require": {"psr/log": "^3.0"}}`,
		"web/node_modules/left-pad/package.json": packageJSON,
	})

	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

// A lockfile git ignores is not in the repository, so the build does not get it.
func TestQK061DoesNotCountALockfileGitIgnores(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"package.json": packageJSON, ".gitignore": "package-lock.json\n"})
	put(t, ctx.Root, "package-lock.json", "locked\n")

	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("result = %+v, want fail with one finding", res)
	}
}

func TestQK061CountsALockfileNotYetCommitted(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"README.md": "x\n"})
	put(t, ctx.Root, "go.mod", goMod)
	put(t, ctx.Root, "go.sum", "locked\n")

	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}

func TestQK061FailsWhenTheLockfileIsDeleted(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"go.mod": goMod, "go.sum": "locked\n"})
	if err := os.Remove(filepath.Join(ctx.Root, "go.sum")); err != nil {
		t.Fatal(err)
	}
	ctx.ChangedOnly = true // the manifest did not change, its lockfile did

	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Fail || len(res.Findings) != 1 {
		t.Errorf("result = %+v, want fail with one finding", res)
	}
}

// With --changed, a manifest nobody touched is left alone: the Stop hook
// reports what this change did.
func TestQK061WithChangedOnlyLooksAtChangedManifests(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{
		"old/go.mod":       goMod,
		"new/go.mod":       "module example.com/new\n\ngo 1.22\n",
		"requirements.txt": "pytest\n",
	})
	ctx.ChangedOnly = true
	put(t, ctx.Root, "README.md", "x\n")

	if res := runRegisteredQK061(t, changed(t, ctx)); res.Status != check.Pass {
		t.Fatalf("nothing touched: result = %+v, want pass", res)
	}

	put(t, ctx.Root, "new/go.mod", goMod)
	res := runRegisteredQK061(t, changed(t, ctx))
	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "new/go.mod" {
		t.Errorf("new/go.mod touched: result = %+v, want fail with one finding on new/go.mod", res)
	}
}

// Without a base there is nothing to compare, and no way to tell what this
// change touched. That a lockfile is there can still be checked.
func TestQK061WithoutAChangeSetChecksThatLockfilesAreThere(t *testing.T) {
	ctx := qk020Repo(t, map[string]string{"go.mod": goMod, "web/package.json": packageJSON, "web/yarn.lock": "locked\n"})
	ctx.ChangesErr = errors.New("cannot find the default branch")
	ctx.ChangedOnly = true

	res := runRegisteredQK061(t, ctx)

	if res.Status != check.Fail || len(res.Findings) != 1 || res.Findings[0].File != "go.mod" {
		t.Fatalf("result = %+v, want fail with one finding on go.mod", res)
	}

	put(t, ctx.Root, "go.sum", "locked\n")
	if res := runRegisteredQK061(t, ctx); res.Status != check.Pass {
		t.Errorf("with go.sum: result = %+v, want pass", res)
	}
}

// Outside git nothing is tracked, so there is nothing to check. A skip here
// would show after every turn in a folder that is not a repository.
func TestQK061PassesOutsideAGitRepository(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	ctx := check.Context{
		Root:       t.TempDir(),
		Manifest:   manifest.Loaded{Present: true, Manifest: manifest.Manifest{Version: 1}},
		ChangesErr: errors.New("not a git repository"),
	}
	put(t, ctx.Root, "package.json", packageJSON)

	if res := runRegisteredQK061(t, ctx); res.Status != check.Pass {
		t.Errorf("result = %+v, want pass", res)
	}
}
