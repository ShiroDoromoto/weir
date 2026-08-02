// Package scan assembles what a command is about to put out of reach, and
// nothing beyond it.
//
// For a commit that is: the message, the lines the commit adds, and the paths
// it changes. Not the rest of the working tree, not what is merely lying around
// unstaged, and not another worktree of the same repository. Widening the
// surface would make things that are not being committed into grounds for
// refusing one — a refusal nobody can act on, since fixing it would mean
// changing something the commit does not touch.
//
// For a push it is the commits the push would send, and no others: the ones the
// destination remote does not have yet, each one's message and each one's own
// changes. A commit made with plain git never went past weir, so the push is
// where it is seen; a commit the remote already holds is not being sent, and
// judging one would refuse a push over something that left long ago.
//
// Lines the commit removes are left out for the same reason. A commit that
// takes a name out of a file is the fix; matching what it removed would refuse
// exactly the change that was wanted.
//
// Nothing here matches anything. It hands over what to look at; the rules are
// internal/rule's, and the looking is done elsewhere.
package scan

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Text is a piece of text to be looked at, and where it came from.
type Text struct {
	// Where is what to call this in a refusal — the commit message, or the
	// path of the file the lines are being added to.
	Where string
	// Body is the text itself.
	Body string
}

// MessageWhere is what Where says for the commit message.
const MessageWhere = "コミットメッセージ"

// What Push answers with when nothing says which remote a push would go to.
// None of them is "nothing to look at": with no destination weir cannot tell
// which commits a push would send, and an empty surface would read as a clean
// one.
//
// They are three errors rather than one because the way out of each is a
// different thing to type, and a refusal that cannot say which one is a refusal
// the reader has to guess their way past.
var (
	// ErrDetachedHead is a HEAD that is on no branch, so there is nothing
	// configured to push and nothing to fall back on either.
	ErrDetachedHead = errors.New("いまブランチの上にいません (何が送られるのかを読み出せません)")
	// ErrNoRemote is a repository with no remote at all — there is nowhere for
	// a push to go, whatever the branch says.
	ErrNoRemote = errors.New("リモートが1つもありません (何が送られるのかを読み出せません)")
	// ErrManyRemotes is a branch with no destination of its own in a repository
	// with several remotes. Picking one would be a guess, and a gate that
	// guesses its destination measures against the wrong set of commits.
	ErrManyRemotes = errors.New("送り先のリモートが決まりません (何が送られるのかを読み出せません)")
)

// Surface is what one command puts in front of the rules.
type Surface struct {
	// Texts is what a word or a regular expression is matched against: the
	// commit message, then the lines each file gains.
	Texts []Text
	// Paths is what a path rule is matched against: the paths this commit
	// changes, in the form git names them (relative to the repository root).
	Paths []string
}

// Commit assembles what `weir commit` in dir would commit: the message it was
// given, plus what git would take.
//
// With all, the changes to tracked files that were never staged are part of it
// — `git commit --all` stages them on the way. Untracked files are not: --all
// never picks one up, so a commit cannot carry it, and refusing over one would
// be refusing over a file the command does not touch.
func Commit(dir, message string, all bool) (Surface, error) {
	s := Surface{Texts: []Text{{Where: MessageWhere, Body: message}}}

	// Staged first, since that is what a commit takes with no flags at all.
	sets := [][]string{{"--cached"}}
	if all {
		sets = append(sets, nil)
	}
	for _, selector := range sets {
		texts, paths, err := diff(dir, selector)
		if err != nil {
			return Surface{}, err
		}
		s.Texts = append(s.Texts, texts...)
		s.Paths = appendNew(s.Paths, paths)
	}
	return s, nil
}

