// Package gitcmd runs git inside a repository weir has already resolved.
//
// git is run as a program and never through a shell, so a message, a path or a
// branch name is handed over as one argument and cannot turn into a second
// command on the way. And git's own output is passed straight through: it says
// what happened more precisely than weir could restate it, and a person who
// was sent here from plain git should still recognise the answer.
package gitcmd

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// Commit makes a commit in the repository at dir, with message as its message.
// With all, changes to tracked files that were never staged are committed too.
func Commit(dir, message string, all bool, stdout, stderr io.Writer) error {
	args := []string{"commit"}
	if all {
		args = append(args, "--all")
	}
	args = append(args, "--message", message)
	return run(dir, args, stdout, stderr)
}

// Push sends the repository at dir to where plain git would send it. Neither a
// remote nor a branch is passed: git's own default — the current branch's
// upstream — is the destination, so weir and plain git cannot disagree about
// where a push landed.
func Push(dir string, stdout, stderr io.Writer) error {
	return run(dir, []string{"push"}, stdout, stderr)
}

// PushTag sends one tag from the repository at dir to remote.
//
// The remote is named here, unlike Push, because a tag has no upstream to
// follow — but it is not the caller's to choose: it is the destination weir
// worked the surface out against. Passing a different one would send the tag
// somewhere other than where the judging was done.
//
// `tag <name>` rather than the bare name: a refspec that is only a name is
// ambiguous when a branch shares it, and this says which one is meant.
func PushTag(dir, remote, name string, stdout, stderr io.Writer) error {
	return run(dir, []string{"push", remote, "tag", name}, stdout, stderr)
}

// run runs git in dir and reports whether it succeeded.
func run(dir string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// git has already said why on stderr. Name what failed and leave
			// the diagnosis to it, rather than guessing at a second one.
			return fmt.Errorf("%s: git %s が失敗しました（終了コード %d）", dir, args[0], exitErr.ExitCode())
		}
		return fmt.Errorf("%s: git を実行できません: %w", dir, err)
	}
	return nil
}
