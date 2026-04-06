//go:build !short

package governance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2eSeedModifyReseedCleanMerge verifies the full cycle: seed original
// content, user modifies one section, plugin reseeds with a different section
// changed. The merge should incorporate both changes with zero conflicts and
// the baseline should be updated to the new plugin content.
func TestE2eSeedModifyReseedCleanMerge(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := BaselineDir(vaultDir)
	relPath := filepath.Join("notes", "clean-merge.md")

	// Original content with three distinct sections.
	original := "# Header\nOriginal header\n\n# Body\nOriginal body\n\n# Changelog\nOriginal changelog\n"

	// Step 1: First seed — creates the file and stores baseline.
	c1 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, original, true, c1)
	if c1.Created != 1 {
		t.Fatalf("initial seed: expected Created=1, got %d", c1.Created)
	}

	// Verify baseline was stored.
	bl, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline after initial seed: %v", err)
	}
	if bl != original {
		t.Fatalf("baseline after initial seed = %q, want %q", bl, original)
	}

	// Step 2: User modifies BODY section only.
	userEdited := "# Header\nOriginal header\n\n# Body\nUser-modified body\n\n# Changelog\nOriginal changelog\n"
	fullPath := filepath.Join(vaultDir, relPath)
	if err := os.WriteFile(fullPath, []byte(userEdited), 0644); err != nil {
		t.Fatalf("writing user edit: %v", err)
	}

	// Step 3: Plugin reseeds with HEADER section changed.
	newPluginContent := "# Header\nPlugin-updated header\n\n# Body\nOriginal body\n\n# Changelog\nOriginal changelog\n"
	c2 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, newPluginContent, true, c2)

	if c2.Merged != 1 {
		t.Fatalf("reseed: expected Merged=1, got Merged=%d Updated=%d Conflicted=%d",
			c2.Merged, c2.Updated, c2.Conflicted)
	}
	if c2.Conflicted != 0 {
		t.Fatalf("reseed: expected Conflicted=0, got %d", c2.Conflicted)
	}

	// Verify merged file has BOTH changes.
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading merged file: %v", err)
	}
	merged := string(data)
	if !strings.Contains(merged, "Plugin-updated header") {
		t.Errorf("merged output missing plugin header change:\n%s", merged)
	}
	if !strings.Contains(merged, "User-modified body") {
		t.Errorf("merged output missing user body change:\n%s", merged)
	}

	// Verify baseline was updated to the new plugin content.
	bl2, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline after merge: %v", err)
	}
	if bl2 != newPluginContent {
		t.Fatalf("baseline after merge = %q, want %q", bl2, newPluginContent)
	}
}

// TestE2eSeedModifyReseedConflict verifies that when the user and plugin both
// modify the same section, conflict markers are inserted, the conflicted
// counter is incremented, and the baseline is NOT updated.
func TestE2eSeedModifyReseedConflict(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := BaselineDir(vaultDir)
	relPath := filepath.Join("notes", "conflict.md")

	// Original content.
	original := "# Title\nOriginal title line\n\n# Details\nOriginal details\n\n# Footer\nOriginal footer\n"

	// Step 1: First seed.
	c1 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, original, true, c1)
	if c1.Created != 1 {
		t.Fatalf("initial seed: expected Created=1, got %d", c1.Created)
	}

	// Step 2: User modifies the SAME section (Title) that the plugin will also change.
	userEdited := "# Title\nUser-modified title line\n\n# Details\nOriginal details\n\n# Footer\nOriginal footer\n"
	fullPath := filepath.Join(vaultDir, relPath)
	if err := os.WriteFile(fullPath, []byte(userEdited), 0644); err != nil {
		t.Fatalf("writing user edit: %v", err)
	}

	// Step 3: Plugin reseeds with a different change to the SAME section.
	newPluginContent := "# Title\nPlugin-modified title line\n\n# Details\nOriginal details\n\n# Footer\nOriginal footer\n"
	c2 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, newPluginContent, true, c2)

	if c2.Conflicted != 1 {
		t.Fatalf("reseed: expected Conflicted=1, got Conflicted=%d Merged=%d Updated=%d",
			c2.Conflicted, c2.Merged, c2.Updated)
	}
	if c2.Merged != 0 {
		t.Fatalf("reseed: expected Merged=0, got %d", c2.Merged)
	}

	// Verify conflict markers are present.
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading conflicted file: %v", err)
	}
	content := string(data)
	for _, marker := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
		if !strings.Contains(content, marker) {
			t.Errorf("conflicted file missing marker %q:\n%s", marker, content)
		}
	}

	// Verify baseline was NOT updated (still the original).
	bl, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline after conflict: %v", err)
	}
	if bl != original {
		t.Fatalf("baseline should remain original after conflict, got %q", bl)
	}
}