// Push assembles what `weir push` in dir would send: the commits that are not
// on the destination remote yet, oldest first, each with its message and what
// it changes.
//
// Not `<upstream>..HEAD`. That range is "newer than this branch's upstream",
// which is not the same set: merge main into a topic branch and main's commits
// — already on the remote, by another ref — fall inside it. A rule matching one
// of those refuses the push over something that left long ago, and there is
// nothing the person can do about it. What is not on the remote yet is asked
// directly instead.
//
// Which remote that is comes from the branch, and from the repository's only
// remote when the branch says nothing — so a branch that has never been pushed
// is still readable. What cannot be settled is answered with one of the three
// errors above, never with an empty surface.
//
// The remote-tracking refs it asks against can be behind what the remote
// actually holds, and weir does not fetch to close that: a gate that reaches
// for the network judges differently offline than on, and quietly rewrites
// refs on its way. Being behind only ever adds commits to the surface — it
// cannot drop one — so the drift falls on the refusing side.
//
// A merge commit is read differently: only the lines that are in none of its
// parents. Its first-parent diff is mostly work that is already on the remote,
// and judging a push on that would refuse it over changes that left long ago —
// what the merge brings in that is genuinely new arrives as commits of its own,
// which are in this set too. What is left over, and in no commit anywhere, is
// what someone typed while resolving a conflict.
func Push(dir string) (Surface, error) {
	remote, err := Destination(dir)
	if err != nil {
		return Surface{}, err
	}
	return carried(dir, remote, "HEAD")
}

// TagNameWhere is what Where says for the name of the tag being pushed. The
// name itself is the body, so a rule can match it — and a refusal says only
// that it was the name, never what it was.
const TagNameWhere = "タグ名"

// ErrNoSuchTag is a tag that is not there to be pushed. It is told apart from
// an empty surface: nothing to send and nothing to find are different answers,
// and only one of them means a push should happen.
var ErrNoSuchTag = errors.New("そのタグがありません")

// Tag assembles what `weir push --tag` in dir would send: the tag's own name
// and, when it is an annotated tag, its message — plus the commits the tag
// carries that the destination does not have yet.
//
// The commits are part of it because pushing a tag pushes whatever the
// destination needs in order to hold it. Tag an unpushed commit and the push
// takes that commit along; judging only the tag would pass it unlooked at.
// Where they are already on the remote — the ordinary release, tagged after
// the branch went out — that set is empty and the tag stands alone.
//
// A lightweight tag has no message of its own. What it points at has one, but
// that is the commit's, judged as a commit; reading it as the tag's would have
// a refusal name a message nobody wrote on a tag.
func Tag(dir, remote, name string) (Surface, error) {
	annotated, message, err := tagOf(dir, name)
	if err != nil {
		return Surface{}, err
	}

	s, err := carried(dir, remote, name)
	if err != nil {
		return Surface{}, err
	}
	// The name goes first: it is the one part every tag has, so the surface is
	// never empty and can never be mistaken for a clean one.
	texts := []Text{{Where: TagNameWhere, Body: name}}
	if annotated {
		texts = append(texts, Text{Where: "タグ " + name + " のメッセージ", Body: message})
	}
	s.Texts = append(texts, s.Texts...)
	return s, nil
}

// tagOf reads a tag: whether it is annotated, and the message if it is.
func tagOf(dir, name string) (annotated bool, message string, err error) {
	// rev-parse first, and on the full ref: it reads the name literally, so a
	// name that is not there answers "not there" rather than being matched as
	// a pattern by what comes next.
	ref := "refs/tags/" + name
	if _, err := git(dir, "タグ", []string{"rev-parse", "--verify", "--quiet", ref}); err != nil {
		return false, "", ErrNoSuchTag
	}

	// %(objecttype) is `tag` for an annotated tag and `commit` for a
	// lightweight one, which is the whole difference between having a message
	// and only appearing to.
	//
	// A space separates them, not a NUL: for-each-ref does not read %x00 the
	// way git log does — it prints those four characters — and an object type
	// is one word of `commit` / `tag` / `tree` / `blob`, so the first space is
	// the only one that could be the separator.
	out, err := git(dir, "タグ", []string{"for-each-ref", "--format=%(objecttype) %(contents)", ref})
	if err != nil {
		return false, "", err
	}
	kind, contents, _ := strings.Cut(out, " ")
	if kind != "tag" {
		return false, "", nil
	}
	return true, strings.TrimRight(contents, "\n"), nil
}

