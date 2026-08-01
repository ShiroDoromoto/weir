package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/weir/internal/rule"
)

// writeConfig puts a config.toml, and a denylist with no words in it, in a
// fresh temporary HOME. It returns the config's path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	return writeStore(t, body, "# 語はまだ1つもない\n")
}

// writeStore puts both files in a fresh temporary HOME and returns the config's
// path. The words go beside the configuration, which is where weir reads them.
func writeStore(t *testing.T, body, words string) string {
	t.Helper()

	dir := writeStoreDir(t)
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
	if err := os.WriteFile(filepath.Join(dir, DenylistName), []byte(words), 0o644); err != nil {
		t.Fatalf("could not write the denylist in %s: %v", dir, err)
	}
	return path
}

// writeStoreDir points HOME at a fresh temporary directory and returns the
// ~/.weir in it, empty.
func writeStoreDir(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", dir, err)
	}
	return dir
}

func TestLoadReadsRegisteredRepos(t *testing.T) {
	writeConfig(t, `
[repos.weir]
path = "/Users/someone/develop/weir"

[repos.notes]
path = "/Users/someone/develop/notes"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}

	if got, want := strings.Join(cfg.Names(), ","), "notes,weir"; got != want {
		t.Errorf("Names() = %q, want %q (sorted)", got, want)
	}

	repo, err := cfg.Repo("weir")
	if err != nil {
		t.Fatalf("Repo(\"weir\") = %v, want no error", err)
	}
	if repo.Name != "weir" {
		t.Errorf("repo.Name = %q, want %q", repo.Name, "weir")
	}
	if want := "/Users/someone/develop/weir"; repo.Path != want {
		t.Errorf("repo.Path = %q, want %q", repo.Path, want)
	}
}

func TestLoadMissingFileIsErrNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() = %v, want ErrNotFound", err)
	}
	// The way out of "not there" differs from the way out of "broken", so the
	// message has to point at it.
	if !strings.Contains(err.Error(), "weir init") {
		t.Errorf("error = %q, want it to name `weir init`", err)
	}
}

// A configuration weir cannot make sense of has to come back as an error.
// Reading one as "nothing is registered" would let the gate open on the day the
// configuration breaks.
func TestLoadRefusesConfigItCannotActOn(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string // substring the error must carry
	}{
		{
			name: "broken syntax",
			body: "[repos.weir\npath = \"/tmp/weir\"\n",
			want: "構文",
		},
		{
			name: "unknown key",
			body: "[repos.weir]\npaht = \"/tmp/weir\"\n",
			want: "知らない項目",
		},
		{
			name: "repo without a path",
			body: "[repos.weir]\n",
			want: "path がありません",
		},
		{
			name: "relative path",
			body: "[repos.weir]\npath = \"develop/weir\"\n",
			want: "絶対パスではありません",
		},
		{
			name: "unexpanded tilde",
			body: "[repos.weir]\npath = \"~/develop/weir\"\n",
			want: "絶対パスではありません",
		},
		{
			name: "repo with no name",
			body: "[repos.\"\"]\npath = \"/tmp/weir\"\n",
			want: "名前のないリポジトリ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeConfig(t, tt.body)

			cfg, err := Load()
			if err == nil {
				t.Fatalf("Load() = %+v, want an error", cfg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestRepoRefusesAnUnregisteredName(t *testing.T) {
	writeConfig(t, "[repos.weir]\npath = \"/tmp/weir\"\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}

	_, err = cfg.Repo("weir-site")
	var unknown *UnknownRepoError
	if !errors.As(err, &unknown) {
		t.Fatalf("Repo(\"weir-site\") = %v, want an *UnknownRepoError", err)
	}
	if unknown.Name != "weir-site" {
		t.Errorf("unknown.Name = %q, want %q", unknown.Name, "weir-site")
	}
	// The message has to say what the caller can name instead.
	if !strings.Contains(err.Error(), "weir") {
		t.Errorf("error = %q, want it to list the registered names", err)
	}
}

func TestRepoOnAnEmptyConfigSaysNothingIsRegistered(t *testing.T) {
	writeConfig(t, "# 何も登録していない\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}
	if len(cfg.Names()) != 0 {
		t.Fatalf("Names() = %v, want none", cfg.Names())
	}

	_, err = cfg.Repo("weir")
	if err == nil {
		t.Fatal("Repo(\"weir\") = nil, want an error")
	}
	if !strings.Contains(err.Error(), "1つもありません") {
		t.Errorf("error = %q, want it to say nothing is registered", err)
	}
}

func TestPathIsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() = %v, want no error", err)
	}
	if want := filepath.Join(home, DirName, FileName); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
	deny, err := DenylistPath()
	if err != nil {
		t.Fatalf("DenylistPath() = %v, want no error", err)
	}
	if want := filepath.Join(home, DirName, DenylistName); deny != want {
		t.Errorf("DenylistPath() = %q, want %q", deny, want)
	}
}

// The words are rules like any other, so they come back as rules — one per
// line, refusing, and knowing which line they came from.
func TestDenylistBecomesWordRules(t *testing.T) {
	writeStore(t, "", `# 拒否する語

山田太郎
  acme-corp

# ここから下も読む
Contoso Ltd
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}

	rules := cfg.DefaultRules()
	if len(rules) != 3 {
		t.Fatalf("DefaultRules() = %+v, want 3 rules (comments and blank lines are not words)", rules)
	}
	want := []string{"山田太郎", "acme-corp", "Contoso Ltd"}
	for i, r := range rules {
		if r.Value != want[i] {
			t.Errorf("rule %d value = %q, want %q", i, r.Value, want[i])
		}
		if r.Kind != rule.Literal {
			t.Errorf("rule %d kind = %q, want %q", i, r.Kind, rule.Literal)
		}
		// The list is what to refuse; there is no column in it saying otherwise.
		if r.Action != rule.Block {
			t.Errorf("rule %d action = %q, want %q", i, r.Action, rule.Block)
		}
		if r.Source == "" {
			t.Errorf("rule %d has no source; a refusal has to be able to name the rule", i)
		}
	}
	// The line it came from, so a refusal can be traced back to it without
	// quoting the word.
	if want := ":7"; !strings.HasSuffix(rules[2].Source, want) {
		t.Errorf("rules[2].Source = %q, want it to end in %q", rules[2].Source, want)
	}
}

