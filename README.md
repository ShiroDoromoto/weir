# weir

設定に書かれた規則だけで、コミットと push を止める門。

`git commit` の代わりに `weir commit` を、`git push` の代わりに `weir push` を呼びます。
weir は自分の規則を1つも持ちません。何を止めるかは、あなたが `~/.weir/config.toml` と
`~/.weir/denylist` に書いたものだけで決まります。バイナリの中に既定の規則は入っていません。

## 何をするか

- 登録したリポジトリでだけ、コミットと push を実行する。
- リポジトリは**名前**でしか指せない。作業ディレクトリからも、打たれたコマンドからも推測しない。
- 設定が読めなければ、実行しない。壊れた設定を「規則が0件」とは読まない。

## 何をしないか

- **手元の履歴操作は止めない**（`tag` / `reset --hard` / `rebase`）。やり直せる操作まで止めると、
  素の git を使う場面で邪魔になる。止めるのは、外に出たら取り返せない commit と push だけ。
- **エージェントに素の git を使わせるかどうかは決めない**。それはそのエージェントの設定の話で、
  weir の責務ではない。weir は git の代わりに呼ぶ門であって、コマンドの監視役ではない。
- **設定ファイルを書き換えから守らない**。weir はエージェントと同じユーザー権限で動くので、
  weir 側で書き換えを止めることは成立しない。成立しない防御は持たない。

## 入れる

git が要ります。weir は git をプログラムとして呼びます。

### Homebrew

```sh
brew install ShiroDoromoto/weir/weir
```

### インストールスクリプト

```sh
curl -fsSL https://github.com/ShiroDoromoto/weir/releases/latest/download/install.sh | sh
```

### Go

```sh
go install github.com/ShiroDoromoto/weir/cmd/weir@latest
```

## 始める

### 1. 雛形を作る

```sh
weir init
```

`~/.weir/config.toml` と `~/.weir/denylist` を、**無いときだけ**作ります。既にあるものには
触りません。何度打っても結果は変わりません。

雛形はどの行もコメントアウトしてあります。有効になるのは、あなたが `#` を外した行だけです。

### 2. リポジトリを登録する

`~/.weir/config.toml` に書きます。

```toml
[repos.weir]
path = "/Users/あなた/develop/weir"

[repos.notes]
path = "/Users/あなた/develop/notes"
```

- `path` は**絶対パス**で書きます。`~` も環境変数も展開しません。
- **worktree ではなく本体**のパスを書きます。worktree から打っても、weir は本体を見つけます。

### 3. 検査する

```sh
weir config check
```

設定を頭から終わりまで読んで、見つかった問題を全部並べます。1件でもあれば非ゼロで終わります。

```
設定: /Users/あなた/.weir/config.toml

  notes  /Users/あなた/develop/notes
         ✗ そのパスがありません
  weir   /Users/あなた/develop/weir

問題が1件あります。/Users/あなた/.weir/config.toml を直してください。
```

## コマンド

| コマンド | すること |
| --- | --- |
| `weir init` | 設定の雛形を作る（無いときだけ） |
| `weir config check` | 設定を検査して報告する |
| `weir repos` | 登録されているリポジトリを一覧する |
| `weir commit` | 登録したリポジトリでコミットする |
| `weir push` | 登録したリポジトリでプッシュする |
| `weir version` | 版を表示する |
| `weir help` | 使い方を表示する |

### weir commit

```sh
weir commit --repo <リポジトリ名> --message "変更の説明" [--all]
```

- `--repo` — 対象のリポジトリ名（`~/.weir/config.toml` の `[repos.<名前>]`）
- `--message` — コミットメッセージ
- `--all` — ステージされていない変更（追跡済みのファイル）も対象にする

git の `-m` / `-a` は取りません。名前を省くこともできません。どのリポジトリで何をしたのかが、
打った行だけで答えられる形にしてあります。

### weir push

```sh
weir push --repo <リポジトリ名>
```

送り先は指定しません。素の git と同じく、いまのブランチの upstream に送ります。weir と素の
git が「どこへ送ったか」で食い違わないようにするためです。

### weir repos

```sh
weir repos
```

登録されているリポジトリと、そのパスを並べます。設定ファイルを開かずに、weir が何を対象に
できるかを確かめられます。

## 設定

| 場所 | 中身 |
| --- | --- |
| `~/.weir/config.toml` | リポジトリの登録 |
| `~/.weir/denylist` | 拒否する語の一覧（1行1語。`#` で始まる行と空行は無視） |

置き場所は変えられません。環境変数でもオプションでも動かせないので、weir が何を読んだかは
常に1つに定まります。

`denylist` は**まだ読んでいません**。語・正規表現・パスの照合は、これからの版で入ります。
いまは `weir init` が雛形を置くだけです。

## 終了コード

| 値 | 意味 |
| --- | --- |
| 0 | 通した |
| 1 | 設定が読めない、検査で問題が見つかった、または git が失敗した |
| 2 | 打ち方が違う（知らないコマンド、足りない・余分な引数、登録されていないリポジトリ名） |

出力を読まなくても、終了コードだけで通ったかどうかが分かります。

## 開発

```sh
make check    # gofmt / go vet / go test / actionlint
make build    # bin/weir を焼く
```

CI も `make check` を呼びます。手元と CI で別のものは回しません。
