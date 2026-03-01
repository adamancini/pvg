// Package dispatcher manages dispatcher mode state for the paivot-graph plugin.
//
// When dispatcher mode is active, the orchestrator (main Claude session) is
// restricted to coordination only -- it cannot write D&F artifacts directly.
// BLT agents (BA, Designer, Architect) are tracked so the guard can distinguish
// agent writes from orchestrator writes.
//
// State is persisted in .vault/.dispatcher-state.json (not .vault/knowledge/,
// to avoid creating the project knowledge directory as a side effect).
package dispatcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// stateFile is the filename within .vault/ for dispatcher state.
// Lives in .vault/ (not .vault/knowledge/) because it is ephemeral runtime
// state, not knowledge. This avoids creating .vault/knowledge/ as a side
// effect in projects that don't use a project vault.
const stateFile = ".dispatcher-state.json"

// State represents the current dispatcher mode state.
type State struct {
	Enabled      bool              `json:"enabled"`
	Since        string            `json:"since"`
	ActiveAgents map[string]string `json:"active_agents"` // agent_id -> agent_type
}

// statePath returns the full path to the state file for a project root.
func statePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".vault", stateFile)
}

// On enables dispatcher mode for the given project root.
// Also initializes the project vault structure if it doesn't exist --
// saying "use Paivot" is an explicit opt-in, so we prepare the soil.
func On(projectRoot string) error {
	initProjectVault(projectRoot)

	state := State{
		Enabled:      true,
		Since:        time.Now().UTC().Format(time.RFC3339),
		ActiveAgents: make(map[string]string),
	}
	return writeState(projectRoot, state)
}

// knowledgeDirs are the subdirectories created under .vault/knowledge/
// when Paivot is activated. Matches the layout that session_start scans.
var knowledgeDirs = []string{
	"conventions",
	"decisions",
	"patterns",
	"debug",
	"skills",
}

// initProjectVault ensures the .vault/ directory tree exists.
// Idempotent: creates only what's missing. Never overwrites existing files.
func initProjectVault(projectRoot string) {
	base := filepath.Join(projectRoot, ".vault")

	// .vault/knowledge/ and its subdirectories
	for _, sub := range knowledgeDirs {
		_ = os.MkdirAll(filepath.Join(base, "knowledge", sub), 0755)
	}

	// .vault/issues/ for nd
	_ = os.MkdirAll(filepath.Join(base, "issues"), 0755)

	// Default .settings.yaml if it doesn't exist
	settingsPath := filepath.Join(base, "knowledge", ".settings.yaml")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		defaultSettings := "# paivot-graph project vault settings\n# Managed by: pvg settings key=value\n\nstack_detection: false\n"
		_ = os.WriteFile(settingsPath, []byte(defaultSettings), 0644)
	}
}

// Off disables dispatcher mode by removing the state file.
func Off(projectRoot string) error {
	path := statePath(projectRoot)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Status prints the current dispatcher mode state to stdout.
func Status(projectRoot string) {
	state, err := ReadState(projectRoot)
	if err != nil {
		fmt.Println("Dispatcher mode: off (no state file)")
		return
	}
	if !state.Enabled {
		fmt.Println("Dispatcher mode: off")
		return
	}

	fmt.Printf("Dispatcher mode: on (since %s)\n", state.Since)
	if len(state.ActiveAgents) == 0 {
		fmt.Println("Active BLT agents: none")
	} else {
		fmt.Println("Active BLT agents:")
		for id, agentType := range state.ActiveAgents {
			fmt.Printf("  %s (%s)\n", id, agentType)
		}
	}
}

// ReadState reads the dispatcher state from disk.
// Returns an error if the file does not exist or cannot be parsed.
func ReadState(projectRoot string) (*State, error) {
	path := statePath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse dispatcher state: %w", err)
	}
	return &state, nil
}

// TrackAgent adds a BLT agent to the active agents map.
func TrackAgent(projectRoot, agentID, agentType string) error {
	state, err := ReadState(projectRoot)
	if err != nil {
		// If no state file, dispatcher mode is off -- nothing to track
		return nil
	}
	if !state.Enabled {
		return nil
	}

	if state.ActiveAgents == nil {
		state.ActiveAgents = make(map[string]string)
	}
	state.ActiveAgents[agentID] = agentType
	return writeState(projectRoot, *state)
}

// UntrackAgent removes a BLT agent from the active agents map.
func UntrackAgent(projectRoot, agentID string) error {
	state, err := ReadState(projectRoot)
	if err != nil {
		return nil
	}
	if !state.Enabled {
		return nil
	}

	delete(state.ActiveAgents, agentID)
	return writeState(projectRoot, *state)
}

// HasActiveBLTAgent returns true if any BLT agent is currently tracked.
func HasActiveBLTAgent(state *State) bool {
	return len(state.ActiveAgents) > 0
}

// StateFileName returns the state file basename (for guard exemption checks).
func StateFileName() string {
	return stateFile
}

func writeState(projectRoot string, state State) error {
	path := statePath(projectRoot)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dispatcher state: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
