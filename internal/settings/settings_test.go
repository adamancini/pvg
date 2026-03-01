package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSettings_NoFile(t *testing.T) {
	s := loadSettings("/nonexistent/.settings.yaml")
	if len(s) != 0 {
		t.Errorf("expected empty map for missing file, got %d entries", len(s))
	}
}

func TestLoadSettings_ParsesYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".settings.yaml")
	content := "session_start_max_notes: 5\nauto_capture: false\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := loadSettings(path)
	if s["session_start_max_notes"] != "5" {
		t.Errorf("expected 5, got %q", s["session_start_max_notes"])
	}
	if s["auto_capture"] != "false" {
		t.Errorf("expected false, got %q", s["auto_capture"])
	}
}

func TestWriteSettings_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", ".settings.yaml")

	settings := map[string]string{
		"session_start_max_notes": "20",
		"staleness_days":          "60",
	}

	if err := writeSettings(path, settings); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "session_start_max_notes: 20") {
		t.Error("expected session_start_max_notes: 20 in output")
	}
	if !strings.Contains(content, "staleness_days: 60") {
		t.Error("expected staleness_days: 60 in output")
	}
}

func TestWriteAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".settings.yaml")

	original := map[string]string{
		"auto_capture":            "true",
		"session_start_max_notes": "15",
		"staleness_days":          "45",
	}

	if err := writeSettings(path, original); err != nil {
		t.Fatal(err)
	}

	loaded := loadSettings(path)
	for k, v := range original {
		if loaded[k] != v {
			t.Errorf("key %q: expected %q, got %q", k, v, loaded[k])
		}
	}
}

func TestLoadSettings_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".settings.yaml")
	content := "# This is a comment\nsession_start_max_notes: 5\n# Another comment\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := loadSettings(path)
	if len(s) != 1 {
		t.Errorf("expected 1 entry (comments should be skipped), got %d", len(s))
	}
	if s["session_start_max_notes"] != "5" {
		t.Errorf("expected 5, got %q", s["session_start_max_notes"])
	}
}

func TestLoadSettings_DotPrefixedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".settings.yaml")
	content := "workflow.fsm: true\nworkflow.sequence: open,closed\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := loadSettings(path)
	if s["workflow.fsm"] != "true" {
		t.Errorf("expected 'true', got %q", s["workflow.fsm"])
	}
	if s["workflow.sequence"] != "open,closed" {
		t.Errorf("expected 'open,closed', got %q", s["workflow.sequence"])
	}
}

func TestLoadSettings_ExitRulesWithColons(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".settings.yaml")
	// The value contains colons -- SplitN(line, ":", 2) should handle this
	content := "workflow.exit_rules: blocked:open,in_progress;rejected:in_progress\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := loadSettings(path)
	expected := "blocked:open,in_progress;rejected:in_progress"
	if s["workflow.exit_rules"] != expected {
		t.Errorf("expected %q, got %q", expected, s["workflow.exit_rules"])
	}
}

func TestLoadFile_Exported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".settings.yaml")
	content := "workflow.fsm: true\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := LoadFile(path)
	if s["workflow.fsm"] != "true" {
		t.Errorf("LoadFile: expected 'true', got %q", s["workflow.fsm"])
	}
}

func TestLoadFile_MissingFile(t *testing.T) {
	s := LoadFile("/nonexistent/.settings.yaml")
	if len(s) != 0 {
		t.Errorf("LoadFile: expected empty map for missing file, got %d entries", len(s))
	}
}

func TestWorkflowKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".settings.yaml")

	original := map[string]string{
		"workflow.fsm":             "true",
		"workflow.sequence":        "open,in_progress,closed",
		"workflow.exit_rules":      "blocked:open;rejected:open",
		"workflow.custom_statuses": "rejected",
	}

	if err := writeSettings(path, original); err != nil {
		t.Fatal(err)
	}

	loaded := loadSettings(path)
	for k, v := range original {
		if loaded[k] != v {
			t.Errorf("key %q: expected %q, got %q", k, v, loaded[k])
		}
	}
}
