package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ClaudeCode reads and writes Claude Code's PreToolUse hook format, and is the
// only adapter weir ships: a format written for an agent we cannot run against
// would be a guess, and a wrong guess costs more than a late one.
type ClaudeCode struct{}

// preToolUse is the payload Claude Code writes to the hook's stdin. It carries
// more fields than these; weir reads only what it judges by.
type preToolUse struct {
	Cwd       string `json:"cwd"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// denial is the reply that stops a tool call. weir writes this or nothing at
// all — there is no shape here for letting something through.
type denial struct {
	HookSpecificOutput denialOutput `json:"hookSpecificOutput"`
}

type denialOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// Decode reads one PreToolUse payload from r.
func (ClaudeCode) Decode(r io.Reader) (Request, error) {
	var in preToolUse
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return Request{}, fmt.Errorf("PreToolUse の JSON を読めません: %w", err)
	}
	// Without a tool name there is no tool call here, whatever else the JSON
	// held — so this is not the payload weir was wired to read.
	if in.ToolName == "" {
		return Request{}, errors.New("PreToolUse の JSON に tool_name がありません")
	}
	return Request{
		Cwd:     in.Cwd,
		Tool:    in.ToolName,
		Command: in.ToolInput.Command,
	}, nil
}

// WriteDenial writes the PreToolUse reply that stops the call, carrying reason
// as the text Claude Code shows for it.
func (ClaudeCode) WriteDenial(w io.Writer, reason string) error {
	return json.NewEncoder(w).Encode(denial{
		HookSpecificOutput: denialOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		},
	})
}
