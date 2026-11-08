// Package hook holds the decisions behind Quality Kit's Claude Code hooks.
package hook

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Output is a hook's JSON answer.
type Output struct {
	HookSpecificOutput Decision `json:"hookSpecificOutput"`
}

// Decision is a PreToolUse permission decision.
type Decision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

const askReason = "Quality Kit: this records or changes an exception to a quality check " +
	"(a waiver, a docs-impact note, or a check's mode). A person must approve it. " +
	"Approve only if you agree with the reason given. " +
	"If this is denied or no one can approve it, stop and tell the user; do not try another way."

var (
	// exceptionCommand: qk waiver add / qk docs-impact, however qk is invoked.
	exceptionCommand = regexp.MustCompile(`(^|[\s"'/\\&;|(])(?i:qk(\.exe)?)["']?\s+(waiver\s+add|docs-impact)\b`)
	// protectedInShell: a command that names .quality/ or quality.yaml.
	// ponytail: any mention asks, reads included; telling reads from writes in a shell line is guesswork.
	protectedInShell = regexp.MustCompile(`(^|[\s"'=/\\<>(:])(\.quality([/\\\s"';&|)]|$)|quality\.yaml\b)`)
)

// PreToolUse asks a person before Claude changes an exception: running the
// commands that record one, or touching .quality/ or quality.yaml (where modes
// can turn a check off). A nil Output means no opinion. This is best effort: it
// reads command text, and qk writes the log itself, so a missed match is caught
// only by review of .quality/ on the PR.
func PreToolUse(input []byte) (*Output, error) {
	var in struct {
		ToolInput struct {
			Command      string `json:"command"`
			FilePath     string `json:"file_path"`
			NotebookPath string `json:"notebook_path"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("reading hook input: %w", err)
	}
	t := in.ToolInput
	if exceptionCommand.MatchString(t.Command) || protectedInShell.MatchString(t.Command) ||
		protectedFile(t.FilePath) || protectedFile(t.NotebookPath) {
		return &Output{Decision{"PreToolUse", "ask", askReason}}, nil
	}
	return nil, nil
}

// ponytail: matches by name at any depth, so fixtures named quality.yaml ask too; compare against cwd if that gets noisy.
func protectedFile(p string) bool {
	if p == "" {
		return false
	}
	p = strings.ReplaceAll(p, `\`, "/")
	return path.Base(p) == "quality.yaml" || slices.Contains(strings.Split(path.Dir(p), "/"), ".quality")
}
