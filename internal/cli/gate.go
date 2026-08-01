package cli

import (
	"fmt"
	"io"

	"github.com/ShiroDoromoto/weir/internal/config"
	"github.com/ShiroDoromoto/weir/internal/rule"
	"github.com/ShiroDoromoto/weir/internal/scan"
)

// finding is one rule matching one thing on the surface: the rule that matched,
// and where it matched. Never what it matched — a refusal that quotes the word
// it found writes that word into the terminal, the scrollback and whatever is
// reading the output, which is the one place it was being kept out of.
type finding struct {
	rule  rule.Rule
	where string
}

// judge matches every rule against everything on the surface, and sorts what
// matched by what the rule says to do.
//
// The surface is walked from the outside in — the message, then each file —
// so the findings arrive in the order the reader will look for them, rather
// than in the order the rules happen to be written.
func judge(matchers []rule.Matcher, s scan.Surface) (blocked, warned []finding) {
	add := func(r rule.Rule, where string) {
		f := finding{rule: r, where: where}
		if r.Action == rule.Block {
			blocked = append(blocked, f)
			return
		}
		warned = append(warned, f)
	}

	for _, t := range s.Texts {
		for _, m := range matchers {
			if m.MatchesText(t.Body) {
				add(m.Rule, t.Where)
			}
		}
	}
	for _, p := range s.Paths {
		for _, m := range matchers {
			if m.MatchesPath(p) {
				add(m.Rule, p)
			}
		}
	}
	return blocked, warned
}

// rulesFor reads the rules that apply to a repository and prepares them for
// matching.
//
// A rule weir cannot act on stops the command. The configuration reader already
// refuses those, so this is the second lock on the same door: whatever gets
// past it, no commit is ever judged against fewer rules than were written for
// it.
func rulesFor(cfg *config.Config, repoName string) ([]rule.Matcher, error) {
	rules, err := cfg.RulesFor(repoName)
	if err != nil {
		return nil, err
	}
	return rule.CompileAll(rules)
}

// warn shows what a warn rule found and says nothing about stopping, because
// nothing is stopping. A rule that works by inference will misfire, and a
// refusal that misfires is the one people learn to ignore.
func warn(stdout io.Writer, cmd string, found []finding) {
	if len(found) == 0 {
		return
	}
	fmt.Fprintf(stdout, "%s: 規則に一致しました（warn なので続行します）\n", cmd)
	writeFindings(stdout, found)
	fmt.Fprintln(stdout)
}

// refuseMatched says a block rule matched, and how to get through. fix is the
// first step, which is not the same for the two commands: what a commit has not
// made yet is still in the working tree, and what a push would send is already
// in the history.
//
// What matched is not in it. Where it matched is, and so is where each rule is
// written — which is how the reader finds out what they were matched against,
// by opening their own configuration, without weir repeating the thing back.
func refuseMatched(stderr io.Writer, cmd, what, fix, example string, found []finding) int {
	fmt.Fprintf(stderr, "%s: 規則に一致したので、%sしませんでした。\n\n", cmd, what)
	fmt.Fprint(stderr, "一致した場所と、当たった規則:\n")
	writeFindings(stderr, found)
	fmt.Fprintf(stderr, `
次にすること:
  1. %s
  2. 何に当たったのかは、上の規則の場所を開いて確かめる（weir は一致した中身を出しません）
  3. 直したら、もう一度:

`, fix)
	fmt.Fprintf(stderr, "  %s\n", example)
	return exitFailure
}

// writeFindings lists where each rule matched, one place per line with the rule
// under it.
func writeFindings(w io.Writer, found []finding) {
	for _, f := range found {
		fmt.Fprintf(w, "  %s\n", f.where)
		fmt.Fprintf(w, "    %s（%s）\n", f.rule.Source, f.rule.Kind)
	}
}
