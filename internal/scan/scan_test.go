package scan

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs git in dir, with a configuration of its own so the machine's does
// not reach into the test.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

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
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}

// newRepo makes a repository with one file already committed.
func newRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitIn(t, dir, "init", "-b", "main")
	write(t, dir, "a.txt", "one\n")
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "-m", "root")
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("could not make %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("could not write %s: %v", path, err)
	}
}

// bodies is every piece of text in the surface, joined — for asking whether
// something is being looked at at all.
func bodies(s Surface) string {
	var b strings.Builder
	for _, t := range s.Texts {
		b.WriteString(t.Body)
		b.WriteString("\n")
	}
	return b.String()
}

func TestCommitSeesTheMessageAndWhatIsStaged(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "secrets/key.txt", "AKIAIOSFODNN7EXAMPLE\n")
	gitIn(t, dir, "add", "secrets/key.txt")

	s, err := Commit(dir, "鍵を足した", false)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}

	if len(s.Texts) == 0 || s.Texts[0].Where != MessageWhere || s.Texts[0].Body != "鍵を足した" {
		t.Fatalf("Texts[0] = %+v, want the commit message first", s.Texts)
	}
	if !strings.Contains(bodies(s), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("surface = %q, want the line the commit adds", bodies(s))
	}
	if len(s.Paths) != 1 || s.Paths[0] != "secrets/key.txt" {
		t.Errorf("Paths = %v, want the one path the commit changes", s.Paths)
	}
	// The lines are named by the file they go into, so a refusal can point at
	// one without the reader having to search the diff for it.
	var found bool
	for _, txt := range s.Texts {
		if txt.Where == "secrets/key.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("Texts = %+v, want the added lines named by their file", s.Texts)
	}
}

// Only what the command will commit. Something merely lying around unstaged is
// not going anywhere, and refusing over it would be refusing over a change the
// command does not touch.
func TestCommitLeavesUnstagedChangesOutUnlessAllIsAsked(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\nAKIAIOSFODNN7EXAMPLE\n")

	s, err := Commit(dir, "message", false)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}
	if strings.Contains(bodies(s), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("surface = %q, want nothing unstaged in it", bodies(s))
	}
	if len(s.Paths) != 0 {
		t.Errorf("Paths = %v, want none — nothing is staged", s.Paths)
	}

	// --all stages tracked changes on the way, so now they are being committed.
	s, err = Commit(dir, "message", true)
	if err != nil {
		t.Fatalf("Commit(all) = %v, want no error", err)
	}
	if !strings.Contains(bodies(s), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("surface = %q, want the unstaged change --all would commit", bodies(s))
	}
	if len(s.Paths) != 1 || s.Paths[0] != "a.txt" {
		t.Errorf("Paths = %v, want a.txt", s.Paths)
	}
}

// --all never picks up an untracked file, so no commit can carry one.
func TestCommitNeverSeesAnUntrackedFile(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "scratch.txt", "AKIAIOSFODNN7EXAMPLE\n")

	for _, all := range []bool{false, true} {
		s, err := Commit(dir, "message", all)
		if err != nil {
			t.Fatalf("Commit(all=%v) = %v, want no error", all, err)
		}
		if strings.Contains(bodies(s), "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("Commit(all=%v) surface = %q, want no untracked file in it", all, bodies(s))
		}
		if len(s.Paths) != 0 {
			t.Errorf("Commit(all=%v) Paths = %v, want none", all, s.Paths)
		}
	}
}

// A commit that takes a name out of a file is the fix. Matching what it removes
// would refuse exactly the change that was wanted.
func TestCommitDoesNotSeeWhatIsBeingRemoved(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\n山田太郎\n")
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "-m", "名前が入ってしまった")

	write(t, dir, "a.txt", "one\n")
	gitIn(t, dir, "add", "a.txt")

	s, err := Commit(dir, "名前を消した", false)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}
	if strings.Contains(bodies(s), "山田太郎") {
		t.Errorf("surface = %q, want the removed line left out", bodies(s))
	}
	// The file is still one the commit changes, so a path rule still sees it.
	if len(s.Paths) != 1 || s.Paths[0] != "a.txt" {
		t.Errorf("Paths = %v, want a.txt", s.Paths)
	}
}

// One working tree: the one weir was pointed at. Another worktree of the same
// repository has its own index, and none of it is going into this commit.
func TestCommitSeesOnlyTheWorkingTreeItWasGiven(t *testing.T) {
	dir := newRepo(t)
	other := filepath.Join(t.TempDir(), "elsewhere")
	gitIn(t, dir, "worktree", "add", "-b", "side", other)
	write(t, other, "b.txt", "AKIAIOSFODNN7EXAMPLE\n")
	gitIn(t, other, "add", "b.txt")

	s, err := Commit(dir, "message", true)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}
	if strings.Contains(bodies(s), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("surface = %q, want nothing from the other worktree", bodies(s))
	}
	if len(s.Paths) != 0 {
		t.Errorf("Paths = %v, want none — the other worktree's index is not this one's", s.Paths)
	}
}

