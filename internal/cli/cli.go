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

// Run dispatches args (os.Args[1:]) and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdin, stdout, stderr)
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

	if _, err := agent.Decode(stdin); err != nil {
		if err := agent.WriteDenial(stdout, fmt.Sprintf(unreadableInput, err)); err != nil {
			fmt.Fprintf(stderr, "weir check: 拒否を書き出せません: %v\n", err)
			return exitFailure
		}
		return exitOK
	}

	// Nothing is matched against the request yet, so nothing is stopped. What
	// weir does not stop, it says nothing about — the agent's own permission
	// check is what decides to run it.
	return exitOK
}
