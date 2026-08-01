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

// gitMayFail is gitIn for a command that is meant to fail — a merge that runs
// into a conflict, which is the only way to set one up to be resolved.
func gitMayFail(t *testing.T, dir string, args ...string) (string, error) {
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
	return string(out), err
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

// A conflict is resolved by typing, and what gets typed is in neither parent —
// so it is in no commit anywhere else, and the merge is the only place it can
// be seen. Reading merges by message alone let it through.
func TestPushSeesWhatAMergeResolvedByHand(t *testing.T) {
	dir := newTracking(t)
	gitIn(t, dir, "checkout", "-b", "side")
	commitFile(t, dir, "x.txt", "枝の側\n", "枝で書いた")
	gitIn(t, dir, "checkout", "main")
	commitFile(t, dir, "x.txt", "幹の側\n", "幹で書いた")

	// The two sides disagree, so the merge stops and waits to be resolved.
	if out, err := gitMayFail(t, dir, "merge", "side"); err == nil {
		t.Fatalf("merge succeeded (%s), want a conflict to resolve", out)
	}
	write(t, dir, "x.txt", "AKIAIOSFODNN7EXAMPLE\n")
	gitIn(t, dir, "add", "x.txt")
	gitIn(t, dir, "commit", "-m", "手で直した")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	// Asked for exactly: a combined diff carries one column per parent, and a
	// line read one column short would still hold the word a substring check
	// looks for while carrying a `+` that was never in the file.
	var found bool
	for _, txt := range s.Texts {
		if txt.Where == "コミット "+shortOf(t, dir)+" の x.txt" {
			found = true
			if txt.Body != "AKIAIOSFODNN7EXAMPLE\n" {
				t.Errorf("body = %q, want the resolved line with none of its columns left on it", txt.Body)
			}
		}
	}
	if !found {
		t.Errorf("Texts = %+v, want the line the merge resolved by hand", s.Texts)
	}

	var inPaths bool
	for _, p := range s.Paths {
		if p == "x.txt" {
			inPaths = true
		}
	}
	if !inPaths {
		t.Errorf("Paths = %v, want the file the resolution is in", s.Paths)
	}
}

// A merge that needed no hand has nothing in it that is in neither parent, so
// it adds nothing at all. Anything else would judge the same work twice — once
// as the commit that wrote it, once as the merge that carried it — and refuse a
// push over a line the person cannot find.
func TestPushAddsNothingForAMergeNobodyTouched(t *testing.T) {
	dir := newTracking(t)
	gitIn(t, dir, "checkout", "-b", "side")
	commitFile(t, dir, "side.txt", "SIDELINE\n", "枝で足した")
	gitIn(t, dir, "checkout", "main")
	gitIn(t, dir, "merge", "--no-ff", "-m", "枝を取り込んだ", "side")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if got := strings.Count(bodies(s), "SIDELINE"); got != 1 {
		t.Errorf("SIDELINE appears %d times, want 1 — the commit that wrote it, and not the merge", got)
	}
	if len(s.Paths) != 1 || s.Paths[0] != "side.txt" {
		t.Errorf("Paths = %v, want only the path the commit changes", s.Paths)
	}
}

// A merge can have more than two parents, and a combined diff then carries one
// more column per line. The count comes off the hunk header rather than being
// assumed, so three parents read as cleanly as two — an octopus merge someone
// edited before committing hides a line exactly the same way.
func TestPushSeesWhatWasTypedIntoAnOctopusMerge(t *testing.T) {
	dir := newTracking(t)
	for _, side := range []string{"one", "two"} {
		gitIn(t, dir, "checkout", "-b", side, "main")
		commitFile(t, dir, side+".txt", side+"\n", side+" で足した")
	}
	gitIn(t, dir, "checkout", "main")
	// main has to have moved on too. With it still an ancestor of both sides,
	// the octopus fast-forwards onto one of them first and the result has two
	// parents, not three.
	commitFile(t, dir, "main.txt", "main\n", "幹でも足した")
	// The octopus strategy commits on its own — --no-commit is not open to it —
	// so the line goes in afterwards, with the parents kept by amending.
	gitIn(t, dir, "merge", "-m", "二つまとめて取り込んだ", "one", "two")
	write(t, dir, "a.txt", "one\nAKIAIOSFODNN7EXAMPLE\n")
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "--amend", "-m", "取り込むついでに書いた")

	// Only worth anything if it really is an octopus: two parents would leave
	// the extra column untested.
	if got := len(strings.Fields(gitIn(t, dir, "log", "-1", "--format=%P"))); got != 3 {
		t.Fatalf("the merge has %d parents, want 3", got)
	}

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	// Asked for exactly, not merely contained: read with one column too few, the
	// line would arrive as "+AKIA…" — still found by a substring check, and
	// still wrong. The width has to come off the header.
	var found bool
	for _, txt := range s.Texts {
		if txt.Where == "コミット "+shortOf(t, dir)+" の a.txt" {
			found = true
			if txt.Body != "AKIAIOSFODNN7EXAMPLE\n" {
				t.Errorf("body = %q, want the line with none of its columns left on it", txt.Body)
			}
		}
	}
	if !found {
		t.Errorf("Texts = %+v, want the line typed into the merge", s.Texts)
	}
}

