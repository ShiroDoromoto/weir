package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/ShiroDoromoto/weir/internal/config"
)

// runRepos lists what is registered, so what weir will act on can be checked
// without opening the configuration file — and, when the answer is nothing,
// says the thing the empty list would otherwise leave you to infer.
func runRepos(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintf(stderr, "weir repos: 引数は取りません: %s\n", strings.Join(args, " "))
		return exitUsage
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "weir repos: %v\n", err)
		return exitFailure
	}

	names := cfg.Names()
	if len(names) == 0 {
		fmt.Fprintln(stdout, "登録されているリポジトリはありません。")
		fmt.Fprintln(stdout, "~/.weir/config.toml に `[repos.<名前>]` と `path = \"/絶対/パス\"` を書いてください。")
		return exitOK
	}

	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, name := range names {
		fmt.Fprintf(w, "%s\t%s\n", name, cfg.Repos[name].Path)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(stderr, "weir repos: 一覧を書き出せません: %v\n", err)
		return exitFailure
	}
	return exitOK
}
