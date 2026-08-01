// Package config reads ~/.weir/config.toml — the one place weir's settings
// live — and resolves a repository by the name written there.
//
// Two properties this package is built to keep:
//
//   - A repository is reached by name and by nothing else. There is no lookup
//     from a path, a working directory or a command string, so a target can
//     never be guessed into existence.
//   - Anything it cannot read or cannot make sense of is an error, never an
//     empty result. A gate that reads a broken configuration as "no rules"
//     would open on exactly the day the configuration breaks.
//
// It only reads. Writing the configuration is the human's, and weir has no
// subcommand that edits it.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Where the configuration lives. Not overridable: one path, so what weir read
// is answerable without knowing how it was launched. Tests point HOME at a
// temporary directory instead.
const (
	DirName  = ".weir"
	FileName = "config.toml"
)

// ErrNotFound reports that the configuration file is not there at all — told
// apart from a broken one, because the way out differs (`weir init` versus a
// fix).
var ErrNotFound = errors.New("設定ファイルがありません")

// Repo is one [repos.<name>] table.
type Repo struct {
	// Name is the table's key — the name a command names this repository by.
	Name string `toml:"-"`
	// Path is the repository's absolute path.
	Path string `toml:"path"`
}

// Config is what ~/.weir/config.toml holds.
type Config struct {
	Repos map[string]Repo `toml:"repos"`
}

// Dir returns ~/.weir.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ホームディレクトリが特定できません: %w", err)
	}
	return filepath.Join(home, DirName), nil
}

// Path returns ~/.weir/config.toml.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// Load reads ~/.weir/config.toml.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFile(path)
}

// LoadFile reads the configuration at path. Every failure — missing,
// unreadable, malformed, or shaped wrong — comes back as an error, so a caller
// that ignores it cannot end up holding an empty configuration that looks
// valid.
func LoadFile(path string) (*Config, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s (`weir init` で雛形を作れます)", ErrNotFound, path)
		}
		return nil, fmt.Errorf("設定ファイルが読めません: %s: %w", path, err)
	}

	var cfg Config
	md, err := toml.Decode(string(src), &cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: 設定ファイルの構文が壊れています: %w", path, err)
	}
	// An unknown key is a typo until proven otherwise, and a typo'd key is a
	// setting that silently does not apply. Say so instead of reading past it.
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		sort.Strings(keys)
		return nil, fmt.Errorf(
			"%s: weir が知らない項目があります: %s (綴りを確かめてください)",
			path, strings.Join(keys, ", "))
	}

	if err := cfg.normalize(path); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// normalize fills in each repository's name and rejects a table weir could not
// act on.
func (c *Config) normalize(path string) error {
	for name, repo := range c.Repos {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf(
				"%s: 名前のないリポジトリがあります (`[repos.<名前>]` の形で名前を書いてください)",
				path)
		}
		switch {
		case repo.Path == "":
			return fmt.Errorf(
				"%s: リポジトリ %q に path がありません (`[repos.%s]` の下に `path = \"/絶対/パス\"` を書いてください)",
				path, name, name)
		case !filepath.IsAbs(repo.Path):
			return fmt.Errorf(
				"%s: リポジトリ %q の path が絶対パスではありません: %s (`~` も環境変数も展開しません。`path = \"/絶対/パス\"` の形で書いてください)",
				path, name, repo.Path)
		}
		repo.Name = name
		c.Repos[name] = repo
	}
	return nil
}

// Names returns the registered repository names, sorted.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Repos))
	for name := range c.Repos {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Repo resolves a repository by the name it is registered under. A name that
// is not registered is refused — weir does not fall back to guessing one from
// a path or a working directory.
func (c *Config) Repo(name string) (Repo, error) {
	repo, ok := c.Repos[name]
	if !ok {
		return Repo{}, &UnknownRepoError{Name: name, Known: c.Names()}
	}
	return repo, nil
}

// UnknownRepoError reports a name that is not in the configuration.
type UnknownRepoError struct {
	Name  string
	Known []string
}

func (e *UnknownRepoError) Error() string {
	if len(e.Known) == 0 {
		return fmt.Sprintf(
			"リポジトリ %q は登録されていません。登録されているリポジトリは1つもありません (~/.weir/config.toml に `[repos.%s]` と `path = \"/絶対/パス\"` を書いてください)",
			e.Name, e.Name)
	}
	return fmt.Sprintf(
		"リポジトリ %q は登録されていません。登録されているのは: %s (--repo には、この一覧の名前を指定してください)",
		e.Name, strings.Join(e.Known, ", "))
}