// shortOf is HEAD's abbreviated sha, which is how the surface names a commit.
func shortOf(t *testing.T, dir string) string {
	t.Helper()

	return strings.TrimSpace(gitIn(t, dir, "rev-parse", "--short", "HEAD"))
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

// With no remote at all there is nowhere a push could go. That is not the same
// as finding nothing, and it must not be answered with an empty surface.
func TestPushWithNoRemoteSaysSo(t *testing.T) {
	dir := newRepo(t)

	_, err := Push(dir)
	if !errors.Is(err, ErrNoRemote) {
		t.Fatalf("Push() = %v, want ErrNoRemote", err)
	}
}

// The push a person cannot avoid making: a branch that has never been pushed
// has no upstream, and pushing is how it gets one. With a single remote there
// is nothing to guess, so weir reads the surface and lets the push through —
// refusing it would send them around weir to make it.
func TestPushWithoutAnUpstreamUsesTheOnlyRemote(t *testing.T) {
	dir := newTracking(t)
	gitIn(t, dir, "checkout", "-b", "topic")
	commitFile(t, dir, "topic.txt", "NEVERPUSHED\n", "まだ送っていない")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want the one remote to settle it", err)
	}
	if !strings.Contains(bodies(s), "NEVERPUSHED") {
		t.Errorf("surface = %q, want the commits the only remote does not have", bodies(s))
	}
}

// Several remotes and nothing saying which one: picking is a guess, and the
// wrong guess measures against a remote that already has commits the real
// destination has never seen. weir does not choose.
func TestPushWithoutAnUpstreamAndSeveralRemotesSaysSo(t *testing.T) {
	dir := newTracking(t)
	other := filepath.Join(t.TempDir(), "other.git")
	gitIn(t, dir, "init", "--bare", other)
	gitIn(t, dir, "remote", "add", "other", other)
	gitIn(t, dir, "checkout", "-b", "topic")
	commitFile(t, dir, "topic.txt", "one\n", "まだ送っていない")

	_, err := Push(dir)
	if !errors.Is(err, ErrManyRemotes) {
		t.Fatalf("Push() = %v, want ErrManyRemotes", err)
	}
}

// A branch that does have an upstream is unaffected by how many remotes there
// are: what it says wins, and the count is never consulted.
func TestPushWithAnUpstreamIgnoresTheOtherRemotes(t *testing.T) {
	dir := newTracking(t)
	other := filepath.Join(t.TempDir(), "other.git")
	gitIn(t, dir, "init", "--bare", other)
	gitIn(t, dir, "remote", "add", "other", other)
	commitFile(t, dir, "a.txt", "ONLYHERE\n", "幹で足した")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want the branch's own upstream to settle it", err)
	}
	if !strings.Contains(bodies(s), "ONLYHERE") {
		t.Errorf("surface = %q, want what the branch's own remote does not have", bodies(s))
	}
}

