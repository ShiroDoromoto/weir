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

// newRepoWithUpstream makes a repository whose branch already tracks a bare
// one, with a commit waiting to be sent. Returns both, so the test can ask the
// bare one what actually arrived.
func newRepoWithUpstream(t *testing.T) (dir, remote string) {
	t.Helper()

	remote = t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run(remote, "init", "--bare", "-b", "main")

	dir = newRepo(t)
	run(dir, "commit", "--message", "一つ目")
	run(dir, "remote", "add", "origin", remote)
	run(dir, "push", "--set-upstream", "origin", "main")

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatalf("could not rewrite a.txt: %v", err)
	}
	run(dir, "add", "a.txt")
	run(dir, "commit", "--message", "送るコミット")

	return dir, remote
}

func TestPushPushesFromTheNamedRepository(t *testing.T) {
	dir, remote := newRepoWithUpstream(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if got := lastSubject(t, remote); got != "送るコミット" {
		t.Errorf("upstream head = %q, want the commit that was pushed", got)
	}
}

// Every refusal has to carry the cause and a line that works — the reader was
// sent here from plain git and does not yet know weir's vocabulary.
func TestPushRefusesAndSaysHow(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantCause string
	}{
		{
			name:      "no repository named",
			args:      []string{"push"},
			wantCause: "--repo がありません",
		},
		{
			name:      "an empty name is no name",
			args:      []string{"push", "--repo", ""},
			wantCause: "--repo がありません",
		},
		{
			name:      "a name that is not registered",
			args:      []string{"push", "--repo", "notes"},
			wantCause: "登録されていません",
		},
		{
			name:      "an option weir does not have",
			args:      []string{"push", "--repo", "weir", "--force"},
			wantCause: "--repo だけ",
		},
		{
			name:      "a destination, which weir does not take",
			args:      []string{"push", "--repo", "weir", "origin", "main"},
			wantCause: "送り先は指定しません",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := newRepoWithUpstream(t)
			withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

			var stdout, stderr bytes.Buffer
			code := Run(tt.args, &stdout, &stderr)
			if code != exitUsage {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantCause) {
				t.Errorf("stderr = %q, want it to say why: %q", stderr.String(), tt.wantCause)
			}
			if !strings.Contains(stderr.String(), pushExample) {
				t.Errorf("stderr = %q, want it to show a line that works", stderr.String())
			}
			// Nothing was pushed, so nothing may look as though it was.
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

// The configuration is what needs fixing, and it already says how — so the
// answer points there rather than at the command's own shape.
func TestPushWithoutAConfigurationPointsAtIt(t *testing.T) {
	withConfig(t, "")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "weir init") {
		t.Errorf("stderr = %q, want it to point at weir init", stderr.String())
	}
}

// git failing is weir failing, not weir passing quietly.
func TestPushFailsWhenThereIsNowhereToSendIt(t *testing.T) {
	dir := newRepo(t)
	cmd := exec.Command("git", "commit", "--message", "一つ目")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "git push") {
		t.Errorf("stderr = %q, want it to name what failed", stderr.String())
	}
}

func TestPushHelpPrintsItsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"push", "--help"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--repo") {
		t.Errorf("stdout = %q, want it to describe the options", stdout.String())
	}
}
