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
		{"the same, with both streams piped", shell("Bash", `echo quality.yaml |& xargs rm`)},
		{"git puts back an old version", shell("Bash", `git checkout main -- quality.yaml`)},
		{"git writes its output there", shell("Bash", `git diff --output=quality.yaml`)},
		{"git writes its output there, option shortened", shell("Bash", `git diff --outp=quality.yaml`)},
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
		// Lines where a careless reading of the quotes hides a command the shell runs.
		{"an escaped quote inside double quotes", shell("Bash", `echo "a \"" ; rm quality.yaml ; echo "b"`)},
		{"PowerShell, where a backslash escapes nothing", shell("PowerShell", `Write-Output "a \" ; Remove-Item quality.yaml ; "b"`)},
		{"ANSI-C quotes", shell("Bash", `echo $'a\'' ; rm quality.yaml ; echo 'b'`)},
		{"a here-document with an apostrophe", shell("Bash", "cat <<EOF\nit's\nEOF\nrm quality.yaml; echo 'x'")},
		{"a comment with an apostrophe", shell("Bash", "echo ok # it's fine\nrm quality.yaml; echo 'x'")},
		{"backticks", shell("Bash", "echo `rm quality.yaml`")},
		{"PowerShell stop-parsing token", shell("PowerShell", `cmd --% /c "echo ' " & del quality.yaml`)},
		{"a hash that is no comment, after an expansion", shell("Bash", `cat quality.yaml; echo ${#x}; rm quality.yaml`)},
		{"a hash that is no comment, after a substitution", shell("Bash", `cat quality.yaml; echo $(true)#x; rm quality.yaml`)},
		{"a comment after a group, with an apostrophe", shell("Bash", "(true)#it's\nrm quality.yaml; echo 'x'")},
		{"PowerShell method call on a group", shell("PowerShell", `(Get-Item quality.yaml).Delete()`)},
		{"PowerShell member of a group", shell("PowerShell", `(Get-Item quality.yaml).psobject.Methods['Delete'].Invoke()`)},
		// What a substitution or a group prints becomes part of the command around it.
		{"a substitution names the file for rm", shell("Bash", `rm "$(echo quality.yaml)"`)},
		{"a substitution names the file for tee", shell("Bash", `cat new.yaml | tee $(echo quality.yaml)`)},
		{"a substitution names the redirect target", shell("Bash", `cat new.yaml > $(echo quality.yaml)`)},
		{"a group prints the name into xargs", shell("Bash", `(echo quality.yaml) | xargs rm`)},
		{"PowerShell group as an argument", shell("PowerShell", `Remove-Item (Get-Item quality.yaml)`)},
		{"PowerShell method call", shell("PowerShell", `[System.IO.File]::WriteAllText("quality.yaml", "version: 1")`)},
		// Redirects.
		{"redirect both streams", shell("Bash", `cat new.yaml >& quality.yaml`)},
		{"redirect both streams, no space", shell("Bash", `cat new.yaml >&quality.yaml`)},
		{"redirect to a variable", shell("Bash", `cat quality.yaml > $OUT`)},
		{"redirect to a pattern", shell("Bash", `grep -v modes quality.yaml > qual*.yaml`)},
		// A git option that takes the next word as its value.
		{"git option value that looks like a subcommand", shell("Bash", `git --git-dir diff checkout -- quality.yaml`)},
		{"git namespace", shell("Bash", `git --namespace log checkout quality.yaml`)},
		{"PowerShell typographic quotes end a string", shell("PowerShell", "Write-Output \"a” ; Remove-Item quality.yaml ; Write-Output “b\"")},
		{"PowerShell quote in the middle of a word", shell("PowerShell", `Write-Output a"b ; Remove-Item quality.yaml ; Write-Output c"d`)},
		// A command elsewhere on the line can change what a reader's name means.
		{"PowerShell alias turns a reader into a writer", shell("PowerShell", `Set-Alias cat Remove-Item; cat quality.yaml`)},
		{"PowerShell alias through the alias drive", shell("PowerShell", `Set-Item alias:cat Remove-Item; cat quality.yaml`)},
		{"Bash alias on an earlier line", shell("Bash", "shopt -s expand_aliases; alias cat=rm\ncat quality.yaml")},
		{"a search path set on the line", shell("Bash", `PATH=/tmp/tools; cat quality.yaml`)},
		{"a name remembered at another path", shell("Bash", `hash -p /tmp/tools/rm cat; cat quality.yaml`)},
		{"a sourced file", shell("Bash", `. ./helpers.sh; cat quality.yaml`)},
		{"a command name worked out when it runs", shell("Bash", `$(echo alias) cat=rm; cat quality.yaml`)},
		{"a function with a subshell for a body", shell("Bash", `cat() (git checkout .); cat quality.yaml`)},
		{"a quote left open", shell("Bash", `cat quality.yaml; echo "x`)},
		{"a quote inside a substitution", shell("Bash", `echo "$(echo "a"; rm quality.yaml)"; cat quality.yaml`)},
		{"a write inside a substitution beside a read", shell("Bash", `cat quality.yaml; echo $(rm quality.yaml)`)},
		{"a bracket that closes nothing", shell("Bash", `cat quality.yaml; echo a) rm quality.yaml`)},
		{"PowerShell group beside a read", shell("PowerShell", `(Import-Module .\tools.psm1); cat quality.yaml`)},
		{"a reader's name with a path in front", shell("Bash", `./cat quality.yaml`)},
		// A variable can carry the name from one command to the next.
		{"the last word of the command before", shell("Bash", `cat quality.yaml; rm $_`)},
		{"the last word, as a redirect target", shell("Bash", `cat quality.yaml; echo x > $_`)},
		{"the last word, through a substitution", shell("Bash", `cat quality.yaml; rm "$(echo $_)"`)},
		{"a variable set by read", shell("Bash", `read -r PATH < tools.txt; cat quality.yaml`)},
		{"a variable set by printf", shell("Bash", `printf -v PATH %s /tmp/tools; cat quality.yaml`)},
		{"a loop variable", shell("Bash", `for f in quality.yaml; do rm "$f"; done`)},
		{"a function, defined with a space before the brackets", shell("Bash", `cat () { git checkout .; }; cat quality.yaml`)},
		// The name can stand in text that another command runs.
		{"a here-document for a shell", shell("Bash", "sh <<'EOF'\nrm quality.yaml\nEOF")},
		{"a here-string for a shell", shell("Bash", `bash <<< "rm quality.yaml"`)},
		{"a write in a process substitution", shell("Bash", `cat new.yaml | tee >(cat > quality.yaml)`)},
		{"zsh's forced overwrite", shell("Bash", `echo x >! quality.yaml`)},
		// The name is there once the quotes, escapes and braces are worked out.
		{"a name spelled with braces", shell("Bash", `rm quality.{yaml,bak}`)},
		{"a name spelled with braces at the front", shell("Bash", `rm {quality,other}.yaml`)},
		{"a name split by quotes", shell("Bash", `rm q""uality.yaml`)},
		{"a name with an escape", shell("Bash", `rm quality\.yaml`)},
		{"a redirect target with a space in quotes", shell("Bash", `cat new.yaml > "my repo/quality.yaml"`)},
		{"PowerShell redirect target in quotes", shell("PowerShell", `'x' > "my repo\quality.yaml"`)},
		{"PowerShell option in quotes", shell("PowerShell", `git diff "--output=quality.yaml"`)},
		{"PowerShell function drive", shell("PowerShell", `Copy-Item function:Remove-Item function:cat; cat quality.yaml`)},
		// Reads the hook asks about all the same: only plain commands with words written out in full pass.
		{"a read beside a command the hook does not know", shell("Bash", `cat quality.yaml && make docs`)},
		{"a read inside a substitution", shell("Bash", `echo "$(cat quality.yaml)"`)},
		{"a read with a variable in the path", shell("Bash", `cat "$HOME/repo/quality.yaml"`)},
		{"a read beside a command that uses a variable", shell("Bash", `cat quality.yaml && ls $HOME`)},
		{"a read with a here-document elsewhere", shell("Bash", "cat quality.yaml; cat <<'EOF'\nit's fine\nEOF")},
		{"a read whose output is kept in a variable", shell("Bash", `m=$(cat quality.yaml)`)},
		{"PowerShell read in a group", shell("PowerShell", `(Get-Content quality.yaml) | Select-String rule`)},
		{"PowerShell read with a variable", shell("PowerShell", `Get-Content $HOME\repo\quality.yaml`)},
		// Reported by reviews of the versions that followed where the name could go.
		{"a builtin run through builtin", shell("Bash", `builtin alias cat=rm; cat quality.yaml`)},
		{"eval run through command", shell("Bash", `command eval "alias cat=rm"; cat quality.yaml`)},
		{"a zsh builtin that sets a variable", shell("Bash", `print -v PATH /tmp/tools; cat quality.yaml`)},
		{"a name in dollar quotes", shell("Bash", `rm $'quality.yaml'`)},
		{"a name behind a numbered escape", shell("Bash", `rm $'quality\x2eyaml'`)},
		{"a name behind an octal escape", shell("Bash", `rm $'\161uality.yaml'`)},
		// Windows and macOS find the file under these names too.
		{"a name in another case", shell("Bash", `rm QUALITY.YAML`)},
		{"PowerShell name in another case", shell("PowerShell", `Remove-Item QUALITY.YAML`)},
		{"PowerShell short name of the manifest", shell("PowerShell", `Remove-Item QUALIT~1.YAM`)},
		{"PowerShell output to the short name", shell("PowerShell", `Get-Content other.yaml > QUALIT~1.YAM`)},
		{"short name of the folder", shell("Bash", `rm QUALIT~1/waivers.yaml`)},
		{"Edit the manifest by its short name", file("Edit", `D:\repo\QUALIT~1.YAM`)},
		{"the folder with a dot after it", shell("Bash", `rm -r .quality./`)},
		{"PowerShell folder with a dot after it", shell("PowerShell", `Remove-Item .quality.\waivers.yaml`)},
		{"PowerShell output to a drive-relative path", shell("PowerShell", `Get-Content other.yaml > C:quality.yaml`)},
		{"PowerShell output to the name with a dot after it", shell("PowerShell", `Get-Content other.yaml > quality.yaml.`)},
		{"Edit the manifest in another case", file("Edit", `D:\repo\QUALITY.YAML`)},
		{"Write to a stream of the manifest", file("Write", `D:\repo\quality.yaml::$DATA`)},
		{"a name split by an empty substitution", shell("Bash", `rm quality$(true).yaml`)},
		{"a name in an extended pattern", shell("Bash", "shopt -s extglob\nrm @(quality.yaml)")},
		{"a zsh pattern that runs its text", shell("Bash", `ls *(e:'rm quality.yaml':)`)},
		{"a here-string for a group", shell("Bash", `( xargs rm ) <<< quality.yaml`)},
		{"a here-string with code for a block", shell("Bash", `{ sh; } <<< 'rm quality.yaml'`)},
		{"the last word, through a here-string", shell("Bash", `cat quality.yaml; xargs rm <<< $_`)},
		{"the last word, down a pipe", shell("Bash", `cat quality.yaml; echo $_ | xargs rm`)},
		{"test works out a subscript", shell("Bash", `test -v 'a[$(rm quality.yaml)]'`)},
		{"test compares numbers", shell("Bash", `[ 'a[$(rm quality.yaml)]' -eq 0 ]`)},
		{"printf stores a count in a variable", shell("Bash", `printf '%n' 'a[$(rm quality.yaml)]'`)},
		{"an escaped bracket that is more than a word", shell("Bash", `cat quality.yaml; echo \(x\) | xargs rm`)},
		{"double brackets compare numbers", shell("Bash", `[[ 'a[$(rm quality.yaml)]' -eq 0 ]]`)},
		{"PowerShell short name for Set-Alias", shell("PowerShell", `sal cat Remove-Item; cat quality.yaml`)},
		{"PowerShell carriage return between statements", shell("PowerShell", "cat quality.yaml\rRemove-Item quality.yaml")},
		{"PowerShell read beside another command", shell("PowerShell", `Get-Content quality.yaml; npm test`)},
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
		{"git log", shell("Bash", `git -C repo log -p -- .quality/`)},
		{"git records it", shell("Bash", `git add quality.yaml .quality/ && git commit -m "Set up quality.yaml"`)},
		{"PowerShell read", shell("PowerShell", `Get-Content quality.yaml`)},
		{"PowerShell read through a pipe", shell("PowerShell", `Get-Content .quality\waivers.yaml | Select-String rule`)},
		{"PowerShell is it there", shell("PowerShell", `Test-Path quality.yaml`)},
		{"PowerShell read, errors discarded", shell("PowerShell", `Get-Content quality.yaml 2>$null`)},
		{"a read with stderr sent to stderr", shell("Bash", `cat quality.yaml >&2`)},
		{"is it there, double brackets", shell("Bash", `[[ -f quality.yaml ]] && cat quality.yaml`)},
		{"change folder, then read", shell("Bash", `cd docs && cat ../quality.yaml`)},
		// A few commands may stand beside a read: qk, cd, which, type.
		{"a look around, as Claude does it", shell("Bash", `ls -a; cat README* 2>/dev/null | head -50; cat quality.yaml 2>/dev/null; which qk`)},
		{"is it there, then which qk", shell("Bash", `ls -a; test -f quality.yaml && echo EXISTS; which qk`)},
		{"list it, then which qk", shell("Bash", `ls -a; ls quality.yaml 2>&1; which qk`)},
		{"either or", shell("Bash", `test -f quality.yaml && echo yes || echo no`)},
		// Bash lines are parsed, so these are read for what they are.
		{"a read in a group", shell("Bash", `(cat quality.yaml) | head -3`)},
		{"a read with a comment", shell("Bash", `cat quality.yaml # the manifest`)},
		{"a read with an escaped quote", shell("Bash", `grep -n "say \"silver\"" quality.yaml`)},
		{"a read from standard input", shell("Bash", `wc -l < quality.yaml`)},
		{"a read in an if", shell("Bash", `if [ -f quality.yaml ]; then cat quality.yaml; else qk init; fi`)},
		{"a comment that names the file", shell("Bash", `ls docs # not quality.yaml`)},
		{"a diff of it against the merge base", shell("Bash", `git diff "$(git merge-base main HEAD)" -- quality.yaml`)},
		{"a read beside a find with grouped tests", shell("Bash", `cat quality.yaml; find . \( -iname 'openapi*' -o -iname 'swagger*' \) -not -path './vendor/*'`)},
		// Lines the skills ran in the evals.
		{"a look at the tags and the manifest", shell("Bash", `git describe --tags --abbrev=0; git tag; git log --oneline | head -20; ls -a; cat quality.yaml 2>/dev/null`)},
		{"the branch and the manifest", shell("Bash", `git branch --show-current; cat quality.yaml`)},
		{"a look around a repository", shell("Bash", `git ls-files; cat quality.yaml; cat README* 2>/dev/null | head -30; grep -rn "go:generate" --include=*.go . | head; ls Makefile 2>/dev/null; find . \( -iname 'openapi*' -o -iname 'swagger*' \) -not -path './node_modules/*' -not -path './vendor/*'; ls docs`)},
		{"a look around, then qk check", shell("Bash", `cat quality.yaml CODEOWNERS package.json; find src docs -type f; find . -path ./.git -prune -o \( -iname 'openapi*' -o -iname 'swagger*' \) -print 2>/dev/null | grep -v bashrc; qk check 2>&1 | grep -v bashrc`)},
		{"a check, a diff and a read", shell("Bash", `qk check --full --format claude; git diff "$(git merge-base main HEAD)" -- src/price.js; ls docs; cat quality.yaml`)},
		{"a look at the top folder", shell("Bash", `ls -R . 2>/dev/null | grep -v '^\./\.' | head -40; cat quality.yaml .github/CODEOWNERS; grep -n 'go:generate' *.go; qk check 2>&1`)},
		{"a read, then a diff against the merge base", shell("Bash", `cat quality.yaml; git ls-files; git diff "$(git merge-base main HEAD)" --stat`)},
		{"a read beside a subshell", shell("Bash", `(cd docs && ls); cat quality.yaml`)},
		{"a format in quotes after an equals sign", shell("Bash", `git log --format='%H %s' -- quality.yaml`)},
		{"git without a pager", shell("Bash", `git --no-pager diff -- quality.yaml`)},
		{"Edit a normal file", file("Edit", `/repo/internal/check/check.go`)},
		{"Edit the manifest template", file("Edit", `/repo/templates/repo/quality.yaml.tmpl`)},
		{"a tab in dollar quotes", shell("Bash", `grep $'\t' data.tsv`)},
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

