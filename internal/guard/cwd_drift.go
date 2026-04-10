package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paivot-ai/pvg/internal/dispatcher"
	"github.com/paivot-ai/pvg/internal/worktree"
)

// CheckCWDDrift detects when the dispatcher's shell CWD has silently drifted
// into a worktree after a developer or PM subagent completed. This catches the
// session-fatal failure mode where the dispatcher removes a worktree while CWD
// is inside it, permanently breaking all subsequent Bash commands.
//
// The check fires when BOTH conditions are met:
//   - CWD is inside .claude/worktrees/
//   - Dispatcher mode is active (Paivot is running)
//
// Developer subagent Bash commands run in the DEVELOPER'S own session, not the
// dispatcher's. The dispatcher never needs CWD inside a worktree. Any CWD drift
// into a worktree while dispatcher mode is on indicates a completed subagent
// whose CWD persisted in the dispatcher shell.
func CheckCWDDrift(projectRoot string) Result {
	cwd, err := os.Getwd()
	if err != nil {
		return Result{Allowed: true}
	}

	// Quick negative: not inside any worktree directory.
	if !strings.Contains(cwd, string(filepath.Separator)+".claude"+string(filepath.Separator)+"worktrees"+string(filepath.Separator)) {
		return Result{Allowed: true}
	}

	// Resolve the main repo root from the worktree path.
	mainRoot, resolveErr := worktree.ResolveProjectRoot(cwd)
	if resolveErr != nil {
		// Can't resolve main repo -- fall back to projectRoot from guard input.
		mainRoot = projectRoot
	}

	// Check if dispatcher mode is active.
	state, _, stateErr := dispatcher.ReadStateRoot(mainRoot)
	if stateErr != nil || !state.Enabled {
		return Result{Allowed: true}
	}

	// CWD is inside a worktree and dispatcher mode is on.
	// This means CWD drifted from a completed subagent.
	return Result{
		Allowed: false,
		Reason: fmt.Sprintf(
			"BLOCKED: Shell CWD has drifted into a worktree directory.\n"+
				"Current CWD: %s\n\n"+
				"This happens when a developer/PM agent completes -- their CWD\n"+
				"persists in the dispatcher's shell. Reset IMMEDIATELY:\n"+
				"  cd %s\n\n"+
				"Then verify with: pwd",
			cwd, mainRoot),
	}
}
