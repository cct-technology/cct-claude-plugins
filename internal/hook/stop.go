package hook

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxBlocksInARow is how many times in a row Stop sends Claude back to fix
// enforced findings. The next stop is allowed and a person takes over.
const maxBlocksInARow = 2

const handover = "Quality Kit: unresolved findings, handing over to a human."

// StopCheck is what qk check found, as the Stop hook needs it.
type StopCheck struct {
	Blocking bool   // an enforced check failed
	Findings string // the report in --format claude
	Notify   bool   // nothing blocks, but a person should see the report
}

// StopOutput is a Stop hook's answer. Reason goes to Claude; SystemMessage to the person.
type StopOutput struct {
	Decision      string `json:"decision,omitempty"`
	Reason        string `json:"reason,omitempty"`
	SystemMessage string `json:"systemMessage,omitempty"`
}

// SessionStartOutput adds text to Claude's context when a session starts.
type SessionStartOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// SessionStart wraps the session summary for Claude's context.
func SessionStart(summary string) SessionStartOutput {
	var out SessionStartOutput
	out.HookSpecificOutput.HookEventName = "SessionStart"
	out.HookSpecificOutput.AdditionalContext = summary
	return out
}

// Stop decides whether Claude may stop. Enforced findings block, with the
// report as the reason, at most maxBlocksInARow times in a row; the next stop
// is allowed and the person is told. The count lives in dir, per session. A nil
// Output means nothing to say.
func Stop(input []byte, c StopCheck, dir string) (*StopOutput, error) {
	var in struct {
		SessionID      string `json:"session_id"`
		StopHookActive bool   `json:"stop_hook_active"` // Claude is already continuing because a Stop hook blocked
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("reading hook input: %w", err)
	}
	count := filepath.Join(dir, "stop-blocks", fmt.Sprintf("%x", sha256.Sum256([]byte(in.SessionID))))

	if !c.Blocking {
		os.Remove(count)
		if c.Notify {
			return &StopOutput{SystemMessage: c.Findings}, nil
		}
		return nil, nil
	}

	blocks := 0
	if in.StopHookActive {
		data, _ := os.ReadFile(count)
		blocks, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	if blocks >= maxBlocksInARow {
		os.Remove(count)
		return &StopOutput{SystemMessage: handover + "\n" + c.Findings}, nil
	}
	// ponytail: a count that cannot be saved is lost, and Claude Code's own
	// limit on Stop-hook continuations ends the loop instead.
	_ = os.MkdirAll(filepath.Dir(count), 0o755)
	_ = os.WriteFile(count, []byte(strconv.Itoa(blocks+1)), 0o644)
	return &StopOutput{Decision: "block", Reason: c.Findings}, nil
}
