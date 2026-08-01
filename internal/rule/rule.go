// Package rule is the shape a rule can take, and the matching that shape
// implies. There is no word, no regexp and no path in here: weir carries no
// rules of its own, so what it matches against comes entirely from
// ~/.weir/config.toml and ~/.weir/denylist. Anything built in would be a rule
// nobody wrote, and reading the configuration would stop answering "what was I
// matched against".
//
// Reading rules is internal/config's, and assembling what to look at is
// internal/scan's. This package says what a rule is, whether weir can act on
// it, and whether it matches what it is shown.
package rule

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
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
	// rather than a regexp, so the configuration stays readable by eye. The
	// dialect is doublestar's, the one .gitignore and editors use: `**`
	// crosses directories, so a rule written before the repository grew a
	// level still lands where it was meant to.
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
//
// It is Compile with the result thrown away, so what gets through the door is
// exactly what matches later. Reading a value twice — once to check it, once to
// use it — is how a rule that was accepted ends up never firing.
func (r Rule) Check() error {
	_, err := Compile(r)
	return err
}

// Matcher is a rule ready to be matched with, holding whatever its kind needs
// prepared. A commit is judged against every rule, over the message and over
// every file it touches, so anything built per call would be built again for
// each of them.
type Matcher struct {
	// Rule is the rule this came from, as written. A refusal names it: Source
	// is how the reader finds the line to change, without the message having
	// to quote what matched.
	Rule Rule

	// re is what Literal and Pattern are both matched with — a literal by way
	// of QuoteMeta, so its characters stay characters, and (?i), which is the
	// whole of "ignoring case". Nil for Path.
	re *regexp.Regexp
}

// Compile prepares one rule for matching, and says why it cannot be if it
// cannot. The reasons are Check's, because this is what Check runs.
func Compile(r Rule) (Matcher, error) {
	if strings.TrimSpace(r.Value) == "" {
		return Matcher{}, errors.New("中身がありません (照合するものを書いてください)")
	}
	switch r.Kind {
	case Literal:
		// QuoteMeta leaves nothing for the regexp to read as syntax, so a word
		// with `.` or `*` in it stays that word. What is left to fail is
		// nothing, but a swallowed error here would be a rule that never fires
		// and never says why.
		re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(r.Value))
		if err != nil {
			return Matcher{}, fmt.Errorf("語として読めません: %w", err)
		}
		return Matcher{Rule: r, re: re}, nil
	case Pattern:
		re, err := regexp.Compile(r.Value)
		if err != nil {
			return Matcher{}, fmt.Errorf("正規表現として読めません: %w (Go の regexp と同じ書き方です。後方参照と先読みは使えません)", err)
		}
		return Matcher{Rule: r, re: re}, nil
	case Path:
		if !doublestar.ValidatePattern(r.Value) {
			return Matcher{}, errors.New("glob として読めません (`*` `?` `**` `[...]` `{a,b}` が使えます。`[` `{` の閉じ忘れを確かめてください)")
		}
		return Matcher{Rule: r}, nil
	default:
		return Matcher{}, fmt.Errorf("知らない種類です: %q (`literal` / `pattern` / `path` のどれかです)", r.Kind)
	}
}

// CompileAll prepares every rule, and stops at the first one it cannot, naming
// where that one is written. The caller holds the whole set, and a set with one
// unusable rule in it is not a set weir can judge with: carrying on with the
// rest would judge a commit against fewer rules than were written for it.
func CompileAll(rules []Rule) ([]Matcher, error) {
	matchers := make([]Matcher, 0, len(rules))
	for _, r := range rules {
		m, err := Compile(r)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", r.Source, err)
		}
		matchers = append(matchers, m)
	}
	return matchers, nil
}

// MatchesText reports whether this rule matches a piece of text — the commit
// message, or the lines a file gains.
//
// A path rule never says yes here. It is written about where something is, not
// about what is in it, and a path that happens to appear inside a diff is not
// the commit touching that path.
func (m Matcher) MatchesText(text string) bool {
	if m.re == nil {
		return false
	}
	return m.re.MatchString(text)
}

// MatchesPath reports whether this rule matches a path the commit changes.
//
// Only a path rule says yes here. A word is matched against text wherever it
// appears; having it stand for a filename as well would be weir deciding a rule
// means something its writer did not write.
func (m Matcher) MatchesPath(path string) bool {
	if m.Rule.Kind != Path {
		return false
	}
	// The pattern was read when this was compiled, so the only error Match
	// returns is one that cannot arrive here.
	ok, _ := doublestar.Match(m.Rule.Value, path)
	return ok
}
