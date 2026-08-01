// Package gitrepo answers which repository a directory belongs to, and which
// registered repository that is.
//
// A worktree is the reason this package exists. Run from one, a directory tells
// you nothing about what is registered: the worktree sits somewhere else
// entirely, under a name the configuration never mentions. So weir asks git
// itself rather than reading the shape of the path — `git rev-parse
// --git-common-dir` points at the .git the worktree shares with the repository
// proper, and its parent is that repository. The judgement is then made on the
// repository proper, for the worktree as much as for the checkout it was cut
// from.
//
// Which working tree a command acts in is a separate question with the same
// root, and Locate answers it: a directory is in the repository's own checkout,
// in a worktree cut from it, or nowhere near it.
package gitrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ShiroDoromoto/weir/internal/config"
)

// Root returns the path of the repository dir belongs to — the repository
// proper, never the worktree. A directory that is not inside a working tree is
// an error, so nothing downstream has to guess what it was handed.
func Root(dir string) (string, error) {
	// One call answers both questions, in the order the flags are given:
	// whether this is a working tree, and where the shared .git is.
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree", "--git-common-dir")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail := strings.TrimSpace(string(exitErr.Stderr))
			return "", fmt.Errorf("%s: git リポジトリとして扱えません: %s", dir, detail)
		}
		return "", fmt.Errorf("%s: git を実行できません: %w", dir, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		return "", fmt.Errorf("%s: git rev-parse の出力を解釈できません: %q", dir, string(out))
	}
	if strings.TrimSpace(lines[0]) != "true" {
		return "", fmt.Errorf("%s: 作業ツリーの中ではありません (bare リポジトリは weir の対象外です)", dir)
	}

	// In the repository proper git answers with a relative ".git"; from a
	// worktree it answers with an absolute path. Resolve against dir either way.
	common := strings.TrimSpace(lines[1])
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	root, err := canonical(filepath.Dir(common))
	if err != nil {
		return "", fmt.Errorf("%s: リポジトリの位置を解決できません: %w", dir, err)
	}
	return root, nil
}

// Where says where a directory sits in relation to a registered repository.
type Where int

const (
	// Elsewhere is a directory in some other repository, or in none at all.
	// This is what weir run from anywhere but the repository looks like, and
	// it is the ordinary case: the repository was named on the command line,
	// not found by looking around.
	Elsewhere Where = iota
	// Proper is a directory inside the repository's own checkout — the one
	// sitting at the registered path.
	Proper
	// Linked is a directory inside a worktree cut from the repository. The
	// history is the repository's, but the files are not the ones at the
	// registered path, and neither is the branch.
	Linked
)

// Locate answers where dir sits in relation to the repository registered at
// path, and which working tree that is — empty for Elsewhere.
//
// A dir outside any working tree is Elsewhere rather than an error. weir is
// run from wherever the person happens to be standing, and standing nowhere
// near the repository is not a problem to report: the repository was named.
func Locate(dir, path string) (Where, string, error) {
	registered, err := canonical(path)
	if err != nil {
		return Elsewhere, "", fmt.Errorf("登録されたパスを解決できません: %s: %w", path, err)
	}

	// Which repository first: a dir that is not in a working tree at all, or is
	// in someone else's, is answered here and git is not asked a second time.
	root, err := Root(dir)
	if err != nil || root != registered {
		return Elsewhere, "", nil
	}

	// From here dir is known to be in this repository, so a git that cannot say
	// which of its trees is a question left unanswered, not an Elsewhere.
	// Reading it as Elsewhere would send weir to the registered path — the one
	// case this whole function exists to stop.
	tree, err := topLevel(dir)
	if err != nil {
		return Elsewhere, "", err
	}
	if tree == registered {
		return Proper, tree, nil
	}
	return Linked, tree, nil
}

// topLevel answers the root of the working tree dir is in, canonical, so two
// spellings of one directory compare equal.
func topLevel(dir string) (string, error) {
	// Asked apart from Root's rev-parse rather than as a third flag on it:
	// --show-toplevel has no answer outside a working tree, and folding it in
	// would turn "not a working tree" into a git failure with a worse message.
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: どの作業ツリーの中かを解決できません: %w", dir, err)
	}
	tree, err := canonical(strings.TrimSpace(string(out)))
	if err != nil {
		return "", fmt.Errorf("%s: どの作業ツリーの中かを解決できません: %w", dir, err)
	}
	return tree, nil
}

// CheckPath reports what stops path from being usable as a registered
// repository, and nil when nothing does. Resolve simply skips a path like that
// — refusing is its job, not explaining — so this is where the explaining
// lives.
func CheckPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errors.New("そのパスがありません")
		}
		return fmt.Errorf("パスを確認できません: %w", err)
	}
	if !info.IsDir() {
		return errors.New("ディレクトリではありません")
	}

	root, err := Root(path)
	if err != nil {
		return err
	}
	here, err := canonical(path)
	if err != nil {
		return fmt.Errorf("パスを解決できません: %w", err)
	}
	// A worktree is a working tree of its own, so everything above passes for
	// one. weir acts on the repository proper, so registering a worktree would
	// have weir working somewhere other than where it was pointed.
	if root != here {
		return fmt.Errorf(
			"リポジトリ本体ではありません（本体は %s）。worktree ではなく本体の絶対パスを書いてください", root)
	}
	return nil
}

// Resolve answers which registered repository dir belongs to. A directory in an
// unregistered repository is refused — being inside a git repository is not by
// itself permission to act on it.
func Resolve(dir string, cfg *config.Config) (config.Repo, error) {
	root, err := Root(dir)
	if err != nil {
		return config.Repo{}, err
	}

	for _, name := range cfg.Names() {
		repo := cfg.Repos[name]
		// A registered path that cannot be canonicalised simply does not match.
		// Refusing is this one's job; CheckPath is what says why.
		registered, err := canonical(repo.Path)
		if err != nil {
			continue
		}
		if registered == root {
			return repo, nil
		}
	}
	return config.Repo{}, &UnregisteredError{Dir: dir, Root: root, Known: cfg.Names()}
}

// canonical resolves a path to the one form two spellings of the same
// directory can be compared in — symlinks included, since /var and /private/var
// are the same directory on macOS and only one of them is what git answers.
func canonical(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// UnregisteredError reports a directory whose repository is not in the
// configuration.
type UnregisteredError struct {
	// Dir is the directory weir was asked about — a worktree, possibly.
	Dir string
	// Root is the repository proper that Dir belongs to. This is what was
	// looked up, and what has to be registered.
	Root  string
	Known []string
}

func (e *UnregisteredError) Error() string {
	var b strings.Builder

	fmt.Fprintf(&b, "リポジトリが登録されていません: %s", e.Root)
	if e.Dir != e.Root {
		// Say both, or a worktree reads as judged on a path nobody wrote.
		fmt.Fprintf(&b, "（%s の worktree の本体）", e.Dir)
	}
	b.WriteString("。")

	if len(e.Known) == 0 {
		b.WriteString("登録されているリポジトリは1つもありません。")
	} else {
		fmt.Fprintf(&b, "登録されているのは: %s。", strings.Join(e.Known, ", "))
	}

	fmt.Fprintf(&b,
		"このリポジトリを足すなら ~/.weir/config.toml に `[repos.<名前>]` と `path = %q` を書いてください",
		e.Root)
	return b.String()
}
