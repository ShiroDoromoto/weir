package gitrepo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/weir/internal/config"
)

// git runs a git command in dir, with the user's own git configuration kept
// out of it so the test answers the same on every machine.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=weir test",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=weir test",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// mustCanonical is what a path looks like once resolved — on macOS a temporary
// directory is reached through a symlink, and git answers with the resolved
// form.
func mustCanonical(t *testing.T, path string) string {
	t.Helper()

	resolved, err := canonical(path)
	if err != nil {
		t.Fatalf("canonical(%s): %v", path, err)
	}
	return resolved
}

// newRepo makes a repository with one commit — enough for a worktree to be cut
// from it.
func newRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "commit", "--allow-empty", "-m", "root")
	return mustCanonical(t, dir)
}

// addWorktree cuts a worktree of repo, outside it.
func addWorktree(t *testing.T, repo string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "wt")
	git(t, repo, "worktree", "add", dir, "-b", "wt")
	return mustCanonical(t, dir)
}

func TestRootAnswersTheRepositoryProper(t *testing.T) {
	repo := newRepo(t)
	sub := filepath.Join(repo, "internal", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", sub, err)
	}
	worktree := addWorktree(t, repo)
	worktreeSub := filepath.Join(worktree, "nested")
	if err := os.MkdirAll(worktreeSub, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", worktreeSub, err)
	}

	// Every one of these is the same repository, and the worktree is the whole
	// point: asked from there, weir still has to answer with what is registered.
	for _, dir := range []string{repo, sub, worktree, worktreeSub} {
		got, err := Root(dir)
		if err != nil {
			t.Errorf("Root(%s) = %v, want no error", dir, err)
			continue
		}
		if got != repo {
			t.Errorf("Root(%s) = %q, want %q", dir, got, repo)
		}
	}
}

func TestRootRefusesWhatIsNotAWorkingTree(t *testing.T) {
	t.Run("outside any repository", func(t *testing.T) {
		dir := t.TempDir()

		if got, err := Root(dir); err == nil {
			t.Fatalf("Root(%s) = %q, want an error", dir, got)
		}
	})

	t.Run("bare repository", func(t *testing.T) {
		dir := t.TempDir()
		git(t, dir, "init", "--bare", "-b", "main")

		got, err := Root(dir)
		if err == nil {
			t.Fatalf("Root(%s) = %q, want an error", dir, got)
		}
		if !strings.Contains(err.Error(), "作業ツリー") {
			t.Errorf("error = %q, want it to say this is not a working tree", err)
		}
	})
}

func TestResolveMatchesAWorktreeToItsRegisteredRepository(t *testing.T) {
	repo := newRepo(t)
	worktree := addWorktree(t, repo)

	cfg := &config.Config{Repos: map[string]config.Repo{
		"weir": {Name: "weir", Path: repo},
	}}

	for _, dir := range []string{repo, worktree} {
		got, err := Resolve(dir, cfg)
		if err != nil {
			t.Errorf("Resolve(%s) = %v, want no error", dir, err)
			continue
		}
		if got.Name != "weir" {
			t.Errorf("Resolve(%s).Name = %q, want %q", dir, got.Name, "weir")
		}
	}
}

// Being inside a git repository is not by itself permission to act on it.
func TestResolveRefusesAnUnregisteredRepository(t *testing.T) {
	repo := newRepo(t)
	worktree := addWorktree(t, repo)
	other := newRepo(t)

	cfg := &config.Config{Repos: map[string]config.Repo{
		"other": {Name: "other", Path: other},
	}}

	_, err := Resolve(worktree, cfg)
	var unregistered *UnregisteredError
	if !errors.As(err, &unregistered) {
		t.Fatalf("Resolve(%s) = %v, want an *UnregisteredError", worktree, err)
	}
	if unregistered.Root != repo {
		t.Errorf("Root = %q, want %q (the repository proper, not the worktree)", unregistered.Root, repo)
	}
	// The message has to name the repository proper — that is what has to be
	// registered — and say the ask came from a worktree, or it reads as a
	// judgement on a path nobody wrote.
	if !strings.Contains(err.Error(), repo) || !strings.Contains(err.Error(), "worktree") {
		t.Errorf("error = %q, want it to name %q and mention the worktree", err, repo)
	}
	if !strings.Contains(err.Error(), "other") {
		t.Errorf("error = %q, want it to list the registered names", err)
	}
}

// A registered path that no longer exists must not turn into a match, and must
// not stop the rest of the configuration from being considered.
func TestResolveSkipsARegisteredPathThatIsGone(t *testing.T) {
	repo := newRepo(t)

	cfg := &config.Config{Repos: map[string]config.Repo{
		"gone": {Name: "gone", Path: filepath.Join(t.TempDir(), "no-such-directory")},
		"weir": {Name: "weir", Path: repo},
	}}

	got, err := Resolve(repo, cfg)
	if err != nil {
		t.Fatalf("Resolve(%s) = %v, want no error", repo, err)
	}
	if got.Name != "weir" {
		t.Errorf("Resolve(%s).Name = %q, want %q", repo, got.Name, "weir")
	}
}

func TestLocateTellsTheWorktreeFromTheRepositoryProper(t *testing.T) {
	repo := newRepo(t)
	worktree := addWorktree(t, repo)
	sub := filepath.Join(repo, "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", sub, err)
	}

	for _, c := range []struct {
		name     string
		dir      string
		want     Where
		wantTree string
	}{
		{"the checkout itself", repo, Proper, repo},
		{"a directory under it", sub, Proper, repo},
		{"a worktree cut from it", worktree, Linked, worktree},
		{"another repository", newRepo(t), Elsewhere, ""},
		// Where weir is usually run from: no repository in sight, and the
		// repository named on the command line is the answer anyway.
		{"no repository at all", t.TempDir(), Elsewhere, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			where, tree, err := Locate(c.dir, repo)
			if err != nil {
				t.Fatalf("Locate(%s, %s): %v", c.dir, repo, err)
			}
			if where != c.want {
				t.Errorf("Locate(%s) = %v, want %v", c.dir, where, c.want)
			}
			if tree != c.wantTree {
				t.Errorf("Locate(%s) tree = %q, want %q", c.dir, tree, c.wantTree)
			}
		})
	}
}

func TestLocateReportsARegisteredPathItCannotResolve(t *testing.T) {
	repo := newRepo(t)
	gone := filepath.Join(t.TempDir(), "gone")

	// A registered path that is not there is a configuration to fix, not a
	// directory that is merely elsewhere — saying "elsewhere" would have weir
	// carry on and run git in a path nobody can stand in.
	if _, _, err := Locate(repo, gone); err == nil {
		t.Fatalf("Locate(%s, %s) = nil error, want the unresolvable path reported", repo, gone)
	}
}
