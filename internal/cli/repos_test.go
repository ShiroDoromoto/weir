package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/weir/internal/config"
)

// withConfig points HOME at a fresh temporary directory holding body as
// ~/.weir/config.toml. Pass an empty body to leave the file out entirely.
func withConfig(t *testing.T, body string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	if body == "" {
		return
	}

	dir := filepath.Join(home, config.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", dir, err)
	}
	path := filepath.Join(dir, config.FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
}

func TestReposListsWhatIsRegistered(t *testing.T) {
	withConfig(t, `
[repos.weir]
path = "/Users/someone/develop/weir"

[repos.notes]
path = "/Users/someone/develop/notes"
`)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"repos"}, strings.NewReader(""), &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want one line per repository", stdout.String())
	}
	// Sorted, so the same configuration always reads the same way.
	if !strings.HasPrefix(lines[0], "notes") || !strings.HasPrefix(lines[1], "weir") {
		t.Errorf("stdout = %q, want the names sorted", stdout.String())
	}
	if !strings.Contains(lines[1], "/Users/someone/develop/weir") {
		t.Errorf("stdout = %q, want it to carry each path", stdout.String())
	}
}

// An empty list is an answer, not a failure — but it has to say what would
// change it, or it reads as weir having lost the configuration.
func TestReposOnAnEmptyConfigSaysSo(t *testing.T) {
	withConfig(t, "# 何も登録していない\n")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"repos"}, strings.NewReader(""), &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ありません") {
		t.Errorf("stdout = %q, want it to say nothing is registered", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[repos.<名前>]") {
		t.Errorf("stdout = %q, want it to show what to write", stdout.String())
	}
}

func TestReposFailsOnAConfigItCannotRead(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "no configuration at all", body: "", want: "weir init"},
		{name: "broken syntax", body: "[repos.weir\n", want: "構文"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withConfig(t, tt.body)

			var stdout, stderr bytes.Buffer
			code := Run([]string{"repos"}, strings.NewReader(""), &stdout, &stderr)
			if code != exitFailure {
				t.Fatalf("exit code = %d, want %d", code, exitFailure)
			}
			// Nothing on stdout: a caller reading the list must not get a
			// half-answer that looks like "nothing is registered".
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestReposTakesNoArguments(t *testing.T) {
	withConfig(t, "[repos.weir]\npath = \"/tmp/weir\"\n")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"repos", "weir"}, strings.NewReader(""), &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "引数は取りません") {
		t.Errorf("stderr = %q, want it to say repos takes no arguments", stderr.String())
	}
}
