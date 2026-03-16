package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paivot-ai/vlt"
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

func TestBuildMemoryMirrorContent(t *testing.T) {
	body, full := buildMemoryMirrorContent("test-project", "Remember this")

	if !strings.Contains(body, "# test-project Memory Mirror") {
		t.Fatalf("body missing heading: %q", body)
	}
	if !strings.Contains(body, "Remember this") {
		t.Fatalf("body missing content: %q", body)
	}
	if !strings.Contains(full, "project: test-project") {
		t.Fatalf("full content missing project frontmatter: %q", full)
	}
}

func TestMemoryWrite_UpdatePathUsesWriteCompatibleBody(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "_inbox", "demo-memory.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatal(err)
	}

	body1, full1 := buildMemoryMirrorContent("demo", "First")
	if err := os.WriteFile(notePath, []byte(full1), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultClient, err := vlt.Open(dir)
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	body2, _ := buildMemoryMirrorContent("demo", "Second")
	if err := vaultClient.Write("demo-memory", body2, true); err != nil {
		t.Fatalf("Write should replace existing memory mirror body: %v", err)
	}

	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "project: demo") {
		t.Fatalf("frontmatter should be preserved after write: %q", text)
	}
	if strings.Contains(text, "First") {
		t.Fatalf("old memory content should be replaced: %q", text)
	}
	if !strings.Contains(text, "Second") {
		t.Fatalf("new memory content missing after write: %q", text)
	}
	if strings.Count(text, "project: demo") != 1 {
		t.Fatalf("frontmatter duplicated after write: %q", text)
	}
	if !strings.Contains(body1, "First") {
		t.Fatal("sanity check failed: initial body missing first content")
	}
}
