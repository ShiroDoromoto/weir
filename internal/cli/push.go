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

// The line every refusal ends with — the one that works. See commit.go for why
// there is both a placeholder and a format.
const (
	pushFormat     = `weir push --repo %s`
	pushHereFormat = `weir push --repo %s --here`
)

var pushExample = fmt.Sprintf(pushFormat, "<リポジトリ名>")

// pushCommand is what workdir says about this command when it refuses.
var pushCommand = worktreeCommand{
	name:       "weir push",
	verb:       "プッシュ",
	format:     pushFormat,
	hereFormat: pushHereFormat,
}

const pushUsage = `weir push — 登録したリポジトリでプッシュする。

使い方:
  weir push --repo <リポジトリ名> [--here]

オプション:
  --repo <名前>     対象のリポジトリ名（~/.weir/config.toml の [repos.<名前>]）
  --here            いまいる作業ツリーからプッシュする（worktree で作業しているとき）

送り先は指定しません。素の git と同じく、いまのブランチの upstream に送ります。upstream が
まだ無くても、リモートが1つならそれが送り先です。2つ以上あるときは選ばずに拒否します。

--here が無いときにプッシュするのは、設定に書かれたリポジトリ本体です。worktree はブランチが
別なので、--repo で名指したリポジトリの worktree の中で --here 無しに打つと拒否します。
`

// runPush pushes in the repository named by --repo. The name is the only way
// in, as with commit; and the destination is not weir's to name — it is git's
// default, so weir and plain git can never disagree about where a push went.
func runPush(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("weir push", flag.ContinueOnError)
	// weir says its own refusals, in its own words and with the way through.
	flags.SetOutput(io.Discard)
	repoName := flags.String("repo", "", "対象のリポジトリ名")
	here := flags.Bool("here", false, "いまいる作業ツリーからプッシュする")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, pushUsage)
			return exitOK
		}
		return refusePush(stderr, fmt.Sprintf(
			"オプションを読めません（%v）。weir push が取るのは --repo / --here だけです", err))
	}
	if flags.NArg() != 0 {
		// `git push origin main` is the habit this catches. Saying only "extra
		// arguments" would leave the reader stripping them one at a time,
		// wondering which one weir wanted.
		return refusePush(stderr, fmt.Sprintf(
			"余分な引数があります: %s（送り先は指定しません。いまのブランチの upstream に送ります）",
			strings.Join(flags.Args(), " ")))
	}
	if *repoName == "" {
		return refusePush(stderr, "--repo がありません。どのリポジトリでプッシュするのかを名前で指定してください")
	}

	cfg, err := config.Load()
	if err != nil {
		// The configuration itself is what needs fixing, and it says how.
		// Repeating the command's own shape here would point at the wrong thing.
		fmt.Fprintf(stderr, "weir push: %v\n", err)
		return exitFailure
	}

	repo, err := cfg.Repo(*repoName)
	if err != nil {
		return refusePush(stderr, err.Error())
	}

	// Which working tree, before anything is read. A worktree is on its own
	// branch with its own upstream, so this decides what a push would even
	// send — not only where git is run.
	dir, code := workdir(pushCommand, repo, *here, stdout, stderr)
	if dir == "" {
		return code
	}

	matchers, err := rulesFor(cfg, *repoName)
	if err != nil {
		fmt.Fprintf(stderr, "weir push: %v\n", err)
		return exitFailure
	}

	if len(matchers) == 0 {
		noRules(stdout, "weir push")
	}

	surface, err := scan.Push(dir)
	if why := whyNoDestination(err); why != "" {
		return refuseNoDestination(stderr, *repoName, why)
	}
	if err != nil {
		fmt.Fprintf(stderr, "weir push: %v\n", err)
		return exitFailure
	}

	blocked, warned := judge(matchers, surface)
	if len(blocked) > 0 {
		// Editing the file is not enough here: what matched is in a commit
		// already made, and a push carries the history, not the working tree.
		return refuseMatched(stderr, "weir push", "プッシュ",
			"上のコミットから、規則に当たるものを取り除く（履歴に残っているので、"+
				"打ち直しが要ります: git commit --amend / git rebase -i）",
			pushExample, blocked)
	}
	warn(stdout, "weir push", warned)

	if err := gitcmd.Push(dir, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "weir push: %v\n", err)
		return exitFailure
	}
	return exitOK
}

// whyNoDestination says what to do about a push whose destination is not
// settled, and "" for every other error.
//
// scan answers with three of these rather than one because the way out of each
// is a different thing to type. A single "there is no upstream" would leave the
// reader with a repository that has no remote at all being told to set an
// upstream on one — advice that cannot be followed.
func whyNoDestination(err error) string {
	switch {
	case errors.Is(err, scan.ErrDetachedHead):
		return `いまブランチの上にいません（detached HEAD）。
weir は「送り先のリモートにまだ無いコミット」を送られるものとして見ます。ブランチが無いと
送り先が決まらないので、何が送られるのかも決まりません。

次にすること:
  1. ブランチの上へ戻る。いまの位置に立てるなら:

  git switch -c <ブランチ名>

  2. 戻ったら、もう一度:

`
	case errors.Is(err, scan.ErrNoRemote):
		return `このリポジトリにはリモートが1つもありません。
送り先が無いので、何が送られるのかも決まりません。

次にすること:
  1. リモートを足す:

  git remote add origin <URL>

  2. 足したら、もう一度:

`
	case errors.Is(err, scan.ErrManyRemotes):
		return `このブランチには送り先が書かれていないのに、リモートが2つ以上あります。
weir はどれに送るのかを推測しません。取り違えると、本当の送り先がまだ見ていないコミットを、
見落としたまま通すことになります。

次にすること:
  1. どのリモートへ送るのかを決める。送り先のブランチが既にリモートにあるなら:

  git branch --set-upstream-to=<リモート>/<ブランチ名>

  まだ無いなら、送らずに送り先だけ決められる:

  git config branch.<ブランチ名>.pushRemote <リモート>

  2. 決めたら、もう一度:

`
	}
	return ""
}

// refuseNoDestination stops a push weir could not read the surface of. A gate
// that passes what it never looked at is not one, so this is a refusal and not
// a warning — even where git would have refused the push anyway.
func refuseNoDestination(stderr io.Writer, repoName, why string) int {
	fmt.Fprint(stderr, "weir push: 何が送られるのかを読み出せないので、プッシュしませんでした。\n\n")
	fmt.Fprint(stderr, why)
	fmt.Fprintf(stderr, "  %s\n", fmt.Sprintf(pushFormat, repoName))
	return exitFailure
}

// refusePush says why the call was refused and shows a line that works. A
// refusal without the way through leaves the reader stopped and no wiser.
func refusePush(stderr io.Writer, why string) int {
	fmt.Fprintf(stderr, "weir push: %s\n", why)
	fmt.Fprintf(stderr, "\n  %s\n", pushExample)
	return exitUsage
}
