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
			wantCause: "--repo / --here だけ",
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

// With no destination weir cannot tell which commits a push would send, so it
// does not run one. Plain git often refuses such a push itself — but with
// push.autoSetupRemote it sends the branch instead, and weir would have passed
// commits it never looked at.
//
// Each way of having no destination is asked for separately, because each one's
// way out is a different thing to type. Telling someone with no remote at all
// to set an upstream is advice they cannot follow.
func TestPushRefusesWhenItCannotTellWhatWouldBeSent(t *testing.T) {
	for _, c := range []struct {
		name string
		// setUp leaves the repository in the state under test.
		setUp func(t *testing.T, dir string)
		// wantFix is the command the reader is sent to.
		wantFix string
	}{
		{
			name:    "no remote at all",
			setUp:   func(*testing.T, string) {},
			wantFix: "git remote add",
		},
		{
			name: "several remotes and nothing choosing one",
			setUp: func(t *testing.T, dir string) {
				for _, name := range []string{"origin", "other"} {
					bare := filepath.Join(t.TempDir(), name+".git")
					pushGit(t, dir, "init", "--bare", bare)
					pushGit(t, dir, "remote", "add", name, bare)
				}
			},
			wantFix: "pushRemote",
		},
		{
			name: "not on a branch",
			setUp: func(t *testing.T, dir string) {
				pushGit(t, dir, "checkout", "--detach", "HEAD")
			},
			wantFix: "git switch",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := newRepo(t)
			pushGit(t, dir, "commit", "--message", "一つ目")
			c.setUp(t, dir)
			withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

			var stdout, stderr bytes.Buffer
			code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
			if code != exitFailure {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
			}
			// The three parts every refusal carries: what happened, what to do
			// about it, and a line that works.
			for _, want := range []string{"読み出せない", c.wantFix, "weir push --repo weir"} {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want it to carry %q", stderr.String(), want)
				}
			}
		})
	}
}

// With one remote and no upstream there is nothing to guess, so weir reads the
// surface and hands the push to git — the push a person cannot avoid making,
// since a branch gets its upstream by being pushed.
//
// Whether git then makes it is git's own answer, and weir does not restate it.
// With push.autoSetupRemote it goes; without, git asks for --set-upstream and
// says so itself. Either way weir is no longer the one in the way, which is the
// whole point: a gate people have to walk around is not a gate.
func TestPushWithoutAnUpstreamGetsAsFarAsGit(t *testing.T) {
	for _, c := range []struct {
		name     string
		autoSet  string
		wantCode int
	}{
		{name: "git will set the upstream itself", autoSet: "true", wantCode: exitOK},
		{name: "git asks for it first", autoSet: "false", wantCode: exitFailure},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := newRepo(t)
			pushGit(t, dir, "commit", "--message", "一つ目")
			bare := filepath.Join(t.TempDir(), "origin.git")
			pushGit(t, dir, "init", "--bare", bare)
			pushGit(t, dir, "remote", "add", "origin", bare)
			pushGit(t, dir, "config", "push.autoSetupRemote", c.autoSet)
			withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

			var stdout, stderr bytes.Buffer
			code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
			if code != c.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", code, c.wantCode, stderr.String())
			}
			// Whatever git decided, the refusal weir used to make is gone.
			if strings.Contains(stderr.String(), "読み出せない") {
				t.Errorf("stderr = %q, want weir to have read the surface", stderr.String())
			}
			if c.wantCode != exitOK {
				return
			}
			// It landed: the bare repository now has the branch.
			if out := pushGit(t, bare, "rev-parse", "--verify", "main"); out == "" {
				t.Errorf("the remote has no main, so the push never reached it")
			}
		})
	}
}

// pushGit runs one git command for these tests and answers with its output.
func pushGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
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
