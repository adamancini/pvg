package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIssue(t *testing.T, dir, filename, content string) {
	t.Helper()
	issuesDir := filepath.Join(dir, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckArtifactCollisions_NoCollisions(t *testing.T) {
	vault := t.TempDir()
	writeIssue(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
# Story A

PRODUCES:
- src/auth.ts -> generateToken(userId: string): string
- src/auth.ts -> verifyToken(token: string): Claims | null
`)
	writeIssue(t, vault, "PROJ-c3d.md", `---
id: PROJ-c3d
status: open
type: task
---
# Story B

PRODUCES:
- src/api/login.ts -> POST /api/login handler

CONSUMES:
- PROJ-a1b: src/auth.ts -> generateToken(), verifyToken()
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected passed, got %d collisions", len(result.Collisions))
	}
	if result.Stories != 2 {
		t.Fatalf("expected 2 stories scanned, got %d", result.Stories)
	}
	if result.Total != 2 {
		t.Fatalf("expected 2 unique artifacts (src/auth.ts, src/api/login.ts), got %d", result.Total)
	}
}

func TestCheckArtifactCollisions_DetectsCollision(t *testing.T) {
	vault := t.TempDir()
	writeIssue(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
# Story A

PRODUCES:
- src/auth.ts -> generateToken()
- src/middleware.ts -> authMiddleware()
`)
	writeIssue(t, vault, "PROJ-c3d.md", `---
id: PROJ-c3d
status: open
type: task
---
# Story B

PRODUCES:
- src/auth.ts -> verifyToken()
- src/middleware.ts -> rateLimiter()
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected failure due to collisions")
	}
	if len(result.Collisions) != 2 {
		t.Fatalf("expected 2 collisions (src/auth.ts, src/middleware.ts), got %d", len(result.Collisions))
	}
}

func TestCheckArtifactCollisions_SkipsClosedIssues(t *testing.T) {
	vault := t.TempDir()
	writeIssue(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
PRODUCES:
- src/auth.ts -> generateToken()
`)
	writeIssue(t, vault, "PROJ-old.md", `---
id: PROJ-old
status: closed
type: task
---
PRODUCES:
- src/auth.ts -> generateToken()
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected passed -- closed story should be excluded")
	}
	if result.Stories != 1 {
		t.Fatalf("expected 1 story scanned (closed excluded), got %d", result.Stories)
	}
}

func TestCheckArtifactCollisions_EmptyVault(t *testing.T) {
	vault := t.TempDir()
	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected passed for empty vault")
	}
}

func TestCheckArtifactCollisions_NoProducesBlock(t *testing.T) {
	vault := t.TempDir()
	writeIssue(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
# Story with no PRODUCES

Just some acceptance criteria.
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected passed for story with no PRODUCES")
	}
	if result.Stories != 1 {
		t.Fatalf("expected 1 story scanned, got %d", result.Stories)
	}
	if result.Total != 0 {
		t.Fatalf("expected 0 artifacts, got %d", result.Total)
	}
}

func TestCheckArtifactCollisions_DeriveIDFromFilename(t *testing.T) {
	vault := t.TempDir()
	// Issue file without id in frontmatter
	writeIssue(t, vault, "PROJ-xyz.md", `---
status: open
type: task
---
PRODUCES:
- src/foo.ts -> bar()
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 artifact, got %d", result.Total)
	}
}

func TestNormalizeArtifact(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"src/auth.ts -> generateToken()", "src/auth.ts"},
		{"src/auth.ts", "src/auth.ts"},
		{"src/api/login.ts -> POST /api/login handler", "src/api/login.ts"},
		{"  src/spaced.ts  -> fn()  ", "src/spaced.ts"},
	}

	for _, tt := range tests {
		got := normalizeArtifact(tt.input)
		if got != tt.want {
			t.Errorf("normalizeArtifact(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseIssueFile(t *testing.T) {
	vault := t.TempDir()
	issuesDir := filepath.Join(vault, "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `---
id: TEST-abc
status: in_progress
type: task
---
# Implement auth

Some context here.

PRODUCES:
- src/auth.ts -> generateToken()
- src/auth.ts -> verifyToken()

CONSUMES:
- TEST-000: src/config.ts -> getSecret()
`
	path := filepath.Join(issuesDir, "TEST-abc.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	id, status, produces, err := parseIssueFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "TEST-abc" {
		t.Errorf("expected id=TEST-abc, got %s", id)
	}
	if status != "in_progress" {
		t.Errorf("expected status=in_progress, got %s", status)
	}
	if len(produces) != 2 {
		t.Fatalf("expected 2 produces entries, got %d: %v", len(produces), produces)
	}
	if produces[0] != "src/auth.ts -> generateToken()" {
		t.Errorf("unexpected first produces: %s", produces[0])
	}
}

func TestFormatText_Passed(t *testing.T) {
	r := LintResult{Passed: true, Stories: 5, Total: 10}
	text := FormatText(r)
	if !contains(text, "PASSED") {
		t.Error("expected PASSED in output")
	}
}

func TestFormatText_Failed(t *testing.T) {
	r := LintResult{
		Passed:  false,
		Stories: 5,
		Total:   10,
		Collisions: []Collision{
			{
				Artifact: "src/auth.ts",
				Stories: []ArtifactEntry{
					{Artifact: "src/auth.ts -> generateToken()", StoryID: "A", Status: "open"},
					{Artifact: "src/auth.ts -> verifyToken()", StoryID: "B", Status: "open"},
				},
			},
		},
	}
	text := FormatText(r)
	if !contains(text, "FAILED") {
		t.Error("expected FAILED in output")
	}
	if !contains(text, "src/auth.ts") {
		t.Error("expected artifact name in output")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
