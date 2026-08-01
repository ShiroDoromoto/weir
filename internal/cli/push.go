package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ShiroDoromoto/weir/internal/config"
	"github.com/ShiroDoromoto/weir/internal/gitcmd"
)

// pushExample is the line every refusal ends with — the one that works.
const pushExample = `weir push --repo <リポジトリ名>`

const pushUsage = `weir push — 登録したリポジトリでプッシュする。

使い方:
  weir push --repo <リポジトリ名>

オプション:
  --repo <名前>     対象のリポジトリ名（~/.weir/config.toml の [repos.<名前>]）

送り先は指定しません。素の git と同じく、いまのブランチの upstream に送ります。
`

// runPush pushes in the repository named by --repo. The name is the only way
// in, as with commit; and the destination is not weir's to name — it is git's
// default, so weir and plain git can never disagree about where a push went.
func runPush(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("weir push", flag.ContinueOnError)
	// weir says its own refusals, in its own words and with the way through.
	flags.SetOutput(io.Discard)
	repoName := flags.String("repo", "", "対象のリポジトリ名")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, pushUsage)
			return exitOK
		}
		return refusePush(stderr, fmt.Sprintf(
			"オプションを読めません（%v）。weir push が取るのは --repo だけです", err))
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

	if err := gitcmd.Push(repo.Path, stdout, stderr); err != nil {
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
