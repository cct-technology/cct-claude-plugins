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
		{"sed in place, flags joined", shell("Bash", `sed -ni '/modes/!p' quality.yaml`)},
		{"redirect to the manifest", shell("Bash", `cat new.yaml > quality.yaml`)},
		{"redirect after a read of it", shell("Bash", `grep -v modes quality.yaml > tmp && cat tmp > quality.yaml`)},
		{"remove", shell("Bash", `rm quality.yaml`)},
		{"move", shell("Bash", `mv quality.yaml old.yaml`)},
		{"copy over it", shell("Bash", `cp other.yaml quality.yaml`)},
		{"tee at the end of a pipe", shell("Bash", `cat new.yaml | tee .quality/waivers.yaml`)},
		{"a read piped into a command the hook does not know", shell("Bash", `echo quality.yaml | xargs rm`)},
		{"git puts back an old version", shell("Bash", `git checkout main -- quality.yaml`)},
		{"git writes its output there", shell("Bash", `git diff --output=quality.yaml`)},
		{"a program", shell("Bash", `python -c "open('quality.yaml','w').write('version: 1')"`)},
		{"a shell in a shell", shell("Bash", `sh -c 'cat quality.yaml'`)},
		{"a variable holds the name", shell("Bash", `f=quality.yaml; rm "$f"`)},
		{"a write inside a command substitution", shell("Bash", `echo "$(rm quality.yaml)"`)},
		{"a write inside a substitution that pipes", shell("Bash", `echo "$(cat new.yaml | tee quality.yaml)"`)},
		{"a write after a quoted substitution", shell("Bash", `echo "$(date)"; rm quality.yaml`)},
		{"a write in the background", shell("Bash", `cat notes.txt & rm quality.yaml`)},
		{"find that deletes", shell("Bash", `find . -name quality.yaml -delete`)},
		{"yq in place", shell("Bash", `yq -i '.maturity = "bronze"' quality.yaml`)},
		{"sort into it", shell("Bash", `sort -o quality.yaml quality.yaml`)},
		{"awk writes it", shell("Bash", `awk '{print > "quality.yaml"}' in.txt`)},
		{"truncate the log", shell("Bash", `truncate -s 0 .quality/exceptions-log.jsonl`)},
		{"qk writes a file in the folder", shell("Bash", `qk check --sarif .quality/report.sarif`)},
		// Commands that look like readers and can still write or run something. The hook does not vet their options, so they ask.
		{"sed, whose script can write and run commands", shell("Bash", `sed -n '1,5p' quality.yaml`)},
		{"awk, whose program can write and run commands", shell("Bash", `awk '{print}' quality.yaml`)},
		{"yq", shell("Bash", `yq '.maturity' quality.yaml`)},
		{"sort with the output flag joined to another", shell("Bash", `sort -uo quality.yaml other.yaml`)},
		{"rg runs a preprocessor", shell("Bash", `rg --pre ./rewrite.sh maturity quality.yaml`)},
		{"git with a config that names a program", shell("Bash", `git -c diff.external=./rewrite.sh diff quality.yaml`)},
		{"git with its exec path moved", shell("Bash", `git --exec-path=/tmp/x diff quality.yaml`)},
		{"git with an external diff", shell("Bash", `git diff --ext-diff quality.yaml`)},
		{"git grep opens a pager", shell("Bash", `git grep -Oless maturity -- quality.yaml`)},
		{"a variable in front names a program", shell("Bash", `GIT_EXTERNAL_DIFF=./rewrite.sh git diff quality.yaml`)},
		{"xxd writes back", shell("Bash", `xxd -r dump.hex quality.yaml`)},
		{"tree writes its output", shell("Bash", `tree -o quality.yaml .quality`)},
		{"less keeps a log", shell("Bash", `less -o quality.yaml notes.txt`)},
		{"PowerShell Set-Content", shell("PowerShell", `Set-Content quality.yaml 'version: 1'`)},
		{"PowerShell read piped into a write", shell("PowerShell", `Get-Content quality.yaml | Set-Content quality.yaml`)},
		{"PowerShell redirect", shell("PowerShell", `'x' > .quality\waivers.yaml`)},
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
		// A read changes nothing, so nobody is asked. A person asked to approve reads learns to say yes.
		{"read the manifest", shell("Bash", `cat quality.yaml`)},
		{"read it with another file", shell("Bash", `cat quality.yaml .github/CODEOWNERS`)},
		{"is it there", shell("Bash", `test -f quality.yaml && echo EXISTS`)},
		{"is it there, bracket form", shell("Bash", `[ -f quality.yaml ] && cat quality.yaml`)},
		{"a read, then qk init", shell("Bash", `ls -a; test -f quality.yaml && echo EXISTS; qk init --data-class internal --maturity silver`)},
		{"search it", shell("Bash", `grep -n maturity quality.yaml`)},
		{"search with a separator in the pattern", shell("Bash", `grep -n "silver;gold|bronze" quality.yaml`)},
		{"read and discard errors", shell("Bash", `head -5 quality.yaml 2>/dev/null`)},
		{"read with stderr joined", shell("Bash", `cat quality.yaml 2>&1`)},
		{"copy its content elsewhere", shell("Bash", `cat quality.yaml > /tmp/copy.yaml`)},
		{"read through a pipe", shell("Bash", `cat .quality/waivers.yaml | grep rule | head -20`)},
		{"list the folder", shell("Bash", `ls -la .quality`)},
		{"search with rg", shell("Bash", `rg -n maturity quality.yaml`)},
		{"find it", shell("Bash", `find . -name quality.yaml`)},
		{"compare it", shell("Bash", `diff quality.yaml other.yaml`)},
		{"name it in text", shell("Bash", `echo "see quality.yaml"`)},
		{"qk check, then a read", shell("Bash", `qk check; cat quality.yaml`)},
		{"git diff", shell("Bash", `git diff main -- quality.yaml`)},
		{"git diff against a merge base", shell("Bash", `git diff "$(git merge-base main HEAD)" -- quality.yaml`)},
		{"git log", shell("Bash", `git -C repo log -p -- .quality/`)},
		{"git records it", shell("Bash", `git add quality.yaml .quality/ && git commit -m "Set up quality.yaml"`)},
		{"PowerShell read", shell("PowerShell", `Get-Content quality.yaml`)},
		{"PowerShell read through a pipe", shell("PowerShell", `Get-Content .quality\waivers.yaml | Select-String rule`)},
		{"PowerShell is it there", shell("PowerShell", `Test-Path quality.yaml`)},
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