// carried assembles the commits a push of rev would send to remote — the ones
// the remote does not have — with each one's message and what it changes.
func carried(dir, remote, rev string) (Surface, error) {
	// A trailing /* is what --remotes= means by a name with no glob in it.
	// Writing it out keeps the pattern readable as the pattern it is.
	commits, err := commitsIn(dir, rev, "--not", "--remotes="+remote+"/*")
	if err != nil {
		return Surface{}, err
	}

	var s Surface
	for _, c := range commits {
		s.Texts = append(s.Texts, Text{Where: "コミット " + c.short + " のメッセージ", Body: c.message})

		read := changes
		if c.parents > 1 {
			read = resolutions
		}
		texts, paths, err := read(dir, c.sha)
		if err != nil {
			return Surface{}, err
		}
		for _, t := range texts {
			s.Texts = append(s.Texts, Text{Where: "コミット " + c.short + " の " + t.Where, Body: t.Body})
		}
		// One file touched by three of these commits is one path, named once.
		// A path rule is about where a change lands, and it lands in one place
		// however many commits carried it there.
		s.Paths = appendNew(s.Paths, paths)
	}
	return s, nil
}

// Destination names the remote a push from dir would go to. A branch that
// says nothing about where it pushes falls back to soleRemote.
//
// It asks for %(push:remotename) rather than %(upstream:remotename) because
// those two are not the same remote. With branch.<name>.pushRemote or
// remote.pushDefault set, a branch fetches from one and pushes to another;
// subtracting what is on the fetch remote would drop commits the push remote
// has never seen out of the surface, which is the one direction weir must not
// fail in.
func Destination(dir string) (string, error) {
	// Whether this is a repository at all is asked first, so a failure to
	// answer the destination question can only be about the destination — and
	// the caller outside a repository is told that, rather than being told its
	// branch is not tracking anything.
	if _, err := git(dir, "送り先", []string{"rev-parse", "--git-dir"}); err != nil {
		return "", err
	}
	// A detached HEAD is not on a branch, so there is nothing configured to push
	// to and no branch for a fallback to be about either.
	branch, err := git(dir, "送り先", []string{"symbolic-ref", "--quiet", "--short", "HEAD"})
	if err != nil {
		return "", ErrDetachedHead
	}
	// refs/heads/<branch> matches that branch and nothing else: for-each-ref
	// only extends a pattern at a slash, and git will not let refs/heads/x and
	// refs/heads/x/y both exist. So this is one line, or none.
	out, err := git(dir, "送り先", []string{
		"for-each-ref", "--format=%(push:remotename)",
		"refs/heads/" + strings.TrimSpace(branch),
	})
	if err != nil {
		return "", err
	}
	if remote := strings.TrimSpace(out); remote != "" {
		return remote, nil
	}
	// Nothing is configured for this branch — which is what every branch looks
	// like before its first push. That is the one push a person cannot avoid
	// making, so refusing it outright would send them around weir to make it.
	return soleRemote(dir)
}

// soleRemote answers the destination of a branch that has none of its own: the
// repository's remote, when it has exactly one.
//
// One remote is not a guess — there is nowhere else a push could go, and git
// would pick the same one. Several is a guess, and measuring against the wrong
// remote drops commits the real destination has never seen out of the surface.
// So the count is what decides, and weir does not choose among them.
//
// Whether git will make the push is git's own question: without
// push.autoSetupRemote it asks for --set-upstream first, and it says so better
// than weir could. What weir answers here is only whether it has seen what
// would be sent.
func soleRemote(dir string) (string, error) {
	out, err := git(dir, "送り先", []string{"remote"})
	if err != nil {
		return "", err
	}

	var remotes []string
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			remotes = append(remotes, name)
		}
	}
	switch len(remotes) {
	case 0:
		return "", ErrNoRemote
	case 1:
		return remotes[0], nil
	default:
		return "", ErrManyRemotes
	}
}

