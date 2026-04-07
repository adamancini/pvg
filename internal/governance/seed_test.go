package governance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestWriteNoteMergesModifiedFile(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	relPath := filepath.Join("methodology", "Test Agent.md")

	// Original content that both baseline and vault start with.
	// Three distinct sections so edits can be non-overlapping.
	original := "# Section A\nAlpha\n\n# Section B\nBravo\n\n# Section C\nCharlie\n"

	// Step 1: Create the note (stores baseline)
	counters := &Counters{}
	writeNote(vaultDir, baseDir, relPath, original, false, counters)
	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	// Step 2: Simulate user editing Section C in the vault file
	userEdited := "# Section A\nAlpha\n\n# Section B\nBravo\n\n# Section C\nCharlie-user-edit\n"
	fullPath := filepath.Join(vaultDir, relPath)
	if err := os.WriteFile(fullPath, []byte(userEdited), 0644); err != nil {
		t.Fatal(err)
	}

	// Step 3: Force-seed with new plugin content that changes Section A
	newPluginContent := "# Section A\nAlpha-plugin-update\n\n# Section B\nBravo\n\n# Section C\nCharlie\n"
	counters2 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, newPluginContent, true, counters2)

	if counters2.Merged != 1 {
		t.Fatalf("expected Merged=1, got Merged=%d, Updated=%d, Conflicted=%d",
			counters2.Merged, counters2.Updated, counters2.Conflicted)
	}

	// Verify merged result contains both changes
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading vault file: %v", err)
	}
	merged := string(data)
	if !strings.Contains(merged, "Alpha-plugin-update") {
		t.Errorf("merged output missing plugin change 'Alpha-plugin-update':\n%s", merged)
	}
	if !strings.Contains(merged, "Charlie-user-edit") {
		t.Errorf("merged output missing user change 'Charlie-user-edit':\n%s", merged)
	}
}

func TestWriteNoteConflictMarkers(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	relPath := filepath.Join("methodology", "Test Agent.md")

	// Original content with a section both sides will edit
	original := "# Title\nOriginal line\n\n# Footer\nEnd\n"

	// Step 1: Create the note (stores baseline)
	counters := &Counters{}
	writeNote(vaultDir, baseDir, relPath, original, false, counters)
	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	// Step 2: Simulate user editing the SAME line
	userEdited := "# Title\nUser-modified line\n\n# Footer\nEnd\n"
	fullPath := filepath.Join(vaultDir, relPath)
	if err := os.WriteFile(fullPath, []byte(userEdited), 0644); err != nil {
		t.Fatal(err)
	}

	// Step 3: Force-seed with plugin content that also changes the SAME line
	newPluginContent := "# Title\nPlugin-modified line\n\n# Footer\nEnd\n"
	counters2 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, newPluginContent, true, counters2)

	if counters2.Conflicted != 1 {
		t.Fatalf("expected Conflicted=1, got Conflicted=%d, Merged=%d, Updated=%d",
			counters2.Conflicted, counters2.Merged, counters2.Updated)
	}

	// Verify conflict markers are present in the vault file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading vault file: %v", err)
	}
	if !strings.Contains(string(data), "<<<<<<<") {
		t.Errorf("expected conflict markers in output:\n%s", string(data))
	}

	// Verify the conflicted file is tracked
	if len(counters2.ConflictedFiles) != 1 || counters2.ConflictedFiles[0] != relPath {
		t.Errorf("ConflictedFiles = %v, want [%s]", counters2.ConflictedFiles, relPath)
	}
}

func TestSeedSessionOperatingMode_UsesConfiguredVaultName(t *testing.T) {
	t.Setenv("PVG_VAULT", "TestVault")
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	counters := &Counters{}

	seedSessionOperatingMode(vaultDir, baseDir, "2026-04-06", false, counters)

	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	data, err := os.ReadFile(filepath.Join(vaultDir, "conventions", "Session Operating Mode.md"))
	if err != nil {
		t.Fatalf("reading seeded note: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `vault="TestVault"`) {
		t.Error("seeded Session Operating Mode should contain configured vault name TestVault")
	}
	if strings.Contains(content, `vault="Claude"`) {
		t.Error("seeded Session Operating Mode should NOT contain hardcoded vault=\"Claude\" when PVG_VAULT is set")
	}
}

func TestSeedPreCompactChecklist_UsesConfiguredVaultName(t *testing.T) {
	t.Setenv("PVG_VAULT", "TestVault")
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	counters := &Counters{}

	seedPreCompactChecklist(vaultDir, baseDir, "2026-04-06", false, counters)

	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	data, err := os.ReadFile(filepath.Join(vaultDir, "conventions", "Pre-Compact Checklist.md"))
	if err != nil {
		t.Fatalf("reading seeded note: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `vault="TestVault"`) {
		t.Error("seeded Pre-Compact Checklist should contain configured vault name TestVault")
	}
	if strings.Contains(content, `vault="Claude"`) {
		t.Error("seeded Pre-Compact Checklist should NOT contain hardcoded vault=\"Claude\" when PVG_VAULT is set")
	}
}

func TestSeedStopCaptureChecklist_UsesConfiguredVaultName(t *testing.T) {
	t.Setenv("PVG_VAULT", "TestVault")
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	counters := &Counters{}

	seedStopCaptureChecklist(vaultDir, baseDir, "2026-04-06", false, counters)

	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	data, err := os.ReadFile(filepath.Join(vaultDir, "conventions", "Stop Capture Checklist.md"))
	if err != nil {
		t.Fatalf("reading seeded note: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `vault="TestVault"`) {
		t.Error("seeded Stop Capture Checklist should contain configured vault name TestVault")
	}
	if strings.Contains(content, `vault="Claude"`) {
		t.Error("seeded Stop Capture Checklist should NOT contain hardcoded vault=\"Claude\" when PVG_VAULT is set")
	}
}
