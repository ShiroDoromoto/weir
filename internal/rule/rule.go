// Package rule is the shape a rule can take, and nothing more. There is no
// word, no regexp and no path in here: weir carries no rules of its own, so
// what it matches against comes entirely from ~/.weir/config.toml and
// ~/.weir/denylist. Anything built in would be a rule nobody wrote, and reading
// the configuration would stop answering "what was I matched against".
//
// Reading rules is internal/config's. This package says what one is, and
// whether weir can act on it.
package rule

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Kind is what a rule is matched against.
type Kind string

const (
	// Literal is a word. It matches anywhere in the text, ignoring case: a
	// name turns up inflected, and joined to other words, so a word boundary
	// would miss it.
	Literal Kind = "literal"
	// Pattern is a regular expression, in Go's own dialect (RE2). No
	// backreferences, no lookahead — and no catastrophic backtracking, which
	// matters for something that runs on every commit.
	Pattern Kind = "pattern"
	// Path is a glob, matched against the paths a commit changes. A glob
	// rather than a regexp, so the configuration stays readable by eye.
	Path Kind = "path"
)

// Action is what a match does.
type Action string

const (
	// Block refuses.
	Block Action = "block"
	// Warn does not refuse. It shows what matched and carries on: a rule that
	// works by inference will misfire, and refusals that misfire are the ones
	// people learn to ignore.
	Warn Action = "warn"
)

// Rule is one rule, as written.
type Rule struct {
	// Kind is what Value is matched against.
	Kind Kind
	// Action is what a match does.
	Action Action
	// Value is the word, the regular expression, or the glob — exactly as it
	// was written.
	Value string
	// Source is where it was written: a file and a line, or a file and a
	// table. A refusal has to name the rule that caused it, and naming where
	// it is written is how it does that without quoting what matched.
	Source string
}

// Check reports why weir cannot act on this rule.
//
// It is run when the configuration is read, not when a commit is judged. A
// regular expression that does not compile has to stop weir at the door, while
// the reader can still be pointed at the line that is wrong — not halfway
// through a commit, and never by being quietly skipped.
func (r Rule) Check() error {
	if strings.TrimSpace(r.Value) == "" {
		return errors.New("中身がありません (照合するものを書いてください)")
	}
	switch r.Kind {
	case Literal:
		return nil
	case Pattern:
		if _, err := regexp.Compile(r.Value); err != nil {
			return fmt.Errorf("正規表現として読めません: %w (Go の regexp と同じ書き方です。後方参照と先読みは使えません)", err)
		}
		return nil
	case Path:
		// Matching an empty name is not the question — it is the one way to
		// ask the pattern itself whether it is well formed.
		if _, err := path.Match(r.Value, ""); err != nil {
			return fmt.Errorf("glob として読めません: %w (`*` `?` `[...]` が使えます。`[` の閉じ忘れを確かめてください)", err)
		}
		return nil
	default:
		return fmt.Errorf("知らない種類です: %q (`literal` / `pattern` / `path` のどれかです)", r.Kind)
	}
}
