// Package manifest loads and validates quality.yaml, the optional file in
// which a repo declares what it owes.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	qualitykit "github.com/rkvalvaa/cct-claude-plugins"
)

// FileName is the manifest's name at the repo root.
const FileName = "quality.yaml"

// SchemaVersion is the manifest version this qk reads.
const SchemaVersion = 1

// Manifest holds the keys qk acts on. Rules add fields as they need them;
// the schema already validates every key.
type Manifest struct {
	Version  int               `yaml:"version"`
	Maturity string            `yaml:"maturity"`
	Modes    map[string]string `yaml:"modes"`
}

// Problem is one reason quality.yaml is invalid.
type Problem struct {
	Path    string // dotted key path; empty for whole-file problems
	Message string
}

func (p Problem) String() string {
	if p.Path == "" {
		return p.Message
	}
	return p.Path + ": " + p.Message
}

// Loaded is the result of reading quality.yaml.
type Loaded struct {
	Present  bool      // the file exists
	Err      error     // the file exists but could not be read: a tooling problem
	Manifest Manifest  // decoded only when there are no problems
	Problems []Problem // why the content is invalid, sorted
}

// Load reads quality.yaml from root.
func Load(root string) Loaded {
	data, err := os.ReadFile(filepath.Join(root, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return Loaded{}
	}
	if err != nil {
		return Loaded{Present: true, Err: err}
	}
	l := Loaded{Present: true}
	l.Problems, err = validate(data)
	if err != nil {
		return Loaded{Present: true, Err: err}
	}
	if len(l.Problems) == 0 {
		if err := yaml.Unmarshal(data, &l.Manifest); err != nil {
			l.Problems = []Problem{{Message: err.Error()}}
		}
	}
	return l
}

// validate returns content problems; the error is for qk's own failures.
func validate(data []byte) ([]Problem, error) {
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []Problem{{Message: err.Error()}}, nil
	}
	if doc == nil {
		return []Problem{{Message: "quality.yaml is empty; it needs at least `version: 1`"}}, nil
	}
	if top, ok := doc.(map[string]any); ok {
		// A newer version may have keys this schema does not know: report only the version.
		if v, ok := top["version"].(int); ok && v > SchemaVersion {
			return []Problem{{Path: "version", Message: fmt.Sprintf(
				"version %d is not supported by this qk, which reads version %d; upgrade qk", v, SchemaVersion)}}, nil
		}
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return []Problem{{Message: err.Error()}}, nil
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
	if err != nil {
		return nil, err
	}
	sch, err := compiledSchema()
	if err != nil {
		return nil, err
	}
	var verr *jsonschema.ValidationError
	if err := sch.Validate(inst); !errors.As(err, &verr) {
		return nil, err // nil when valid
	}
	var problems []Problem
	collect(verr, &problems)
	slices.SortFunc(problems, func(a, b Problem) int {
		return strings.Compare(a.String(), b.String())
	})
	return problems, nil
}

var printer = message.NewPrinter(language.English)

// collect keeps leaf errors only; inner nodes just say "failed validation".
func collect(e *jsonschema.ValidationError, out *[]Problem) {
	if len(e.Causes) == 0 {
		*out = append(*out, Problem{Path: keyPath(e.InstanceLocation), Message: e.ErrorKind.LocalizedString(printer)})
		return
	}
	for _, c := range e.Causes {
		collect(c, out)
	}
}

// keyPath turns ["docs","impact_map","0","paths"] into docs.impact_map[0].paths.
func keyPath(loc []string) string {
	if len(loc) == 0 {
		return "(top level)"
	}
	var b strings.Builder
	for i, seg := range loc {
		if _, err := strconv.Atoi(seg); err == nil {
			b.WriteString("[" + seg + "]")
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	return b.String()
}

var compiledSchema = sync.OnceValues(compileSchema)

func compileSchema() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(qualitykit.ManifestSchema))
	if err != nil {
		return nil, err
	}
	// An absolute URL: a relative one is resolved against the working directory,
	// and the compiler then tries to read it from disk.
	const url = "https://github.com/rkvalvaa/cct-claude-plugins/quality.schema.json"
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, doc); err != nil {
		return nil, err
	}
	return c.Compile(url)
}
