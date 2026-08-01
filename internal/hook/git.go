package hook

import (
	"errors"
	"strings"
)

// GitOperation is an operation run through plain git that weir stops.
type GitOperation string

const (
	// NoGitOperation is the answer for a command weir does not stop.
	NoGitOperation GitOperation = ""
	// GitCommit and GitPush are the only two. The test is whether the work
	// can still be taken back: a commit and a push are what leave, and
	// everything else — tag, reset --hard, rebase — is local history that can
	// be redone. A gate that also stopped those would be in the way of
	// ordinary git, and a gate people resent is a gate people route around.
	GitCommit GitOperation = "commit"
	GitPush   GitOperation = "push"
)

// stopped maps a git subcommand to the operation weir stops it as. push
// --force is not a case of its own: it is a push, and it is stopped as one.
var stopped = map[string]GitOperation{
	"commit": GitCommit,
	"push":   GitPush,
}

// gitOptionsTakingAValue are git's own options that carry their value in the
// next word. They have to be stepped over as a pair, or the value would be
// read as the subcommand.
var gitOptionsTakingAValue = map[string]bool{
	"-C":             true,
	"-c":             true,
	"--git-dir":      true,
	"--work-tree":    true,
	"--namespace":    true,
	"--exec-path":    true,
	"--config-env":   true,
	"--super-prefix": true,
}

// ErrUnreadableCommand is returned for a command weir cannot split into words
// at all. It is not "no operation found": weir did not get to look.
var ErrUnreadableCommand = errors.New("引用符が閉じていません")

// StoppedGitOperation reports which operation a command runs through plain
// git, out of the two weir stops. It answers NoGitOperation for everything
// else, reading git's own commands included: weir never speaks about what it
// does not stop.
//
// It sees only the command line it was handed. git run from inside a script,
// a Makefile target, or a shell alias is not in that line and is not found —
// this gate stands where the agent types, not around git itself.
func StoppedGitOperation(command string) (GitOperation, error) {
	segments, err := splitCommand(command)
	if err != nil {
		return NoGitOperation, err
	}
	for _, words := range segments {
		if op := operationOf(words); op != NoGitOperation {
			return op, nil
		}
	}
	return NoGitOperation, nil
}

// operationOf reads one simple command: the program it runs, and — when that
// is git — the subcommand it runs.
func operationOf(words []string) GitOperation {
	// A leading NAME=value is the environment the command runs in, not the
	// command itself.
	for len(words) > 0 && isAssignment(words[0]) {
		words = words[1:]
	}
	if len(words) == 0 || programName(words[0]) != "git" {
		return NoGitOperation
	}
	for i := 1; i < len(words); i++ {
		if !strings.HasPrefix(words[i], "-") {
			return stopped[words[i]]
		}
		if gitOptionsTakingAValue[words[i]] {
			i++
		}
	}
	return NoGitOperation
}

// programName is what a word names once its directory is dropped, so that
// /usr/bin/git is read as git.
func programName(word string) string {
	if i := strings.LastIndex(word, "/"); i >= 0 {
		return word[i+1:]
	}
	return word
}

// isAssignment reports whether word is a NAME=value put in front of a command.
func isAssignment(word string) bool {
	name, _, found := strings.Cut(word, "=")
	if !found || name == "" {
		return false
	}
	for i, c := range name {
		isLetter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if isLetter || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

// splitCommand cuts a command line into the simple commands it runs, each as
// its words. It expands nothing — no variable, no glob, no substitution's
// result — because weir gives no meaning to what the line did not spell out;
// it only needs to see where one command ends and the next begins.
func splitCommand(command string) ([][]string, error) {
	var (
		segments [][]string
		words    []string
		word     strings.Builder
	)

	endWord := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	endSegment := func() {
		endWord()
		if len(words) > 0 {
			segments = append(segments, words)
			words = nil
		}
	}

	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; c {
		case '\\':
			// The next character is itself, whatever it is.
			if i+1 < len(runes) {
				i++
				word.WriteRune(runes[i])
			}
		case '\'':
			// Inside single quotes nothing is special, so this is one word of
			// text — an argument, never another command.
			end := -1
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == '\'' {
					end = j
					break
				}
			}
			if end < 0 {
				return nil, ErrUnreadableCommand
			}
			word.WriteString(string(runes[i+1 : end]))
			i = end
		case '"':
			end := -1
			for j := i + 1; j < len(runes); j++ {
				if runes[j] == '\\' && j+1 < len(runes) {
					j++
					word.WriteRune(runes[j])
					continue
				}
				if runes[j] == '"' {
					end = j
					break
				}
				word.WriteRune(runes[j])
			}
			if end < 0 {
				return nil, ErrUnreadableCommand
			}
			i = end
		case ' ', '\t':
			endWord()
		case ';', '\n', '|', '&', '(', ')', '`':
			endSegment()
		case '$':
			// $( opens a command substitution, and what runs inside it is a
			// command of weir's concern like any other.
			if i+1 < len(runes) && runes[i+1] == '(' {
				endSegment()
				i++
				continue
			}
			word.WriteRune(c)
		default:
			word.WriteRune(c)
		}
	}
	endSegment()

	return segments, nil
}
