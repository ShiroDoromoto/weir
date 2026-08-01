package gitrepo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPathPassesARepositoryProper(t *testing.T) {
	repo := newRepo(t)

	if err := CheckPath(repo); err != nil {
		t.Errorf("CheckPath(%s) = %v, want no error", repo, err)
	}
}

// The point of the check: a path that Resolve would silently skip has to come
// back with a reason someone can act on.
func TestCheckPathNamesWhatIsWrong(t *testing.T) {
	repo := newRepo(t)

	notThere := filepath.Join(t.TempDir(), "gone")

	aFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(aFile, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("could not write %s: %v", aFile, err)
	}

	notGit := t.TempDir()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "nothing there", path: notThere, want: "ありません"},
		{name: "a file, not a directory", path: aFile, want: "ディレクトリではありません"},
		{name: "not a git repository", path: notGit, want: "git"},
		{name: "a worktree, not the repository", path: addWorktree(t, repo), want: "本体"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckPath(tt.path)
			if err == nil {
				t.Fatalf("CheckPath(%s) = nil, want an error", tt.path)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("CheckPath(%s) = %q, want it to mention %q", tt.path, err, tt.want)
			}
		})
	}
}

// A worktree passes every earlier test — it is a directory, and git calls it a
// working tree — so the reason has to say where the repository proper is, or
// the reader has no way to write the right path.
func TestCheckPathOnAWorktreePointsAtTheRepository(t *testing.T) {
	repo := newRepo(t)
	worktree := addWorktree(t, repo)

	err := CheckPath(worktree)
	if err == nil {
		t.Fatalf("CheckPath(%s) = nil, want an error", worktree)
	}
	if !strings.Contains(err.Error(), repo) {
		t.Errorf("CheckPath(%s) = %q, want it to name %s", worktree, err, repo)
	}
}
