// Package scaffold writes what a repository needs to start with Quality Kit:
// the manifest, the folders it names, and what the repository's git host needs.
// It is `qk init`. It never overwrites a file.
package scaffold

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	qualitykit "github.com/rkvalvaa/cct-claude-plugins"
	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/host"
	"github.com/rkvalvaa/cct-claude-plugins/internal/waivers"
)

// Options are the answers `qk init` is given.
type Options struct {
	DataClass string   // the most sensitive data the service handles
	Maturity  string   // sets which checks block
	Replaces  []string // the tools the system replaces
	Version   string   // this qk's version, "dev" when it is no release
}

// The values quality.yaml allows; a test runs QK001 on what Init writes.
var (
	DataClasses = []string{"internal", "client", "public"}
	Maturities  = []string{"bronze", "silver", "gold"}
)

// templates is where the files to write live in qualitykit.Templates: `repo`
// for every repository, and one folder for each host with an add-on, named
// after the host in lower case. A file ending in .tmpl is a text/template
// with [[ ]] as delimiters, and is written without that ending.
const templates = "plugins/quality-kit/templates"

// reviewSetting says, for each host with an add-on, what makes the host
// require a person's review of the exceptions before a merge. Only the host
// can require it (SPEC §1 rule 8).
var reviewSetting = map[string]string{
	"GitHub": "To make GitHub require a person's review of `.quality/` and `quality.yaml` before a merge: " +
		"give both an owner in CODEOWNERS, then add a ruleset for the default branch (Settings → Rules → Rulesets) " +
		"with \"Require a pull request before merging\" and \"Require review from Code Owners\". " +
		"A private repository on GitHub Free has no rulesets: agree on that review in the team.",
}

// unionMerge keeps the line of each branch that records an exception.
const unionMerge = waivers.LogName + " merge=union"

// stacks maps a file to the stack it means, in the order they are reported.
var stacks = []struct{ file, name string }{
	{"go.mod", "Go"},
	{"package.json", "Node.js"},
	{"composer.json", "PHP"},
	{"Cargo.toml", "Rust"},
	{"pyproject.toml", "Python"},
	{"Pipfile", "Python"},
	{"requirements.txt", "Python"},
}

// Init writes the skeleton into root and reports each file on w: `wrote`,
// `changed` or `skipped`. A file that exists is left as it is, so a second
// run writes only what is still missing.
func Init(w io.Writer, root string, o Options) error {
	for _, answer := range []struct {
		flag, got string
		allowed   []string
	}{{"--data-class", o.DataClass, DataClasses}, {"--maturity", o.Maturity, Maturities}} {
		if !slices.Contains(answer.allowed, answer.got) {
			return fmt.Errorf("unknown %s %q (want one of %s)", answer.flag, answer.got, strings.Join(answer.allowed, ", "))
		}
	}

	fmt.Fprintf(w, "Stack: %s\n", stack(root))
	hostName, known, hasOrigin := host.Of(root)
	setting := ""
	if known != nil {
		setting = reviewSetting[known.Name]
	}
	const coreOnly = "no CI workflow and no CODEOWNERS are written"
	switch {
	case setting != "":
		fmt.Fprintf(w, "Host: %s\n", known.Name)
	case hasOrigin:
		fmt.Fprintf(w, "Host: `%s`. Quality Kit has no add-on for it: %s, and it knows no setting there that requires review of `.quality/` and `quality.yaml` before a merge.\n", hostName, coreOnly)
	default:
		fmt.Fprintf(w, "Host: unknown, no remote named `origin`: %s. Run `qk init` again once `origin` is set.\n", coreOnly)
	}

	in := initRun{w: w, root: root}
	in.data.Options, in.data.Service, in.data.Ref = o, filepath.Base(root), "main"
	if o.Version != "dev" {
		in.data.Ref = "v" + o.Version
	}
	if err := in.writeTree("repo"); err != nil {
		return err
	}
	if err := in.mergeLogByUnion(); err != nil {
		return err
	}
	if setting == "" {
		return nil
	}
	in.codeowners = known.Codeowners
	if err := in.writeTree(strings.ToLower(known.Name)); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%s\n", setting)
	return nil
}

