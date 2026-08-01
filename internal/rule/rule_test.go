package rule

import (
	"strings"
	"testing"
)

func TestCheckPassesRulesWeirCanActOn(t *testing.T) {
	tests := []struct {
		name string
		rule Rule
	}{
		{"a word", Rule{Kind: Literal, Action: Block, Value: "山田太郎"}},
		{"a word with a space in it", Rule{Kind: Literal, Action: Block, Value: "Contoso Ltd"}},
		{"a regular expression", Rule{Kind: Pattern, Action: Block, Value: `AKIA[0-9A-Z]{16}`}},
		{"a glob", Rule{Kind: Path, Action: Warn, Value: "secrets/*.pem"}},
		{"a glob that crosses directories", Rule{Kind: Path, Action: Warn, Value: "**/.env"}},
		{"a glob with alternatives", Rule{Kind: Path, Action: Warn, Value: "*.{pem,key}"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.rule.Check(); err != nil {
				t.Errorf("Check() = %v, want no error", err)
			}
		})
	}
}

// A rule weir cannot act on has to say so when the configuration is read. The
// alternative — finding out mid-commit, or skipping it quietly — is a rule that
// was written and never applied.
func TestCheckRefusesRulesWeirCannotActOn(t *testing.T) {
	tests := []struct {
		name string
		rule Rule
		want string // substring the message must carry
	}{
		{
			name: "nothing to match",
			rule: Rule{Kind: Literal, Action: Block, Value: ""},
			want: "中身がありません",
		},
		{
			name: "only whitespace",
			rule: Rule{Kind: Pattern, Action: Block, Value: "   "},
			want: "中身がありません",
		},
		{
			name: "a regular expression that does not compile",
			rule: Rule{Kind: Pattern, Action: Block, Value: "a(b"},
			want: "正規表現として読めません",
		},
		{
			name: "a glob that does not parse",
			rule: Rule{Kind: Path, Action: Warn, Value: "secrets/[a"},
			want: "glob として読めません",
		},
		{
			name: "a kind weir does not have",
			rule: Rule{Kind: Kind("word"), Action: Block, Value: "x"},
			want: "知らない種類です",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Check()
			if err == nil {
				t.Fatal("Check() = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Check() = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// RE2 is the whole dialect: what a regular expression cannot express here, it
// cannot express by being written some other way.
func TestCheckRefusesWhatRE2DoesNotHave(t *testing.T) {
	for _, value := range []string{`(a)\1`, `(?=x)`, `(?!x)`} {
		if err := (Rule{Kind: Pattern, Action: Block, Value: value}).Check(); err == nil {
			t.Errorf("Check() on %q = nil, want an error (RE2 has no backreference or lookahead)", value)
		}
	}
}

// mustCompile is the test's own way of getting a matcher for a rule that is
// meant to be usable. A rule that will not compile here is the test being
// wrong, not the code.
func mustCompile(t *testing.T, r Rule) Matcher {
	t.Helper()
	m, err := Compile(r)
	if err != nil {
		t.Fatalf("Compile(%+v) = %v, want no error", r, err)
	}
	return m
}

// A word matches anywhere in the text and ignores case, so it still lands on
// the forms a name actually turns up in.
func TestLiteralMatchesText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		text  string
		want  bool
	}{
		{"the word itself", "山田太郎", "山田太郎", true},
		{"joined to other words", "山田太郎", "担当は山田太郎さんです", true},
		{"in the middle of an identifier", "Contoso", "acmeContosoClient", true},
		{"ignoring case", "Contoso", "CONTOSO LTD", true},
		{"ignoring case the other way", "CONTOSO", "contoso ltd", true},
		{"a word that is not there", "Contoso", "Fabrikam Ltd", false},
		{"characters stay characters", "a.c", "abc", false},
		{"and match themselves", "a.c", "xa.cx", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mustCompile(t, Rule{Kind: Literal, Action: Block, Value: tt.value})
			if got := m.MatchesText(tt.text); got != tt.want {
				t.Errorf("MatchesText(%q) with %q = %v, want %v", tt.text, tt.value, got, tt.want)
			}
		})
	}
}

// A regular expression matches as Go's regexp does, over the whole text it is
// shown — which is many lines at once, so ^ and $ mean what they mean there.
func TestPatternMatchesText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		text  string
		want  bool
	}{
		{"a key shaped like the pattern", `AKIA[0-9A-Z]{16}`, "aws_key = AKIA1234567890ABCDEF", true},
		{"nothing shaped like it", `AKIA[0-9A-Z]{16}`, "aws_key = ${AWS_KEY}", false},
		{"case is not ignored unless asked", `secret`, "SECRET", false},
		{"case ignored when the rule asks", `(?i)secret`, "SECRET", true},
		{"over several lines", `token`, "一行目\n二行目に token\n三行目", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mustCompile(t, Rule{Kind: Pattern, Action: Block, Value: tt.value})
			if got := m.MatchesText(tt.text); got != tt.want {
				t.Errorf("MatchesText(%q) with %q = %v, want %v", tt.text, tt.value, got, tt.want)
			}
		})
	}
}