// A read that shares a line with a command the hook does not know asks. Claude
// gets the reason when nobody approves, and must learn from it that the read
// itself is fine, or it gives the task up.
func TestPreToolUseSaysHowToReadWhenALineIsNotAPlainRead(t *testing.T) {
	const advice = "command of its own"
	tests := []struct {
		name, input string
		want        bool
	}{
		{"a read beside a command the hook does not know", shell("Bash", `make docs; cat quality.yaml`), true},
		{"the same in PowerShell", shell("PowerShell", `npm test; Get-Content quality.yaml`), true},
		{"a command that records an exception", shell("Bash", `qk waiver add QK010 --reason r`), false},
		{"an edit", file("Edit", `/repo/.quality/waivers.yaml`), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := PreToolUse([]byte(tt.input))
			if err != nil || out == nil {
				t.Fatalf("out = %v, err = %v, want ask", out, err)
			}
			if got := strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, advice); got != tt.want {
				t.Errorf("reason = %q, has the advice = %v, want %v", out.HookSpecificOutput.PermissionDecisionReason, got, tt.want)
			}
		})
	}
}

func TestPreToolUseMalformedInputIsAnError(t *testing.T) {
	if out, err := PreToolUse([]byte("not json")); err == nil || out != nil {
		t.Errorf("out = %+v, err = %v", out, err)
	}
}
