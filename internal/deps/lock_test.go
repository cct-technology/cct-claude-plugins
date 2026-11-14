package deps

import (
	"fmt"
	"slices"
	"testing"
)

func locked(file, data string) []string {
	out := []string{}
	for name, versions := range Locked(file, []byte(data)) {
		for _, v := range versions {
			out = append(out, fmt.Sprintf("%s %s", name, v))
		}
	}
	slices.Sort(out)
	return out
}

func TestLockedReadsTheVersionsALockfileHolds(t *testing.T) {
	cases := []struct {
		name, file, data string
		want             []string
	}{
		{
			"package-lock.json: what is installed at the top, and in a workspace; not a copy nested under another package",
			"package-lock.json",
			`{"lockfileVersion": 3, "packages": {
				"": {"name": "app", "dependencies": {"react": "^18.3.1"}},
				"node_modules/react": {"version": "18.3.1"},
				"node_modules/@types/node": {"version": "22.1.0"},
				"node_modules/react/node_modules/loose-envify": {"version": "1.4.0"},
				"packages/web/node_modules/react": {"version": "19.0.0"},
				"packages/web": {"version": "1.0.0"}
			}}`,
			[]string{"@types/node 22.1.0", "react 18.3.1", "react 19.0.0"},
		},
		{
			"package-lock.json, version 1",
			"package-lock.json",
			`{"lockfileVersion": 1, "dependencies": {"left-pad": {"version": "1.3.0"}, "@scope/pkg": {"version": "2.0.0", "dependencies": {"inner": {"version": "0.1.0"}}}}}`,
			[]string{"@scope/pkg 2.0.0", "left-pad 1.3.0"},
		},
		{
			"pnpm-lock.yaml: every importer, without the peer suffix",
			"pnpm-lock.yaml",
			"lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n      react:\n        specifier: ^18.3.1\n        version: 18.3.1\n    devDependencies:\n      '@types/node':\n        specifier: ^22\n        version: 22.1.0\n  packages/web:\n    dependencies:\n      react-dom:\n        specifier: ^18.3.1\n        version: 18.3.1(react@18.3.1)\npackages:\n  loose-envify@1.4.0:\n    resolution: {integrity: sha512-x}\n",
			[]string{"@types/node 22.1.0", "react 18.3.1", "react-dom 18.3.1"},
		},
		{
			"pnpm-lock.yaml, version 5",
			"pnpm-lock.yaml",
			"lockfileVersion: 5.4\ndependencies:\n  left-pad: 1.3.0\ndevDependencies:\n  typescript: 5.5.4\n",
			[]string{"left-pad 1.3.0", "typescript 5.5.4"},
		},
		{
			"yarn.lock, classic: every entry, a name once per version",
			"yarn.lock",
			"# yarn lockfile v1\n\n\"@types/node@^22\":\n  version \"22.1.0\"\n  resolved \"https://registry.yarnpkg.com/x\"\n\nleft-pad@^1.3.0, left-pad@~1.3.0:\n  version \"1.3.0\"\n\nleft-pad@^1.1.0:\n  version \"1.1.3\"\n",
			[]string{"@types/node 22.1.0", "left-pad 1.1.3", "left-pad 1.3.0"},
		},
		{
			"yarn.lock, berry",
			"yarn.lock",
			"__metadata:\n  version: 8\n\n\"left-pad@npm:^1.3.0\":\n  version: 1.3.0\n  resolution: \"left-pad@npm:1.3.0\"\n\n\"@scope/pkg@npm:^2.0.0, @scope/pkg@npm:^2.1.0\":\n  version: 2.1.0\n",
			[]string{"@scope/pkg 2.1.0", "left-pad 1.3.0"},
		},
		{
			"composer.lock: packages and dev packages",
			"composer.lock",
			`{"packages": [{"name": "monolog/monolog", "version": "3.7.0"}], "packages-dev": [{"name": "phpunit/phpunit", "version": "11.3.1"}]}`,
			[]string{"monolog/monolog 3.7.0", "phpunit/phpunit 11.3.1"},
		},
		{
			"Cargo.lock: registry packages, a name once per version; not the workspace's own crates",
			"Cargo.lock",
			"version = 3\n\n[[package]]\nname = \"app\"\nversion = \"0.1.0\"\ndependencies = [\n \"serde\",\n]\n\n[[package]]\nname = \"serde\"\nversion = \"1.0.229\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\n\n[[package]]\nname = \"syn\"\nversion = \"1.0.109\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\n\n[[package]]\nname = \"syn\"\nversion = \"2.0.72\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\n",
			[]string{"serde 1.0.229", "syn 1.0.109", "syn 2.0.72"},
		},
		{
			"poetry.lock and uv.lock: names as PyPI compares them",
			"uv.lock",
			"version = 1\n\n[[package]]\nname = \"Typing_Extensions\"\nversion = \"4.12.2\"\nsource = { registry = \"https://pypi.org/simple\" }\n\n[[package]]\nname = \"app\"\nversion = \"0.1.0\"\nsource = { virtual = \".\" }\n",
			[]string{"app 0.1.0", "typing-extensions 4.12.2"},
		},
		{
			"Pipfile.lock: both groups, without the ==",
			"Pipfile.lock",
			`{"_meta": {"hash": {"sha256": "x"}}, "default": {"flask": {"version": "==3.0.3"}}, "develop": {"black": {"version": "==24.8.0"}, "local": {"path": "."}}}`,
			[]string{"black 24.8.0", "flask 3.0.3"},
		},
		{"a lockfile that cannot be read holds nothing", "package-lock.json", `{"packages": `, []string{}},
		{"a file that is not a lockfile", "go.sum", "github.com/spf13/cobra v1.8.0 h1:x\n", []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := locked(c.file, c.data); !slices.Equal(got, c.want) {
				t.Errorf("Locked(%s)\n got %q\nwant %q", c.file, got, c.want)
			}
		})
	}
}

