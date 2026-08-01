// Package config reads ~/.weir/config.toml and ~/.weir/denylist — the one place
// weir's settings live — resolves a repository by the name written there, and
// says which rules apply to it.
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
// It only reads. Writing the configuration is the human's: `weir init` lays
// down a template when there is none, and nothing in weir edits a file that is
// already there.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/ShiroDoromoto/weir/internal/rule"
)

// Where the configuration lives. Not overridable: one path, so what weir read
// is answerable without knowing how it was launched. Tests point HOME at a
// temporary directory instead.
//
// The words to refuse live beside config.toml rather than inside it, so that
// showing someone the configuration does not mean showing them the list of
// names.
const (
	DirName      = ".weir"
	FileName     = "config.toml"
	DenylistName = "denylist"
)

// ErrNotFound reports that the configuration file is not there at all — told
// apart from a broken one, because the way out differs (`weir init` versus a
// fix).
var ErrNotFound = errors.New("設定ファイルがありません")

// RuleSpec is one rule as it is written in config.toml — an entry under
// [[rules]], or one under [[repos.<name>.rules]]. It is the raw shape and
// nothing else; normalize is what turns a checked one into a rule.Rule.
//
// Every field is required. weir does not fill one in: a rule whose action was
// supplied by the binary is a rule you cannot read off the configuration.
type RuleSpec struct {
	// Type is the rule's kind — `pattern` or `path`. Words are not written
	// here; they go in ~/.weir/denylist, one per line.
	Type string `toml:"type"`
	// Value is the regular expression or the glob.
	Value string `toml:"value"`
	// Action is `block` (refuse) or `warn` (show it and carry on).
	Action string `toml:"action"`
}

// Repo is one [repos.<name>] table.
type Repo struct {
	// Name is the table's key — the name a command names this repository by.
	Name string `toml:"-"`
	// Path is the repository's absolute path.
	Path string `toml:"path"`
	// Rules are this repository's own rules, as written under
	// [[repos.<name>.rules]]. They are added to the defaults; there is no way
	// to switch a default off from here.
	Rules []RuleSpec `toml:"rules"`

	// rules is Rules once it has been checked. Filled at load, so a rule weir
	// cannot act on stops the whole configuration rather than one commit.
	rules []rule.Rule
}

// Config is what ~/.weir/config.toml holds.
type Config struct {
	// Rules are the rules that apply to every repository, as written under
	// [[rules]].
	Rules []RuleSpec      `toml:"rules"`
	Repos map[string]Repo `toml:"repos"`

	// defaults is what applies everywhere, checked at load: the words from
	// ~/.weir/denylist, then Rules.
	defaults []rule.Rule
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

// DenylistPath returns ~/.weir/denylist.
func DenylistPath() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	return denylistPathFor(path), nil
}

// denylistPathFor is where the words live for a given configuration file:
// beside it. One rule for both, so what LoadFile read and what a command tells
// the human it read cannot drift apart.
func denylistPathFor(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), DenylistName)
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

// normalize fills in each repository's name, reads the words that go with this
// configuration, and rejects anything weir could not act on.
func (c *Config) normalize(path string) error {
	denyPath := denylistPathFor(path)
	words, err := loadDenylist(denyPath)
	if err != nil {
		return err
	}
	defaults, err := rulesFrom(c.Rules, path+" の [[rules]]", denyPath)
	if err != nil {
		return err
	}
	c.defaults = append(words, defaults...)

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

		repo.rules, err = rulesFrom(repo.Rules, fmt.Sprintf("%s の [[repos.%s.rules]]", path, name), denyPath)
		if err != nil {
			return err
		}
		repo.Name = name
		c.Repos[name] = repo
	}
	return nil
}

