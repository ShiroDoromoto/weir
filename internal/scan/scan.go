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
// For a push it is the commits the push would send, and no others: everything
// between the current branch's upstream and HEAD, each one's message and each
// one's own changes. A commit made with plain git never went past weir, so the
// push is where it is seen; a commit already upstream is not being sent, and
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

// ErrNoUpstream is what Push answers with when the current branch has no
// upstream.
//
// It is not "nothing to look at": with no upstream weir cannot tell which
// commits a push would send, and an empty surface would read as a clean one.
// Plain git usually refuses such a push itself, but not always — with
// push.autoSetupRemote it sends the branch and sets the upstream on the way —
// so the caller is told the difference rather than handed a surface that looks
// judged.
var ErrNoUpstream = errors.New("このブランチには upstream がありません (何が送られるのかを読み出せません)")

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

// Push assembles what `weir push` in dir would send: the commits between the
// current branch's upstream and HEAD, oldest first, each with its message and
// what it changes.
//
// The destination is git's own — the current branch's upstream — because that
// is where a push with no arguments goes, and weir passes none. A branch with
// no upstream answers ErrNoUpstream: what would be sent is not knowable, and
// saying so is not the same as finding nothing.
//
// A merge commit contributes its message and nothing else. Its first-parent
// diff is mostly work that is already upstream, and judging a push on that
// would refuse it over changes that left long ago; what the merge brings in
// that is genuinely new is in the commits themselves, which are in this range
// too.
func Push(dir string) (Surface, error) {
	upstream, err := upstreamOf(dir)
	if err != nil {
		return Surface{}, err
	}

	commits, err := commitsIn(dir, upstream+"..HEAD")
	if err != nil {
		return Surface{}, err
	}

	var s Surface
	for _, c := range commits {
		s.Texts = append(s.Texts, Text{Where: "コミット " + c.short + " のメッセージ", Body: c.message})
		if c.merge {
			continue
		}
		texts, paths, err := changes(dir, c.sha)
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

// upstreamOf names where a push from dir would go.
func upstreamOf(dir string) (string, error) {
	// Whether this is a repository at all is asked first, so a failure to
	// answer the upstream question can only be about the upstream — and the
	// caller outside a repository is told that, rather than being told its
	// branch is not tracking anything.
	if _, err := git(dir, "送り先", []string{"rev-parse", "--git-dir"}); err != nil {
		return "", err
	}
	out, err := git(dir, "送り先", []string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"})
	if err != nil {
		return "", ErrNoUpstream
	}
	return strings.TrimSpace(out), nil
}

// commit is one commit a push would send.
type commit struct {
	sha     string
	short   string
	merge   bool
	message string
}

// commitsIn lists the commits in a range, oldest first — the order they were
// written, which is the order a reader looks for them in.
func commitsIn(dir, rng string) ([]commit, error) {
	// NUL separates the entries. A commit message can hold any line at all,
	// this one's own header included, so nothing that could be typed into one
	// can be what the entries are split on.
	const format = "--format=%x00%H %h %P%n%B"
	out, err := git(dir, "送られるコミット", []string{"log", "--reverse", format, rng})
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
			merge:   len(fields) > 3,
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
	base := []string{"diff-tree", "--root", "--no-commit-id", "-r", sha}

	out, err := git(dir, "送られるコミット", append([]string{}, append(base, "--name-only", "-z")...))
	if err != nil {
		return nil, nil, err
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}

	body, err := git(dir, "送られるコミット", append([]string{}, append(base, "-p", "-U0")...))
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
// The `+++ b/…` header and an added line beginning with `++` are told apart by
// where they are: a header only ever comes before the first `@@` of a file, and
// every line inside a hunk carries a `+` or `-` in front of it. Matching on the
// text alone would read `++x` as a filename.
func addedLines(body string) []Text {
	var (
		texts  []Text
		where  string
		inHunk bool
		b      strings.Builder
	)
	flush := func() {
		if b.Len() > 0 {
			texts = append(texts, Text{Where: where, Body: b.String()})
			b.Reset()
		}
	}

	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			where, inHunk = "", false
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case inHunk && strings.HasPrefix(line, "+"):
			b.WriteString(line[len("+"):])
			b.WriteString("\n")
		case !inHunk && strings.HasPrefix(line, "+++ "):
			// b/ is git's own prefix, not part of the path. A deleted file
			// says /dev/null here and adds no lines, so it never surfaces.
			where = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
		}
	}
	flush()
	return texts
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
