package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/weir/internal/config"
)

// weirPaths points HOME at a fresh temporary directory and returns the two
// paths init is responsible for.
func weirPaths(t *testing.T) (dir, cfgPath, denyPath string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir = filepath.Join(home, config.DirName)
	return dir, filepath.Join(dir, config.FileName), filepath.Join(dir, config.DenylistName)
}

func runInitOK(t *testing.T) string {
	t.Helper()

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	return stdout.String()
}

func TestInitCreatesBothTemplates(t *testing.T) {
	_, cfgPath, denyPath := weirPaths(t)

	out := runInitOK(t)

	for _, path := range []string{cfgPath, denyPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was not created: %v", path, err)
		}
		if !strings.Contains(out, path) {
			t.Errorf("stdout = %q, want it to name %s", out, path)
		}
	}
}

// The template has to be readable by the loader that will read it for real.
// A template weir itself refuses would send everyone straight into an error
// they did not write.
func TestInitWritesAConfigWeirCanRead(t *testing.T) {
	_, cfgPath, _ := weirPaths(t)
	runInitOK(t)

	cfg, err := config.LoadFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFile(%s) = %v, want the template to load", cfgPath, err)
	}
	// Nothing is registered until the human writes it: weir carries no rules of
	// its own, and a template that came with one would be a rule nobody chose.
	if len(cfg.Repos) != 0 {
		t.Errorf("the template registers %d repositories, want none", len(cfg.Repos))
	}
	// The template shows how to write a rule, which means it holds rules as
	// text. None of them may be live: an example that loads is a rule the
	// reader never chose, matching from the moment they ran init.
	if got := cfg.DefaultRules(); len(got) != 0 {
		t.Errorf("the template carries %d live rules, want none", len(got))
	}
}

// Same property on the other file: every line is commented out, so a fresh
// install matches nothing.
func TestInitWritesADenylistWithNoWordsLive(t *testing.T) {
	_, _, denyPath := weirPaths(t)
	runInitOK(t)

	body, err := os.ReadFile(denyPath)
	if err != nil {
		t.Fatalf("could not read %s: %v", denyPath, err)
	}
	for i, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		t.Errorf("denylist line %d is live: %q, want every line commented out", i+1, line)
	}
}

// What the human wrote is the one thing weir acts on. Overwriting it would
// destroy that, so an existing file is left exactly as it is.
func TestInitLeavesExistingFilesAlone(t *testing.T) {
	dir, cfgPath, denyPath := weirPaths(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("could not make %s: %v", dir, err)
	}

	const (
		mine     = "[repos.mine]\npath = \"/tmp/mine\"\n"
		myWords  = "山田太郎\n"
		wantWord = "山田太郎"
	)
	if err := os.WriteFile(cfgPath, []byte(mine), 0o600); err != nil {
		t.Fatalf("could not write %s: %v", cfgPath, err)
	}
	if err := os.WriteFile(denyPath, []byte(myWords), 0o600); err != nil {
		t.Fatalf("could not write %s: %v", denyPath, err)
	}

	out := runInitOK(t)

	if got := readFile(t, cfgPath); got != mine {
		t.Errorf("%s = %q, want it untouched (%q)", cfgPath, got, mine)
	}
	if got := readFile(t, denyPath); !strings.Contains(got, wantWord) {
		t.Errorf("%s = %q, want it untouched", denyPath, got)
	}
	if !strings.Contains(out, "触りません") {
		t.Errorf("stdout = %q, want it to say the files were left alone", out)
	}
}

// Running it twice has to read the same as running it once — that is what makes
// it safe to type when you are not sure whether you already did.
func TestInitIsRepeatable(t *testing.T) {
	_, cfgPath, _ := weirPaths(t)

	runInitOK(t)
	first := readFile(t, cfgPath)
	runInitOK(t)

	if second := readFile(t, cfgPath); second != first {
		t.Errorf("the second run changed %s: %q, want %q", cfgPath, second, first)
	}
}

// Nothing writable, nothing created: say so and exit non-zero, rather than
// reporting success over a directory that is not there.
func TestInitFailsWhenItCannotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A file where ~/.weir has to be: MkdirAll cannot get past it.
	if err := os.WriteFile(filepath.Join(home, config.DirName), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("could not write the blocking file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init"}, &stdout, &stderr); code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "weir init:") {
		t.Errorf("stderr = %q, want it to say which command failed", stderr.String())
	}
}

func TestInitTakesNoArguments(t *testing.T) {
	weirPaths(t)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", "--force"}, &stdout, &stderr); code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "引数は取りません") {
		t.Errorf("stderr = %q, want it to say init takes no arguments", stderr.String())
	}
}

func TestInitHelpIsNotAnError(t *testing.T) {
	weirPaths(t)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", "--help"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "weir init") {
		t.Errorf("stdout = %q, want the usage", stdout.String())
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not read %s: %v", path, err)
	}
	return string(body)
}
