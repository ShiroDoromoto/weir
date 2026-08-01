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