// TestE2eSeedUnmodifiedReseed verifies that reseeding an unmodified file
// takes the fast path: direct overwrite, Updated counter incremented, and
// baseline updated to the new content.
func TestE2eSeedUnmodifiedReseed(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := BaselineDir(vaultDir)
	relPath := filepath.Join("notes", "unmodified.md")

	original := "# Note\nOriginal content v1\n"

	// Step 1: First seed.
	c1 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, original, true, c1)
	if c1.Created != 1 {
		t.Fatalf("initial seed: expected Created=1, got %d", c1.Created)
	}

	// Step 2: Do NOT modify the vault file. Reseed with new content.
	newContent := "# Note\nUpdated content v2\n"
	c2 := &Counters{}
	writeNote(vaultDir, baseDir, relPath, newContent, true, c2)

	if c2.Updated != 1 {
		t.Fatalf("reseed: expected Updated=1, got Updated=%d Merged=%d Conflicted=%d",
			c2.Updated, c2.Merged, c2.Conflicted)
	}
	if c2.Merged != 0 {
		t.Fatalf("reseed: expected Merged=0, got %d", c2.Merged)
	}

	// Verify vault file has new content.
	fullPath := filepath.Join(vaultDir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading vault file: %v", err)
	}
	if string(data) != newContent {
		t.Fatalf("vault file = %q, want %q", string(data), newContent)
	}

	// Verify baseline was updated.
	bl, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline after reseed: %v", err)
	}
	if bl != newContent {
		t.Fatalf("baseline = %q, want %q", bl, newContent)
	}
}

// TestE2eFirstTimeSeedNoBaseline verifies that when a vault file already
// exists but has no baseline (e.g., manually created by the user), force
// seeding overwrites the file, creates a baseline, and increments Updated.
func TestE2eFirstTimeSeedNoBaseline(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := BaselineDir(vaultDir)
	relPath := filepath.Join("notes", "manual.md")

	// Pre-create a vault file with no baseline.
	fullPath := filepath.Join(vaultDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("creating parent dir: %v", err)
	}
	manualContent := "# User-created note\nThis was written by the user directly.\n"
	if err := os.WriteFile(fullPath, []byte(manualContent), 0644); err != nil {
		t.Fatalf("writing manual file: %v", err)
	}

	// Seed with force=true and no baseline existing.
	seedContent := "# Seeded note\nThis is the plugin-seeded content.\n"
	c := &Counters{}
	writeNote(vaultDir, baseDir, relPath, seedContent, true, c)

	if c.Updated != 1 {
		t.Fatalf("expected Updated=1, got Updated=%d Created=%d Merged=%d Conflicted=%d",
			c.Updated, c.Created, c.Merged, c.Conflicted)
	}

	// Verify vault file was overwritten.
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("reading vault file: %v", err)
	}
	if string(data) != seedContent {
		t.Fatalf("vault file = %q, want %q", string(data), seedContent)
	}

	// Verify baseline was created.
	bl, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline after first seed: %v", err)
	}
	if bl != seedContent {
		t.Fatalf("baseline = %q, want %q", bl, seedContent)
	}
}

