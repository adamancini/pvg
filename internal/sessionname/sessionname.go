// Package sessionname manages one-shot rename-suggestion state per Claude Code session.
//
// When a main Claude Code session starts in a paivot-graph project, the
// UserPromptSubmit hook emits a rename suggestion on the first prompt.
// State is persisted in .vault/.sessionname-state.json and keyed by session_id
// so the suggestion is idempotent per session.
package sessionname

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// stateFile is the filename within .vault/ for sessionname state.
const stateFile = ".sessionname-state.json"

// Emission records whether a rename suggestion has been emitted for a session.
type Emission struct {
	Emitted   bool   `json:"emitted"`
	Timestamp string `json:"timestamp,omitempty"`
}

// State tracks rename-suggestion emissions keyed by session_id.
type State struct {
	Emissions map[string]Emission `json:"emissions"`
}

// ShouldSuggest returns true if a rename suggestion has not yet been emitted
// for the given sessionID in the project rooted at projectRoot.
func ShouldSuggest(projectRoot, sessionID string) bool {
	state, err := readState(projectRoot)
	if err != nil {
		return true // no state yet, suggest
	}
	if state.Emissions == nil {
		return true
	}
	emission, ok := state.Emissions[sessionID]
	return !ok || !emission.Emitted
}

// MarkSuggested records that a rename suggestion has been emitted for the
// given sessionID. Idempotent: safe to call multiple times.
func MarkSuggested(projectRoot, sessionID string) error {
	state, err := readState(projectRoot)
	if err != nil {
		state = &State{Emissions: make(map[string]Emission)}
	}
	if state.Emissions == nil {
		state.Emissions = make(map[string]Emission)
	}
	state.Emissions[sessionID] = Emission{
		Emitted:   true,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	return writeState(projectRoot, *state)
}

// StateFileName returns the state file basename (for gitignore checks).
func StateFileName() string {
	return stateFile
}

func statePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".vault", stateFile)
}

func readState(projectRoot string) (*State, error) {
	path, _, err := findStateFile(projectRoot)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse sessionname state: %w", err)
	}
	if state.Emissions == nil {
		state.Emissions = make(map[string]Emission)
	}
	return &state, nil
}

func writeState(start string, state State) error {
	root := findVaultRoot(start)
	path := statePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create sessionname state directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sessionname state: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func findStateFile(start string) (path, root string, err error) {
	dir := filepath.Clean(start)
	for {
		candidate := statePath(dir)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", os.ErrNotExist
}

// findVaultRoot walks up from start looking for a .vault/ directory.
// If found, returns that directory; otherwise returns start itself.
func findVaultRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if _, statErr := os.Stat(filepath.Join(dir, ".vault")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Clean(start)
}
