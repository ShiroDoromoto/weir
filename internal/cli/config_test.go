package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShiroDoromoto/weir/internal/config"
)

// newTestRepo makes a repository with one commit, and answers with the path git
// itself would answer with (on macOS a temporary directory is reached through a
// symlink).
func newTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"commit", "--allow-empty", "-m", "root"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_CONFIG_SYSTEM="+os.DevNull,
			"GIT_AUTHOR_NAME=weir test",
			"GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=weir test",
			"GIT_COMMITTER_EMAIL=test@example.invalid",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
		}
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("could not resolve %s: %v", dir, err)
	}
	return resolved
}

func TestConfigCheckPassesAWorkingConfiguration(t *testing.T) {
	repo := newTestRepo(t)
	withConfig(t, "[repos.weir]\npath = \""+repo+"\"\n")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"config", "check"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout: %s, stderr: %s)", code, exitOK, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "問題はありません") {
		t.Errorf("stdout = %q, want it to say the configuration is fine", out)
	}
	if !strings.Contains(out, repo) {
		t.Errorf("stdout = %q, want it to name what it checked", out)
	}
}

// The whole reason this command exists: a refusal names the first thing it
// tripped on, and this has to name every one of them at once.
func TestConfigCheckReportsEveryBadPath(t *testing.T) {
	repo := newTestRepo(t)
	gone := filepath.Join(t.TempDir(), "gone")
	notGit := t.TempDir()

	withConfig(t, "[repos.ok]\npath = \""+repo+"\"\n"+
		"[repos.gone]\npath = \""+gone+"\"\n"+
		"[repos.plain]\npath = \""+notGit+"\"\n")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"config", "check"}, &stdout, &stderr); code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stdout: %s)", code, exitFailure, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "問題が2件あります") {
		t.Errorf("stdout = %q, want both problems counted", out)
	}
	for _, want := range []string{gone, notGit, repo} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want it to name %s", out, want)
		}
	}
}

// A configuration weir cannot read is the answer, not a failure to run — so it
// goes to stdout with the reason, and the exit code says it did not pass.
func TestConfigCheckSaysWhyAConfigurationDoesNotLoad(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "no configuration at all", body: "", want: "weir init"},
		{name: "broken syntax", body: "[repos.weir\n", want: "構文"},
		{name: "a key weir does not know", body: "[repos.weir]\npath = \"/tmp/w\"\nmode = \"strict\"\n", want: "知らない項目"},
		{name: "a relative path", body: "[repos.weir]\npath = \"develop/weir\"\n", want: "絶対パス"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withConfig(t, tt.body)

			var stdout, stderr bytes.Buffer
			if code := Run([]string{"config", "check"}, &stdout, &stderr); code != exitFailure {
				t.Fatalf("exit code = %d, want %d", code, exitFailure)
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.want)
			}
		})
	}
}

// Registering nothing is a configuration weir can read and act on. It is not a
// fault, so it passes — and it still says what would change it.
func TestConfigCheckPassesAnEmptyConfiguration(t *testing.T) {
	withConfig(t, "# 何も登録していない\n")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"config", "check"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[repos.<名前>]") {
		t.Errorf("stdout = %q, want it to show what to write", stdout.String())
	}
}

// The point of reading rules from the configuration is that you can tell what
// weir will be matched against. The check has to say it: which files it read,
// and how many rules came out of them for each repository.
func TestConfigCheckCountsTheRulesThatApply(t *testing.T) {
	repo := newTestRepo(t)
	withStore(t, `
[[rules]]
type = "pattern"
value = "AKIA[0-9A-Z]{16}"
action = "block"

[repos.weir]
path = "`+repo+`"

[[repos.weir.rules]]
type = "path"
value = "secrets/*"
action = "warn"
`, "山田太郎\nacme-corp\n")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"config", "check"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stdout: %s, stderr: %s)", code, exitOK, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"denylist",
		"既定の規則 3件",
		"規則 4件（既定 3 + このリポジトリ 1）",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want it to contain %q", out, want)
		}
	}
}

// Fail-closed, and said out loud: without the word list weir does not run, so
// the check has to name it rather than pass a configuration that will not work.
func TestConfigCheckReportsAMissingDenylist(t *testing.T) {
	repo := newTestRepo(t)
	withConfig(t, "[repos.weir]\npath = \""+repo+"\"\n")
	denyPath, err := config.DenylistPath()
	if err != nil {
		t.Fatalf("DenylistPath() = %v, want no error", err)
	}
	if err := os.Remove(denyPath); err != nil {
		t.Fatalf("could not remove %s: %v", denyPath, err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"config", "check"}, &stdout, &stderr); code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stdout: %s)", code, exitFailure, stdout.String())
	}
	if !strings.Contains(stdout.String(), "拒否する語の一覧がありません") {
		t.Errorf("stdout = %q, want it to say the word list is not there", stdout.String())
	}
}

// It reads and never writes: what is in the configuration is the human's.
func TestConfigCheckLeavesTheConfigurationAlone(t *testing.T) {
	const body = "[repos.gone]\npath = \"/nowhere/at/all\"\n"
	withConfig(t, body)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"config", "check"}, &stdout, &stderr); code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not read HOME: %v", err)
	}
	got := readFile(t, filepath.Join(home, ".weir", "config.toml"))
	if got != body {
		t.Errorf("config.toml = %q, want it untouched (%q)", got, body)
	}
}

func TestConfigRefusesWhatItDoesNotHave(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no subcommand", args: []string{"config"}, want: "weir config check"},
		{name: "an unknown subcommand", args: []string{"config", "fix"}, want: "知らないコマンド"},
		{name: "an argument check does not take", args: []string{"config", "check", "--repo", "weir"}, want: "引数は取りません"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withConfig(t, "[repos.weir]\npath = \"/tmp/weir\"\n")

			var stdout, stderr bytes.Buffer
			if code := Run(tt.args, &stdout, &stderr); code != exitUsage {
				t.Fatalf("exit code = %d, want %d", code, exitUsage)
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
			// Every refusal carries the line that works.
			if !strings.Contains(stderr.String(), "weir config check") {
				t.Errorf("stderr = %q, want it to show the way through", stderr.String())
			}
		})
	}
}

func TestConfigCheckHelpIsNotAnError(t *testing.T) {
	withConfig(t, "")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"config", "check", "--help"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "weir config check") {
		t.Errorf("stdout = %q, want the usage", stdout.String())
	}
}
