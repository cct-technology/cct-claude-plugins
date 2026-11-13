package rules

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/rkvalvaa/cct-claude-plugins/internal/changes"
	"github.com/rkvalvaa/cct-claude-plugins/internal/check"
)

// The folders from the manifest example in docs/SPEC.md §3.
const (
	defaultSpecsDir     = "docs/specs"
	defaultThreatModels = "docs/security/threat-models"
)

func qk040() check.Rule {
	return check.Rule{
		ID:           "QK040",
		Check:        "security.threat-model",
		Title:        "A new spec has a threat model",
		Phase:        1,
		EnforcedFrom: check.Gold,
		Why: "Security thinking starts when a feature is specified, not after it ships. " +
			"Each Markdown file added under `security.specs_dir` in `quality.yaml` needs a threat model under `security.threat_models`, which is `" + defaultThreatModels + "` unless you name another folder. " +
			"A threat model belongs to a spec when it has the same file name, or when its frontmatter has `spec:` with the spec's path from the repository root. " +
			"Only new specs are checked: editing or moving a spec needs nothing. " +
			"Without a `quality.yaml`, QK040 checks `" + defaultSpecsDir + "` if that folder exists. With a `quality.yaml` that names no `specs_dir`, it checks nothing.",
		Fix: "Write a threat model for the spec and save it under `security.threat_models`, with the spec's file name or with `spec: <path of the spec>` in its frontmatter. If the spec needs none, ask a person to approve a waiver with `qk waiver add QK040`.",
		Run: runQK040,
	}
}

func runQK040(ctx check.Context) check.Result {
	specs, models := securityDirs(ctx)
	if specs == "" {
		return check.Result{Status: check.Pass}
	}
	if ctx.Changes == nil {
		return noChangeSet(ctx)
	}
	var added []string
	for _, f := range ctx.Changes.Files {
		// ponytail: a spec is a Markdown file, and one moved in from outside the
		// folder is a rename, not a new spec. Widen both if specs turn up otherwise.
		if f.Status == changes.Added && path.Ext(f.Path) == ".md" && under(specs, f.Path) && !under(models, f.Path) {
			added = append(added, f.Path)
		}
	}
	if len(added) == 0 {
		return check.Result{Status: check.Pass}
	}

	names, named := threatModels(ctx.Root, models)
	var findings []check.Finding
	for _, spec := range added {
		if names[path.Base(spec)] || named[spec] {
			continue
		}
		findings = append(findings, check.Finding{File: spec, Message: fmt.Sprintf(
			"new spec without a threat model: add `%s/%s`, or put `spec: %s` in the frontmatter of a threat model under `%s`",
			models, path.Base(spec), spec, models)})
	}
	if len(findings) > 0 {
		return check.Result{Status: check.Fail, Findings: findings}
	}
	return check.Result{Status: check.Pass}
}

// securityDirs returns the specs and threat model folders, slash-separated
// and relative to the root. specs is empty when there is nothing to check.
func securityDirs(ctx check.Context) (specs, models string) {
	sec := ctx.Manifest.Manifest.Security
	specs, models = sec.SpecsDir, sec.ThreatModels
	if !ctx.Manifest.Present {
		if info, err := os.Stat(filepath.Join(ctx.Root, filepath.FromSlash(defaultSpecsDir))); err == nil && info.IsDir() {
			specs = defaultSpecsDir
		}
	}
	if specs == "" {
		return "", ""
	}
	if models == "" {
		models = defaultThreatModels
	}
	return path.Clean(filepath.ToSlash(specs)), path.Clean(filepath.ToSlash(models))
}

func under(dir, file string) bool {
	return strings.HasPrefix(file, dir+"/")
}

// threatModels reads every file under dir: names holds their file names,
// named the specs their frontmatter names. A missing dir gives none.
func threatModels(root, dir string) (names, named map[string]bool) {
	names, named = map[string]bool{}, map[string]bool{}
	_ = filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // an unreadable entry is no threat model
		}
		names[d.Name()] = true
		if data, err := os.ReadFile(p); err == nil {
			if spec := frontmatterSpec(data); spec != "" {
				named[path.Clean(filepath.ToSlash(spec))] = true
			}
		}
		return nil
	})
	return names, named
}

// frontmatterSpec returns the `spec` key of a file's YAML frontmatter, or "".
func frontmatterSpec(data []byte) string {
	rest, ok := bytes.CutPrefix(data, []byte("---"))
	if !ok {
		return ""
	}
	front, _, ok := bytes.Cut(rest, []byte("\n---"))
	if !ok {
		return ""
	}
	var fm struct {
		Spec string `yaml:"spec"`
	}
	if yaml.Unmarshal(front, &fm) != nil {
		return ""
	}
	return fm.Spec
}
