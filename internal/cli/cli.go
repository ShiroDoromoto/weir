// Package cli dispatches weir's subcommands.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/ShiroDoromoto/weir/internal/hook"
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
  check      フックからの入力を読み、止めるものだけを拒否する
  commit     登録したリポジトリでコミットする
  repos      登録されているリポジトリを一覧する
  version    版を表示する
  help       この使い方を表示する
`

// agent is the agent weir speaks to. There is one, and swapping it is the
// whole point of the adapter: nothing past this line knows Claude Code's shape.
var agent hook.Adapter = hook.ClaudeCode{}

// unreadableInput is what the gate says when it cannot read the hook's input.
// It cannot tell what was about to run, so it stops: a gate that opens when it
// cannot see is not a gate.
const unreadableInput = `weir はフックからの入力を読めませんでした（%v）。
何が実行されようとしていたのか判断できないため、止めています。

settings.json の PreToolUse フックが weir check を呼んでいるか、
そのフックが標準入力を weir へそのまま渡しているかを確認してください。

weir check が受け取るのは、次の形の JSON です:
  {"cwd":"/path/to/repo","tool_name":"Bash","tool_input":{"command":"git commit -m ..."}}`

// unreadableCommand is what the gate says when the command line will not come
// apart into words. weir cannot see whether a commit or a push is in there, so
// it stops for the same reason it stops unreadable hook input.
const unreadableCommand = `weir はコマンドを読めませんでした（%v）。
commit や push が含まれているのか判断できないため、止めています。

開いたままの引用符を閉じて、もう一度実行してください。
コミットや push のつもりだったのなら、weir から実行してください。

  %s`

// stoppedByTheGate is what the gate says when it stops plain git. It has to
// carry three things — why it stopped, what to do instead, and a line that
// works — or the reader is turned away without being told the way through.
const stoppedByTheGate = `weir は素の git の %s を止めました。

素の git は weir を通らないため、設定に書かれた規則との照合を受けないまま外へ出てしまいます。
weir から実行してください。対象はパスではなく名前で指定します
（名前は ~/.weir/config.toml の [repos.<名前>] に書かれているものです）。

  %s`

// The line to run instead, one per operation weir stops.
const (
	commitExample = `weir commit --repo <リポジトリ名> --message "変更の説明"`
	pushExample   = `weir push --repo <リポジトリ名>`
)

// Run dispatches args (os.Args[1:]) and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdin, stdout, stderr)
	case "commit":
		return runCommit(args[1:], stdout, stderr)
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

// runCheck reads one hook payload from stdin and answers on stdout. The
// judgement travels in that answer and nowhere else: check exits 0 whether it
// stopped the call or not, so a non-zero exit means weir itself failed and can
// never be mistaken for a refusal.
func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	// A mis-wired hook that hands check arguments must not look like a pass:
	// exiting 0 in silence here would open the gate for everything.
	if len(args) != 0 {
		fmt.Fprintf(stderr, "weir check: 引数は取りません: %s\n", strings.Join(args, " "))
		return exitUsage
	}

	reason, stop := judge(stdin)
	if !stop {
		// What weir does not stop, it says nothing about — the agent's own
		// permission check is what decides to run it.
		return exitOK
	}

	if err := agent.WriteDenial(stdout, reason); err != nil {
		fmt.Fprintf(stderr, "weir check: 拒否を書き出せません: %v\n", err)
		return exitFailure
	}
	return exitOK
}

// judge reads one hook payload and answers with the reason to stop the call,
// or with stop=false when weir has nothing to say about it. Everything it
// cannot read, it stops on: not knowing what is about to run is not a reason
// to let it run.
func judge(stdin io.Reader) (reason string, stop bool) {
	req, err := agent.Decode(stdin)
	if err != nil {
		return fmt.Sprintf(unreadableInput, err), true
	}

	op, err := hook.StoppedGitOperation(req.Command)
	if err != nil {
		return fmt.Sprintf(unreadableCommand, err, commitExample), true
	}

	switch op {
	case hook.GitCommit:
		return fmt.Sprintf(stoppedByTheGate, op, commitExample), true
	case hook.GitPush:
		return fmt.Sprintf(stoppedByTheGate, op, pushExample), true
	}
	return "", false
}
