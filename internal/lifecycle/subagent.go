package lifecycle

import (
	"encoding/json"
	"os"

	"github.com/paivot-ai/pvg/internal/dispatcher"
)

// subagentInput matches the JSON Claude Code sends to SubagentStart/SubagentStop hooks.
type subagentInput struct {
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
}

// bltAgentTypes are the agent types that produce D&F artifacts.
var bltAgentTypes = map[string]bool{
	"paivot-graph:business-analyst": true,
	"paivot-graph:designer":         true,
	"paivot-graph:architect":        true,
}

// SubagentStart tracks a BLT agent when it starts.
// Silent output -- does not block agent launch.
func SubagentStart() error {
	var input subagentInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return nil
	}

	if !bltAgentTypes[input.AgentType] {
		return nil
	}

	cwd, _ := os.Getwd()
	if cwd == "" {
		return nil
	}

	_ = dispatcher.TrackAgent(cwd, input.AgentID, input.AgentType)
	return nil
}

// SubagentStop untracks a BLT agent when it completes.
// Silent output -- does not block agent completion.
func SubagentStop() error {
	var input subagentInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return nil
	}

	if !bltAgentTypes[input.AgentType] {
		return nil
	}

	cwd, _ := os.Getwd()
	if cwd == "" {
		return nil
	}

	_ = dispatcher.UntrackAgent(cwd, input.AgentID)
	return nil
}
