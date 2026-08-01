package hook

import "testing"

func TestStoppedGitOperation(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    GitOperation
	}{
		// What leaves, and cannot be taken back.
		{"commit", `git commit -m "wip"`, GitCommit},
		{"commit with everything staged for it", `git commit -a -m x`, GitCommit},
		{"push", `git push`, GitPush},
		{"push to a named remote and branch", `git push origin main`, GitPush},
		{"force push is a push, not a case of its own", `git push --force origin main`, GitPush},
		{"force-with-lease is a push too", `git push --force-with-lease`, GitPush},

		// Local history: it can be redone, so the gate stays out of the way.
		{"status", `git status --short`, NoGitOperation},
		{"diff", `git diff --cached`, NoGitOperation},
		{"log", `git log --oneline -5`, NoGitOperation},
		{"show", `git show HEAD`, NoGitOperation},
		{"tag", `git tag -a v1.0.0 -m x`, NoGitOperation},
		{"reset --hard", `git reset --hard origin/main`, NoGitOperation},
		{"rebase", `git rebase -i main`, NoGitOperation},
		{"git with no subcommand", `git`, NoGitOperation},
		{"git asked for its version", `git --version`, NoGitOperation},

		// Not git at all.
		{"gh pr create is out of the gate's reach", `gh pr create --fill`, NoGitOperation},
		{"weir's own commands go through untouched", `weir commit --repo weir --message x`, NoGitOperation},
		{"a make target that may run git inside is not seen", `make release`, NoGitOperation},

		// The command line is a line of shell, not one program.
		{"a commit after a cd", `cd /repo && git commit -m x`, GitCommit},
		{"a commit after a staging step", `git add -A && git commit -m wip`, GitCommit},
		{"a push after a semicolon", `git status; git push`, GitPush},
		{"a push inside a command substitution", `echo $(git push)`, GitPush},
		{"a push inside backticks", "echo `git push`", GitPush},
		{"a commit whose output is redirected", `git commit -m x 2>&1`, GitCommit},
		{"a commit reached by its full path", `/usr/bin/git commit -m x`, GitCommit},
		{"a commit with the environment set in front", `GIT_AUTHOR_NAME=x git commit -m y`, GitCommit},
		{"a commit with git's own -C", `git -C /repo commit -m x`, GitCommit},
		{"a commit with git's own -c", `git -c user.name=x commit -m y`, GitCommit},
		{"a commit with git's own --git-dir=", `git --git-dir=/repo/.git commit -m x`, GitCommit},

		// Quoted text is an argument. The word being inside it is not the
		// command being run — this is where a substring match goes wrong.
		{"a git line quoted as text for echo", `echo 'git push --force'`, NoGitOperation},
		{"a git line quoted as text for echo, in double quotes", `echo "git push --force"`, NoGitOperation},
		{"a commit whose message talks about pushing", `git commit -m "do not git push this"`, GitCommit},
		{"a message that talks about committing", `git status -m "git commit"`, NoGitOperation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StoppedGitOperation(tt.command)
			if err != nil {
				t.Fatalf("StoppedGitOperation(%q) error = %v, want none", tt.command, err)
			}
			if got != tt.want {
				t.Errorf("StoppedGitOperation(%q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

// A line weir cannot come apart is a line weir cannot see into, and it says so
// rather than answering "nothing found".
func TestStoppedGitOperationCannotReadAnUnclosedQuote(t *testing.T) {
	for _, command := range []string{`git commit -m "oops`, `git commit -m 'oops`} {
		if _, err := StoppedGitOperation(command); err == nil {
			t.Errorf("StoppedGitOperation(%q) = no error, want one", command)
		}
	}
}
