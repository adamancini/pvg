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
// For developer agents, proactively removes their worktree to prevent the
// session-fatal CWD corruption that occurs when a worktree is removed while
// the dispatcher's CWD points into it.
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

	if input.AgentType == "paivot-graph:developer" {
		cleanupDevWorktrees(cwd)
	}

	if worktreeAgentTypes[input.AgentType] {
		emitCWDResetWarning(cwd, input.AgentType)
	}
	return nil
}

// cleanupDevWorktrees removes developer worktrees (dev-*) that have no active
// agent. Uses git -C to operate from the project root, making it immune to
// CWD corruption. This runs in SubagentStop so the worktree is removed before
// Claude Code can propagate the subagent's CWD to the parent session.
func cleanupDevWorktrees(cwd string) {
	root := resolveGitRootQuiet()
	if root == "" {
		return
	}

	// Only act when dispatcher mode is active.
	state, _, err := dispatcher.ReadStateRoot(root)
	if err != nil || !state.Enabled {
		return
	}

	worktreeDir := filepath.Join(root, ".claude", "worktrees")
	entries, err := os.ReadDir(worktreeDir)
	if err != nil {
		return
	}

	// Check if any other developer agent is still active. If so, we cannot
	// safely determine which worktree belongs to which agent, so skip cleanup.
	for _, activeType := range state.ActiveAgents {
		if activeType == "paivot-graph:developer" {
			return // another developer is running -- do not disturb
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "dev-") {
			continue
		}
		wtPath := filepath.Join(worktreeDir, entry.Name())
		// Use -C to run from project root, bypassing any CWD issues.
		cmd := exec.Command("git", "-C", root, "worktree", "remove", "--force", wtPath)
		_ = cmd.Run()
	}

	// Prune stale worktree metadata.
	cmd := exec.Command("git", "-C", root, "worktree", "prune")
	_ = cmd.Run()
}

// emitCWDResetWarning checks if dispatcher mode is active and outputs a
// mandatory CWD reset instruction. This is the last line of defense against
// session-fatal CWD corruption caused by Claude Code leaking the subagent's
// CWD to the parent session.
func emitCWDResetWarning(cwd, agentType string) {
	// Only emit when Paivot dispatcher is active.
	state, _, err := dispatcher.ReadStateRoot(cwd)
	if err != nil || !state.Enabled {
		return
	}

	root := resolveGitRootQuiet()
	if root == "" {
		root = cwd
	}

	fmt.Printf("[CWD-RESET MANDATORY] %s agent completed.\n", agentType)
	fmt.Printf("Your VERY FIRST Bash command MUST be:\n")
	fmt.Printf("  cd %s && pwd\n", root)
	fmt.Printf("If you skip this, your session WILL die. This is not optional.\n")
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