// commit is one commit a push would send.
type commit struct {
	sha   string
	short string
	// parents is how many parents it has: none for a root commit, one for an
	// ordinary one, two or more for a merge. It is a count rather than a flag
	// because a combined diff writes one column per parent, and reading one
	// means knowing how many there are.
	parents int
	message string
}

// commitsIn lists the commits a revision selection picks out, oldest first —
// the order they were written, which is the order a reader looks for them in.
func commitsIn(dir string, selection ...string) ([]commit, error) {
	// NUL separates the entries. A commit message can hold any line at all,
	// this one's own header included, so nothing that could be typed into one
	// can be what the entries are split on.
	const format = "--format=%x00%H %h %P%n%B"
	out, err := git(dir, "送られるコミット", append([]string{"log", "--reverse", format}, selection...))
	if err != nil {
		return nil, err
	}

	var commits []commit
	for _, entry := range strings.Split(out, "\x00") {
		header, message, found := strings.Cut(entry, "\n")
		if !found {
			// The output opens with the first separator, so what comes before
			// it is nothing at all.
			continue
		}
		fields := strings.Fields(header)
		if len(fields) < 2 {
			return nil, fmt.Errorf("%s: 送られるコミットを読み出せません: git log の答えが読めません", dir)
		}
		commits = append(commits, commit{
			sha:   fields[0],
			short: fields[1],
			// Everything after the commit and its abbreviation is its parents.
			parents: len(fields) - 2,
			message: strings.TrimRight(message, "\n"),
		})
	}
	return commits, nil
}

// changes reads what one commit changes — the lines it adds and the paths it
// touches — against the parent it was made on.
func changes(dir, sha string) ([]Text, []string, error) {
	// diff-tree rather than show: it answers with the diff and nothing else,
	// so no header has to be trimmed back off. --root is what makes a commit
	// with no parent answer at all, instead of answering with nothing.
	return diffTree(dir, []string{"diff-tree", "--root", "--no-commit-id", "-r", sha})
}

// resolutions reads what a merge commit holds that none of its parents do —
// the lines someone typed while resolving a conflict, and the files they are
// in.
//
// --cc is what asks that question: it drops any file whose result matches a
// parent, and inside the ones left it shows only the hunks that differ from
// every parent. A merge that went through cleanly answers with nothing at all,
// so this adds no surface where there was no hand in it.
//
// The alternative — the first-parent diff — would drag in everything the merge
// took from the other side, all of it already judged as the commits it came
// from, and much of it already on the remote.
func resolutions(dir, sha string) ([]Text, []string, error) {
	return diffTree(dir, []string{"diff-tree", "--cc", "--no-commit-id", "-r", sha})
}

// diffTree runs one diff-tree two ways — for the paths, and for the body — and
// answers with the lines it adds and the paths it touches.
//
// The paths are asked for separately, NUL-separated, rather than read out of
// the diff's own headers: that answer is exact whatever a path is spelled with.
func diffTree(dir string, base []string) ([]Text, []string, error) {
	out, err := git(dir, "送られるコミット", append(append([]string{}, base...), "--name-only", "-z"))
	if err != nil {
		return nil, nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}

	body, err := git(dir, "送られるコミット", append(append([]string{}, base...), "-p", "-U0"))
	if err != nil {
		return nil, nil, err
	}
	return addedLines(body), paths, nil
}

// diff reads one diff — staged with selector {"--cached"}, unstaged with none —
// and answers with the lines it adds and the paths it touches.
func diff(dir string, selector []string) ([]Text, []string, error) {
	// The paths are asked for separately, NUL-separated, rather than read out
	// of the diff's own headers: that answer is exact whatever a path is
	// spelled with, and paths are what a path rule is judged on.
	out, err := git(dir, "コミットされるもの", append([]string{"diff", "--name-only", "-z"}, selector...))
	if err != nil {
		return nil, nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}

	// -U0: the unchanged lines around a change are not being committed, so
	// they are not part of what is being judged.
	body, err := git(dir, "コミットされるもの", append([]string{"diff", "-U0"}, selector...))
	if err != nil {
		return nil, nil, err
	}
	return addedLines(body), paths, nil
}