// rulesFrom checks one table of rules and turns it into rules weir can act on.
// where names that table for the reader, and each entry is numbered from 1 —
// TOML gives an array entry no other name to be called by.
func rulesFrom(specs []RuleSpec, where, denyPath string) ([]rule.Rule, error) {
	rules := make([]rule.Rule, 0, len(specs))
	for i, spec := range specs {
		at := fmt.Sprintf("%s の%d番目", where, i+1)

		var kind rule.Kind
		switch spec.Type {
		case string(rule.Pattern), string(rule.Path):
			kind = rule.Kind(spec.Type)
		case "":
			return nil, fmt.Errorf(
				"%s: type がありません (`type = \"pattern\"`（正規表現）か `type = \"path\"`（変更されたパス）を書いてください)",
				at)
		case string(rule.Literal):
			// Keeping the words in one file of their own is the point: showing
			// someone the configuration should not mean showing them the list
			// of names.
			return nil, fmt.Errorf(
				"%s: 語は設定ファイルには書けません (拒否する語は %s に1行1語で書いてください)",
				at, denyPath)
		default:
			return nil, fmt.Errorf(
				"%s: type が知らない値です: %q (`pattern` か `path` を書いてください。語は %s に書きます)",
				at, spec.Type, denyPath)
		}

		var action rule.Action
		switch spec.Action {
		case string(rule.Block), string(rule.Warn):
			action = rule.Action(spec.Action)
		case "":
			return nil, fmt.Errorf(
				"%s: action がありません (`action = \"block\"`（拒否する）か `action = \"warn\"`（拒否しない）を書いてください)",
				at)
		default:
			return nil, fmt.Errorf(
				"%s: action が知らない値です: %q (`block` か `warn` を書いてください)",
				at, spec.Action)
		}

		r := rule.Rule{Kind: kind, Action: action, Value: spec.Value, Source: at}
		if err := r.Check(); err != nil {
			return nil, fmt.Errorf("%s: %w", at, err)
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// loadDenylist reads the words to refuse, one per line. A blank line, and a
// line starting with #, is not a word.
//
// Every failure is an error, the missing file included. A gate that read an
// absent or unreadable word list as "no words" would stop refusing on exactly
// the day the file went missing — silently, and at the moment it mattered.
//
// The words carry `block`: this file is the list of what to refuse, and it has
// no column to say otherwise. `warn` exists for rules that work by inference
// and will misfire; a name written out in full is not one of those.
func loadDenylist(path string) ([]rule.Rule, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf(
				"拒否する語の一覧がありません: %s (`weir init` で雛形を作れます。既にある設定ファイルには触りません)",
				path)
		}
		return nil, fmt.Errorf(
			"拒否する語の一覧が読めません: %s: %w (1行1語のテキストファイルとして、読める形で置いてください)",
			path, err)
	}
	if !utf8.Valid(src) {
		return nil, fmt.Errorf(
			"%s: UTF-8 として読めません (1行1語、UTF-8 で書いてください)", path)
	}

	var rules []rule.Rule
	for i, line := range strings.Split(string(src), "\n") {
		word := strings.TrimSpace(line)
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		rules = append(rules, rule.Rule{
			Kind:   rule.Literal,
			Action: rule.Block,
			Value:  word,
			Source: fmt.Sprintf("%s:%d", path, i+1),
		})
	}
	return rules, nil
}

// DefaultRules returns the rules that apply to every repository — the words
// from ~/.weir/denylist, then what is written under [[rules]].
func (c *Config) DefaultRules() []rule.Rule {
	return append([]rule.Rule(nil), c.defaults...)
}

// RulesFor returns the rules that apply to the named repository: everything
// that applies everywhere, then that repository's own.
//
// A repository's rules are added to the defaults and can never switch one off.
// That is what makes the defaults worth reading: they answer "what applies at
// minimum, everywhere" on their own, without going through every repository's
// table first.
func (c *Config) RulesFor(name string) ([]rule.Rule, error) {
	repo, err := c.Repo(name)
	if err != nil {
		return nil, err
	}
	rules := make([]rule.Rule, 0, len(c.defaults)+len(repo.rules))
	rules = append(rules, c.defaults...)
	rules = append(rules, repo.rules...)
	return rules, nil
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
