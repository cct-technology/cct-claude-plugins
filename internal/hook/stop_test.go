package hook

import (
	"encoding/json"
	"strings"
	"testing"
)

func stopInput(session string, active bool) []byte {
	b, _ := json.Marshal(map[string]any{
		"session_id": session, "hook_event_name": "Stop", "stop_hook_active": active,
	})
	return b
}

var failing = StopCheck{Blocking: true, Findings: "Quality Kit: 1 fail, 0 warn.\nQK010 fail: docs\n"}

func stop(t *testing.T, dir string, input []byte, c StopCheck) *StopOutput {
	t.Helper()
	out, err := Stop(input, c, dir)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func wantBlock(t *testing.T, out *StopOutput, step string) {
	t.Helper()
	if out == nil || out.Decision != "block" || out.Reason != failing.Findings || out.SystemMessage != "" {
		t.Fatalf("%s: got %+v, want a block with the findings as the reason", step, out)
	}
}

func wantHandover(t *testing.T, out *StopOutput, step string) {
	t.Helper()
	if out == nil || out.Decision != "" || !strings.HasPrefix(out.SystemMessage, handover) || !strings.Contains(out.SystemMessage, failing.Findings) {
		t.Fatalf("%s: got %+v, want the stop allowed with the hand-over message and the findings", step, out)
	}
}

func TestStopBlocksEnforcedFailures(t *testing.T) {
	wantBlock(t, stop(t, t.TempDir(), stopInput("s", false), failing), "first stop")
}

func TestStopShowsFindingsThatDoNotBlock(t *testing.T) {
	c := StopCheck{Notify: true, Findings: "Quality Kit: 0 fail, 1 warn.\n"}

	out := stop(t, t.TempDir(), stopInput("s", false), c)

	if out == nil || out.Decision != "" || out.SystemMessage != c.Findings {
		t.Errorf("got %+v, want the findings as a message and the stop allowed", out)
	}
}

func TestStopStaysQuietWithNothingToShow(t *testing.T) {
	if out := stop(t, t.TempDir(), stopInput("s", false), StopCheck{Findings: "Quality Kit: no findings.\n"}); out != nil {
		t.Errorf("got %+v, want no output", out)
	}
}

func TestStopAllowsTheThirdBlockInARow(t *testing.T) {
	dir := t.TempDir()

	wantBlock(t, stop(t, dir, stopInput("s", false), failing), "first stop")
	wantBlock(t, stop(t, dir, stopInput("s", true), failing), "second stop")
	wantHandover(t, stop(t, dir, stopInput("s", true), failing), "third stop")

	// The next turn starts again: the person has seen the hand-over.
	wantBlock(t, stop(t, dir, stopInput("s", false), failing), "first stop of the next turn")
}

func TestStopANewTurnStartsTheCountAgain(t *testing.T) {
	// The person interrupted while Claude was being sent back; the count is stale.
	dir := t.TempDir()
	wantBlock(t, stop(t, dir, stopInput("s", false), failing), "first stop")
	wantBlock(t, stop(t, dir, stopInput("s", true), failing), "second stop")

	wantBlock(t, stop(t, dir, stopInput("s", false), failing), "first stop of the next turn")
}

func TestStopCountsEachSessionSeparately(t *testing.T) {
	dir := t.TempDir()
	wantBlock(t, stop(t, dir, stopInput("a", false), failing), "a, first stop")
	wantBlock(t, stop(t, dir, stopInput("a", true), failing), "a, second stop")

	wantBlock(t, stop(t, dir, stopInput("b", true), failing), "b, while a has used its blocks")
	wantHandover(t, stop(t, dir, stopInput("a", true), failing), "a, third stop")
}

func TestStopCountsOnlyBlocksInARow(t *testing.T) {
	dir := t.TempDir()
	wantBlock(t, stop(t, dir, stopInput("s", false), failing), "first stop")
	wantBlock(t, stop(t, dir, stopInput("s", true), failing), "second stop")

	// Claude fixed the findings, so this stop is allowed and the run ends.
	if out := stop(t, dir, stopInput("s", true), StopCheck{}); out != nil {
		t.Fatalf("clean stop: got %+v, want no output", out)
	}

	wantBlock(t, stop(t, dir, stopInput("s", true), failing), "new failure after a clean stop")
}

func TestStopMalformedInputIsAnError(t *testing.T) {
	if out, err := Stop([]byte("not json"), failing, t.TempDir()); err == nil || out != nil {
		t.Errorf("out = %+v, err = %v", out, err)
	}
}
