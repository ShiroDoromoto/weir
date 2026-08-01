package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/ShiroDoromoto/weir/internal/config"
	"github.com/ShiroDoromoto/weir/internal/gitrepo"
)

const configUsage = `weir config — 設定を扱う。

使い方:
  weir config check    設定を検査して報告する
`

const configCheckUsage = `weir config check — 設定を検査して報告する。

使い方:
  weir config check

見るもの:
  ~/.weir/config.toml が読めるか（構文・項目の綴り・path の形・規則の書き方）
  ~/.weir/denylist が読めるか
  登録された各リポジトリが、そこに実在する git リポジトリの本体か

各リポジトリに何件の規則が効くかも並べます。

問題が1つでもあれば、非ゼロで終わります。
`

// runConfig dispatches the `config` subcommands.
func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, "weir config: 何をするかがありません\n")
		fmt.Fprint(stderr, "\n  weir config check\n")
		return exitUsage
	}

	switch args[0] {
	case "check":
		return runConfigCheck(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, configUsage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "weir config: 知らないコマンドです: %s\n", args[0])
		fmt.Fprint(stderr, "\n  weir config check\n")
		return exitUsage
	}
}

// runConfigCheck says what is wrong with the configuration, and where. Every
// other command refuses on a broken configuration and stops there — correctly,
// since a gate that reads a broken configuration as "no rules" opens on the day
// it breaks. But a refusal names the first thing it tripped on, and the reader
// is left fixing one line at a time. This one walks the whole thing and reports
// what it found.
//
// It reads and never writes. Nothing here repairs a configuration: what is in
// there is the human's, and a check that edited it would be answering a
// question nobody asked.
func runConfigCheck(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			fmt.Fprint(stdout, configCheckUsage)
			return exitOK
		}
		fmt.Fprintf(stderr, "weir config check: 引数は取りません: %s\n", strings.Join(args, " "))
		fmt.Fprint(stderr, "\n  weir config check\n")
		return exitUsage
	}

	path, err := config.Path()
	if err != nil {
		fmt.Fprintf(stderr, "weir config check: %v\n", err)
		return exitFailure
	}
	denyPath, err := config.DenylistPath()
	if err != nil {
		fmt.Fprintf(stderr, "weir config check: %v\n", err)
		return exitFailure
	}
	// Both files, named before anything is read: what weir matched against is
	// answerable only if the reader knows which files it came out of.
	fmt.Fprintf(stdout, "設定: %s\n", path)
	fmt.Fprintf(stdout, "語の一覧: %s\n\n", denyPath)

	// The configuration failing to load is the check's answer, not an error in
	// running it — so it goes to stdout with everything else the check has to
	// say. The exit code is what tells a caller it did not pass.
	cfg, err := config.LoadFile(path)
	if err != nil {
		fmt.Fprintf(stdout, "  ✗ %v\n", err)
		fmt.Fprint(stdout, "\n問題が1件あります。\n")
		return exitFailure
	}

	// What applies everywhere, said once. A repository's own rules are added to
	// these and can never take one away, so this line is the floor.
	defaults := len(cfg.DefaultRules())
	fmt.Fprintf(stdout, "  既定の規則 %d件（すべてのリポジトリに効きます）\n\n", defaults)

	names := cfg.Names()
	if len(names) == 0 {
		fmt.Fprintln(stdout, "  登録されているリポジトリはありません。")
		fmt.Fprintln(stdout, "  `[repos.<名前>]` と `path = \"/絶対/パス\"` を書いてください。")
		// Nothing registered is a configuration weir can read and act on — an
		// empty one. It is not a fault, so it does not fail the check.
		fmt.Fprint(stdout, "\n問題はありません。\n")
		return exitOK
	}

	problems := 0
	// One block, flushed once: every line goes through the tabwriter, so the
	// reasons line up under the paths instead of starting a column of their
	// own. A reason is the last cell on its line, so a long one widens nothing.
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, name := range names {
		repo := cfg.Repos[name]
		fmt.Fprintf(w, "  %s\t%s\n", name, repo.Path)
		// RulesFor cannot fail on a name that came out of Names(), so the
		// error here is unreachable — reported rather than dropped, since a
		// check that swallowed one would be checking nothing.
		rules, err := cfg.RulesFor(name)
		if err != nil {
			fmt.Fprintf(w, "  \t✗ %v\n", err)
			problems++
		} else {
			fmt.Fprintf(w, "  \t規則 %d件（既定 %d + このリポジトリ %d）\n",
				len(rules), defaults, len(repo.Rules))
		}
		if err := gitrepo.CheckPath(repo.Path); err != nil {
			fmt.Fprintf(w, "  \t✗ %v\n", err)
			problems++
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(stderr, "weir config check: 結果を書き出せません: %v\n", err)
		return exitFailure
	}

	if problems == 0 {
		fmt.Fprint(stdout, "\n問題はありません。\n")
		return exitOK
	}
	fmt.Fprintf(stdout, "\n問題が%d件あります。%s を直してください。\n", problems, path)
	return exitFailure
}
