package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ShiroDoromoto/weir/internal/config"
)

// The templates weir lays down. Every line is commented out on purpose: weir
// carries no rules of its own, so a fresh install has to match nothing until
// the human writes something. A template that came with a rule already live
// would be a rule nobody chose.
const (
	configTemplate = `# weir の設定。
# weir はここに書かれたものだけで動きます。書かれていない規則は効きません。
# weir 自身は既定の規則を持っていません。
#
# この雛形はどの行もコメントアウトしてあります。効くのは # を外した行だけです。


# --- リポジトリを登録する ---------------------------------------------
#
# weir commit / weir push は、ここに書いた名前でしかリポジトリを指せません。
#
# 例:
#
#   [repos.weir]
#   path = "/Users/あなた/develop/weir"
#
# path は絶対パスで書いてください。~ も環境変数も展開しません。


# --- 規則を書く -------------------------------------------------------
#
# ここに書ける型は2つです。どちらも type / value / action の3つを書きます
# （どれも省略できません。weir が埋めると、設定を読んでも何が効くか分からなくなります）。
#
#   type = "pattern"   Go の正規表現（RE2）。コミットメッセージと、追加される行に当てます。
#                      後方参照と先読みは使えません。
#   type = "path"      glob。変更されるファイルのパスに当てます。
#                      * は / を跨ぎません。** は跨ぎます。? [...] {a,b} も使えます。
#
# 拒否する語（実名など）は、この設定ファイルには書けません。denylist に1行1語で書きます。
#
# action は2つです。
#
#   action = "block"   拒否します。実行しません。
#   action = "warn"    拒否しません。一致した場所を示して続行します。
#
# 推定に頼る規則（実名らしき形、帰属表記など）は warn で書いてください。
# 誤検知で拒否すると、やがて拒否そのものが無視されるようになります。
#
# 例:
#
#   [[rules]]
#   type = "pattern"
#   value = "AKIA[0-9A-Z]{16}"
#   action = "block"
#
#   [[rules]]
#   type = "path"
#   value = "**/.env"
#   action = "block"
#
# [[rules]] はすべてのリポジトリに効きます。1つのリポジトリにだけ足すなら、
# その名前の下に書きます。
#
#   [[repos.weir.rules]]
#   type = "path"
#   value = "docs/private/**"
#   action = "warn"
#
# リポジトリごとの規則は既定に追加されます。既定を無効化する書き方はありません。
`

	denylistTemplate = `# weir が拒否する語を、1行に1つ書きます。
# # で始まる行と空行は読み飛ばします。
#
# 大小文字は区別せず、部分一致で照合します（語境界は見ません）。
# 当てる先は、コミットメッセージと、追加される行です。ファイルのパスには当てません。
# ここに書いた語は必ず拒否します（block）。warn にはできません。
#
# 例（この行は # で始まるので効きません）:
#
#   山田太郎
#   acme-corp
#
# ここに書いた語だけが効きます。weir 自身は語を1つも持っていません。
`
)

const initUsage = `weir init — 設定の雛形を作る。

使い方:
  weir init

作るもの:
  ~/.weir/config.toml   リポジトリの登録先
  ~/.weir/denylist      拒否する語の一覧

既にあるファイルには触りません。何度打っても結果は変わりません。
`

// runInit lays down ~/.weir/config.toml and ~/.weir/denylist when they are not
// there, and leaves them alone when they are. The configuration is the human's
// writing; weir overwriting it would destroy the one thing it acts on. Not
// touching what exists is also what makes this safe to run again.
func runInit(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			fmt.Fprint(stdout, initUsage)
			return exitOK
		}
		// No --force, and nothing to point one at: the whole shape of init is
		// that it does not overwrite. Show the line that works.
		fmt.Fprintf(stderr, "weir init: 引数は取りません: %s\n", strings.Join(args, " "))
		fmt.Fprint(stderr, "\n  weir init\n")
		return exitUsage
	}

	dir, err := config.Dir()
	if err != nil {
		fmt.Fprintf(stderr, "weir init: %v\n", err)
		return exitFailure
	}
	// 0700: what goes in here is a list of names someone does not want said out
	// loud. No reason for the rest of the machine to read it.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintf(stderr, "weir init: %s を作れません: %v\n", dir, err)
		return exitFailure
	}

	files := []struct {
		path string
		body string
	}{
		{filepath.Join(dir, config.FileName), configTemplate},
		{filepath.Join(dir, config.DenylistName), denylistTemplate},
	}

	created := 0
	for _, f := range files {
		wrote, err := createIfAbsent(f.path, f.body)
		if err != nil {
			fmt.Fprintf(stderr, "weir init: %v\n", err)
			return exitFailure
		}
		if wrote {
			created++
			fmt.Fprintf(stdout, "作りました: %s\n", f.path)
			continue
		}
		fmt.Fprintf(stdout, "既にあります（触りません）: %s\n", f.path)
	}

	if created > 0 {
		fmt.Fprintf(stdout, "\n登録するリポジトリを %s に書いてください。\n",
			filepath.Join(dir, config.FileName))
	}
	return exitOK
}

// createIfAbsent writes body to path only when nothing is there, and reports
// whether it wrote. O_EXCL is what makes "only when absent" true at the moment
// of writing rather than at the moment of checking — a separate stat first
// would leave a window where an existing file is clobbered.
func createIfAbsent(path, body string) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("%s を作れません: %w", path, err)
	}
	defer f.Close()

	if _, err := io.WriteString(f, body); err != nil {
		return false, fmt.Errorf("%s に書けません: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("%s に書けません: %w", path, err)
	}
	return true, nil
}
