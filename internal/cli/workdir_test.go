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

// newRepoWithWorktree makes a repository with one commit and a worktree cut
// from it, and answers with both. Each has a file staged and ready, so a test
// can tell which of the two a commit landed in by which file it carries.
func newRepoWithWorktree(t *testing.T) (proper, worktree string) {
	t.Helper()

	proper = t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "weir test"},
		{"config", "user.email", "weir@example.invalid"},
		{"commit", "--allow-empty", "-m", "root"},
	} {
		runGit(t, proper, args...)
	}

	// Outside the repository, as weir's own worktrees are cut.
	worktree = filepath.Join(t.TempDir(), "wt")
	runGit(t, proper, "worktree", "add", "-b", "topic", worktree)

	stage(t, proper, "proper.txt")
	stage(t, worktree, "worktree.txt")
	return proper, worktree
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// stage writes a file and stages it, so there is something to commit.
func stage(t *testing.T, dir, name string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", name, err)
	}
	runGit(t, dir, "add", name)
}

// committed reports whether the last commit in dir carries the named file —
// which tree a commit landed in, asked of the history rather than of weir.
func committed(t *testing.T, dir, name string) bool {
	t.Helper()

	cmd := exec.Command("git", "show", "--name-only", "--format=", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show in %s: %v", dir, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func TestCommitInAWorktreeWithHereCommitsThere(t *testing.T) {
	proper, worktree := newRepoWithWorktree(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", proper))
	t.Chdir(worktree)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--here", "--message", "worktree で打ったコミット"},
		&stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !committed(t, worktree, "worktree.txt") {
		t.Errorf("the commit did not land in the worktree it was made from")
	}
	if committed(t, proper, "proper.txt") {
		t.Errorf("the commit landed in the repository proper, which nobody asked for")
	}
	// Which tree was acted on is not in the line that was typed, so it has to
	// be in what weir said.
	if !strings.Contains(stdout.String(), worktree) {
		t.Errorf("stdout = %q, want it to name the worktree it committed in", stdout.String())
	}
}

func TestCommitInAWorktreeWithoutHereRefuses(t *testing.T) {
	proper, worktree := newRepoWithWorktree(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", proper))
	t.Chdir(worktree)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "どちらでもない"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
	}
	// The whole point is that the other tree is left alone.
	if committed(t, proper, "proper.txt") {
		t.Errorf("the repository proper was committed while weir was refusing")
	}
	if committed(t, worktree, "worktree.txt") {
		t.Errorf("the worktree was committed while weir was refusing")
	}
	for _, want := range []string{worktree, proper, "--here"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to say %q", stderr.String(), want)
		}
	}
}

func TestCommitWithHereOutsideTheRepositoryRefuses(t *testing.T) {
	proper, _ := newRepoWithWorktree(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", proper))
	// Not a repository at all — the case a --here typed out of habit lands in.
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--here", "--message", "ここではない"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
	}
	if committed(t, proper, "proper.txt") {
		t.Errorf("the repository proper was committed while weir was refusing")
	}
	if !strings.Contains(stderr.String(), "--here") {
		t.Errorf("stderr = %q, want it to say which option was the problem", stderr.String())
	}
}

func TestCommitInTheRepositoryProperWithHereCommitsThere(t *testing.T) {
	proper, _ := newRepoWithWorktree(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", proper))
	// --here in the checkout the configuration already names is not a
	// contradiction; it names the same tree.
	t.Chdir(proper)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--here", "--message", "本体で打ったコミット"},
		&stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !committed(t, proper, "proper.txt") {
		t.Errorf("the commit did not land in the repository proper")
	}
}

func TestCommitFromAnotherRepositoryStillCommitsTheNamedOne(t *testing.T) {
	proper, _ := newRepoWithWorktree(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", proper))
	// Somewhere else entirely, with a repository of its own: naming a
	// repository has always been enough, and --here is what changed, not this.
	elsewhere := t.TempDir()
	runGit(t, elsewhere, "init", "-b", "main")
	t.Chdir(elsewhere)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "名前で指したコミット"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !committed(t, proper, "proper.txt") {
		t.Errorf("the commit did not land in the repository that was named")
	}
}

func TestPushInAWorktreeWithoutHereRefuses(t *testing.T) {
	proper, worktree := newRepoWithWorktree(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", proper))
	t.Chdir(worktree)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
	}
	// A worktree is on its own branch, so the tree decides what would be sent.
	// Refusing has to come before weir goes looking for an upstream.
	if strings.Contains(stderr.String(), "upstream") {
		t.Errorf("stderr = %q, want the working tree settled before the upstream is read", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--here") {
		t.Errorf("stderr = %q, want it to say how to push from this worktree", stderr.String())
	}
}
