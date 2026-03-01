package dispatcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOn_CreatesStateFile(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".vault")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := On(dir); err != nil {
		t.Fatalf("On() error: %v", err)
	}

	state, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState() error: %v", err)
	}
	if !state.Enabled {
		t.Error("expected enabled=true")
	}
	if state.Since == "" {
		t.Error("expected non-empty since timestamp")
	}
	if state.ActiveAgents == nil {
		t.Error("expected non-nil active_agents")
	}
}

func TestOff_RemovesStateFile(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".vault")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := On(dir); err != nil {
		t.Fatal(err)
	}

	if err := Off(dir); err != nil {
		t.Fatalf("Off() error: %v", err)
	}

	_, err := ReadState(dir)
	if err == nil {
		t.Error("expected error reading state after Off()")
	}
}

func TestOff_NoopWhenNoStateFile(t *testing.T) {
	dir := t.TempDir()
	if err := Off(dir); err != nil {
		t.Fatalf("Off() with no state file should not error: %v", err)
	}
}

func TestReadState_ErrorOnMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadState(dir)
	if err == nil {
		t.Error("expected error for missing state file")
	}
}

func TestTrackAgent_AddsToActiveAgents(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".vault")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := On(dir); err != nil {
		t.Fatal(err)
	}

	if err := TrackAgent(dir, "agent-123", "paivot-graph:designer"); err != nil {
		t.Fatalf("TrackAgent() error: %v", err)
	}

	state, err := ReadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveAgents["agent-123"] != "paivot-graph:designer" {
		t.Errorf("expected agent-123=paivot-graph:designer, got %v", state.ActiveAgents)
	}
}

func TestTrackAgent_NoopWhenNotEnabled(t *testing.T) {
	dir := t.TempDir()
	// No state file -- TrackAgent should be a no-op
	if err := TrackAgent(dir, "agent-123", "paivot-graph:designer"); err != nil {
		t.Fatalf("TrackAgent() without state should not error: %v", err)
	}
}

func TestUntrackAgent_RemovesFromActiveAgents(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".vault")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := On(dir); err != nil {
		t.Fatal(err)
	}
	if err := TrackAgent(dir, "agent-123", "paivot-graph:designer"); err != nil {
		t.Fatal(err)
	}

	if err := UntrackAgent(dir, "agent-123"); err != nil {
		t.Fatalf("UntrackAgent() error: %v", err)
	}

	state, err := ReadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.ActiveAgents) != 0 {
		t.Errorf("expected empty active_agents, got %v", state.ActiveAgents)
	}
}

func TestHasActiveBLTAgent_TrueWhenAgentPresent(t *testing.T) {
	state := &State{
		Enabled:      true,
		ActiveAgents: map[string]string{"agent-1": "paivot-graph:architect"},
	}
	if !HasActiveBLTAgent(state) {
		t.Error("expected true when agent is present")
	}
}

func TestHasActiveBLTAgent_FalseWhenEmpty(t *testing.T) {
	state := &State{
		Enabled:      true,
		ActiveAgents: map[string]string{},
	}
	if HasActiveBLTAgent(state) {
		t.Error("expected false when no agents")
	}
}

func TestStateFileName(t *testing.T) {
	if StateFileName() != ".dispatcher-state.json" {
		t.Errorf("unexpected state file name: %s", StateFileName())
	}
}

func TestOn_CreatesDirectoryIfNeeded(t *testing.T) {
	dir := t.TempDir()
	// Don't pre-create .vault/
	if err := On(dir); err != nil {
		t.Fatalf("On() should create directories: %v", err)
	}

	state, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState() error after On(): %v", err)
	}
	if !state.Enabled {
		t.Error("expected enabled=true")
	}
}

func TestOn_InitializesProjectVault(t *testing.T) {
	dir := t.TempDir()
	// Start from nothing -- On() should build the full vault tree
	if err := On(dir); err != nil {
		t.Fatalf("On() error: %v", err)
	}

	// Check knowledge subdirectories
	for _, sub := range knowledgeDirs {
		path := filepath.Join(dir, ".vault", "knowledge", sub)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected .vault/knowledge/%s/ to exist: %v", sub, err)
		} else if !info.IsDir() {
			t.Errorf("expected .vault/knowledge/%s/ to be a directory", sub)
		}
	}

	// Check .vault/issues/
	issuesDir := filepath.Join(dir, ".vault", "issues")
	info, err := os.Stat(issuesDir)
	if err != nil {
		t.Errorf("expected .vault/issues/ to exist: %v", err)
	} else if !info.IsDir() {
		t.Error("expected .vault/issues/ to be a directory")
	}

	// Check default .settings.yaml
	settingsPath := filepath.Join(dir, ".vault", "knowledge", ".settings.yaml")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("expected .settings.yaml to exist: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty .settings.yaml")
	}
}

func TestOn_DoesNotOverwriteExistingSettings(t *testing.T) {
	dir := t.TempDir()
	settingsDir := filepath.Join(dir, ".vault", "knowledge")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	custom := "# custom settings\nstack_detection: true\nworkflow.fsm: true\n"
	settingsPath := filepath.Join(settingsDir, ".settings.yaml")
	if err := os.WriteFile(settingsPath, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}

	if err := On(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Errorf("settings were overwritten: got %q, want %q", string(data), custom)
	}
}

func TestStateJSON_RoundTrip(t *testing.T) {
	original := State{
		Enabled: true,
		Since:   "2026-02-26T10:00:00Z",
		ActiveAgents: map[string]string{
			"a1": "paivot-graph:business-analyst",
			"a2": "paivot-graph:designer",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var restored State
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.Enabled != original.Enabled {
		t.Errorf("enabled mismatch: %v vs %v", restored.Enabled, original.Enabled)
	}
	if len(restored.ActiveAgents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(restored.ActiveAgents))
	}
}
