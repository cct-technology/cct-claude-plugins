// Package waivers reads and writes .quality/waivers.yaml: agreed, owned,
// time-limited exceptions to a rule.
package waivers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/rkvalvaa/cct-claude-plugins/internal/glob"
)

// FileName is the waivers file, relative to the repo root.
const FileName = ".quality/waivers.yaml"

// NeverWaived is the rule that checks the waivers themselves.
const NeverWaived = "QK100"

const dateLayout = "2006-01-02"

// Waiver is one agreed exception.
type Waiver struct {
	Rule    string   `yaml:"rule"`
	Paths   []string `yaml:"paths,omitempty"` // globs; empty waives every finding of the rule
	Reason  string   `yaml:"reason"`
	Owner   string   `yaml:"owner"`
	Expires string   `yaml:"expires"` // YYYY-MM-DD, valid through that day
}

// Loaded is the result of reading the waivers file.
type Loaded struct {
	Present  bool     // the file exists
	Err      error    // the file exists but could not be read: a tooling problem
	Waivers  []Waiver // empty when the file cannot be parsed
	Problems []string // why the file cannot be parsed
}

// Load reads the waivers file under root.
func Load(root string) Loaded {
	data, err := os.ReadFile(path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return Loaded{}
	}
	if err != nil {
		return Loaded{Present: true, Err: err}
	}
	var file struct {
		Waivers []Waiver `yaml:"waivers"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return Loaded{Present: true, Problems: []string{err.Error()}}
	}
	return Loaded{Present: true, Waivers: file.Waivers}
}

// Problems says why w is invalid on the given day. A waiver with problems never applies.
func (w Waiver) Problems(today time.Time) []string {
	var out []string
	if strings.TrimSpace(w.Rule) == "" {
		out = append(out, "no rule")
	}
	if w.Rule == NeverWaived {
		out = append(out, NeverWaived+" cannot be waived")
	}
	if strings.TrimSpace(w.Reason) == "" {
		out = append(out, "no reason")
	}
	if strings.TrimSpace(w.Owner) == "" {
		out = append(out, "no owner")
	}
	if _, err := time.Parse(dateLayout, w.Expires); err != nil {
		out = append(out, "expires must be a date like 2026-12-31")
	} else if w.Expires < today.Format(dateLayout) {
		out = append(out, "expired on "+w.Expires)
	}
	return out
}

// Find returns the waiver covering a finding of rule on file, or nil.
func (l Loaded) Find(rule, file string, today time.Time) *Waiver {
	for i, w := range l.Waivers {
		if w.Rule == rule && len(w.Problems(today)) == 0 && w.covers(file) {
			return &l.Waivers[i]
		}
	}
	return nil
}

func (w Waiver) covers(file string) bool {
	if len(w.Paths) == 0 {
		return true
	}
	if file == "" {
		return false
	}
	for _, p := range w.Paths {
		if glob.Match(p, file) {
			return true
		}
	}
	return false
}

// ExpiringWithin returns the valid waivers that end within the given number of days.
func (l Loaded) ExpiringWithin(days int, today time.Time) []Waiver {
	last := today.AddDate(0, 0, days).Format(dateLayout)
	var out []Waiver
	for _, w := range l.Waivers {
		if len(w.Problems(today)) == 0 && w.Expires <= last {
			out = append(out, w)
		}
	}
	return out
}

// Add appends w to the waivers file under root, creating it if needed and
// keeping existing entries and comments.
func Add(root string, w Waiver) error {
	var doc yaml.Node
	data, err := os.ReadFile(path(root))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w", FileName, err)
	}
	if doc.Kind == 0 { // no file, or an empty one
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	top := doc.Content[0]
	if top.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: expected a mapping with a waivers list", FileName)
	}
	list := waiversNode(top)
	if list.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s: waivers must be a list", FileName)
	}

	var entry yaml.Node
	if err := entry.Encode(w); err != nil {
		return err
	}
	list.Style = 0 // block style, even if the file had `waivers: []`
	list.Content = append(list.Content, &entry)

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path(root)), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path(root), out.Bytes(), 0o644)
}

// waiversNode returns the value of the waivers key, adding the key if missing.
func waiversNode(top *yaml.Node) *yaml.Node {
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "waivers" {
			return top.Content[i+1]
		}
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "waivers"}
	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	top.Content = append(top.Content, key, list)
	return list
}

func path(root string) string {
	return filepath.Join(root, filepath.FromSlash(FileName))
}
