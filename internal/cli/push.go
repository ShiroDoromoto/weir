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

送り先は指定しません。素の git と同じく、いまのブランチの upstream に送ります。

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
	if errors.Is(err, scan.ErrNoUpstream) {
		// git may well refuse this push itself — but not always, and weir
		// cannot tell which git will do. What it can tell is that it has not
		// seen what would be sent, and a gate that passes what it never looked
		// at is not one.
		fmt.Fprintf(stderr, "weir push: 何が送られるのかを読み出せないので、プッシュしませんでした。\n\n")
		fmt.Fprint(stderr, `いまのブランチには upstream がありません。
weir は「送り先のリモートにまだ無いコミット」を送られるものとして見ます。upstream が無いと
送り先が決まらないので、何が送られるのかも決まりません。

次にすること:
  1. 送り先のブランチが既にリモートにあるなら、送らずに upstream だけを決める:

  git branch --set-upstream-to=<リモート>/<ブランチ名>

  2. 決めたら、もう一度:

`)
		fmt.Fprintf(stderr, "  %s\n", pushExample)
		return exitFailure
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

// refusePush says why the call was refused and shows a line that works. A
// refusal without the way through leaves the reader stopped and no wiser.
func refusePush(stderr io.Writer, why string) int {
	fmt.Fprintf(stderr, "weir push: %s\n", why)
	fmt.Fprintf(stderr, "\n  %s\n", pushExample)
	return exitUsage
}
