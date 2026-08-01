package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ShiroDoromoto/weir/internal/config"
	"github.com/ShiroDoromoto/weir/internal/gitcmd"
	"github.com/ShiroDoromoto/weir/internal/scan"
)

// The line every refusal ends with — the one that works. The repository is a
// placeholder in commitExample because a refusal can come before weir knows
// which one was meant; where it does know, the format is filled in with the
// name, so the reader gets a line they can run rather than one to edit.
const (
	commitFormat     = `weir commit --repo %s --message "変更の説明"`
	commitHereFormat = `weir commit --repo %s --here --message "変更の説明"`
)

var commitExample = fmt.Sprintf(commitFormat, "<リポジトリ名>")

// commitCommand is what workdir says about this command when it refuses.
var commitCommand = worktreeCommand{
	name:       "weir commit",
	verb:       "コミット",
	format:     commitFormat,
	hereFormat: commitHereFormat,
}

const commitUsage = `weir commit — 登録したリポジトリでコミットする。

使い方:
  weir commit --repo <リポジトリ名> --message "変更の説明" [--all] [--here]

オプション:
  --repo <名前>     対象のリポジトリ名（~/.weir/config.toml の [repos.<名前>]）
  --message <文>    コミットメッセージ
  --all             ステージされていない変更も対象にする
  --here            いまいる作業ツリーでコミットする（worktree で作業しているとき）

--here が無いときにコミットするのは、設定に書かれたリポジトリ本体です。--repo で名指した
リポジトリの worktree の中で --here 無しに打つと、どちらのことか決まらないので拒否します。
`

// runCommit commits in the repository named by --repo. The name is the only
// way in: weir does not read the working directory or guess a target from what
// was typed, so what it acted on is answerable from the command line alone.
func runCommit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("weir commit", flag.ContinueOnError)
	// weir says its own refusals, in its own words and with the way through.
	flags.SetOutput(io.Discard)
	repoName := flags.String("repo", "", "対象のリポジトリ名")
	message := flags.String("message", "", "コミットメッセージ")
	all := flags.Bool("all", false, "ステージされていない変更も対象にする")
	here := flags.Bool("here", false, "いまいる作業ツリーでコミットする")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, commitUsage)
			return exitOK
		}
		// Most of these are a git habit — -m, -a — arriving at a command that
		// is not git. Say what weir does take, rather than only what it does not.
		return refuseCommit(stderr, fmt.Sprintf(
			"オプションを読めません（%v）。weir commit が取るのは --repo / --message / --all / --here だけです", err))
	}
	if flags.NArg() != 0 {
		return refuseCommit(stderr, fmt.Sprintf("余分な引数があります: %s", strings.Join(flags.Args(), " ")))
	}
	if *repoName == "" {
		return refuseCommit(stderr, "--repo がありません。どのリポジトリでコミットするのかを名前で指定してください")
	}
	if *message == "" {
		return refuseCommit(stderr, "--message がありません。何をした変更なのかを書いてください")
	}

	cfg, err := config.Load()
	if err != nil {
		// The configuration itself is what needs fixing, and it says how.
		// Repeating the command's own shape here would point at the wrong thing.
		fmt.Fprintf(stderr, "weir commit: %v\n", err)
		return exitFailure
	}

	repo, err := cfg.Repo(*repoName)
	if err != nil {
		return refuseCommit(stderr, err.Error())
	}

	// Which working tree, before anything is read: what is judged and what is
	// committed have to be the one tree, and this is where that is settled.
	dir, code := workdir(commitCommand, repo, *here, stdout, stderr)
	if dir == "" {
		return code
	}

	matchers, err := rulesFor(cfg, *repoName)
	if err != nil {
		fmt.Fprintf(stderr, "weir commit: %v\n", err)
		return exitFailure
	}

	if len(matchers) == 0 {
		noRules(stdout, "weir commit")
	}

	// What would be committed is read before anything is committed. A gate that
	// judges after the fact is not a gate.
	surface, err := scan.Commit(dir, *message, *all)
	if err != nil {
		// Nothing was read, so nothing was judged. Committing anyway would be
		// passing something weir never looked at.
		fmt.Fprintf(stderr, "weir commit: %v\n", err)
		return exitFailure
	}

	blocked, warned := judge(matchers, surface)
	if len(blocked) > 0 {
		return refuseMatched(stderr, "weir commit", "コミット",
			"上の場所から、規則に当たるものを取り除く", commitExample, blocked)
	}
	warn(stdout, "weir commit", warned)

	if err := gitcmd.Commit(dir, *message, *all, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "weir commit: %v\n", err)
		return exitFailure
	}
	return exitOK
}

// refuseCommit says why the call was refused and shows a line that works. A
// refusal without the way through leaves the reader stopped and no wiser.
func refuseCommit(stderr io.Writer, why string) int {
	fmt.Fprintf(stderr, "weir commit: %s\n", why)
	fmt.Fprintf(stderr, "\n  %s\n", commitExample)
	return exitUsage
}
