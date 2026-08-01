package gitcmd

import (
	"bytes"
	"strings"
	"testing"
)

// newRepoWithUpstream makes a repository whose branch already tracks a bare one
// — the state plain `git push` needs, and the only state weir's push assumes.
// It returns the working repository and the bare one it pushes to.
func newRepoWithUpstream(t *testing.T) (dir, remote string) {
	t.Helper()

	remote = t.TempDir()
	git(t, remote, "init", "--bare", "-b", "main")

	dir = newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")

	var out bytes.Buffer
	if err := Commit(dir, "一つ目", false, &out, &out); err != nil {
		t.Fatalf("Commit() error = %v (output: %s)", err, out.String())
	}
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "push", "--set-upstream", "origin", "main")

	return dir, remote
}

func TestPushSendsToTheUpstream(t *testing.T) {
	dir, remote := newRepoWithUpstream(t)

	write(t, dir, "a.txt", "two\n")
	git(t, dir, "add", "a.txt")
	var out bytes.Buffer
	if err := Commit(dir, "二つ目", false, &out, &out); err != nil {
		t.Fatalf("Commit() error = %v (output: %s)", err, out.String())
	}

	var stdout, stderr bytes.Buffer
	if err := Push(dir, &stdout, &stderr); err != nil {
		t.Fatalf("Push() error = %v (stderr: %s)", err, stderr.String())
	}

	if got := git(t, remote, "log", "-1", "--format=%s"); got != "二つ目" {
		t.Errorf("upstream head = %q, want the commit that was pushed", got)
	}
	// git's own report is what the caller sees; weir does not swallow it.
	if stdout.Len() == 0 && stderr.Len() == 0 {
		t.Error("git's own output was swallowed, want it passed through")
	}
}

// Nowhere to send it is git's answer to give, not weir's to work around by
// picking a remote itself.
func TestPushFailsWithoutAnUpstream(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\n")
	git(t, dir, "add", "a.txt")
	var out bytes.Buffer
	if err := Commit(dir, "一つ目", false, &out, &out); err != nil {
		t.Fatalf("Commit() error = %v (output: %s)", err, out.String())
	}

	var stdout, stderr bytes.Buffer
	err := Push(dir, &stdout, &stderr)
	if err == nil {
		t.Fatal("Push() = no error, want one: there is no upstream to push to")
	}
	if stdout.Len() == 0 && stderr.Len() == 0 {
		t.Error("git's own output was swallowed, want it passed through")
	}
	if !strings.Contains(err.Error(), "git push") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}
