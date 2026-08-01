package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo makes a repository with one file staged and ready to be committed,
// carrying an identity of its own so the test does not lean on the machine's.
func newRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("could not write a.txt: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "weir test"},
		{"config", "user.email", "weir@example.invalid"},
		{"add", "a.txt"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return dir
}

func lastSubject(t *testing.T, dir string) string {
	t.Helper()

	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestCommitCommitsInTheNamedRepository(t *testing.T) {
	dir := newRepo(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "門を通したコミット"},
		strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if got := lastSubject(t, dir); got != "門を通したコミット" {
		t.Errorf("commit subject = %q, want the message it was given", got)
	}
}

func TestCommitWithAllTakesUnstagedChangesToo(t *testing.T) {
	dir := newRepo(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "一つ目"},
		strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("could not rewrite a.txt: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"commit", "--repo", "weir", "--message", "二つ目", "--all"},
		strings.NewReader(""), &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if got := lastSubject(t, dir); got != "二つ目" {
		t.Errorf("commit subject = %q, want the second commit to have been made", got)
	}
}

// Every refusal has to carry the cause and a line that works — the reader was
// sent here from plain git and does not yet know weir's vocabulary.
func TestCommitRefusesAndSaysHow(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantCause string
	}{
		{
			name:      "no repository named",
			args:      []string{"commit", "--message", "x"},
			wantCause: "--repo がありません",
		},
		{
			name:      "no message",
			args:      []string{"commit", "--repo", "weir"},
			wantCause: "--message がありません",
		},
		{
			name:      "an empty message is no message",
			args:      []string{"commit", "--repo", "weir", "--message", ""},
			wantCause: "--message がありません",
		},
		{
			name:      "a name that is not registered",
			args:      []string{"commit", "--repo", "notes", "--message", "x"},
			wantCause: "登録されていません",
		},
		{
			name:      "an option weir does not have",
			args:      []string{"commit", "--repo", "weir", "-m", "x"},
			wantCause: "--repo / --message / --all",
		},
		{
			name:      "something left over on the line",
			args:      []string{"commit", "--repo", "weir", "--message", "x", "HEAD~1"},
			wantCause: "余分な引数があります",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newRepo(t)
			withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

			var stdout, stderr bytes.Buffer
			code := Run(tt.args, strings.NewReader(""), &stdout, &stderr)
			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantCause) {
				t.Errorf("stderr = %q, want it to say why: %q", stderr.String(), tt.wantCause)
			}
			if !strings.Contains(stderr.String(), commitExample) {
				t.Errorf("stderr = %q, want it to show a line that works", stderr.String())
			}
			// Nothing was committed, so nothing may look as though it was.
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

// The configuration is what needs fixing, and it already says how — so the
// answer points there rather than at the command's own shape.
func TestCommitWithoutAConfigurationPointsAtIt(t *testing.T) {
	withConfig(t, "")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "x"},
		strings.NewReader(""), &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "weir init") {
		t.Errorf("stderr = %q, want it to point at weir init", stderr.String())
	}
}

// git failing is weir failing, not weir passing quietly.
func TestCommitFailsWhenThereIsNothingToCommit(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "weir test"},
		{"config", "user.email", "weir@example.invalid"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "空"},
		strings.NewReader(""), &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "git commit") {
		t.Errorf("stderr = %q, want it to name what failed", stderr.String())
	}
}

func TestCommitHelpPrintsItsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"commit", "--help"}, strings.NewReader(""), &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--all") {
		t.Errorf("stdout = %q, want it to describe the options", stdout.String())
	}
}