// TestE2eMultipleFilesMixedResults seeds three files, each exercising a
// different merge path: A is unmodified (Updated), B is modified in a
// non-conflicting section (Merged), C is modified in the same section as
// the plugin update (Conflicted). All three are processed through a single
// Counters instance to verify aggregate totals.
func TestE2eMultipleFilesMixedResults(t *testing.T) {
	vaultDir := t.TempDir()
	baseDir := BaselineDir(vaultDir)

	relA := filepath.Join("notes", "fileA.md")
	relB := filepath.Join("notes", "fileB.md")
	relC := filepath.Join("notes", "fileC.md")

	// Common original content with three distinct sections.
	original := "# Header\nOriginal header\n\n# Body\nOriginal body\n\n# Footer\nOriginal footer\n"

	// Step 1: Seed all three files.
	cInit := &Counters{}
	writeNote(vaultDir, baseDir, relA, original, true, cInit)
	writeNote(vaultDir, baseDir, relB, original, true, cInit)
	writeNote(vaultDir, baseDir, relC, original, true, cInit)
	if cInit.Created != 3 {
		t.Fatalf("initial seed: expected Created=3, got %d", cInit.Created)
	}

	// Step 2: Set up each file for its respective merge path.

	// A: leave unmodified (fast-path overwrite).

	// B: user modifies Footer (non-conflicting with plugin's Header change).
	pathB := filepath.Join(vaultDir, relB)
	userB := "# Header\nOriginal header\n\n# Body\nOriginal body\n\n# Footer\nUser-modified footer\n"
	if err := os.WriteFile(pathB, []byte(userB), 0644); err != nil {
		t.Fatalf("writing user edit for B: %v", err)
	}

	// C: user modifies Header (conflicts with plugin's Header change).
	pathC := filepath.Join(vaultDir, relC)
	userC := "# Header\nUser-modified header\n\n# Body\nOriginal body\n\n# Footer\nOriginal footer\n"
	if err := os.WriteFile(pathC, []byte(userC), 0644); err != nil {
		t.Fatalf("writing user edit for C: %v", err)
	}

	// Step 3: Reseed all three with new plugin content that changes the Header.
	newPlugin := "# Header\nPlugin-updated header\n\n# Body\nOriginal body\n\n# Footer\nOriginal footer\n"
	cReseed := &Counters{}
	writeNote(vaultDir, baseDir, relA, newPlugin, true, cReseed)
	writeNote(vaultDir, baseDir, relB, newPlugin, true, cReseed)
	writeNote(vaultDir, baseDir, relC, newPlugin, true, cReseed)

	// Verify aggregate counters.
	if cReseed.Updated != 1 {
		t.Errorf("expected Updated=1, got %d", cReseed.Updated)
	}
	if cReseed.Merged != 1 {
		t.Errorf("expected Merged=1, got %d", cReseed.Merged)
	}
	if cReseed.Conflicted != 1 {
		t.Errorf("expected Conflicted=1, got %d", cReseed.Conflicted)
	}

	// Verify file A has new plugin content (direct overwrite).
	dataA, err := os.ReadFile(filepath.Join(vaultDir, relA))
	if err != nil {
		t.Fatalf("reading file A: %v", err)
	}
	if string(dataA) != newPlugin {
		t.Errorf("file A should have new plugin content, got:\n%s", string(dataA))
	}

	// Verify file B has merged content (both plugin header and user footer).
	dataB, err := os.ReadFile(pathB)
	if err != nil {
		t.Fatalf("reading file B: %v", err)
	}
	mergedB := string(dataB)
	if !strings.Contains(mergedB, "Plugin-updated header") {
		t.Errorf("file B missing plugin header change:\n%s", mergedB)
	}
	if !strings.Contains(mergedB, "User-modified footer") {
		t.Errorf("file B missing user footer change:\n%s", mergedB)
	}

	// Verify file C has conflict markers.
	dataC, err := os.ReadFile(pathC)
	if err != nil {
		t.Fatalf("reading file C: %v", err)
	}
	contentC := string(dataC)
	for _, marker := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
		if !strings.Contains(contentC, marker) {
			t.Errorf("file C missing conflict marker %q:\n%s", marker, contentC)
		}
	}
}