// The glob is doublestar's dialect: `**` crosses directories. How deep a
// repository will be laid out is not known when the rule is written, and a rule
// that quietly stops reaching is one nobody notices has stopped.
func TestPathMatchesPath(t *testing.T) {
	tests := []struct {
		name  string
		value string
		path  string
		want  bool
	}{
		{"right beside the pattern", "secrets/*.pem", "secrets/server.pem", true},
		{"one level deeper than a single star", "secrets/*.pem", "secrets/prod/server.pem", false},
		{"everything under a directory", "secrets/**", "secrets/prod/server.pem", true},
		{"a file at the root of that directory", "secrets/**", "secrets/server.pem", true},
		{"not a sibling of that directory", "secrets/**", "secretsish/server.pem", false},
		{"any level at all", "**/.env", "apps/api/.env", true},
		{"including the top level", "**/.env", ".env", true},
		{"not a file that merely ends that way", "**/.env", "apps/api/x.env", false},
		{"alternatives", "*.{pem,key}", "server.key", true},
		{"outside the alternatives", "*.{pem,key}", "server.crt", false},
		{"the whole path, not a piece of it", "*.pem", "secrets/server.pem", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mustCompile(t, Rule{Kind: Path, Action: Block, Value: tt.value})
			if got := m.MatchesPath(tt.path); got != tt.want {
				t.Errorf("MatchesPath(%q) with %q = %v, want %v", tt.path, tt.value, got, tt.want)
			}
		})
	}
}

// Each kind is asked its own question. A rule answers no to the other one
// rather than being stretched to cover it — a word standing in for a filename,
// or a glob read out of a diff, would be weir matching against something nobody
// wrote a rule for.
func TestEachKindAnswersOnlyItsOwnQuestion(t *testing.T) {
	word := mustCompile(t, Rule{Kind: Literal, Action: Block, Value: "secrets"})
	if word.MatchesPath("secrets") {
		t.Error("a literal rule matched a path, want it to match text only")
	}

	pattern := mustCompile(t, Rule{Kind: Pattern, Action: Block, Value: `secrets/.*\.pem`})
	if pattern.MatchesPath("secrets/server.pem") {
		t.Error("a pattern rule matched a path, want it to match text only")
	}

	glob := mustCompile(t, Rule{Kind: Path, Action: Block, Value: "secrets/**"})
	if glob.MatchesText("secrets/server.pem を追加した") {
		t.Error("a path rule matched text, want it to match paths only")
	}
}

// The matcher carries the rule it came from, because that is what a refusal
// names: where it is written, never what it matched.
func TestMatcherCarriesTheRuleItCameFrom(t *testing.T) {
	r := Rule{Kind: Literal, Action: Warn, Value: "Contoso", Source: "~/.weir/denylist:12"}
	if got := mustCompile(t, r).Rule; got != r {
		t.Errorf("Matcher.Rule = %+v, want %+v", got, r)
	}
}

// One rule weir cannot act on takes the whole set down, and says where that
// rule is. Judging a commit against the rules that happened to compile is
// judging it against fewer rules than were written for it.
func TestCompileAllStopsAtTheRuleItCannotRead(t *testing.T) {
	rules := []Rule{
		{Kind: Literal, Action: Block, Value: "Contoso", Source: "~/.weir/denylist:1"},
		{Kind: Pattern, Action: Block, Value: "a(b", Source: "~/.weir/config.toml の [[rules]] の1番目"},
		{Kind: Path, Action: Warn, Value: "secrets/**", Source: "~/.weir/config.toml の [[rules]] の2番目"},
	}

	matchers, err := CompileAll(rules)
	if err == nil {
		t.Fatal("CompileAll() = nil error, want an error")
	}
	if matchers != nil {
		t.Errorf("CompileAll() = %v matchers, want none alongside an error", len(matchers))
	}
	for _, want := range []string{"config.toml の [[rules]] の1番目", "正規表現として読めません"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CompileAll() = %q, want it to contain %q", err, want)
		}
	}
}

func TestCompileAllKeepsTheOrderItWasGiven(t *testing.T) {
	rules := []Rule{
		{Kind: Literal, Action: Block, Value: "Contoso"},
		{Kind: Pattern, Action: Warn, Value: `AKIA[0-9A-Z]{16}`},
		{Kind: Path, Action: Block, Value: "secrets/**"},
	}

	matchers, err := CompileAll(rules)
	if err != nil {
		t.Fatalf("CompileAll() = %v, want no error", err)
	}
	if len(matchers) != len(rules) {
		t.Fatalf("CompileAll() = %d matchers, want %d", len(matchers), len(rules))
	}
	for i, m := range matchers {
		if m.Rule != rules[i] {
			t.Errorf("matcher %d is %+v, want %+v", i, m.Rule, rules[i])
		}
	}
}
