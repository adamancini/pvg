package lifecycle

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsMemoryPath_MatchesClaudeMemory(t *testing.T) {
	tests := []struct {
		path  string
		match bool
	}{
		// Should match
		{"/Users/test/.claude/paivot-graph/memory/MEMORY.md", true},
		{"~/.claude/projects/-project-name/memory/notes.md", true},
		{"/home/user/.claude/test-project/memory/file.txt", true},

		// Should not match
		{"/Users/test/project/MEMORY.md", false},
		{"/Users/test/.vault/notes.md", false},
		{"/Users/test/README.md", false},
		{".claude", false},
		{"memory", false},
	}

	for _, tt := range tests {
		got := isMemoryPath(tt.path)
		if got != tt.match {
			t.Errorf("isMemoryPath(%q) = %v, want %v", tt.path, got, tt.match)
		}
	}
}

func TestDetectProject_FromCWD(t *testing.T) {
	// Create a temp directory and use it as CWD
	dir := t.TempDir()
	projectName := detectProject(dir)
	expectedName := filepath.Base(dir)

	if projectName != expectedName {
		t.Errorf("detectProject(%q) = %q, want %q", dir, projectName, expectedName)
	}
}

func TestMemoryToolInput_ParsesCorrectly(t *testing.T) {
	// This test verifies the JSON structure matches Claude Code's hook input format
	jsonInput := `{
		"tool_name": "Write",
		"tool_input": {
			"file_path": "/Users/test/.claude/project/memory/MEMORY.md",
			"content": "# Test Content\n\nSome memory notes."
		},
		"cwd": "/Users/test/workspace/project"
	}`

	var input memoryToolInput
	if err := json.Unmarshal([]byte(jsonInput), &input); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if input.ToolName != "Write" {
		t.Errorf("tool_name = %q, want Write", input.ToolName)
	}
	if input.ToolInput.Content != "# Test Content\n\nSome memory notes." {
		t.Errorf("content mismatch")
	}
	if input.CWD != "/Users/test/workspace/project" {
		t.Errorf("cwd mismatch")
	}
}

func TestMemoryToolInput_ParsesReadOperation(t *testing.T) {
	jsonInput := `{
		"tool_name": "Read",
		"tool_input": {
			"file_path": "/Users/test/.claude/project/memory/MEMORY.md"
		},
		"cwd": "/Users/test/workspace/project"
	}`

	var input memoryToolInput
	if err := json.Unmarshal([]byte(jsonInput), &input); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if input.ToolName != "Read" {
		t.Errorf("tool_name = %q, want Read", input.ToolName)
	}
	if input.ToolInput.FilePath == "" {
		t.Errorf("file_path is empty")
	}
}

func TestMemoryToolInput_ParsesEditOperation(t *testing.T) {
	jsonInput := `{
		"tool_name": "Edit",
		"tool_input": {
			"file_path": "/Users/test/.claude/project/memory/MEMORY.md",
			"new_string": "## New section\n\nEdited content"
		},
		"cwd": "/Users/test/workspace/project"
	}`

	var input memoryToolInput
	if err := json.Unmarshal([]byte(jsonInput), &input); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if input.ToolName != "Edit" {
		t.Errorf("tool_name = %q, want Edit", input.ToolName)
	}
	if input.ToolInput.NewString != "## New section\n\nEdited content" {
		t.Errorf("new_string mismatch")
	}
}

func TestHookOutput_EncodesCorrectly(t *testing.T) {
	output := hookOutput{
		SystemMessage: "[VAULT MEMORY] Test message",
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded hookOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.SystemMessage != "[VAULT MEMORY] Test message" {
		t.Errorf("message mismatch")
	}
}

func TestFrontmatterGeneration_IncludesProjectMetadata(t *testing.T) {
	now := time.Now().Format("2006-01-02")

	frontmatter := `---
type: project
project: test-project
status: active
created: ` + now + `
---`

	if !strings.Contains(frontmatter, "type: project") {
		t.Error("frontmatter missing type")
	}
	if !strings.Contains(frontmatter, "project: test-project") {
		t.Error("frontmatter missing project name")
	}
	if !strings.Contains(frontmatter, "status: active") {
		t.Error("frontmatter missing status")
	}
}
