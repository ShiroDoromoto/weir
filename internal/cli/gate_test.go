package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs git in dir and fails the test if it will not.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// commitCount is how many commits a repository has — the answer to "did that
// refusal actually stop anything".
func commitCount(t *testing.T, dir string) int {
	t.Helper()

	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// No commits at all: rev-list has no HEAD to count from.
		return 0
	}
	n := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n); err != nil {
		t.Fatalf("could not read the commit count: %v", err)
	}
	return n
}

// A word in the list stops the commit, and the commit does not happen.
func TestCommitIsRefusedByAWordInTheMessage(t *testing.T) {
	dir := newRepo(t)
	withStore(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir), "山田太郎\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "山田太郎の分を直した"},
		&stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	if n := commitCount(t, dir); n != 0 {
		t.Errorf("the repository has %d commits, want 0 — the refusal must stop the commit", n)
	}

	said := stderr.String()
	// The word itself is the one thing a refusal must not repeat: saying it
	// writes it into the terminal, the scrollback, and whatever is reading.
	if strings.Contains(said, "山田太郎") {
		t.Errorf("stderr = %q, want it not to repeat what matched", said)
	}
	// What happened, where to look, and a line that works.
	for _, want := range []string{"コミットメッセージ", "denylist", commitExample} {
		if !strings.Contains(said, want) {
			t.Errorf("stderr = %q, want it to carry %q", said, want)
		}
	}
}