func TestChangedFindsNewNamesAndNewVersions(t *testing.T) {
	lock := func(react string) string {
		return `{"lockfileVersion": 3, "packages": {"node_modules/react": {"version": "` + react + `"}, "node_modules/left-pad": {"version": "1.3.0"}, "node_modules/transitive": {"version": "` + react + `"}}}`
	}
	const manifest = `{"dependencies": {"react": "^18.2.0", "left-pad": "^1.3.0"}}`
	cases := []struct {
		name             string
		oldManifest, now string
		oldLock, nowLock string
		want             []string
	}{
		{"nothing changed", manifest, manifest, lock("18.2.0"), lock("18.2.0"), []string{}},
		{
			"the lockfile alone moves a dependency to a new version: that version is checked, exactly",
			manifest, manifest, lock("18.2.0"), lock("18.3.1"),
			[]string{"npm react 18.3.1"},
		},
		{
			"a new range in the manifest that resolves to the same version changes nothing",
			manifest, `{"dependencies": {"react": "^18.0.0", "left-pad": "^1.3.0"}}`, lock("18.2.0"), lock("18.2.0"),
			[]string{},
		},
		{
			"a new name: the version the lockfile holds",
			`{"dependencies": {"react": "^18.2.0"}}`, manifest, lock("18.2.0"), lock("18.2.0"),
			[]string{"npm left-pad 1.3.0"},
		},
		{
			"a new manifest: everything in it",
			"", manifest, "", lock("18.3.1"),
			[]string{"npm left-pad 1.3.0", "npm react 18.3.1"},
		},
		{
			"only what the manifest names: a package it does not name is not looked at",
			manifest, manifest, lock("18.2.0"), `{"lockfileVersion": 3, "packages": {"node_modules/react": {"version": "18.2.0"}, "node_modules/left-pad": {"version": "1.3.0"}, "node_modules/transitive": {"version": "9.9.9"}}}`,
			[]string{},
		},
		{
			"no lockfile: the range the manifest names, when it changes",
			manifest, `{"dependencies": {"react": "^19.0.0", "left-pad": "^1.3.0"}}`, "", "",
			[]string{"npm react ^19.0.0"},
		},
		{
			"a lockfile that does not hold the name yet: the range the manifest names",
			`{"dependencies": {"react": "^18.2.0"}}`, `{"dependencies": {"react": "^18.2.0", "is-odd": "^3.0.1"}}`, lock("18.2.0"), lock("18.2.0"),
			[]string{"npm is-odd ^3.0.1"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := names(Changed("package.json",
				[]byte(c.oldManifest), []byte(c.now),
				Locked("package-lock.json", []byte(c.oldLock)), Locked("package-lock.json", []byte(c.nowLock))))
			if !slices.Equal(got, c.want) {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
}
