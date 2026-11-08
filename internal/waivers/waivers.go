// Package waivers reads and writes .quality/waivers.yaml: agreed, owned,
// time-limited exceptions to a rule.
package waivers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/rkvalvaa/cct-claude-plugins/internal/glob"
)

// FileName is the waivers file, relative to the repo root.
const FileName = ".quality/waivers.yaml"

// LogName is the committed, append-only record of approved exceptions.
const LogName = ".quality/exceptions-log.jsonl"

const actionWaiverAdd = "waiver.add"

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
	Problems []string // why the file or the log cannot be parsed
	Log      []LogEntry
}

// Approval says who approved an exception and when.
type Approval struct {
	By       string // the person who approved it
	Identity string // git identity of whoever recorded it
	Time     time.Time
}

// LogEntry is one line of the log. A waiver counts only with a matching entry.
type LogEntry struct {
	Time       string   `json:"time"`
	Action     string   `json:"action"`
	Identity   string   `json:"identity,omitempty"`
	ApprovedBy string   `json:"approved_by"`
	Rule       string   `json:"rule"`
	Paths      []string `json:"paths,omitempty"`
	Reason     string   `json:"reason"`
	Owner      string   `json:"owner"`
	Expires    string   `json:"expires"`
}

// NewLogEntry records that a approved w.
func NewLogEntry(w Waiver, a Approval) LogEntry {
	return LogEntry{
		Time: a.Time.UTC().Format(time.RFC3339), Action: actionWaiverAdd,
		Identity: a.Identity, ApprovedBy: a.By,
		Rule: w.Rule, Paths: w.Paths, Reason: w.Reason, Owner: w.Owner, Expires: w.Expires,
	}
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
	log, problems, err := readLog(root)
	if err != nil {
		return Loaded{Present: true, Err: err}
	}
	return Loaded{Present: true, Waivers: file.Waivers, Log: log, Problems: problems}
}

func readLog(root string) ([]LogEntry, []string, error) {
	data, err := os.ReadFile(logPath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var log []LogEntry
	var problems []string
	lines := bufio.NewScanner(bytes.NewReader(data))
	lines.Buffer(nil, 1<<20)
	for n := 1; lines.Scan(); n++ {
		if strings.TrimSpace(lines.Text()) == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal(lines.Bytes(), &e); err != nil {
			problems = append(problems, fmt.Sprintf("%s line %d: %v", LogName, n, err))
			continue
		}
		log = append(log, e)
	}
	return log, problems, lines.Err()
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

// ProblemsOf says why w does not apply: its own problems, or no recorded approval.
func (l Loaded) ProblemsOf(w Waiver, today time.Time) []string {
	out := w.Problems(today)
	if !l.approved(w) {
		out = append(out, "no approval in "+LogName+"; record it with qk waiver add")
	}
	return out
}

// approved requires an entry matching every field, so editing a waiver after
// approval (a later date, wider paths) voids it.
func (l Loaded) approved(w Waiver) bool {
	return slices.ContainsFunc(l.Log, func(e LogEntry) bool {
		return e.Action == actionWaiverAdd && strings.TrimSpace(e.ApprovedBy) != "" &&
			e.Rule == w.Rule && e.Reason == w.Reason && e.Owner == w.Owner && e.Expires == w.Expires &&
			slices.Equal(e.Paths, w.Paths)
	})
}

// Find returns the waiver covering a finding of rule on file, or nil.
func (l Loaded) Find(rule, file string, today time.Time) *Waiver {
	for i, w := range l.Waivers {
		if w.Rule == rule && len(l.ProblemsOf(w, today)) == 0 && w.covers(file) {
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
		if len(l.ProblemsOf(w, today)) == 0 && w.Expires <= last {
			out = append(out, w)
		}
	}
	return out
}

// Add records the approval in the log and appends w to the waivers file under
// root, creating both if needed and keeping existing entries and comments.
func Add(root string, w Waiver, a Approval) error {
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
	// The log goes first: a stray log line grants nothing, while a waiver
	// written without its line simply does not apply.
	if err := appendLog(root, NewLogEntry(w, a)); err != nil {
		return err
	}
	return os.WriteFile(path(root), out.Bytes(), 0o644)
}

func appendLog(root string, e LogEntry) error {
	var line bytes.Buffer
	enc := json.NewEncoder(&line) // Encode ends the line
	enc.SetEscapeHTML(false)      // people read this file: keep <email> as written
	if err := enc.Encode(e); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath(root), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(line.Bytes()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
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

func logPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(LogName))
}
