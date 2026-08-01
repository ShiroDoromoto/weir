package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/weir/internal/version"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantCode   int
		wantStdout string // substring; "" means stdout must be empty
		wantStderr string // substring; "" means stderr must be empty
	}{
		{
			name:       "version prints the build version",
			args:       []string{"version"},
			wantCode:   0,
			wantStdout: "weir " + version.Version,
		},
		{
			name:       "help prints the usage on stdout",
			args:       []string{"help"},
			wantCode:   0,
			wantStdout: "使い方:",
		},
		{
			name:       "no argument is a usage error",
			args:       nil,
			wantCode:   2,
			wantStderr: "使い方:",
		},
		{
			name:       "an unknown command names itself and does not pass",
			args:       []string{"kommit"},
			wantCode:   2,
			wantStderr: "kommit",
		},
		{
			name:     "check says nothing about what it does not stop",
			args:     []string{"check"},
			stdin:    `{"cwd":"/repo","tool_name":"Bash","tool_input":{"command":"git status"}}`,
			wantCode: 0,
		},
		{
			name:       "check takes no arguments, and a mis-wired call is not a pass",
			args:       []string{"check", "--repo", "weir"},
			stdin:      `{"cwd":"/repo","tool_name":"Bash","tool_input":{"command":"git status"}}`,
			wantCode:   2,
			wantStderr: "引数は取りません",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tt.args, strings.NewReader(tt.stdin), &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantStdout == "" {
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q, want empty", stdout.String())
				}
			} else if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr == "" {
				if stderr.Len() != 0 {
					t.Errorf("stderr = %q, want empty", stderr.String())
				}
			} else if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// Input weir cannot read is input weir cannot judge, so the gate closes.
func TestRunCheckStopsUnreadableInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"check"}, strings.NewReader(`{"tool_name":`), &stdout, &stderr)

	if code != 0 {
		t.Errorf("exit code = %d, want 0 — the judgement travels in the output, not the code", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}

	var got struct {
		HookSpecificOutput struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout = %q, which is not JSON: %v", stdout.String(), err)
	}
	if got.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("permissionDecision = %q, want %q", got.HookSpecificOutput.PermissionDecision, "deny")
	}

	// A refusal has to carry the cause, the way out, and what a right call
	// looks like — otherwise the reader is stopped without being told anything.
	reason := got.HookSpecificOutput.PermissionDecisionReason
	for _, want := range []string{"読めませんでした", "確認してください", "tool_input"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason = %q, want it to contain %q", reason, want)
		}
	}
}

// failingWriter stands in for a stdout that has gone away.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("書き込めません") }

func TestRunCheckReportsAnUndeliverableDenial(t *testing.T) {
	var stderr bytes.Buffer
	code := Run([]string{"check"}, strings.NewReader(`{`), failingWriter{}, &stderr)

	// The refusal never reached the agent. Exiting 0 would read as a pass, so
	// weir has to say it failed instead.
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "拒否を書き出せません") {
		t.Errorf("stderr = %q, want it to say the denial could not be written", stderr.String())
	}
}
