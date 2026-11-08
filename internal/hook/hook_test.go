package hook

import (
	"encoding/json"
	"strings"
	"testing"
)

func shell(tool, command string) string {
	b, _ := json.Marshal(map[string]any{
		"session_id": "s", "hook_event_name": "PreToolUse", "tool_name": tool,
		"tool_input": map[string]any{"command": command, "description": "d"},
	})
	return string(b)
}

func file(tool, path string) string {
	b, _ := json.Marshal(map[string]any{
		"session_id": "s", "hook_event_name": "PreToolUse", "tool_name": tool,
		"tool_input": map[string]any{"file_path": path, "content": "x"},
	})
	return string(b)
}

func TestPreToolUseAsksAPersonForExceptions(t *testing.T) {
	tests := []struct{ name, input string }{
		{"Bash waiver add", shell("Bash", `qk waiver add QK010 --reason r --owner o --expires 2099-01-01 --approved-by a`)},
		{"PowerShell waiver add", shell("PowerShell", `qk waiver add QK010 --reason r`)},
		{"PowerShell call operator", shell("PowerShell", `& qk.exe waiver add QK010`)},
		{"after a separator", shell("PowerShell", `cd D:\repo; qk waiver add QK010`)},
		{"after &&", shell("Bash", `cd repo && qk waiver add QK010`)},
		{"plugin shim path", shell("Bash", `"${CLAUDE_PLUGIN_ROOT}/bin/qk" waiver add QK010`)},
		{"go run", shell("Bash", `go run ./cmd/qk waiver add QK010`)},
		{"Windows upper-case binary", shell("PowerShell", `QK.EXE waiver add QK010`)},
		{"cd into the folder", shell("Bash", `cd .quality && rm waivers.yaml`)},
		{"docs-impact note", shell("Bash", `qk docs-impact none --reason "internal refactor"`)},
		{"shell write under .quality", shell("Bash", `echo x >> .quality/waivers.yaml`)},
		{"PowerShell write with backslash", shell("PowerShell", `Add-Content .quality\exceptions-log.jsonl 'x'`)},
		{"shell write to the manifest", shell("Bash", `sed -i 's/silver/bronze/' quality.yaml`)},
		{"Edit a waiver", file("Edit", `/repo/.quality/waivers.yaml`)},
		{"Write the log", file("Write", `/repo/.quality/exceptions-log.jsonl`)},
		{"Windows path", file("Edit", `D:\repo\.quality\waivers.yaml`)},
		{"Edit the manifest", file("Edit", `/repo/quality.yaml`)},
		{"Write the manifest, Windows", file("Write", `D:\repo\quality.yaml`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := PreToolUse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			if out == nil {
				t.Fatal("no decision, want ask")
			}
			if d := out.HookSpecificOutput; d.HookEventName != "PreToolUse" || d.PermissionDecision != "ask" || !strings.Contains(d.PermissionDecisionReason, "person must approve") || !strings.Contains(d.PermissionDecisionReason, "do not try another way") {
				t.Errorf("output = %+v", d)
			}
		})
	}
}

func TestPreToolUseStaysOutOfTheWay(t *testing.T) {
	tests := []struct{ name, input string }{
		{"qk check", shell("Bash", `qk check --changed --format claude`)},
		{"qk explain", shell("Bash", `qk explain QK100`)},
		{"tests", shell("Bash", `go test ./internal/waivers`)},
		{"git", shell("PowerShell", `git status`)},
		{"word that only resembles the folder", shell("Bash", `grep -r quality docs/`)},
		{"similar folder", shell("Bash", `ls .qualityish/x`)},
		{"similar file", shell("Bash", `cat my-quality.yaml.bak`)},
		{"Edit a normal file", file("Edit", `/repo/internal/check/check.go`)},
		{"Write under a similar folder", file("Write", `/repo/quality/waivers.yaml`)},
		{"Edit a similar file", file("Edit", `/repo/docs/not-quality.yaml`)},
		{"tool with neither", `{"tool_name":"Grep","tool_input":{"pattern":"x"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := PreToolUse([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			if out != nil {
				t.Errorf("decision = %+v, want none", out.HookSpecificOutput)
			}
		})
	}
}

func TestPreToolUseMalformedInputIsAnError(t *testing.T) {
	if out, err := PreToolUse([]byte("not json")); err == nil || out != nil {
		t.Errorf("out = %+v, err = %v", out, err)
	}
}
