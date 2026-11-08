package waivers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var today = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func load(t *testing.T, fixture string) Loaded {
	t.Helper()
	return Load(filepath.Join("testdata", fixture))
}

// approved is a Loaded in which every waiver has its log entry.
func approved(ws ...Waiver) Loaded {
	l := Loaded{Present: true, Waivers: ws}
	for _, w := range ws {
		l.Log = append(l.Log, NewLogEntry(w, approval))
	}
	return l
}

var approval = Approval{By: "a person", Identity: "Test <test@example.com>", Time: today}

func TestLoadWithoutFile(t *testing.T) {
	l := Load(t.TempDir())

	if l.Present || l.Err != nil || len(l.Problems) != 0 || len(l.Waivers) != 0 {
		t.Errorf("Load = %+v", l)
	}
}

func TestLoadValidFileWithQuotedAndUnquotedDates(t *testing.T) {
	l := load(t, "valid")

	if !l.Present || l.Err != nil || len(l.Problems) != 0 || len(l.Waivers) != 2 || len(l.Log) != 2 {
		t.Fatalf("Load = %+v", l)
	}
	for _, w := range l.Waivers {
		if p := l.ProblemsOf(w, today); len(p) != 0 {
			t.Errorf("waiver %s has problems: %q", w.Rule, p)
		}
	}
	if got := l.Waivers[0]; got.Rule != "QK010" || got.Expires != "2099-12-31" || got.Paths[0] != "internal/legacy/**" {
		t.Errorf("waiver 0 = %+v", got)
	}
	if got := l.Waivers[1].Expires; got != "2099-06-30" {
		t.Errorf("quoted expires = %q", got)
	}
}

func TestLoadEmptyFileHasNoWaivers(t *testing.T) {
	l := load(t, "empty")

	if !l.Present || len(l.Problems) != 0 || len(l.Waivers) != 0 {
		t.Errorf("Load = %+v", l)
	}
}

func TestLoadReportsUnparseableFiles(t *testing.T) {
	tests := []struct{ fixture, want string }{
		{"bad-yaml", "did not find expected"},
		{"unknown-key", "reson"},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			l := load(t, tt.fixture)
			if len(l.Problems) != 1 || !strings.Contains(l.Problems[0], tt.want) || len(l.Waivers) != 0 {
				t.Errorf("Load = %+v, want one problem containing %q and no waivers", l, tt.want)
			}
		})
	}
}

func TestLoadReportsABadLogLine(t *testing.T) {
	l := load(t, "bad-log")

	if len(l.Problems) != 1 || !strings.Contains(l.Problems[0], "exceptions-log.jsonl line 1") {
		t.Errorf("Load = %+v", l)
	}
}

func TestProblemsOfRequiresAMatchingLogEntry(t *testing.T) {
	w := Waiver{Rule: "QK010", Paths: []string{"a/**"}, Reason: "r", Owner: "o", Expires: "2099-01-01"}
	entry := func(change func(*Waiver)) LogEntry {
		logged := w
		change(&logged)
		return NewLogEntry(logged, approval)
	}
	noApprover := NewLogEntry(w, Approval{By: "  ", Time: today})

	tests := []struct {
		name string
		log  []LogEntry
		want bool // true: "no approval" is reported
	}{
		{"logged", []LogEntry{entry(func(*Waiver) {})}, false},
		{"no log at all", nil, true},
		{"entry for another rule", []LogEntry{entry(func(w *Waiver) { w.Rule = "QK020" })}, true},
		{"reason edited after approval", []LogEntry{entry(func(w *Waiver) { w.Reason = "other" })}, true},
		{"date pushed out after approval", []LogEntry{entry(func(w *Waiver) { w.Expires = "2098-01-01" })}, true},
		{"owner changed after approval", []LogEntry{entry(func(w *Waiver) { w.Owner = "x" })}, true},
		{"paths widened after approval", []LogEntry{entry(func(w *Waiver) { w.Paths = nil })}, true},
		{"entry with no approver", []LogEntry{noApprover}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := Loaded{Present: true, Waivers: []Waiver{w}, Log: tt.log}
			got := strings.Join(l.ProblemsOf(w, today), "; ")
			if has := strings.Contains(got, "no approval in .quality/exceptions-log.jsonl"); has != tt.want {
				t.Errorf("ProblemsOf = %q, want no-approval = %v", got, tt.want)
			}
		})
	}
}

