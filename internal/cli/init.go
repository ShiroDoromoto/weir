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

# リポジトリを名前で登録します。weir commit は、ここに書いた名前でしか
# リポジトリを指せません。
#
# 例（この行は # で始まるので効きません。外して書き換えてください）:
#
#   [repos.weir]
#   path = "/Users/あなた/develop/weir"
#
# path は絶対パスで書いてください。~ も環境変数も展開しません。
`

	denylistTemplate = `# weir が拒否する語を、1行に1つ書きます。
# # で始まる行と空行は読み飛ばします。
# 大小文字は区別せず、部分一致で照合します。
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
