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

func TestSeedThreeWayMergeForSeededVaultNotes_CreatesConceptNote(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	counters := &Counters{}

	seedThreeWayMergeForSeededVaultNotes(vaultDir, baseDir, "2026-05-01", false, counters)

	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	notePath := filepath.Join(vaultDir, "concepts", "Three-way merge for seeded vault notes.md")
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("reading seeded note at %s: %v", notePath, err)
	}
	content := string(data)

	// Frontmatter checks (AC #4): type, scope: system, project: paivot,
	// stack, domain, status, created.
	wantFrontmatter := []string{
		"type: concept",
		"scope: system",
		"project: paivot",
		"stack: [claude-code]",
		"domain: dev-tools-knowledge",
		"status: active",
		"created: 2026-05-01",
	}
	for _, want := range wantFrontmatter {
		if !strings.Contains(content, want) {
			t.Errorf("rendered note missing frontmatter line %q", want)
		}
	}

	// Body coverage (AC #2): .seed-baselines/, Merge3 via diff3 -m, five
	// counter outcomes.
	wantBody := []string{
		".seed-baselines/",
		"diff3 -m",
		"Merge3",
		"Created",
		"Updated",
		"Merged",
		"Conflicted",
		"Skipped",
	}
	for _, want := range wantBody {
		if !strings.Contains(content, want) {
			t.Errorf("rendered note missing body content %q", want)
		}
	}

	// Conflict marker description and resolution guidance (AC #3).
	// Normalize whitespace so phrases that wrap across lines still match.
	flat := strings.Join(strings.Fields(content), " ")
	conflictGuidance := []string{
		"seven less-than characters",
		"seven equals characters",
		"seven greater-than characters",
		"resolve a conflict",
	}
	for _, want := range conflictGuidance {
		if !strings.Contains(flat, want) {
			t.Errorf("rendered note (whitespace-normalized) missing conflict marker description %q", want)
		}
	}

	// Required wikilinks (AC #5).
	wantLinks := []string{
		"[[Vault as runtime not reference]]",
		"[[Delivery Workflow]]",
	}
	for _, want := range wantLinks {
		if !strings.Contains(content, want) {
			t.Errorf("rendered note missing wikilink %q", want)
		}
	}

	// AC #7: must NOT reference a "seed/" directory. Allow ".seed-baselines/"
	// (the canonical baseline dir name) but not a bare "seed/" path.
	scrubbed := strings.ReplaceAll(content, ".seed-baselines/", "")
	if strings.Contains(scrubbed, "seed/") {
		t.Errorf("rendered note must not reference a `seed/` directory; remaining occurrences after stripping `.seed-baselines/`:\n%s",
			scrubbed)
	}

	// AC #8: no raw 7-character conflict markers.
	rawMarkers := []string{
		"<<<<<<<",
		"=======",
		">>>>>>>",
	}
	for _, marker := range rawMarkers {
		if strings.Contains(content, marker) {
			t.Errorf("rendered note must not contain raw conflict marker %q", marker)
		}
	}

	// Cross-story consistency with PAI-1f5w canon: command names must be
	// `pvg seed` / `pvg seed --force`, NOT `make seed` / `make reseed`.
	if strings.Contains(content, "make seed") || strings.Contains(content, "make reseed") {
		t.Errorf("rendered note must not reference removed `make seed` / `make reseed` targets; use `pvg seed` / `pvg seed --force`")
	}
	if !strings.Contains(content, "pvg seed --force") {
		t.Errorf("rendered note must reference the canonical `pvg seed --force` command")
	}
}

func TestSeed_IncludesThreeWayMergeNote_E2E(t *testing.T) {
	// E2E: render through the full Seed() call ordering by invoking the
	// seeder directly alongside the others, asserting the new concept file
	// is written between seedDFSequentialAlignment's note and any later
	// addition.
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	counters := &Counters{}

	seedDFSequentialAlignment(vaultDir, baseDir, "2026-05-01", false, counters)
	seedThreeWayMergeForSeededVaultNotes(vaultDir, baseDir, "2026-05-01", false, counters)

	if counters.Created != 2 {
		t.Fatalf("expected Created=2 after both seeders, got %d", counters.Created)
	}

	notePath := filepath.Join(vaultDir, "concepts", "Three-way merge for seeded vault notes.md")
	if _, err := os.Stat(notePath); err != nil {
		t.Fatalf("expected concept note at %s after fresh seed: %v", notePath, err)
	}
}

func TestSeedVaultAsRuntimeNotReference_UsesConfiguredVaultName(t *testing.T) {
	t.Setenv("PVG_VAULT", "TestVault")
	vaultDir := t.TempDir()
	baseDir := filepath.Join(vaultDir, ".seed-baselines")
	counters := &Counters{}

	seedVaultAsRuntimeNotReference(vaultDir, baseDir, "2026-04-06", false, counters)

	if counters.Created != 1 {
		t.Fatalf("expected Created=1, got %d", counters.Created)
	}

	data, err := os.ReadFile(filepath.Join(vaultDir, "concepts", "Vault as runtime not reference.md"))
	if err != nil {
		t.Fatalf("reading seeded note: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `vault="TestVault"`) {
		t.Error("seeded Vault as runtime note should contain configured vault name TestVault")
	}
	if strings.Contains(content, `vault="Claude"`) {
		t.Error("seeded Vault as runtime note should NOT contain hardcoded vault=\"Claude\" when PVG_VAULT is set")
	}
}

func TestFindAgentSource_HappyPath(t *testing.T) {
	agentsDir := t.TempDir()
	agentFile := filepath.Join(agentsDir, "sr-pm.md")
	if err := os.WriteFile(agentFile, []byte("# Sr PM Agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := findAgentSource(agentsDir, "sr-pm", "Sr PM Agent")
	if got != agentFile {
		t.Fatalf("findAgentSource() = %q, want %q", got, agentFile)
	}
}

func TestFindAgentSource_MissingFile(t *testing.T) {
	agentsDir := t.TempDir()

	got := findAgentSource(agentsDir, "nonexistent", "Nonexistent Agent")
	if got != "" {
		t.Fatalf("findAgentSource() = %q, want empty string", got)
	}
}

func TestFindAgentSource_NoSeedProbeRegression(t *testing.T) {
	// Verify findAgentSource does not panic or error when no seed/ sibling exists.
	agentsDir := t.TempDir()
	// Intentionally do NOT create a seed/ sibling.

	got := findAgentSource(agentsDir, "sr-pm", "Sr PM Agent")
	if got != "" {
		t.Fatalf("findAgentSource() = %q, want empty string when agent file is absent", got)
	}
}