// An added line that begins with ++ looks exactly like a diff's file header.
// Reading it as one would name a file that does not exist, and lose the line.
func TestCommitReadsAnAddedLineThatLooksLikeAHeader(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\n++ b/山田太郎\n")
	gitIn(t, dir, "add", "a.txt")

	s, err := Commit(dir, "message", false)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}
	if !strings.Contains(bodies(s), "++ b/山田太郎") {
		t.Errorf("surface = %q, want the line itself, read as content", bodies(s))
	}
	if len(s.Paths) != 1 || s.Paths[0] != "a.txt" {
		t.Errorf("Paths = %v, want only the file that changed", s.Paths)
	}
}

// With --all a file can be staged and then changed again. It is still one file.
func TestCommitNamesAPathOnceWhenItIsBothStagedAndChanged(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "a.txt", "one\ntwo\n")
	gitIn(t, dir, "add", "a.txt")
	write(t, dir, "a.txt", "one\ntwo\nthree\n")

	s, err := Commit(dir, "message", true)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}
	if len(s.Paths) != 1 || s.Paths[0] != "a.txt" {
		t.Errorf("Paths = %v, want a.txt once", s.Paths)
	}
	body := bodies(s)
	for _, want := range []string{"two", "three"} {
		if !strings.Contains(body, want) {
			t.Errorf("surface = %q, want %q — both are being committed", body, want)
		}
	}
}

// A path outside ASCII comes back as it was written. Escaped, a path rule would
// be matched against something nobody typed.
func TestCommitNamesAPathAsItWasWritten(t *testing.T) {
	dir := newRepo(t)
	gitIn(t, dir, "config", "core.quotePath", "true")
	write(t, dir, "書類/秘密.txt", "x\n")
	gitIn(t, dir, "add", "書類/秘密.txt")

	s, err := Commit(dir, "message", false)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}
	if len(s.Paths) != 1 || s.Paths[0] != "書類/秘密.txt" {
		t.Errorf("Paths = %v, want the path unescaped", s.Paths)
	}
}

// Nothing staged is not a fault: it is a commit git will refuse on its own, and
// weir has nothing to judge.
func TestCommitOnAnUnchangedRepositoryIsEmpty(t *testing.T) {
	dir := newRepo(t)

	s, err := Commit(dir, "message", true)
	if err != nil {
		t.Fatalf("Commit() = %v, want no error", err)
	}
	if len(s.Paths) != 0 {
		t.Errorf("Paths = %v, want none", s.Paths)
	}
	if len(s.Texts) != 1 {
		t.Errorf("Texts = %+v, want the message and nothing else", s.Texts)
	}
}

