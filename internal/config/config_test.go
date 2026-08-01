package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig puts a config.toml in a fresh temporary HOME and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", dir, err)
	}
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
	return path
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
}
