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
