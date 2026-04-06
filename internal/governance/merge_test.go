package governance

import (
	"strings"
	"testing"
)

func TestMerge3CleanMerge(t *testing.T) {
	// Base has three lines; ours changes line 1, theirs changes line 3.
	// These are non-overlapping edits — should merge cleanly.
	base := "line A\nline B\nline C\n"
	ours := "line A-prime\nline B\nline C\n"
	theirs := "line A\nline B\nline C-prime\n"

	merged, hasConflict, err := Merge3(base, ours, theirs)
	if err != nil {
		t.Fatalf("Merge3() error: %v", err)
	}
	if hasConflict {
		t.Fatalf("Merge3() hasConflict=true, want false. Merged:\n%s", merged)
	}

	// The merged result should contain both changes
	if !strings.Contains(merged, "line A-prime") {
		t.Errorf("merged output missing ours change 'line A-prime':\n%s", merged)
	}
	if !strings.Contains(merged, "line C-prime") {
		t.Errorf("merged output missing theirs change 'line C-prime':\n%s", merged)
	}
	if !strings.Contains(merged, "line B") {
		t.Errorf("merged output missing unchanged 'line B':\n%s", merged)
	}
}

func TestMerge3Conflict(t *testing.T) {
	// Both ours and theirs modify the same line — should produce a conflict.
	base := "line A\nline B\nline C\n"
	ours := "line A\nline B1\nline C\n"
	theirs := "line A\nline B2\nline C\n"

	merged, hasConflict, err := Merge3(base, ours, theirs)
	if err != nil {
		t.Fatalf("Merge3() error: %v", err)
	}
	if !hasConflict {
		t.Fatalf("Merge3() hasConflict=false, want true. Merged:\n%s", merged)
	}
	if !strings.Contains(merged, "<<<<<<<") {
		t.Errorf("merged output missing conflict markers:\n%s", merged)
	}
}

func TestMerge3IdenticalInputs(t *testing.T) {
	// All three inputs are identical — trivial merge, no conflict.
	content := "line A\nline B\nline C\n"

	merged, hasConflict, err := Merge3(content, content, content)
	if err != nil {
		t.Fatalf("Merge3() error: %v", err)
	}
	if hasConflict {
		t.Fatalf("Merge3() hasConflict=true for identical inputs. Merged:\n%s", merged)
	}
	if merged != content {
		t.Errorf("Merge3() = %q, want %q", merged, content)
	}
}

func TestMerge3BaseEqualsTheirs(t *testing.T) {
	// Theirs is unchanged from base; only ours has edits.
	// The merge should accept ours' changes.
	base := "line A\nline B\nline C\n"
	ours := "line A-modified\nline B\nline C\n"
	theirs := base // unchanged

	merged, hasConflict, err := Merge3(base, ours, theirs)
	if err != nil {
		t.Fatalf("Merge3() error: %v", err)
	}
	if hasConflict {
		t.Fatalf("Merge3() hasConflict=true, want false. Merged:\n%s", merged)
	}
	if merged != ours {
		t.Errorf("Merge3() = %q, want %q (ours content)", merged, ours)
	}
}

func TestMerge3BaseEqualsOurs(t *testing.T) {
	// Ours is unchanged from base; only theirs has edits.
	// The merge should accept theirs' changes.
	base := "line A\nline B\nline C\n"
	ours := base // unchanged
	theirs := "line A\nline B\nline C-modified\n"

	merged, hasConflict, err := Merge3(base, ours, theirs)
	if err != nil {
		t.Fatalf("Merge3() error: %v", err)
	}
	if hasConflict {
		t.Fatalf("Merge3() hasConflict=true, want false. Merged:\n%s", merged)
	}
	if merged != theirs {
		t.Errorf("Merge3() = %q, want %q (theirs content)", merged, theirs)
	}
}
