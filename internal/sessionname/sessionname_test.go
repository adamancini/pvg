package sessionname

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldSuggest_NoState_ReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	if !ShouldSuggest(dir, "sess-123") {
		t.Error("expected ShouldSuggest=true when no state file exists")
	}
}

func TestShouldSuggest_UnknownSession_ReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".vault")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatal(err)
	}
	state := State{Emissions: map[string]Emission{
		"sess-abc": {Emitted: true, Timestamp: "2026-01-01T00:00:00Z"},
	}}
	if err := writeState(dir, state); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	if !ShouldSuggest(dir, "sess-xyz") {
		t.Error("expected ShouldSuggest=true for unknown session")
	}
}

func TestShouldSuggest_AlreadyEmitted_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".vault")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatal(err)
	}
	state := State{Emissions: map[string]Emission{
		"sess-abc": {Emitted: true, Timestamp: "2026-01-01T00:00:00Z"},
	}}
	if err := writeState(dir, state); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	if ShouldSuggest(dir, "sess-abc") {
		t.Error("expected ShouldSuggest=false for already-emitted session")
	}
}

func TestMarkSuggested_CreatesStateFile(t *testing.T) {
	dir := t.TempDir()
	if err := MarkSuggested(dir, "sess-123"); err != nil {
		t.Fatalf("MarkSuggested: %v", err)
	}
	if ShouldSuggest(dir, "sess-123") {
		t.Error("expected ShouldSuggest=false after MarkSuggested")
	}
}

func TestMarkSuggested_WalksUpToVault(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, ".vault")
	if err := os.MkdirAll(vaultDir, 0755); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(dir, "sub", "project")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := MarkSuggested(subDir, "sess-up"); err != nil {
		t.Fatalf("MarkSuggested from subdir: %v", err)
	}
	if ShouldSuggest(subDir, "sess-up") {
		t.Error("expected ShouldSuggest=false after MarkSuggested from subdir")
	}
	// Verify state file is at the vault root, not in subdir
	if _, err := os.Stat(filepath.Join(subDir, ".vault", stateFile)); !os.IsNotExist(err) {
		t.Error("expected no state file in subdir .vault/")
	}
	if _, err := os.Stat(filepath.Join(vaultDir, stateFile)); os.IsNotExist(err) {
		t.Error("expected state file at vault root")
	}
}

func TestMarkSuggested_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := MarkSuggested(dir, "sess-idem"); err != nil {
		t.Fatalf("MarkSuggested first call: %v", err)
	}
	if err := MarkSuggested(dir, "sess-idem"); err != nil {
		t.Fatalf("MarkSuggested second call: %v", err)
	}
	if ShouldSuggest(dir, "sess-idem") {
		t.Error("expected ShouldSuggest=false after repeated MarkSuggested")
	}
}

func TestStateFileName(t *testing.T) {
	if got := StateFileName(); got != ".sessionname-state.json" {
		t.Errorf("StateFileName() = %q, want %q", got, ".sessionname-state.json")
	}
}
