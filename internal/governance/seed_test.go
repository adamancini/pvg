package governance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgentSrc_PrefersLocalPluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	localAgents := filepath.Join(pluginDir, "agents")
	if err := os.MkdirAll(localAgents, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveAgentSrc(pluginDir)
	if err != nil {
		t.Fatalf("resolveAgentSrc() error: %v", err)
	}
	if got != localAgents {
		t.Fatalf("resolveAgentSrc() = %q, want %q", got, localAgents)
	}
}

func TestResolveAgentSrc_UsesExplicitOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom-agents")
	if err := os.Setenv("AGENT_SRC", override); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("AGENT_SRC") }()

	got, err := resolveAgentSrc("")
	if err != nil {
		t.Fatalf("resolveAgentSrc() error: %v", err)
	}
	if got != override {
		t.Fatalf("resolveAgentSrc() = %q, want %q", got, override)
	}
}

func TestWriteNoteStoresBaseline(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	relPath := filepath.Join("methodology", "Test Agent.md")
	content := "# Test Agent\n\nContent here.\n"
	counters := &Counters{}

	// Create the note (force=false, file does not exist yet)
	writeNote(vaultDir, baseDir, relPath, content, false, counters)

	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	// Verify baseline was stored
	baseline, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline() error: %v", err)
	}
	if baseline != content {
		t.Fatalf("baseline = %q, want %q", baseline, content)
	}
}

func TestWriteNoteUnmodifiedFastPath(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	relPath := filepath.Join("methodology", "Test Agent.md")
	originalContent := "# Test Agent v1\n"
	newContent := "# Test Agent v2\n"
	counters := &Counters{}

	// Step 1: Create the note (stores baseline)
	writeNote(vaultDir, baseDir, relPath, originalContent, false, counters)
	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	// Step 2: Force-overwrite with new content. The vault file is unmodified
	// (matches baseline), so this is the fast path.
	writeNote(vaultDir, baseDir, relPath, newContent, true, counters)
	if counters.Updated != 1 {
		t.Fatalf("expected Updated=1, got %d", counters.Updated)
	}

	// Verify the vault file was updated
	data, err := os.ReadFile(filepath.Join(vaultDir, relPath))
	if err != nil {
		t.Fatalf("reading vault file: %v", err)
	}
	if string(data) != newContent {
		t.Fatalf("vault file = %q, want %q", string(data), newContent)
	}

	// Verify the baseline was updated to the new content
	baseline, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline() error: %v", err)
	}
	if baseline != newContent {
		t.Fatalf("baseline = %q, want %q", baseline, newContent)
	}
}

func TestWriteNoteFirstTimeSeedNoBaseline(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	relPath := filepath.Join("methodology", "Test Agent.md")
	existingContent := "# User-created content\n"
	newContent := "# Seeded content\n"

	// Pre-create the vault file (simulates a file that exists but has no baseline)
	fullPath := filepath.Join(vaultDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	counters := &Counters{}

	// Force-overwrite with no baseline existing
	writeNote(vaultDir, baseDir, relPath, newContent, true, counters)
	if counters.Updated != 1 {
		t.Fatalf("expected Updated=1, got %d", counters.Updated)
	}

	// Verify the vault file was overwritten
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading vault file: %v", err)
	}
	if string(data) != newContent {
		t.Fatalf("vault file = %q, want %q", string(data), newContent)
	}

	// Verify baseline was created
	baseline, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Fatal("baseline was not created after force overwrite with no prior baseline")
		}
		t.Fatalf("ReadBaseline() error: %v", err)
	}
	if baseline != newContent {
		t.Fatalf("baseline = %q, want %q", baseline, newContent)
	}
}