// Outside a repository there is nothing to read, and weir has to say so rather
// than judge an empty surface as clean.
func TestCommitFailsOutsideARepository(t *testing.T) {
	_, err := Commit(t.TempDir(), "message", false)
	if err == nil {
		t.Fatal("Commit() = nil, want an error")
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}

// newTracking makes a repository whose main branch is pushed and tracking, so
// there is an upstream to measure a push against.
func newTracking(t *testing.T) string {
	t.Helper()

	dir := newRepo(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	gitIn(t, dir, "init", "--bare", remote)
	gitIn(t, dir, "remote", "add", "origin", remote)
	gitIn(t, dir, "push", "-u", "origin", "main")
	return dir
}

// commitFile writes a file and commits it, in one step.
func commitFile(t *testing.T, dir, name, body, message string) {
	t.Helper()

	write(t, dir, name, body)
	gitIn(t, dir, "add", name)
	gitIn(t, dir, "commit", "-m", message)
}

// wheres is every Where in the surface, joined — for asking what a refusal
// would be able to point at.
func wheres(s Surface) string {
	var b strings.Builder
	for _, t := range s.Texts {
		b.WriteString(t.Where)
		b.WriteString("\n")
	}
	return b.String()
}

// What is already upstream is not being sent. Judging it would refuse a push
// over something that left long ago, which is a refusal nobody can act on.
func TestPushSeesOnlyWhatIsNotYetUpstream(t *testing.T) {
	dir := newTracking(t)
	commitFile(t, dir, "sent.txt", "ALREADYSENT\n", "先に送ったもの")
	gitIn(t, dir, "push")
	commitFile(t, dir, "waiting.txt", "AKIAIOSFODNN7EXAMPLE\n", "まだ送っていないもの")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if strings.Contains(bodies(s), "ALREADYSENT") {
		t.Errorf("surface = %q, want nothing that is already upstream", bodies(s))
	}
	if !strings.Contains(bodies(s), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("surface = %q, want the line the push would send", bodies(s))
	}
	if len(s.Paths) != 1 || s.Paths[0] != "waiting.txt" {
		t.Errorf("Paths = %v, want only the path the push would send", s.Paths)
	}
}

// A push sends commits, not a net result. A name added in one commit and taken
// out in the next still arrives on the remote, in the history, forever — so it
// is still what the push is judged on.
func TestPushSeesACommitALaterOneUndoes(t *testing.T) {
	dir := newTracking(t)
	commitFile(t, dir, "a.txt", "one\n山田太郎\n", "名前が入ってしまった")
	commitFile(t, dir, "a.txt", "one\n", "名前を消した")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if !strings.Contains(bodies(s), "山田太郎") {
		t.Errorf("surface = %q, want the line the first commit adds — the push carries it", bodies(s))
	}
}

// Every commit's message goes too, named by the commit it belongs to: a refusal
// has to say which of them to fix.
func TestPushSeesEveryMessage(t *testing.T) {
	dir := newTracking(t)
	commitFile(t, dir, "a.txt", "one\ntwo\n", "一つ目")
	commitFile(t, dir, "a.txt", "one\ntwo\nthree\n", "二つ目")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	for _, want := range []string{"一つ目", "二つ目"} {
		if !strings.Contains(bodies(s), want) {
			t.Errorf("surface = %q, want the message %q", bodies(s), want)
		}
	}

	short := strings.TrimSpace(gitIn(t, dir, "rev-parse", "--short", "HEAD"))
	if !strings.Contains(wheres(s), "コミット "+short+" のメッセージ") {
		t.Errorf("Wheres = %q, want each commit named by its own", wheres(s))
	}
	if !strings.Contains(wheres(s), "コミット "+short+" の a.txt") {
		t.Errorf("Wheres = %q, want added lines named by commit and file", wheres(s))
	}
}

// A merge commit brings its message and nothing else. What it merges in is
// already in this range as commits of its own, and its first-parent diff would
// also drag in work that is long since upstream.
func TestPushDoesNotJudgeAMergeTwice(t *testing.T) {
	dir := newTracking(t)
	gitIn(t, dir, "checkout", "-b", "side")
	commitFile(t, dir, "side.txt", "SIDESECRET\n", "枝で足した")
	gitIn(t, dir, "checkout", "main")
	commitFile(t, dir, "main.txt", "MAINLINE\n", "幹で足した")
	gitIn(t, dir, "merge", "--no-ff", "-m", "枝を取り込んだ", "side")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if got := strings.Count(bodies(s), "SIDESECRET"); got != 1 {
		t.Errorf("SIDESECRET appears %d times, want 1 — the commit itself, not the merge as well", got)
	}
	if !strings.Contains(bodies(s), "枝を取り込んだ") {
		t.Errorf("surface = %q, want the merge commit's message", bodies(s))
	}
	if len(s.Paths) != 2 {
		t.Errorf("Paths = %v, want the two files the commits change", s.Paths)
	}
}

// One file changed by several of these commits is one path. A path rule is
// about where a change lands, and it lands in one place however many commits
// carried it there.
func TestPushNamesAPathOnceAcrossCommits(t *testing.T) {
	dir := newTracking(t)
	commitFile(t, dir, "a.txt", "one\ntwo\n", "一つ目")
	commitFile(t, dir, "a.txt", "one\ntwo\nthree\n", "二つ目")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if len(s.Paths) != 1 || s.Paths[0] != "a.txt" {
		t.Errorf("Paths = %v, want a.txt once", s.Paths)
	}
}

// Nothing to send is a surface with nothing in it, and that is the truth: git
// will say everything is up to date.
func TestPushWithNothingToSendIsEmpty(t *testing.T) {
	dir := newTracking(t)

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if len(s.Texts) != 0 || len(s.Paths) != 0 {
		t.Errorf("surface = %+v, want nothing — there is nothing to send", s)
	}
}

// With no upstream weir cannot tell what would be sent. That is not the same as
// finding nothing, and it must not be answered with an empty surface.
func TestPushWithoutAnUpstreamSaysSo(t *testing.T) {
	dir := newRepo(t)

	_, err := Push(dir)
	if !errors.Is(err, ErrNoUpstream) {
		t.Fatalf("Push() = %v, want ErrNoUpstream", err)
	}
}

// Outside a repository the answer is about the repository, not about a branch
// that is not tracking anything.
func TestPushFailsOutsideARepository(t *testing.T) {
	_, err := Push(t.TempDir())
	if err == nil {
		t.Fatal("Push() = nil, want an error")
	}
	if errors.Is(err, ErrNoUpstream) {
		t.Errorf("error = %v, want it to be about the repository, not the upstream", err)
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}
