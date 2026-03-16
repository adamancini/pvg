package guard

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paivot-ai/pvg/internal/dispatcher"
)

// dfArtifacts are the D&F file basenames that dispatcher mode protects.
var dfArtifacts = map[string]string{
	"BUSINESS.md":     "business-analyst",
	"DESIGN.md":       "designer",
	"ARCHITECTURE.md": "architect",
}

var ndMutatingCommandRe = regexp.MustCompile(`(?:^|[;&|]\s*)(?:\S*/)?nd\s+(?:--\S+(?:\s+\S+)?\s+)*(create|update|close|reopen|delete|defer|undefer|labels\s+(?:add|remove|rm)|comments\s+add|dep\s+(?:add|rm|relate|unrelate)|link|unlink)\b`)

var dispatcherMutatingAgents = []string{
	"paivot-graph:sr-pm",
	"paivot-graph:developer",
	"paivot-graph:pm",
}

// CheckDispatcher enforces dispatcher mode: blocks D&F file writes when
// dispatcher mode is active and no BLT agent is currently tracked.
// Fail-open: if state file is missing or unreadable, allows the operation.
func CheckDispatcher(projectRoot string, input HookInput) Result {
	if projectRoot == "" {
		return Result{Allowed: true}
	}

	state, _, err := dispatcher.ReadStateRoot(projectRoot)
	if err != nil || !state.Enabled {
		return Result{Allowed: true}
	}

	switch input.ToolName {
	case "Edit", "Write":
		return checkDFFilePath(projectRoot, state, input.ToolInput.FilePath)
	case "Bash":
		if result := checkDFBashCommand(projectRoot, state, input.ToolInput.Command); !result.Allowed {
			return result
		}
		return checkDispatcherNDMutation(projectRoot, state, input.ToolInput.Command)
	default:
		return Result{Allowed: true}
	}
}

func checkDFFilePath(projectRoot string, state *dispatcher.State, filePath string) Result {
	if filePath == "" {
		return Result{Allowed: true}
	}

	base := filepath.Base(filePath)
	agentName, isDFFile := dfArtifacts[base]
	if !isDFFile {
		return Result{Allowed: true}
	}

	if dfWriteAllowed(projectRoot, state, agentName) {
		return Result{Allowed: true}
	}

	return Result{
		Allowed: false,
		Reason:  dfBlockMsg(base, agentName),
	}
}

func checkDFBashCommand(projectRoot string, state *dispatcher.State, command string) Result {
	if command == "" {
		return Result{Allowed: true}
	}

	// Check if the command targets any D&F artifact
	for artifact, agentName := range dfArtifacts {
		if !strings.Contains(command, artifact) {
			continue
		}

		// Check for write operations targeting this artifact
		hasWriteOp := false
		for _, op := range []string{">>", ">"} {
			if idx := strings.Index(command, op); idx >= 0 {
				if strings.Contains(command[idx:], artifact) {
					hasWriteOp = true
					break
				}
			}
		}
		if !hasWriteOp {
			writePatterns := []string{
				"tee ", "cp ", "mv ", "cat >",
				"sed -i", "perl -pi",
			}
			for _, pattern := range writePatterns {
				if strings.Contains(command, pattern) {
					hasWriteOp = true
					break
				}
			}
		}

		if hasWriteOp && !dfWriteAllowed(projectRoot, state, agentName) {
			return Result{
				Allowed: false,
				Reason:  dfBlockMsg(artifact, agentName),
			}
		}
	}

	return Result{Allowed: true}
}

func dfWriteAllowed(projectRoot string, state *dispatcher.State, agentName string) bool {
	if projectRoot == "" {
		return false
	}
	return dispatcher.HasActiveAgentTypeAtPath(state, "paivot-graph:"+agentName, projectRoot)
}

func dfBlockMsg(artifact, agentName string) string {
	return fmt.Sprintf(
		"BLOCKED: Dispatcher mode is active. D&F artifacts must be produced by BLT agents.\n"+
			"Only the matching BLT agent may write each artifact.\n"+
			"Spawn the appropriate agent:\n"+
			"  - BUSINESS.md --> business-analyst agent\n"+
			"  - DESIGN.md --> designer agent\n"+
			"  - ARCHITECTURE.md --> architect agent\n\n"+
			"To write %s, spawn the %s agent.",
		artifact, agentName)
}

func checkDispatcherNDMutation(projectRoot string, state *dispatcher.State, command string) Result {
	if command == "" || !ndMutatingCommandRe.MatchString(command) {
		return Result{Allowed: true}
	}

	if dispatcherWriteAllowed(projectRoot, state) {
		return Result{Allowed: true}
	}

	return Result{
		Allowed: false,
		Reason: "BLOCKED: Dispatcher mode is active. Mutating nd commands must be delegated to a tracked production agent.\n" +
			"The coordinator may read nd state, but story/backlog mutations must come from the responsible agent worktree.\n" +
			"Use:\n" +
			"  - sr-pm for story/backlog creation and repair\n" +
			"  - developer for delivery/progress updates\n" +
			"  - pm for accept/reject and close/reopen actions",
	}
}

func dispatcherWriteAllowed(projectRoot string, state *dispatcher.State) bool {
	for _, agentType := range dispatcherMutatingAgents {
		if dispatcher.HasActiveAgentTypeAtPath(state, agentType, projectRoot) {
			return true
		}
	}
	return false
}
