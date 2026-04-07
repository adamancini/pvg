package guard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckCWDDrift_NormalCWD(t *testing.T) {
	// CWD not inside a worktree -- always allowed.
	result := CheckCWDDrift("/some/project")
	if !result.Allowed {
		t.Fatalf("expected allowed for normal CWD, got blocked: %s", result.Reason)
	}
}

func TestCheckCWDDrift_WorktreeCWDWithActiveAgent(t *testing.T) {
	// Simulate: CWD inside worktree, dispatcher active, developer agent active.
	// This should be ALLOWED (developer legitimately working there).

	root := t.TempDir()
	worktreeDir := filepath.Join(root, ".claude", "worktrees", "dev-TEST-001")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Resolve symlinks so the worktree path matches what os.Getwd() returns
	// (macOS /var/folders -> /private/var/folders).
	resolvedWorktree := worktreeDir
	if resolved, err := filepath.EvalSymlinks(worktreeDir); err == nil {
		resolvedWorktree = resolved
	}

	// Create dispatcher state with an active developer at the worktree path.
	stateDir := filepath.Join(root, ".vault")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := map[string]interface{}{
		"enabled":         true,
		"since":           "2026-01-01T00:00:00Z",
		"active_agents":   map[string]string{"agent-1": "paivot-graph:developer"},
		"agent_worktrees": map[string]string{"agent-1": resolvedWorktree},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(stateDir, ".dispatcher-state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// cd into the worktree to simulate drift.
	origDir, _ := os.Getwd()
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	result := CheckCWDDrift(root)
	if !result.Allowed {
		t.Fatalf("expected allowed (active developer agent), got blocked: %s", result.Reason)
	}
}

func TestCheckCWDDrift_WorktreeCWDNoActiveAgent(t *testing.T) {
	// Simulate: CWD inside worktree, dispatcher active, NO active agents.
	// This should be BLOCKED (CWD drifted from completed agent).

	root := t.TempDir()
	worktreeDir := filepath.Join(root, ".claude", "worktrees", "dev-TEST-001")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create dispatcher state with NO active agents.
	stateDir := filepath.Join(root, ".vault")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := map[string]interface{}{
		"enabled":         true,
		"since":           "2026-01-01T00:00:00Z",
		"active_agents":   map[string]string{},
		"agent_worktrees": map[string]string{},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(stateDir, ".dispatcher-state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// cd into the worktree to simulate drift.
	origDir, _ := os.Getwd()
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	result := CheckCWDDrift(root)
	if result.Allowed {
		t.Fatal("expected BLOCKED (CWD drifted, no active agent), got allowed")
	}
	if result.Reason == "" {
		t.Fatal("expected a reason message")
	}
}

func TestCheckCWDDrift_DispatcherModeOff(t *testing.T) {
	// CWD inside worktree but dispatcher mode is OFF.
	// Should be ALLOWED (not running Paivot).

	root := t.TempDir()
	worktreeDir := filepath.Join(root, ".claude", "worktrees", "dev-TEST-001")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No dispatcher state file -- mode is off.

	origDir, _ := os.Getwd()
	if err := os.Chdir(worktreeDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	result := CheckCWDDrift(root)
	if !result.Allowed {
		t.Fatalf("expected allowed (dispatcher mode off), got blocked: %s", result.Reason)
	}
}
