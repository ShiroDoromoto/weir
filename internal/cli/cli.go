// Package cli dispatches weir's subcommands.
package cli

import (
	"fmt"
	"io"

	"github.com/ShiroDoromoto/weir/internal/version"
)

// Exit codes. weir is a gate, so anything it could not answer for has to be
// distinguishable from a pass by the exit code alone.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

const usage = `weir — 設定に書かれた規則だけで、コミットと push を止める門。

使い方:
  weir <コマンド> [引数...]

コマンド:
  init       設定の雛形を作る
  config     設定を検査して報告する（weir config check）
  commit     登録したリポジトリでコミットする
  push       登録したリポジトリでプッシュする
  repos      登録されているリポジトリを一覧する
  version    版を表示する
  help       この使い方を表示する
`

// Run dispatches args (os.Args[1:]) and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "commit":
		return runCommit(args[1:], stdout, stderr)
	case "push":
		return runPush(args[1:], stdout, stderr)
	case "repos":
		return runRepos(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "weir %s\n", version.Version)
		return exitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "weir: 知らないコマンドです: %s\n\n", args[0])
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
}
