package governance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBaselineDir(t *testing.T) {
	got := BaselineDir("/vault")
	want := "/vault/.seed-baselines"
	if got != want {
		t.Fatalf("BaselineDir(%q) = %q, want %q", "/vault", got, want)
	}
}

func TestWriteAndReadBaseline(t *testing.T) {
	baseDir := t.TempDir()
	relPath := "methodology/Agent.md"
	content := "# Agent\n\nThis is the agent content.\n"

	if err := WriteBaseline(baseDir, relPath, content); err != nil {
		t.Fatalf("WriteBaseline() error: %v", err)
	}

	got, err := ReadBaseline(baseDir, relPath)
	if err != nil {
		t.Fatalf("ReadBaseline() error: %v", err)
	}
	if got != content {
		t.Fatalf("ReadBaseline() = %q, want %q", got, content)
	}
}

func TestReadBaselineMissing(t *testing.T) {
	baseDir := t.TempDir()
	_, err := ReadBaseline(baseDir, "nonexistent/file.md")
	if err == nil {
		t.Fatal("ReadBaseline() expected error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadBaseline() error = %v, want error wrapping os.ErrNotExist", err)
	}
}

func TestWriteBaselineCreatesParentDirs(t *testing.T) {
	baseDir := t.TempDir()
	relPath := "methodology/Agent.md"
	content := "# Agent\n"

	if err := WriteBaseline(baseDir, relPath, content); err != nil {
		t.Fatalf("WriteBaseline() error: %v", err)
	}

	// Verify the parent directory was created
	parentDir := filepath.Join(baseDir, "methodology")
	info, err := os.Stat(parentDir)
	if err != nil {
		t.Fatalf("parent directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", parentDir)
	}

	// Verify the file exists with correct content
	data, err := os.ReadFile(filepath.Join(baseDir, relPath))
	if err != nil {
		t.Fatalf("reading baseline file: %v", err)
	}
	if string(data) != content {
		t.Fatalf("baseline content = %q, want %q", string(data), content)
	}
}