// A regular expression is matched against the lines the commit adds.
func TestCommitIsRefusedByAPatternInWhatIsStaged(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("AKIAIOSFODNN7EXAMPLE\n"), 0o644); err != nil {
		t.Fatalf("could not write a.txt: %v", err)
	}
	gitIn(t, dir, "add", "a.txt")
	withConfig(t, fmt.Sprintf(`[repos.weir]
path = %q

[[rules]]
type = "pattern"
action = "block"
value = "AKIA[0-9A-Z]{16}"
`, dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "鍵を足した"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	if n := commitCount(t, dir); n != 0 {
		t.Errorf("the repository has %d commits, want 0", n)
	}
	said := stderr.String()
	if strings.Contains(said, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("stderr = %q, want it not to repeat what matched", said)
	}
	if !strings.Contains(said, "a.txt") {
		t.Errorf("stderr = %q, want it to name where the match was", said)
	}
}

// A glob is matched against the paths the commit changes, at any depth.
func TestCommitIsRefusedByAPathAtAnyDepth(t *testing.T) {
	dir := newRepo(t)
	deep := filepath.Join(dir, "apps", "api")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("could not make %s: %v", deep, err)
	}
	if err := os.WriteFile(filepath.Join(deep, ".env"), []byte("x=1\n"), 0o644); err != nil {
		t.Fatalf("could not write .env: %v", err)
	}
	gitIn(t, dir, "add", "apps/api/.env")
	withConfig(t, fmt.Sprintf(`[repos.weir]
path = %q

[[rules]]
type = "path"
action = "block"
value = "**/.env"
`, dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "設定を足した"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	if !strings.Contains(stderr.String(), "apps/api/.env") {
		t.Errorf("stderr = %q, want it to name the path that matched", stderr.String())
	}
}

// warn does not refuse. It says what it found on stdout and the commit is made
// — a rule that works by inference will misfire, and refusals that misfire are
// the ones people learn to ignore.
func TestCommitWithAWarnRuleGoesThroughAndSaysSo(t *testing.T) {
	dir := newRepo(t)
	withConfig(t, fmt.Sprintf(`[repos.weir]
path = %q

[[rules]]
type = "pattern"
action = "warn"
value = "秘密"
`, dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "秘密の設定を足した"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if n := commitCount(t, dir); n != 1 {
		t.Errorf("the repository has %d commits, want 1 — warn does not stop anything", n)
	}
	// What weir said, up to the blank line where it hands over to git. Past
	// that is git's own output, which echoes the message the human typed —
	// that is git passing something through, not weir repeating a match.
	said, _, _ := strings.Cut(stdout.String(), "\n\n")
	if !strings.Contains(said, "コミットメッセージ") {
		t.Errorf("stdout = %q, want it to say where the warn rule matched", said)
	}
	if strings.Contains(said, "秘密") {
		t.Errorf("stdout = %q, want neither what matched nor the rule's own word in it", said)
	}
}

// A repository's own rules are added to the ones that apply everywhere, so a
// rule written under one repository stops that repository's commit.
func TestCommitIsRefusedByTheRepositorysOwnRule(t *testing.T) {
	dir := newRepo(t)
	withConfig(t, fmt.Sprintf(`[repos.weir]
path = %q

[[repos.weir.rules]]
type = "literal"
action = "block"
value = "社外秘"
`, dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "社外秘の資料を入れた"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	if n := commitCount(t, dir); n != 0 {
		t.Errorf("the repository has %d commits, want 0", n)
	}
}

// newTracking makes a repository whose main branch is pushed and tracking, so a
// push has an upstream to be measured against.
func newTracking(t *testing.T) (dir, remote string) {
	t.Helper()

	dir = newRepo(t)
	gitIn(t, dir, "commit", "--message", "最初のコミット")
	remote = filepath.Join(t.TempDir(), "origin.git")
	gitIn(t, dir, "init", "--bare", remote)
	gitIn(t, dir, "remote", "add", "origin", remote)
	gitIn(t, dir, "push", "-u", "origin", "main")
	return dir, remote
}

// A commit made with plain git never went past weir. The push is where it is
// seen, and where it is stopped.
func TestPushIsRefusedByWhatWasCommittedWithPlainGit(t *testing.T) {
	dir, remote := newTracking(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("AKIAIOSFODNN7EXAMPLE\n"), 0o644); err != nil {
		t.Fatalf("could not write a.txt: %v", err)
	}
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "--message", "素の git で入れた")
	before := gitIn(t, dir, "rev-parse", "origin/main")

	withConfig(t, fmt.Sprintf(`[repos.weir]
path = %q

[[rules]]
type = "pattern"
action = "block"
value = "AKIA[0-9A-Z]{16}"
`, dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	// The refusal is worth nothing if the push happened anyway.
	after := gitIn(t, dir, "rev-parse", "origin/main")
	if before != after {
		t.Errorf("origin/main moved from %s to %s, want the push stopped", before, after)
	}
	if head := strings.TrimSpace(gitIn(t, dir, "--git-dir", remote, "rev-parse", "main")); head != strings.TrimSpace(before) {
		t.Errorf("the remote is at %s, want it where it was", head)
	}
	said := stderr.String()
	if strings.Contains(said, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("stderr = %q, want it not to repeat what matched", said)
	}
	for _, want := range []string{"次にすること", fmt.Sprintf(pushFormat, "weir")} {
		if !strings.Contains(said, want) {
			t.Errorf("stderr = %q, want it to carry %q", said, want)
		}
	}
}

// Nothing matching is a push that goes, and says nothing weir did not need to
// say.
func TestPushGoesThroughWhenNothingMatches(t *testing.T) {
	dir, _ := newTracking(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ふつうの行\n"), 0o644); err != nil {
		t.Fatalf("could not write a.txt: %v", err)
	}
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "--message", "ふつうの変更")

	withStore(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir), "山田太郎\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	local := strings.TrimSpace(gitIn(t, dir, "rev-parse", "HEAD"))
	sent := strings.TrimSpace(gitIn(t, dir, "rev-parse", "origin/main"))
	if local != sent {
		t.Errorf("origin/main is at %s, want the pushed %s", sent, local)
	}
}

// A commit already upstream is not being sent, so it is not what the push is
// judged on. Refusing over one would be refusing over something that left long
// ago, and nobody could act on it.
func TestPushIsNotJudgedOnWhatAlreadyLeft(t *testing.T) {
	dir, _ := newTracking(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("AKIAIOSFODNN7EXAMPLE\n"), 0o644); err != nil {
		t.Fatalf("could not write a.txt: %v", err)
	}
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "--message", "先に出てしまった")
	gitIn(t, dir, "push")

	// Now a clean commit, with the rule that would have caught the one before.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("ふつうの行\n"), 0o644); err != nil {
		t.Fatalf("could not write b.txt: %v", err)
	}
	gitIn(t, dir, "add", "b.txt")
	gitIn(t, dir, "commit", "--message", "そのあとの変更")

	withConfig(t, fmt.Sprintf(`[repos.weir]
path = %q

[[rules]]
type = "pattern"
action = "block"
value = "AKIA[0-9A-Z]{16}"
`, dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
}

// No rules is a configuration weir can read, so the commit goes through — but
// never quietly. Silence would read as a pass, and someone who has not written
// a rule yet would go on believing weir was holding something back.
func TestCommitWithNoRulesSaysSoAndGoesThrough(t *testing.T) {
	dir := newRepo(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "ふつうの変更"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if n := commitCount(t, dir); n != 1 {
		t.Errorf("the repository has %d commits, want 1 — no rules is not a refusal", n)
	}
	said := stdout.String()
	if !strings.Contains(said, "規則が0件") {
		t.Errorf("stdout = %q, want it to say there is nothing to match against", said)
	}
	// And where to write one, or the reader is told a fact and no way to act on it.
	for _, want := range []string{"denylist", "[[rules]]"} {
		if !strings.Contains(said, want) {
			t.Errorf("stdout = %q, want it to say where to write a rule (%q)", said, want)
		}
	}
}

func TestPushWithNoRulesSaysSoAndGoesThrough(t *testing.T) {
	dir, _ := newTracking(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ふつうの行\n"), 0o644); err != nil {
		t.Fatalf("could not write a.txt: %v", err)
	}
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "--message", "ふつうの変更")
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	var stdout, stderr bytes.Buffer
	code := Run([]string{"push", "--repo", "weir"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "規則が0件") {
		t.Errorf("stdout = %q, want it to say there is nothing to match against", stdout.String())
	}
	local := strings.TrimSpace(gitIn(t, dir, "rev-parse", "HEAD"))
	if sent := strings.TrimSpace(gitIn(t, dir, "rev-parse", "origin/main")); local != sent {
		t.Errorf("origin/main is at %s, want the pushed %s", sent, local)
	}
}

// It is said every time, not once. A notice that fades after the first run is
// one nobody sees on the day it matters.
func TestTheNoRulesNoticeIsSaidEveryTime(t *testing.T) {
	dir := newRepo(t)
	withConfig(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir))

	for i, message := range []string{"一つ目", "二つ目"} {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(message+"\n"), 0o644); err != nil {
			t.Fatalf("could not write a.txt: %v", err)
		}
		gitIn(t, dir, "add", "a.txt")

		var stdout, stderr bytes.Buffer
		if code := Run([]string{"commit", "--repo", "weir", "--message", message}, &stdout, &stderr); code != exitOK {
			t.Fatalf("commit %d: exit code = %d, want %d (stderr: %s)", i+1, code, exitOK, stderr.String())
		}
		if !strings.Contains(stdout.String(), "規則が0件") {
			t.Errorf("commit %d: stdout = %q, want the notice again", i+1, stdout.String())
		}
	}
}

// With rules to match against, there is nothing to announce.
func TestNothingIsSaidWhenThereAreRules(t *testing.T) {
	dir := newRepo(t)
	withStore(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir), "山田太郎\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "ふつうの変更"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if strings.Contains(stdout.String(), "規則が0件") {
		t.Errorf("stdout = %q, want no notice — there is a rule", stdout.String())
	}
}

// A name turns up in filenames as readily as in lines, and a commit whose diff
// holds nothing at all would go through if only the lines were read.
func TestCommitIsRefusedByAWordInAFilename(t *testing.T) {
	dir := newRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("could not make docs: %v", err)
	}
	// The name is in the path and nowhere else — not in the message, not in a
	// line of the file.
	name := filepath.Join("docs", "山田太郎-履歴書.md")
	if err := os.WriteFile(filepath.Join(dir, name), []byte("面談は火曜です\n"), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", name, err)
	}
	gitIn(t, dir, "add", "docs")
	withStore(t, fmt.Sprintf("[repos.weir]\npath = %q\n", dir), "山田太郎\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "資料を置いた"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	if n := commitCount(t, dir); n != 0 {
		t.Errorf("the repository has %d commits, want 0 — the refusal must stop the commit", n)
	}

	said := stderr.String()
	// The refusal names the path so the reader can find the file — but the
	// name is written in that path, and printing it would put it in the
	// terminal, which is the one place the denylist exists to keep it out of.
	if strings.Contains(said, "山田太郎") {
		t.Errorf("stderr = %q, want the refusal not to print the word it is refusing over", said)
	}
	for _, want := range []string{"docs/", "履歴書.md", "denylist"} {
		if !strings.Contains(said, want) {
			t.Errorf("stderr = %q, want it to carry %q so the file can be found", said, want)
		}
	}
}

// The word is taken out of what is printed however the finding arose: a file
// whose name holds a name, gaining a line that a different rule matches, would
// otherwise have that name printed by the other rule's finding.
func TestARefusalNeverPrintsAWordEvenViaAnotherRule(t *testing.T) {
	dir := newRepo(t)
	name := "山田太郎.md"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("AKIAIOSFODNN7EXAMPLE\n"), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", name, err)
	}
	gitIn(t, dir, "add", name)
	withStore(t, fmt.Sprintf(
		"[repos.weir]\npath = %q\n\n[[rules]]\ntype = \"pattern\"\nvalue = \"AKIA[0-9A-Z]{16}\"\naction = \"block\"\n",
		dir), "山田太郎\n")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"commit", "--repo", "weir", "--message", "鍵を置いた"}, &stdout, &stderr)
	if code != exitFailure {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitFailure, stderr.String())
	}
	if strings.Contains(stderr.String(), "山田太郎") {
		t.Errorf("stderr = %q, want the word gone whichever rule named the file", stderr.String())
	}
}
