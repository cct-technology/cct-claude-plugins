package deps

import (
	"fmt"
	"slices"
	"testing"
)

func names(pkgs []Package) []string {
	out := []string{}
	for _, p := range pkgs {
		out = append(out, fmt.Sprintf("%s %s %s", p.Ecosystem, p.Name, p.Version))
	}
	slices.Sort(out)
	return out
}

// Without a lockfile, what the manifest names is all there is: a name that is
// new, or a version that is written differently than before.
func TestChangedReadsEveryManifest(t *testing.T) {
	cases := []struct {
		name, file, old, new string
		want                 []string
	}{
		{
			"npm: new names in any dependency group, and a version that changed",
			"web/package.json",
			`{"dependencies": {"react": "^18.0.0"}}`,
			`{"dependencies": {"react": "^19.0.0", "left-pad": "^1.3.0"}, "devDependencies": {"@types/node": "22.1.0"}, "optionalDependencies": {"fsevents": "*"}}`,
			[]string{"npm @types/node 22.1.0", "npm fsevents *", "npm left-pad ^1.3.0", "npm react ^19.0.0"},
		},
		{
			"npm: files, links, workspaces and git are not in the registry; an alias is looked up by its real name",
			"package.json",
			`{}`,
			`{"dependencies": {"a": "file:../a", "b": "workspace:*", "c": "git+https://example.com/c.git", "d": "github:x/d", "e": "link:../e", "f": "https://example.com/f.tgz", "g": "npm:@scope/real@^2.0.0"}}`,
			[]string{"npm @scope/real ^2.0.0"},
		},
		{
			"go: direct requirements; indirect ones and replaced ones are left out",
			"go.mod",
			"module example.com/app\n\nrequire github.com/google/uuid v1.6.0\n",
			"module example.com/app\n\nrequire (\n\tgithub.com/google/uuid v1.6.0\n\tgithub.com/spf13/cobra v1.8.0\n\tgolang.org/x/text v0.14.0 // indirect\n\texample.com/local v0.0.0\n)\n\nreplace example.com/local => ../local\n",
			[]string{"go github.com/spf13/cobra v1.8.0"},
		},
		{
			"composer: packages, not the platform",
			"composer.json",
			`{"require": {"php": "^8.2"}}`,
			`{"require": {"php": "^8.3", "ext-json": "*", "monolog/monolog": "^3.0"}, "require-dev": {"phpunit/phpunit": "^11"}}`,
			[]string{"composer monolog/monolog ^3.0", "composer phpunit/phpunit ^11"},
		},
		{
			"cargo: every dependency table and both ways to write a dependency, a bare version being a caret range; paths, git and the workspace are left out",
			"Cargo.toml",
			"[package]\nname = \"app\"\n\n[dependencies]\nserde = \"1\"\n",
			"[package]\nname = \"app\"\n\n[dependencies]\nserde = \"1\"\ntokio = { version = \"1.38\", features = [\"full\"] }\nlocal = { path = \"../local\" }\nforked = { git = \"https://example.com/forked\" }\nshared = { workspace = true }\nrenamed = { package = \"real-name\", version = \"0.3\" }\n\n[dependencies.clap]\nversion = \"4.5\"\nfeatures = [\"derive\"]\n\n[dev-dependencies]\ntempfile = \"3\"\n\n[target.'cfg(unix)'.dependencies]\nlibc = \"0.2\"\n",
			[]string{"cargo clap ^4.5", "cargo libc ^0.2", "cargo real-name ^0.3", "cargo tempfile ^3", "cargo tokio ^1.38"},
		},
		{
			"cargo: dotted keys, as workspace members write them",
			"crates/api/Cargo.toml",
			"[dependencies]\n",
			"[dependencies]\nserde.workspace = true\nlocal.path = \"../local\"\nforked.git = \"https://example.com/forked\"\nregex.version = \"1.10\"\nregex.features = [\"std\"]\nanyhow = \"1\"\n",
			[]string{"cargo anyhow ^1", "cargo regex ^1.10"},
		},
		{
			"pypi: a dependency group that includes another group",
			"pyproject.toml",
			"[project]\nname = \"app\"\n",
			"[project]\nname = \"app\"\n\n[dependency-groups]\ntest = [\"pytest>=8\"]\ndev = [{include-group = \"test\"}, \"ruff>=0.5\"]\n",
			[]string{"pypi pytest >=8", "pypi ruff >=0.5"},
		},
		{
			"pypi: a quoted name with a dot is one name",
			"pyproject.toml",
			"",
			"[tool.poetry.dependencies]\n\"zope.interface\" = \"^6.4\"\n",
			[]string{"pypi zope-interface ^6.4"},
		},
		{
			"pypi: requirements file, with extras, markers, comments and options",
			"requirements-dev.txt",
			"requests==2.31.0\n",
			"# tools\nrequests==2.31.0\nRuff>=0.5 ; python_version >= '3.9'\nuvicorn[standard]~=0.30.0  # server\n-r base.txt\n-e ./local\nhttps://example.com/x.whl\npytest\n",
			[]string{"pypi pytest ", "pypi ruff >=0.5", "pypi uvicorn ~=0.30.0"},
		},
		{
			"pypi: pyproject.toml, as a project, with Poetry and with dependency groups",
			"pyproject.toml",
			"[project]\nname = \"app\"\ndependencies = [\"requests>=2.31\"]\n",
			"[project]\nname = \"app\"\ndependencies = [\n    \"requests>=2.31\",\n    \"httpx>=0.27\",\n]\n\n[project.optional-dependencies]\ndocs = [\"mkdocs>=1.6\"]\n\n[dependency-groups]\ndev = [\"pytest>=8\"]\n\n[tool.poetry.dependencies]\npython = \"^3.12\"\npydantic = \"^2.8\"\nlocal = { path = \"../local\" }\n\n[tool.poetry.group.dev.dependencies]\nmypy = { version = \"^1.11\" }\n",
			[]string{"pypi httpx >=0.27", "pypi mkdocs >=1.6", "pypi mypy ^1.11", "pypi pydantic ^2.8", "pypi pytest >=8"},
		},
		{
			"pypi: Pipfile",
			"Pipfile",
			"[packages]\nrequests = \"*\"\n",
			"[packages]\nrequests = \"*\"\nflask = \"==3.0.3\"\n\n[dev-packages]\nblack = { version = \">=24\" }\n\n[requires]\npython_version = \"3.12\"\n",
			[]string{"pypi black >=24", "pypi flask ==3.0.3"},
		},
		{
			"a dependency that moves to another group with the same version has not changed",
			"package.json",
			`{"dependencies": {"react": "^18.0.0"}}`,
			`{"devDependencies": {"react": "^18.0.0"}}`,
			[]string{},
		},
		{
			"pypi names compare as PyPI compares them",
			"requirements.txt",
			"Typing_Extensions==4.12.2\n",
			"typing-extensions==4.12.2\n",
			[]string{},
		},
		{
			"a manifest that cannot be read declares nothing",
			"package.json",
			`{}`,
			`{"dependencies": `,
			[]string{},
		},
		{
			"a file that is not a manifest",
			"README.md",
			"",
			"requests==2.31.0\n",
			[]string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := names(Changed(c.file, []byte(c.old), []byte(c.new), nil, nil)); !slices.Equal(got, c.want) {
				t.Errorf("Changed(%s)\n got %q\nwant %q", c.file, got, c.want)
			}
		})
	}
}

// Only an exact version is looked up as written. A range resolves at install
// time, so it is judged by the newest release.
func TestNamedVersion(t *testing.T) {
	for spec, want := range map[string]string{
		"1.2.3": "1.2.3", "==3.0.3": "3.0.3", "=1.2.3": "1.2.3", "1.38": "1.38", " 18.3.1 ": "18.3.1",
		"v1.8.0": "v1.8.0", "v0.0.0-20240102150405-abcdef123456": "v0.0.0-20240102150405-abcdef123456",
		"^1.2.3": "", "~=0.30.0": "", ">=2.0,<3": "", "~1.2": "", "1.x": "", "1.2.*": "", ">=18": "",
		"^8.0 || ^9.0": "", "1.0 - 2.0": "", "*": "", "": "",
		"latest": "latest", "next": "next",
	} {
		if got := namedVersion(spec); got != want {
			t.Errorf("namedVersion(%q) = %q, want %q", spec, got, want)
		}
	}
}
