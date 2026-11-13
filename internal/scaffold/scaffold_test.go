package scaffold

import (
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
	"github.com/rkvalvaa/cct-claude-plugins/internal/manifest"
	"github.com/rkvalvaa/cct-claude-plugins/internal/rules"
)

const onGitHub = "https://github.com/acme/app.git"

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// gitRepo makes an empty git repository whose `origin` is remote ("" adds no
// remote), holding files, isolated from the user's git config.
func gitRepo(t *testing.T, remote string, files map[string]string) string {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if remote != "" {
		git(t, dir, "remote", "add", "origin", remote)
	}
	put(t, dir, files)
	return dir
}

func put(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// runInit runs Init with the answers the CLI gives by default, and returns what it printed.
func runInit(t *testing.T, root string, o Options) string {
	t.Helper()
	if o.DataClass == "" {
		o.DataClass = "internal"
	}
	if o.Maturity == "" {
		o.Maturity = "bronze"
	}
	if o.Version == "" {
		o.Version = "1.2.3"
	}
	var out strings.Builder
	if err := Init(&out, root, o); err != nil {
		t.Fatalf("Init: %v\n%s", err, out.String())
	}
	return out.String()
}

// tree returns every file under root except git's own, by slash-separated path.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

var coreFiles = []string{
	".gitattributes",
	"docs/explanation/README.md",
	"docs/how-to/README.md",
	"docs/reference/README.md",
	"docs/security/threat-models/README.md",
	"docs/testing/uat/README.md",
	"docs/tutorials/README.md",
	"quality.yaml",
}

var gitHubFiles = []string{".github/CODEOWNERS", ".github/workflows/quality-kit.yml"}

func TestInitWritesTheSkeletonInAnEmptyGitHubRepo(t *testing.T) {
	root := gitRepo(t, onGitHub, nil)

	out := runInit(t, root, Options{})

	files := tree(t, root)
	want := slices.Sorted(slices.Values(slices.Concat(coreFiles, gitHubFiles)))
	if got := slices.Sorted(maps.Keys(files)); !slices.Equal(got, want) {
		t.Fatalf("files = %v\nwant    %v", got, want)
	}
	for _, f := range want {
		if !strings.Contains(out, "wrote "+f+"\n") {
			t.Errorf("output does not report `wrote %s`:\n%s", f, out)
		}
		if strings.Contains(files[f], "[[") {
			t.Errorf("%s still holds template text:\n%s", f, files[f])
		}
	}
	for _, owned := range []string{"/.quality/", "/quality.yaml"} {
		if !strings.Contains(files[".github/CODEOWNERS"], owned) {
			t.Errorf("CODEOWNERS does not name %s:\n%s", owned, files[".github/CODEOWNERS"])
		}
	}
	// Two branches that each record an exception must merge without a conflict.
	if attr := git(t, root, "check-attr", "merge", ".quality/exceptions-log.jsonl"); !strings.Contains(attr, "merge: union") {
		t.Errorf("git check-attr = %q, want merge: union", attr)
	}
	if !strings.Contains(out, "Host: GitHub") || !strings.Contains(out, "Require review from Code Owners") {
		t.Errorf("output does not name the host and its review setting:\n%s", out)
	}
}

func TestInitWritesAManifestThatPassesQK001AndHoldsTheAnswers(t *testing.T) {
	root := gitRepo(t, onGitHub, nil)
	replaces := []string{`Old "CRM": v2`, "Excel # sheets"}

	runInit(t, root, Options{DataClass: "client", Maturity: "gold", Replaces: replaces})

	loaded := manifest.Load(root)
	at := slices.IndexFunc(rules.All(), func(r check.Rule) bool { return r.ID == "QK001" })
	if res := rules.All()[at].Run(check.Context{Root: root, Manifest: loaded}); res.Status != check.Pass {
		t.Fatalf("QK001 = %+v, want pass", res)
	}
	var m struct {
		Service   string
		DataClass string `yaml:"data_class"`
		Maturity  string
		Docs      struct{ Diataxis bool }
		Security  struct {
			ThreatModels string `yaml:"threat_models"`
			SpecsDir     string `yaml:"specs_dir"`
		}
		Testing struct {
			UATDir   string `yaml:"uat_dir"`
			Replaces []string
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "quality.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Service != filepath.Base(root) || m.DataClass != "client" || m.Maturity != "gold" || !slices.Equal(m.Testing.Replaces, replaces) {
		t.Errorf("manifest = %+v, want the folder name as service and the answers given", m)
	}
	// Every folder the manifest names is one init made, except the specs folder: its first spec makes it.
	if !m.Docs.Diataxis || m.Security.ThreatModels != "docs/security/threat-models" || m.Security.SpecsDir != "docs/specs" || m.Testing.UATDir != "docs/testing/uat" {
		t.Errorf("manifest = %+v, want the folders init writes", m)
	}
}

func TestInitLeavesOutReplacesWhenNothingIsReplaced(t *testing.T) {
	root := gitRepo(t, onGitHub, nil)

	runInit(t, root, Options{})

	if raw := tree(t, root)["quality.yaml"]; strings.Contains(raw, "replaces") {
		t.Errorf("quality.yaml names replaced tools though none were given:\n%s", raw)
	}
	if loaded := manifest.Load(root); !loaded.Present || len(loaded.Problems) > 0 || loaded.Err != nil {
		t.Errorf("manifest = %+v, want valid", loaded)
	}
}

func TestInitLeavesExistingFilesAlone(t *testing.T) {
	existing := map[string]string{
		"quality.yaml":            "version: 1\nmaturity: silver\n",
		"docs/how-to/README.md":   "# Our guides\n",
		"docs/how-to/deploy.md":   "# Deploy\n",
		"docs/reference/api.md":   "# API\n",
		".github/workflows/x.yml": "name: x\n",
	}
	root := gitRepo(t, onGitHub, existing)

	out := runInit(t, root, Options{Maturity: "gold"})

	files := tree(t, root)
	for name, content := range existing {
		if files[name] != content {
			t.Errorf("%s was changed to:\n%s", name, files[name])
		}
	}
	for _, f := range []string{"quality.yaml", "docs/how-to/README.md"} {
		if !strings.Contains(out, "skipped "+f+": it exists\n") {
			t.Errorf("output does not report `skipped %s`:\n%s", f, out)
		}
	}
	// What was missing is still written.
	for _, f := range []string{"docs/reference/README.md", "docs/tutorials/README.md", ".github/CODEOWNERS"} {
		if _, ok := files[f]; !ok || !strings.Contains(out, "wrote "+f+"\n") {
			t.Errorf("%s was not written and reported:\n%s", f, out)
		}
	}
}

func TestInitTwiceChangesNothing(t *testing.T) {
	root := gitRepo(t, onGitHub, map[string]string{".gitattributes": "* text=auto\n"})
	runInit(t, root, Options{})
	before := tree(t, root)

	out := runInit(t, root, Options{Maturity: "gold", Replaces: []string{"Excel"}})

	if after := tree(t, root); !maps.Equal(before, after) {
		t.Errorf("the second run changed files:\nbefore %v\nafter  %v", before, after)
	}
	if strings.Contains(out, "wrote ") || strings.Contains(out, "changed ") {
		t.Errorf("the second run reports a change:\n%s", out)
	}
	for f := range before {
		if !strings.Contains(out, "skipped "+f+": ") {
			t.Errorf("the second run does not report `skipped %s`:\n%s", f, out)
		}
	}
}

func TestInitAddsItsLineToAnExistingGitattributes(t *testing.T) {
	root := gitRepo(t, onGitHub, map[string]string{".gitattributes": "* text=auto eol=lf"}) // no newline at the end

	out := runInit(t, root, Options{})

	got := tree(t, root)[".gitattributes"]
	if !strings.HasPrefix(got, "* text=auto eol=lf\n") || !strings.HasSuffix(got, ".quality/exceptions-log.jsonl merge=union\n") {
		t.Errorf(".gitattributes = %q, want the old line kept and the union merge added", got)
	}
	if !strings.Contains(out, "changed .gitattributes: ") {
		t.Errorf("output does not report the change:\n%s", out)
	}
}

// GitHub reads .github/CODEOWNERS before the others, so a stub there would
// take every owner away from a CODEOWNERS the repository already has.
func TestInitWritesNoCodeownersWhereTheHostAlreadyReadsOne(t *testing.T) {
	root := gitRepo(t, onGitHub, map[string]string{"CODEOWNERS": "* @acme/everyone\n"})

	out := runInit(t, root, Options{})

	if _, ok := tree(t, root)[".github/CODEOWNERS"]; ok {
		t.Error(".github/CODEOWNERS was written beside the CODEOWNERS at the root")
	}
	const hint = "give `/.quality/` and `/quality.yaml` an owner in it"
	if !strings.Contains(out, "skipped .github/CODEOWNERS: `CODEOWNERS` exists; "+hint) {
		t.Errorf("output does not say which lines to add to the existing CODEOWNERS:\n%s", out)
	}

	// Once the existing file names the paths, there is nothing left to say.
	put(t, root, map[string]string{"CODEOWNERS": "* @acme/everyone\n/.quality/ @acme/leads\n/quality.yaml @acme/leads\n"})
	if out := runInit(t, root, Options{}); strings.Contains(out, hint) || !strings.Contains(out, "skipped .github/CODEOWNERS: `CODEOWNERS` exists\n") {
		t.Errorf("output still asks for lines the CODEOWNERS has:\n%s", out)
	}
}

func TestInitWritesOnlyCoreFilesForAHostWithoutAnAddOn(t *testing.T) {
	for name, tc := range map[string]struct{ remote, says string }{
		"gitlab":    {"git@gitlab.com:acme/app.git", "`gitlab.com`"},
		"bitbucket": {"https://ann:s3cret@bitbucket.org/acme/app.git", "`bitbucket.org`"},
		"no origin": {"", "no remote named `origin`"},
	} {
		t.Run(name, func(t *testing.T) {
			root := gitRepo(t, tc.remote, nil)

			out := runInit(t, root, Options{})

			if got := slices.Sorted(maps.Keys(tree(t, root))); !slices.Equal(got, coreFiles) {
				t.Errorf("files = %v\nwant    %v", got, coreFiles)
			}
			if !strings.Contains(out, tc.says) || strings.Contains(out, "s3cret") || strings.Contains(out, "Rulesets") {
				t.Errorf("output must say %q, keep the password out, and name no GitHub setting:\n%s", tc.says, out)
			}
		})
	}
}

func TestInitWorksOutsideGit(t *testing.T) {
	root := t.TempDir()
	put(t, root, map[string]string{"go.mod": "module example.com/app\n"})

	out := runInit(t, root, Options{})

	if got := slices.Sorted(maps.Keys(tree(t, root))); !slices.Equal(got, slices.Sorted(slices.Values(append([]string{"go.mod"}, coreFiles...)))) {
		t.Errorf("files = %v, want go.mod and the core files", got)
	}
	if !strings.Contains(out, "Stack: Go\n") {
		t.Errorf("output does not name the stack:\n%s", out)
	}
}

func TestInitNamesTheStack(t *testing.T) {
	root := gitRepo(t, onGitHub, map[string]string{
		"services/api/go.mod":    "module example.com/api\n",
		"web/package.json":       "{}\n",
		"tools/requirements.txt": "requests\n",
		"tools/pyproject.toml":   "[project]\n",
		// Other people's code, even when it is committed, is not this repository's stack.
		"services/api/vendor/x/Cargo.toml": "[package]\n",
		"web/node_modules/y/composer.json": "{}\n",
	})

	out := runInit(t, root, Options{})

	if !strings.Contains(out, "Stack: Go, Node.js, Python\n") {
		t.Errorf("output does not name each stack once:\n%s", out)
	}

	if out := runInit(t, gitRepo(t, onGitHub, nil), Options{}); !strings.Contains(out, "Stack: none found\n") {
		t.Errorf("an empty repository: output does not say no stack was found:\n%s", out)
	}
}

// The workflow runs the reusable workflow of the same release as this qk, so
// CI checks with the qk that wrote the manifest.
func TestInitPinsTheWorkflowToItsOwnRelease(t *testing.T) {
	for version, want := range map[string]string{
		"1.2.3": "rkvalvaa/cct-claude-plugins/.github/workflows/quality.yml@v1.2.3",
		"dev":   "rkvalvaa/cct-claude-plugins/.github/workflows/quality.yml@main",
	} {
		root := gitRepo(t, onGitHub, nil)
		runInit(t, root, Options{Version: version})

		var wf struct {
			On   string
			Jobs map[string]struct {
				Uses        string
				Permissions map[string]string
				Secrets     map[string]string
			}
		}
		raw := tree(t, root)[".github/workflows/quality-kit.yml"]
		if err := yaml.Unmarshal([]byte(raw), &wf); err != nil {
			t.Fatalf("%v\n%s", err, raw)
		}
		job := wf.Jobs["quality"]
		if job.Uses != want || wf.On != "pull_request" {
			t.Errorf("qk %s: workflow runs %q on %q, want %q on pull_request", version, job.Uses, wf.On, want)
		}
		// The comment needs pull-requests: write; the secret is passed on as GitHub writes it.
		if job.Permissions["pull-requests"] != "write" || job.Secrets["QK_RELEASE_TOKEN"] != "${{ secrets.QK_RELEASE_TOKEN }}" {
			t.Errorf("qk %s: job = %+v, want the permissions and the secret of the how-to", version, job)
		}
	}
}

func TestInitRejectsAnswersTheManifestDoesNotAllow(t *testing.T) {
	for name, o := range map[string]Options{
		"data class": {DataClass: "secret", Maturity: "bronze"},
		"maturity":   {DataClass: "internal", Maturity: "platinum"},
	} {
		root := gitRepo(t, onGitHub, nil)
		var out strings.Builder

		err := Init(&out, root, o)

		if err == nil || !strings.Contains(err.Error(), "want one of") {
			t.Errorf("%s: err = %v, want one that lists the allowed values", name, err)
		}
		if files := tree(t, root); len(files) > 0 {
			t.Errorf("%s: files were written before the answers were checked: %v", name, slices.Sorted(maps.Keys(files)))
		}
	}
}