// Fail-closed: an absent or unreadable word list is a fault, never "no words".
// Read as an empty list, the gate would stop refusing on the day the file went
// missing — silently, and at the moment it mattered.
func TestLoadRefusesADenylistItCannotRead(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		dir := writeStoreDir(t)
		path := filepath.Join(dir, FileName)
		if err := os.WriteFile(path, []byte("# 何も登録していない\n"), 0o644); err != nil {
			t.Fatalf("could not write %s: %v", path, err)
		}

		_, err := Load()
		if err == nil {
			t.Fatal("Load() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "weir init") {
			t.Errorf("error = %q, want it to say how to get the file back", err)
		}
	})

	t.Run("unreadable", func(t *testing.T) {
		dir := writeStoreDir(t)
		path := filepath.Join(dir, FileName)
		if err := os.WriteFile(path, []byte("# 何も登録していない\n"), 0o644); err != nil {
			t.Fatalf("could not write %s: %v", path, err)
		}
		// A directory where the file goes: unreadable in a way that does not
		// depend on who is running the test.
		if err := os.Mkdir(filepath.Join(dir, DenylistName), 0o755); err != nil {
			t.Fatalf("could not make a directory at the denylist's path: %v", err)
		}

		_, err := Load()
		if err == nil {
			t.Fatal("Load() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "読めません") {
			t.Errorf("error = %q, want it to say the list could not be read", err)
		}
	})

	t.Run("not UTF-8", func(t *testing.T) {
		writeStore(t, "", "山田太郎\n\xff\xfe not text\n")

		_, err := Load()
		if err == nil {
			t.Fatal("Load() = nil, want an error")
		}
		if !strings.Contains(err.Error(), "UTF-8") {
			t.Errorf("error = %q, want it to say what is wrong with the file", err)
		}
	})
}

