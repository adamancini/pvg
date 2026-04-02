package guard

import (
	"regexp"
	"strings"
)

// worktreeCdRe matches cd commands that navigate into .claude/worktrees/.
// Captures explicit cd, pushd, or chained cd after && or ;.
// Does NOT match commands that merely reference the path without cd'ing
// (e.g., git worktree add, pvg worktree remove, ls .claude/worktrees/).
var worktreeCdRe = regexp.MustCompile(
	`(?:^|[;&|]\s*)(?:cd|pushd)\s+[^\s;|&]*\.claude/worktrees/`,
)

const worktreeCdBlockMsg = "BLOCKED: Dispatcher must never cd into a worktree directory.\n" +
	"CWD inside a worktree becomes invalid when the worktree is removed,\n" +
	"permanently breaking all Bash commands for the rest of the session.\n\n" +
	"Instead:\n" +
	"  - Spawn an Agent to work inside the worktree\n" +
	"  - Use pvg/nd/git commands that accept paths as arguments\n" +
	"  - Use `git -C <worktree-path>` to run git commands from outside"

// CheckWorktreeCd blocks Bash commands that would cd into .claude/worktrees/.
// The dispatcher has no legitimate reason to change its CWD into a worktree --
// agents work inside worktrees, the dispatcher manages them from the project root.
func CheckWorktreeCd(command string) Result {
	if command == "" {
		return Result{Allowed: true}
	}

	// Quick negative: skip regex if the command doesn't even mention worktrees.
	if !strings.Contains(command, ".claude/worktrees/") {
		return Result{Allowed: true}
	}

	if worktreeCdRe.MatchString(command) {
		return Result{Allowed: false, Reason: worktreeCdBlockMsg}
	}

	return Result{Allowed: true}
}
