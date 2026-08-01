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

// checkDenial runs check over payload and returns the reason it refused with,
// failing the test if it did not refuse.
func checkDenial(t *testing.T, payload string) string {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"check"}, strings.NewReader(payload), &stdout, &stderr)

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
	return got.HookSpecificOutput.PermissionDecisionReason
}

// bashPayload is one Bash tool call, as Claude Code hands it to the hook.
func bashPayload(t *testing.T, command string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"cwd":       "/repo",
		"tool_name": "Bash",
		"tool_input": map[string]string{
			"command": command,
		},
	})
	if err != nil {
		t.Fatalf("could not build the payload: %v", err)
	}
	return string(payload)
}

// Input weir cannot read is input weir cannot judge, so the gate closes.
func TestRunCheckStopsUnreadableInput(t *testing.T) {
	reason := checkDenial(t, `{"tool_name":`)

	// A refusal has to carry the cause, the way out, and what a right call
	// looks like — otherwise the reader is stopped without being told anything.
	for _, want := range []string{"読めませんでした", "確認してください", "tool_input"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason = %q, want it to contain %q", reason, want)
		}
	}
}

func TestRunCheckStopsPlainGit(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		wantCause   string
		wantExample string
	}{
		{
			name:        "a commit is sent back to weir commit",
			command:     `git commit -m "wip"`,
			wantCause:   "素の git の commit を止めました",
			wantExample: "weir commit --repo",
		},
		{
			name:        "a push is sent back to weir push",
			command:     `git push --force`,
			wantCause:   "素の git の push を止めました",
			wantExample: "weir push --repo",
		},
		{
			name:        "a command that will not come apart is stopped unread",
			command:     `git commit -m "oops`,
			wantCause:   "読めませんでした",
			wantExample: "weir commit --repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := checkDenial(t, bashPayload(t, tt.command))

			if !strings.Contains(reason, tt.wantCause) {
				t.Errorf("reason = %q, want it to say why: %q", reason, tt.wantCause)
			}
			if !strings.Contains(reason, "してください") {
				t.Errorf("reason = %q, want it to say what to do next", reason)
			}
			if !strings.Contains(reason, tt.wantExample) {
				t.Errorf("reason = %q, want it to show a line that works: %q", reason, tt.wantExample)
			}
		})
	}
}

// What weir does not stop, it does not speak about — a read-only git included.
func TestRunCheckPassesWhatItDoesNotStop(t *testing.T) {
	for _, command := range []string{
		`git status --short`,
		`git log --oneline -5`,
		`git rebase -i main`,
		`gh pr create --fill`,
		`weir commit --repo weir --message x`,
		`echo 'git push --force'`,
	} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"check"}, strings.NewReader(bashPayload(t, command)), &stdout, &stderr)

		if code != 0 {
			t.Errorf("Run(check) over %q = %d, want 0", command, code)
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Errorf("Run(check) over %q wrote stdout=%q stderr=%q, want both empty",
				command, stdout.String(), stderr.String())
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
