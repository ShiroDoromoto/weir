package gitcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo makes a repository weir can commit in: an identity of its own, so
// the test does not depend on whoever is running it having one configured.
func newRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "weir test"},
		{"config", "user.email", "weir@example.invalid"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", name, err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}

func TestCommitCommitsWhatIsStaged(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")

	var stdout, stderr bytes.Buffer
	if err := Commit(dir, "最初のコミット", false, &stdout, &stderr); err != nil {
		t.Fatalf("Commit() error = %v (stderr: %s)", err, stderr.String())
	}

	if got := git(t, dir, "log", "-1", "--format=%s"); got != "最初のコミット" {
		t.Errorf("commit subject = %q, want the message it was given", got)
	}
	// git's own report is what the caller sees; weir does not swallow it.
	if stdout.Len() == 0 {
		t.Error("stdout is empty, want git's own report of the commit")
	}
}

// Without --all, what was never staged stays out: weir does not widen what was
// asked for.
func TestCommitLeavesUnstagedChangesAlone(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")

	var out bytes.Buffer
	if err := Commit(dir, "一つ目", false, &out, &out); err != nil {
		t.Fatalf("Commit() error = %v (output: %s)", err, out.String())
	}

	write(t, dir, "a.txt", "two\n")
	write(t, dir, "b.txt", "new\n")
	git(t, dir, "add", "b.txt")

	out.Reset()
	if err := Commit(dir, "二つ目", false, &out, &out); err != nil {
		t.Fatalf("Commit() error = %v (output: %s)", err, out.String())
	}

	if names := git(t, dir, "show", "--name-only", "--format=", "HEAD"); names != "b.txt" {
		t.Errorf("committed %q, want only the staged b.txt", names)
	}
}

func TestCommitWithAllTakesUnstagedChangesToo(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")

	var out bytes.Buffer
	if err := Commit(dir, "一つ目", false, &out, &out); err != nil {
		t.Fatalf("Commit() error = %v (output: %s)", err, out.String())
	}

	write(t, dir, "a.txt", "two\n")

	out.Reset()
	if err := Commit(dir, "二つ目", true, &out, &out); err != nil {
		t.Fatalf("Commit(all) error = %v (output: %s)", err, out.String())
	}

	if names := git(t, dir, "show", "--name-only", "--format=", "HEAD"); names != "a.txt" {
		t.Errorf("committed %q, want the unstaged a.txt", names)
	}
}

// A message is one argument, never a piece of a command line — so a message
// that reads like shell is committed as text.
func TestCommitTreatsTheMessageAsText(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")

	message := `"; rm -rf / #`
	var out bytes.Buffer
	if err := Commit(dir, message, false, &out, &out); err != nil {
		t.Fatalf("Commit() error = %v (output: %s)", err, out.String())
	}

	if got := git(t, dir, "log", "-1", "--format=%s"); got != message {
		t.Errorf("commit subject = %q, want %q", got, message)
	}
}

func TestCommitFailsWhenGitDoes(t *testing.T) {
	dir := newRepo(t)

	var stdout, stderr bytes.Buffer
	err := Commit(dir, "何も無い", false, &stdout, &stderr)
	if err == nil {
		t.Fatal("Commit() = no error, want one: there is nothing to commit")
	}
	// git said why. weir names what failed and does not talk over it.
	if stdout.Len() == 0 && stderr.Len() == 0 {
		t.Error("git's own output was swallowed, want it passed through")
	}
	if !strings.Contains(err.Error(), "git commit") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}