// A detached HEAD is on no branch, so there is no branch for a fallback to be
// about either. What it must not be is an empty surface, which would read as a
// clean one.
func TestPushOnADetachedHeadSaysSo(t *testing.T) {
	dir := newTracking(t)
	commitFile(t, dir, "a.txt", "one\ntwo\n", "一つ目")
	gitIn(t, dir, "checkout", "--detach", "HEAD")

	_, err := Push(dir)
	if !errors.Is(err, ErrDetachedHead) {
		t.Fatalf("Push() = %v, want ErrDetachedHead", err)
	}
}

// The one this range used to get wrong. `<upstream>..HEAD` means "newer than
// this branch's upstream", which sweeps in commits the remote already holds by
// another ref — merge main into a topic branch and main's commits land in it.
// Refusing over one of those refuses a push over something that left long ago,
// and there is nothing to fix.
func TestPushLeavesOutWhatTheRemoteAlreadyHasByAnotherRef(t *testing.T) {
	dir := newTracking(t)
	gitIn(t, dir, "checkout", "-b", "topic")
	commitFile(t, dir, "topic.txt", "TOPICLINE\n", "枝で足した")
	gitIn(t, dir, "push", "-u", "origin", "topic")

	// main moves on and is pushed, so what it carries is already on the remote.
	gitIn(t, dir, "checkout", "main")
	commitFile(t, dir, "main.txt", "ALREADYSENT\n", "幹で足した")
	gitIn(t, dir, "push", "origin", "main")

	// Taking it into the topic branch does not send it a second time.
	gitIn(t, dir, "checkout", "topic")
	gitIn(t, dir, "merge", "--no-ff", "-m", "幹を取り込んだ", "main")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if strings.Contains(bodies(s), "ALREADYSENT") {
		t.Errorf("surface = %q, want nothing from a commit the remote already has", bodies(s))
	}
	if strings.Contains(bodies(s), "幹で足した") {
		t.Errorf("surface = %q, want no message from a commit the remote already has", bodies(s))
	}
	for _, p := range s.Paths {
		if p == "main.txt" {
			t.Errorf("Paths = %v, want no path from a commit the remote already has", s.Paths)
		}
	}
	// The merge itself has not been sent, so it is still judged.
	if !strings.Contains(bodies(s), "幹を取り込んだ") {
		t.Errorf("surface = %q, want the merge commit's own message", bodies(s))
	}
}

// Fetching from one remote and pushing to another is a configuration git
// supports, and the two are different sets of commits. The surface has to be
// measured against the one being pushed to: measuring against the fetch remote
// would drop commits the destination has never seen, which is the direction
// weir must not fail in.
func TestPushMeasuresAgainstThePushRemoteNotTheFetchRemote(t *testing.T) {
	dir := newTracking(t)
	commitFile(t, dir, "sent.txt", "ONLYONORIGIN\n", "origin にだけ送ったもの")
	gitIn(t, dir, "push", "origin", "main")

	// A second remote that has never seen any of this, set as where main pushes.
	other := filepath.Join(t.TempDir(), "other.git")
	gitIn(t, dir, "init", "--bare", other)
	gitIn(t, dir, "remote", "add", "other", other)
	gitIn(t, dir, "config", "branch.main.pushRemote", "other")

	s, err := Push(dir)
	if err != nil {
		t.Fatalf("Push() = %v, want no error", err)
	}
	if !strings.Contains(bodies(s), "ONLYONORIGIN") {
		t.Errorf("surface = %q, want a commit the push remote does not have", bodies(s))
	}
}

// Outside a repository the answer is about the repository, not about a branch
// or a remote that a repository would have had.
func TestPushFailsOutsideARepository(t *testing.T) {
	_, err := Push(t.TempDir())
	if err == nil {
		t.Fatal("Push() = nil, want an error")
	}
	for _, wrong := range []error{ErrDetachedHead, ErrNoRemote, ErrManyRemotes} {
		if errors.Is(err, wrong) {
			t.Errorf("error = %v, want it to be about the repository, not %v", err, wrong)
		}
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("error = %q, want it to name what failed", err)
	}
}
