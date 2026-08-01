package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/weir/internal/version"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tt.args, &stdout, &stderr)

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
