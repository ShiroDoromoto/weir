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

// diff reads one diff — staged with selector {"--cached"}, unstaged with none —
// and answers with the lines it adds and the paths it touches.
func diff(dir string, selector []string) ([]Text, []string, error) {
	// The paths are asked for separately, NUL-separated, rather than read out
	// of the diff's own headers: that answer is exact whatever a path is
	// spelled with, and paths are what a path rule is judged on.
	out, err := git(dir, append([]string{"diff", "--name-only", "-z"}, selector...))
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
	body, err := git(dir, append([]string{"diff", "-U0"}, selector...))
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

// git runs one read-only git command in dir and answers with its output.
//
// dir is the working tree weir was pointed at, and git is run there and nowhere
// else: another worktree of the same repository has its own index and its own
// changes, and none of them are going into this commit.
func git(dir string, args []string) (string, error) {
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
				"%s: コミットされるものを読み出せません: git %s が失敗しました: %s",
				dir, args[0], detail)
		}
		return "", fmt.Errorf("%s: git を実行できません: %w", dir, err)
	}
	return string(out), nil
}
