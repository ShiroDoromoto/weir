package hook

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeCodeDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Request
		wantErr bool
	}{
		{
			name: "a Bash tool call carries cwd, tool and command",
			input: `{"session_id":"s1","hook_event_name":"PreToolUse","cwd":"/repo",
				"tool_name":"Bash","tool_input":{"command":"git commit -m x","description":"d"}}`,
			want: Request{Cwd: "/repo", Tool: "Bash", Command: "git commit -m x"},
		},
		{
			name:  "a tool that runs no command decodes with an empty command",
			input: `{"cwd":"/repo","tool_name":"Read","tool_input":{"file_path":"/repo/a.go"}}`,
			want:  Request{Cwd: "/repo", Tool: "Read"},
		},
		{
			name:    "broken JSON is not readable input",
			input:   `{"tool_name":"Bash",`,
			wantErr: true,
		},
		{
			name:    "empty input is not readable input",
			input:   ``,
			wantErr: true,
		},
		{
			name:    "JSON without a tool name is not a tool call",
			input:   `{"cwd":"/repo","tool_input":{"command":"git push"}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ClaudeCode{}.Decode(strings.NewReader(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Decode() = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode() error = %v, want none", err)
			}
			if got != tt.want {
				t.Errorf("Decode() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClaudeCodeWriteDenial(t *testing.T) {
	var out bytes.Buffer
	if err := (ClaudeCode{}).WriteDenial(&out, "止めました"); err != nil {
		t.Fatalf("WriteDenial() error = %v, want none", err)
	}

	var got struct {
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("WriteDenial() wrote %q, which is not JSON: %v", out.String(), err)
	}

	if got.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("hookEventName = %q, want %q", got.HookSpecificOutput.HookEventName, "PreToolUse")
	}
	if got.HookSpecificOutput.PermissionDecision != "deny" {
		t.Errorf("permissionDecision = %q, want %q", got.HookSpecificOutput.PermissionDecision, "deny")
	}
	if got.HookSpecificOutput.PermissionDecisionReason != "止めました" {
		t.Errorf("permissionDecisionReason = %q, want the reason it was given",
			got.HookSpecificOutput.PermissionDecisionReason)
	}
	// weir never approves: the agent's own permission check has to stay in the
	// path for everything weir lets through.
	if strings.Contains(out.String(), "allow") {
		t.Errorf("WriteDenial() wrote %q, which mentions allow", out.String())
	}
}