// stack names what the repository is built with, from the files git tracks or would add.
func stack(root string) string {
	files, err := changes.Tree(root)
	if err != nil { // outside git: the top folder
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			files = append(files, e.Name())
		}
	}
	var found []string
	for _, s := range stacks {
		if !slices.Contains(found, s.name) && slices.ContainsFunc(files, func(f string) bool {
			f = "/" + f
			return path.Base(f) == s.file && !strings.Contains(f, "/vendor/") && !strings.Contains(f, "/node_modules/")
		}) {
			found = append(found, s.name)
		}
	}
	if len(found) == 0 {
		return "none found"
	}
	return strings.Join(found, ", ")
}

type initRun struct {
	w    io.Writer
	root string
	data struct {
		Options
		Service string // the name of the repository's folder
		Ref     string // the tag of this qk's release, for the reusable workflow
	}
	codeowners []string // where the host reads CODEOWNERS; set while its files are written
}

// writeTree writes each file under dir of the templates to the same path under the root.
func (in *initRun) writeTree(dir string) error {
	tree, err := fs.Sub(qualitykit.Templates, templates+"/"+dir)
	if err != nil {
		return err
	}
	return fs.WalkDir(tree, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := fs.ReadFile(tree, name)
		if err != nil {
			return err
		}
		if written, ok := strings.CutSuffix(name, ".tmpl"); ok {
			name = written
			if content, err = in.render(name, content); err != nil {
				return err
			}
		}
		if slices.Contains(in.codeowners, name) {
			if skip, err := in.hasCodeowners(name); skip || err != nil {
				return err
			}
		}
		return in.write(name, content)
	})
}

func (in *initRun) render(name string, content []byte) ([]byte, error) {
	// JSON is YAML too, so a name with a quote or a colon in it stays one value.
	asJSON := func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	}
	tpl, err := template.New(name).Delims("[[", "]]").Funcs(template.FuncMap{"json": asJSON}).Parse(string(content))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = tpl.Execute(&out, in.data)
	return out.Bytes(), err
}

// write creates the file, or reports that it exists.
func (in *initRun) write(name string, content []byte) error {
	full := filepath.Join(in.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		fmt.Fprintf(in.w, "skipped %s: it exists\n", name)
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		fmt.Fprintf(in.w, "wrote %s\n", name)
	}
	return err
}

// hasCodeowners reports whether the host already reads a CODEOWNERS file, and
// says so. The host reads the first one it finds, so a stub in an earlier
// place would take every owner away from the file the repository has.
func (in *initRun) hasCodeowners(stub string) (bool, error) {
	for _, location := range in.codeowners {
		data, err := os.ReadFile(filepath.Join(in.root, filepath.FromSlash(location)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		what := "it exists"
		if location != stub {
			what = fmt.Sprintf("`%s` exists", location)
		}
		// ponytail: looks for the two names as text, not for a pattern that owns
		// them. Read the patterns as QK110 does if `*` lines make this noisy.
		if !bytes.Contains(data, []byte(".quality/")) || !bytes.Contains(data, []byte("quality.yaml")) {
			what += "; give `/.quality/` and `/quality.yaml` an owner in it, so a change to an exception needs a person's review"
		}
		fmt.Fprintf(in.w, "skipped %s: %s\n", stub, what)
		return true, nil
	}
	return false, nil
}

// mergeLogByUnion adds the union merge of the exceptions log to .gitattributes:
// two branches that each record an exception then merge without a conflict.
// It is the one file init adds to, because the repository often has one.
func (in *initRun) mergeLogByUnion() error {
	const name = ".gitattributes"
	full := filepath.Join(in.root, name)
	data, err := os.ReadFile(full)
	if errors.Is(err, fs.ErrNotExist) {
		return in.write(name, []byte("# Quality Kit: each branch that records an exception keeps its line.\n"+unionMerge+"\n"))
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) > 1 && f[0] == waivers.LogName && slices.Contains(f[1:], "merge=union") {
			fmt.Fprintf(in.w, "skipped %s: it has the line\n", name)
			return nil
		}
	}
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	if err := os.WriteFile(full, append(data, unionMerge+"\n"...), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(in.w, "changed %s: added `%s`\n", name, unionMerge)
	return nil
}