func TestLoadUnreadableFileIsAnError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".quality", "waivers.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	if l := Load(root); !l.Present || l.Err == nil {
		t.Errorf("Load = %+v, want a read error", l)
	}
}

func TestProblems(t *testing.T) {
	good := Waiver{Rule: "QK010", Reason: "r", Owner: "o", Expires: "2026-11-08"}
	with := func(change func(*Waiver)) Waiver { w := good; change(&w); return w }

	tests := []struct {
		name string
		w    Waiver
		want string // empty: no problems
	}{
		{"valid through its expiry day", good, ""},
		{"expired yesterday", with(func(w *Waiver) { w.Expires = "2026-11-07" }), "expired on 2026-11-07"},
		{"no owner", with(func(w *Waiver) { w.Owner = "" }), "no owner"},
		{"blank reason", with(func(w *Waiver) { w.Reason = "  " }), "no reason"},
		{"no rule", with(func(w *Waiver) { w.Rule = "" }), "no rule"},
		{"bad date", with(func(w *Waiver) { w.Expires = "next spring" }), "expires must be a date like 2026-12-31"},
		{"impossible date", with(func(w *Waiver) { w.Expires = "2026-02-31" }), "expires must be a date"},
		{"QK100 cannot be waived", with(func(w *Waiver) { w.Rule = "QK100" }), "QK100 cannot be waived"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(tt.w.Problems(today), "; ")
			if tt.want == "" && got != "" {
				t.Errorf("Problems = %q, want none", got)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("Problems = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFind(t *testing.T) {
	l := approved(
		Waiver{Rule: "QK010", Paths: []string{"internal/legacy/**", "web/app/(admin)/**"}, Reason: "r", Owner: "o", Expires: "2026-11-08"},
		Waiver{Rule: "QK110", Reason: "whole rule", Owner: "o", Expires: "2099-01-01"},
		Waiver{Rule: "QK020", Reason: "expired", Owner: "o", Expires: "2026-11-07"},
		Waiver{Rule: "QK030", Reason: "", Owner: "o", Expires: "2099-01-01"},
		Waiver{Rule: "QK100", Reason: "never applies", Owner: "o", Expires: "2099-01-01"},
	)
	l.Waivers = append(l.Waivers, Waiver{Rule: "QK060", Reason: "hand-written, never logged", Owner: "o", Expires: "2099-01-01"})

	tests := []struct {
		name, rule, file string
		want             bool
	}{
		{"path matches", "QK010", "internal/legacy/old.go", true},
		{"second path matches", "QK010", "web/app/(admin)/page.tsx", true},
		{"path does not match", "QK010", "internal/new/x.go", false},
		{"finding with no file and a path-scoped waiver", "QK010", "", false},
		{"other rule", "QK040", "internal/legacy/old.go", false},
		{"no paths waives the whole rule", "QK110", "anything/at/all", true},
		{"no paths waives a finding with no file", "QK110", "", true},
		{"expired waiver never applies", "QK020", "a.go", false},
		{"invalid waiver never applies", "QK030", "a.go", false},
		{"QK100 is never waived", "QK100", "a.go", false},
		{"waiver with no log entry never applies", "QK060", "a.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := l.Find(tt.rule, tt.file, today); (got != nil) != tt.want {
				t.Errorf("Find = %+v, want found = %v", got, tt.want)
			}
		})
	}
}

func TestExpiringWithin(t *testing.T) {
	l := approved(
		Waiver{Rule: "QK010", Reason: "r", Owner: "o", Expires: "2026-11-08"}, // today
		Waiver{Rule: "QK020", Reason: "r", Owner: "o", Expires: "2026-11-22"}, // exactly 14 days
		Waiver{Rule: "QK030", Reason: "r", Owner: "o", Expires: "2026-11-23"}, // 15 days
		Waiver{Rule: "QK040", Reason: "r", Owner: "o", Expires: "2026-11-07"}, // already expired
	)

	var rules []string
	for _, w := range l.ExpiringWithin(14, today) {
		rules = append(rules, w.Rule)
	}

	if got := strings.Join(rules, ","); got != "QK010,QK020" {
		t.Errorf("expiring = %s, want QK010,QK020", got)
	}
}

func TestAddCreatesTheFile(t *testing.T) {
	root := t.TempDir()
	w := Waiver{Rule: "QK010", Paths: []string{"a/**"}, Reason: "because: reasons", Owner: "rk", Expires: "2099-12-31"}

	if err := Add(root, w, approval); err != nil {
		t.Fatal(err)
	}

	l := Load(root)
	if len(l.Problems) != 0 || len(l.Waivers) != 1 || len(l.Log) != 1 {
		t.Fatalf("Load = %+v", l)
	}
	entry := l.Log[0]
	if entry.Action != "waiver.add" || entry.ApprovedBy != "a person" || entry.Identity != "Test <test@example.com>" ||
		entry.Time != "2026-11-08T12:00:00Z" || entry.Rule != "QK010" || entry.Expires != "2099-12-31" {
		t.Errorf("log entry = %+v", entry)
	}
	if p := l.ProblemsOf(l.Waivers[0], today); len(p) != 0 {
		t.Errorf("added waiver has problems: %q", p)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(LogName)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Test <test@example.com>") {
		t.Errorf("log is not human-readable: %s", raw)
	}
	got := l.Waivers[0]
	if got.Rule != "QK010" || got.Paths[0] != "a/**" || got.Reason != "because: reasons" || got.Owner != "rk" || got.Expires != "2099-12-31" {
		t.Errorf("waiver = %+v", got)
	}
	if p := got.Problems(today); len(p) != 0 {
		t.Errorf("round-tripped waiver has problems: %q", p)
	}
}

func writeWaivers(t *testing.T, root, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(FileName))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAddKeepsExistingWaiversAndComments(t *testing.T) {
	root := t.TempDir()
	writeWaivers(t, root, "# Agreed with the security team.\nwaivers:\n  - rule: QK110\n    reason: first\n    owner: a\n    expires: 2099-01-01\n")

	if err := Add(root, Waiver{Rule: "QK010", Reason: "second", Owner: "b", Expires: "2099-02-02"}, approval); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(FileName)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Agreed with the security team.") {
		t.Errorf("comment lost:\n%s", data)
	}
	l := Load(root)
	if len(l.Waivers) != 2 || l.Waivers[0].Reason != "first" || l.Waivers[1].Reason != "second" {
		t.Errorf("Load = %+v", l)
	}
}

func TestAddToFlowStyleEmptyList(t *testing.T) {
	root := t.TempDir()
	writeWaivers(t, root, "waivers: []\n")

	if err := Add(root, Waiver{Rule: "QK010", Reason: "r", Owner: "o", Expires: "2099-02-02"}, approval); err != nil {
		t.Fatal(err)
	}

	if l := Load(root); len(l.Problems) != 0 || len(l.Waivers) != 1 {
		t.Errorf("Load = %+v", l)
	}
}

func TestAddToEmptyFile(t *testing.T) {
	root := t.TempDir()
	writeWaivers(t, root, "")

	if err := Add(root, Waiver{Rule: "QK010", Reason: "r", Owner: "o", Expires: "2099-02-02"}, approval); err != nil {
		t.Fatal(err)
	}

	if l := Load(root); len(l.Problems) != 0 || len(l.Waivers) != 1 {
		t.Errorf("Load = %+v", l)
	}
}

func TestAddRefusesAFileItCannotParse(t *testing.T) {
	root := t.TempDir()
	const broken = "waivers:\n  - rule: QK010\n   reason: bad indent\n"
	writeWaivers(t, root, broken)

	if err := Add(root, Waiver{Rule: "QK010", Reason: "r", Owner: "o", Expires: "2099-02-02"}, approval); err == nil {
		t.Fatal("want an error")
	}

	data, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(FileName)))
	if string(data) != broken {
		t.Errorf("file changed to:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(LogName))); err == nil {
		t.Error("logged an approval for a waiver that was not written")
	}
}

func TestAddAppendsToTheLog(t *testing.T) {
	root := t.TempDir()
	first := Waiver{Rule: "QK010", Reason: "one", Owner: "o", Expires: "2099-02-02"}
	second := Waiver{Rule: "QK020", Reason: "two", Owner: "o", Expires: "2099-02-02"}

	if err := Add(root, first, approval); err != nil {
		t.Fatal(err)
	}
	if err := Add(root, second, approval); err != nil {
		t.Fatal(err)
	}

	l := Load(root)
	if len(l.Log) != 2 || l.Log[0].Rule != "QK010" || l.Log[1].Rule != "QK020" {
		t.Errorf("log = %+v", l.Log)
	}
}
