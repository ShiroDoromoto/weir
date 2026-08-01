package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/ShiroDoromoto/weir/internal/config"
	"github.com/ShiroDoromoto/weir/internal/gitrepo"
)

// worktreeCommand is what one command needs said about itself when weir has to
// talk about which working tree it was pointed at.
type worktreeCommand struct {
	// name is how the command names itself in its own output.
	name string
	// verb is what it does, for a sentence saying it did not do it.
	verb string
	// format is the line that works and hereFormat the same line acting on the
	// working tree it was run in, each taking the repository's name. The name
	// is known by the time these are used, so what weir shows is a line to run
	// rather than one to fill in.
	format     string
	hereFormat string
}

// workdir answers which working tree the command acts in — the one weir judges
// and the one it runs git in, which are never allowed to be two different
// trees. An empty answer means weir refused, and code is what to exit with.
//
// The repository is still named and only named: --here does not pick a
// repository, so which rules apply cannot change with where the command was
// run. All it picks is which of that repository's working trees was meant.
//
// Without it weir acts on the registered path, as it always has — except when
// the command was run inside a worktree cut from that same repository, which is
// refused rather than answered with the other tree. Someone standing in a
// worktree who types a commit means the files in front of them, and a gate that
// acts on something other than what was in front of it is worse than no gate.
func workdir(c worktreeCommand, repo config.Repo, here bool, stdout, stderr io.Writer) (string, int) {
	cwd, err := os.Getwd()
	if err != nil {
		// Where weir is standing is part of the answer now, so failing to read
		// it is not something to carry on past.
		fmt.Fprintf(stderr, "%s: いまの作業ディレクトリを読めません: %v\n", c.name, err)
		return "", exitFailure
	}

	where, tree, err := gitrepo.Locate(cwd, repo.Path)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return "", exitFailure
	}

	switch {
	case here && where == gitrepo.Elsewhere:
		return "", refuseNotHere(stderr, c, repo, cwd)
	case here:
		if where == gitrepo.Linked {
			// The line that was typed says "here"; it does not say where here
			// is. Saying it makes the tree that was acted on answerable from
			// weir's own output.
			fmt.Fprintf(stdout, "%s: %s で打ちます（%s の worktree）。\n", c.name, tree, repo.Name)
		}
		return tree, exitOK
	case where == gitrepo.Linked:
		return "", refuseInWorktree(stderr, c, repo, tree)
	default:
		return repo.Path, exitOK
	}
}

// refuseInWorktree stops a command run inside a worktree of the repository it
// names, with no --here.
//
// Acting on the registered path here would work on a tree the person cannot see
// from where they are standing — a commit of whatever happened to be staged
// somewhere else. Refusing is the only answer that cannot be the wrong one.
func refuseInWorktree(stderr io.Writer, c worktreeCommand, repo config.Repo, tree string) int {
	fmt.Fprintf(stderr, "%s: どの作業ツリーで%sするのかが決まらないので、%sしませんでした。\n\n", c.name, c.verb, c.verb)
	fmt.Fprintf(stderr, "  いまいる作業ツリー: %s\n", tree)
	fmt.Fprintf(stderr, "  リポジトリ本体:     %s\n", repo.Path)
	fmt.Fprintf(stderr, `
いまいるのは %s から切った worktree です。--here が無いとき weir が打つのは本体なので、
このまま通すと、いま見ているものとは別の作業ツリーを%sすることになります。

次にすること:
  1. いまいる作業ツリーで%sするなら、--here を付けて打ち直す:

  %s

  2. 本体で%sするなら、worktree の外へ出てから打つ:

  %s
`, repo.Name, c.verb, c.verb,
		fmt.Sprintf(c.hereFormat, repo.Name), c.verb, fmt.Sprintf(c.format, repo.Name))
	return exitUsage
}

// refuseNotHere stops a --here that has no here to act on: the command was run
// somewhere that is not a working tree of the repository it named.
//
// Falling back to the registered path would make --here mean nothing wherever
// it happened to be wrong, which is the one place it needs to mean something.
func refuseNotHere(stderr io.Writer, c worktreeCommand, repo config.Repo, cwd string) int {
	fmt.Fprintf(stderr, "%s: --here が付いていますが、いまいるのは %s の作業ツリーではないので、%sしませんでした。\n\n",
		c.name, repo.Name, c.verb)
	fmt.Fprintf(stderr, "  いまの作業ディレクトリ: %s\n", cwd)
	fmt.Fprintf(stderr, "  リポジトリ本体:         %s\n", repo.Path)
	fmt.Fprintf(stderr, `
--here は「いまいる作業ツリーで打つ」という指定です。%s と関係のない場所では、
どこで打てばよいのかが決まりません。

次にすること:
  1. %s の作業ツリー（本体か、そこから切った worktree）へ移ってから打つ
  2. いまの場所のまま本体で%sするなら、--here を外す:

  %s
`, repo.Name, repo.Name, c.verb, fmt.Sprintf(c.format, repo.Name))
	return exitUsage
}
