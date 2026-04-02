package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/paivot-ai/pvg/internal/dispatcher"
)

// subagentInput matches the JSON Claude Code sends to SubagentStart/SubagentStop hooks.
type subagentInput struct {
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
}

// trackedAgentTypes are the agent types that dispatcher mode must track for
// structural enforcement.
var trackedAgentTypes = map[string]bool{
	"paivot-graph:business-analyst": true,
	"paivot-graph:designer":         true,
	"paivot-graph:architect":        true,
	"paivot-graph:sr-pm":            true,
	"paivot-graph:developer":        true,
	"paivot-graph:pm":               true,
}

// worktreeAgentTypes are agent types that typically work inside git worktrees.
// After these agents complete, the dispatcher's CWD may have drifted into the
// worktree. Emitting a reset instruction prevents the session-fatal CWD
// corruption that occurs when a worktree is removed while CWD points into it.
var worktreeAgentTypes = map[string]bool{
	"paivot-graph:developer": true,
	"paivot-graph:pm":        true,
}

// SubagentStart tracks a dispatcher-relevant subagent when it starts.
// Silent output -- does not block agent launch.
func SubagentStart() error {
	var input subagentInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return nil
	}

	if !trackedAgentTypes[input.AgentType] {
		return nil
	}

	cwd, _ := os.Getwd()
	if cwd == "" {
		return nil
	}

	_ = dispatcher.TrackAgent(cwd, input.AgentID, input.AgentType)
	return nil
}

// SubagentStop untracks a dispatcher-relevant subagent when it completes.
// For worktree-using agents (developer, PM), emits a CWD verification
// instruction to prevent session-fatal CWD corruption.
func SubagentStop() error {
	var input subagentInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return nil
	}

	if !trackedAgentTypes[input.AgentType] {
		return nil
	}

	cwd, _ := os.Getwd()
	if cwd == "" {
		return nil
	}

	_ = dispatcher.UntrackAgent(cwd, input.AgentID)

	if worktreeAgentTypes[input.AgentType] {
		emitCWDResetWarning(cwd)
	}
	return nil
}

// emitCWDResetWarning checks if any worktrees exist and outputs a structural
// CWD reset instruction. The output is visible to the model via the hook
// result. This fires automatically after every developer/PM agent, making it
// impossible to forget.
func emitCWDResetWarning(cwd string) {
	root := resolveGitRootQuiet()
	if root == "" {
		root = cwd
	}

	worktreeDir := filepath.Join(root, ".claude", "worktrees")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil || len(entries) == 0 {
		return // no worktrees -- no risk
	}

	fmt.Printf("[CWD-SAFETY] Agent completed. Worktrees exist at .claude/worktrees/.\n")
	fmt.Printf("Your shell CWD may have drifted. BEFORE any worktree operation, run:\n")
	fmt.Printf("  cd %s\n", root)
	fmt.Printf("Then verify with: pwd\n")
}

// resolveGitRootQuiet returns the git repo root or empty string on failure.
func resolveGitRootQuiet() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