// Rules weir cannot act on stop the configuration at the door, where the reader
// can still be told which entry is wrong.
func TestLoadRefusesRulesItCannotActOn(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "no type",
			body: "[[rules]]\nvalue = \"x\"\naction = \"block\"\n",
			want: "type がありません",
		},
		{
			name: "unknown type",
			body: "[[rules]]\ntype = \"regex\"\nvalue = \"x\"\naction = \"block\"\n",
			want: "type が知らない値です",
		},
		{
			name: "a word written in the configuration",
			body: "[[rules]]\ntype = \"literal\"\nvalue = \"山田太郎\"\naction = \"block\"\n",
			want: "語は設定ファイルには書けません",
		},
		{
			name: "no action",
			body: "[[rules]]\ntype = \"pattern\"\nvalue = \"x\"\n",
			want: "action がありません",
		},
		{
			name: "unknown action",
			body: "[[rules]]\ntype = \"pattern\"\nvalue = \"x\"\naction = \"stop\"\n",
			want: "action が知らない値です",
		},
		{
			name: "no value",
			body: "[[rules]]\ntype = \"pattern\"\naction = \"block\"\n",
			want: "中身がありません",
		},
		{
			name: "a regular expression that does not compile",
			body: "[[rules]]\ntype = \"pattern\"\nvalue = \"a(b\"\naction = \"block\"\n",
			want: "正規表現として読めません",
		},
		{
			name: "a glob that does not parse",
			body: "[[rules]]\ntype = \"path\"\nvalue = \"secrets/[a\"\naction = \"warn\"\n",
			want: "glob として読めません",
		},
		{
			name: "a bad rule under a repository",
			body: "[repos.weir]\npath = \"/tmp/weir\"\n[[repos.weir.rules]]\ntype = \"pattern\"\nvalue = \"a(b\"\naction = \"block\"\n",
			want: "[[repos.weir.rules]]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeConfig(t, tt.body)

			cfg, err := Load()
			if err == nil {
				t.Fatalf("Load() = %+v, want an error", cfg)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// A repository's rules are added to what applies everywhere. Nothing written
// under a repository can take a default away — that is what makes reading the
// defaults enough to know the minimum that applies.
func TestRulesForAddsRepoRulesToTheDefaults(t *testing.T) {
	writeStore(t, `
[[rules]]
type = "pattern"
value = "AKIA[0-9A-Z]{16}"
action = "block"

[repos.weir]
path = "/tmp/weir"

[[repos.weir.rules]]
type = "path"
value = "secrets/*"
action = "warn"

[repos.notes]
path = "/tmp/notes"
`, "山田太郎\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}

	if got := len(cfg.DefaultRules()); got != 2 {
		t.Fatalf("DefaultRules() = %d rules, want 2 (the word, and the pattern)", got)
	}

	weir, err := cfg.RulesFor("weir")
	if err != nil {
		t.Fatalf("RulesFor(\"weir\") = %v, want no error", err)
	}
	if len(weir) != 3 {
		t.Fatalf("RulesFor(\"weir\") = %+v, want the 2 defaults and the 1 of its own", weir)
	}
	if weir[0].Kind != rule.Literal || weir[1].Kind != rule.Pattern {
		t.Errorf("RulesFor(\"weir\") = %+v, want what applies everywhere first", weir)
	}
	if weir[2].Kind != rule.Path || weir[2].Action != rule.Warn {
		t.Errorf("weir's own rule = %+v, want the path rule it was given", weir[2])
	}

	// A repository that wrote none of its own is still held to the defaults.
	notes, err := cfg.RulesFor("notes")
	if err != nil {
		t.Fatalf("RulesFor(\"notes\") = %v, want no error", err)
	}
	if len(notes) != 2 {
		t.Errorf("RulesFor(\"notes\") = %+v, want the 2 defaults", notes)
	}
}

func TestRulesForRefusesAnUnregisteredName(t *testing.T) {
	writeConfig(t, "[repos.weir]\npath = \"/tmp/weir\"\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() = %v, want no error", err)
	}

	var unknown *UnknownRepoError
	if _, err := cfg.RulesFor("weir-site"); !errors.As(err, &unknown) {
		t.Fatalf("RulesFor(\"weir-site\") = %v, want an *UnknownRepoError", err)
	}
}