// addedLines pulls the added lines out of a diff, grouped by the file they are
// being added to.
//
// It reads an ordinary diff and a merge's combined diff with one pass, because
// the diff says which it is: a hunk opens with one more `@` than the commit has
// parents — `@@` for an ordinary one, `@@@` for a two-parent merge — and that
// same count is how many columns each line inside carries, one per parent. A
// column holds `+` when the line is not in that parent, so a line whose columns
// are all `+` is in none of them. Those are the ones taken: in an ordinary diff
// that is every added line, and in a merge it is only what was typed by hand.
//
// The `+++ b/…` header and an added line beginning with `+++` are told apart by
// where they are: a header only ever comes before the first `@@` of a file, and
// every line inside a hunk carries its columns in front of it. Matching on the
// text alone would read `+++x` as a filename.
func addedLines(body string) []Text {
	var (
		texts []Text
		where string
		// columns is how wide a line's prefix is inside the hunk being read,
		// and zero when no hunk is open — the two things a line has to be
		// judged against, and one answers both.
		columns int
		b       strings.Builder
	)
	flush := func() {
		if b.Len() > 0 {
			texts = append(texts, Text{Where: where, Body: b.String()})
			b.Reset()
		}
	}

	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --"):
			// `diff --git` opens an ordinary file, `diff --cc` a combined one.
			// Nothing inside a hunk can look like either: every line in one
			// starts with its columns.
			flush()
			where, columns = "", 0
		case strings.HasPrefix(line, "@@"):
			columns = len(line) - len(strings.TrimLeft(line, "@")) - 1
		case columns > 0 && addedInEveryParent(line, columns):
			b.WriteString(line[columns:])
			b.WriteString("\n")
		case columns == 0 && strings.HasPrefix(line, "+++ "):
			// b/ is git's own prefix, not part of the path. A deleted file
			// says /dev/null here and adds no lines, so it never surfaces.
			where = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
		}
	}
	flush()
	return texts
}

// addedInEveryParent reports whether every one of a diff line's columns says
// the line is new — which for an ordinary diff, having one column, is simply
// that it is an added line.
func addedInEveryParent(line string, columns int) bool {
	if len(line) < columns {
		return false
	}
	for i := 0; i < columns; i++ {
		if line[i] != '+' {
			return false
		}
	}
	return true
}

// appendNew adds the paths that are not in have already, keeping the order they
// arrived in. With --all a file can be both staged and modified again, and
// naming it twice would have it refused twice for the one change.
func appendNew(have, add []string) []string {
	for _, p := range add {
		found := false
		for _, seen := range have {
			if seen == p {
				found = true
				break
			}
		}
		if !found {
			have = append(have, p)
		}
	}
	return have
}

// git runs one read-only git command in dir and answers with its output. what
// names the thing being read, so a failure says which question went unanswered
// rather than only which git command failed.
//
// dir is the working tree weir was pointed at, and git is run there and nowhere
// else: another worktree of the same repository has its own index, its own
// changes and its own branch, and none of that is what this command is about
// to send.
func git(dir, what string, args []string) (string, error) {
	// core.quotePath: a path outside ASCII comes back as it was written rather
	// than in escapes. color.ui: a diff read by a program must not carry
	// colour, whatever the human's git is configured to do.
	full := append([]string{"-c", "core.quotePath=false", "-c", "color.ui=false"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail := strings.TrimSpace(string(exitErr.Stderr))
			return "", fmt.Errorf(
				"%s: %sを読み出せません: git %s が失敗しました: %s",
				dir, what, args[0], detail)
		}
		return "", fmt.Errorf("%s: git を実行できません: %w", dir, err)
	}
	return string(out), nil
}
