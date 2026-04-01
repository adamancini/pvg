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

func TestCheckArtifactCollisions_BlockedByChainNoCollision(t *testing.T) {
	vault := t.TempDir()
	// Story A creates the file
	writeIssue(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
PRODUCES:
- src/auth.ts -> AuthService class (initial implementation)
`)
	// Story B modifies the same file, blocked_by A
	writeIssue(t, vault, "PROJ-c3d.md", `---
id: PROJ-c3d
status: open
type: task
blocked_by: [PROJ-a1b]
---
PRODUCES:
- src/auth.ts -> AuthService.refreshToken() (extends AuthService)

CONSUMES:
- PROJ-a1b: src/auth.ts -> AuthService class
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected passed (blocked_by chain), got %d collisions", len(result.Collisions))
	}
}

func TestCheckArtifactCollisions_ConsumesChainNoCollision(t *testing.T) {
	vault := t.TempDir()
	// Story A creates the file
	writeIssue(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
PRODUCES:
- src/auth.ts -> generateToken()
`)
	// Story B modifies the same file, CONSUMES from A (no blocked_by)
	writeIssue(t, vault, "PROJ-c3d.md", `---
id: PROJ-c3d
status: open
type: task
---
PRODUCES:
- src/auth.ts -> refreshToken()

CONSUMES:
- PROJ-a1b: src/auth.ts -> generateToken()
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected passed (CONSUMES chain), got %d collisions", len(result.Collisions))
	}
}

func TestCheckArtifactCollisions_ThreeStoryChainNoCollision(t *testing.T) {
	vault := t.TempDir()
	writeIssue(t, vault, "PROJ-a.md", `---
id: PROJ-a
status: open
type: task
---
PRODUCES:
- src/auth.ts -> AuthService (initial)
`)
	writeIssue(t, vault, "PROJ-b.md", `---
id: PROJ-b
status: open
type: task
blocked_by: [PROJ-a]
---
PRODUCES:
- src/auth.ts -> AuthService.login()

CONSUMES:
- PROJ-a: src/auth.ts -> AuthService
`)
	writeIssue(t, vault, "PROJ-c.md", `---
id: PROJ-c
status: open
type: task
blocked_by: [PROJ-b]
---
PRODUCES:
- src/auth.ts -> AuthService.logout()

CONSUMES:
- PROJ-b: src/auth.ts -> AuthService.login()
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected passed (3-story chain), got %d collisions", len(result.Collisions))
	}
}

func TestCheckArtifactCollisions_PartialChainDetectsCollision(t *testing.T) {
	vault := t.TempDir()
	// A -> B is chained, but C is independent -- C collides with A and B
	writeIssue(t, vault, "PROJ-a.md", `---
id: PROJ-a
status: open
type: task
---
PRODUCES:
- src/auth.ts -> AuthService (initial)
`)
	writeIssue(t, vault, "PROJ-b.md", `---
id: PROJ-b
status: open
type: task
blocked_by: [PROJ-a]
---
PRODUCES:
- src/auth.ts -> AuthService.login()

CONSUMES:
- PROJ-a: src/auth.ts -> AuthService
`)
	writeIssue(t, vault, "PROJ-c.md", `---
id: PROJ-c
status: open
type: task
---
PRODUCES:
- src/auth.ts -> AuthService.unrelated()
`)

	result, err := CheckArtifactCollisions(vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected collision (PROJ-c is unchained)")
	}
	if len(result.Collisions) != 1 {
		t.Fatalf("expected 1 collision, got %d", len(result.Collisions))
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
blocked_by: [TEST-upstream, TEST-other]
---
# Implement auth

Some context here.

PRODUCES:
- src/auth.ts -> generateToken()
- src/auth.ts -> verifyToken()

CONSUMES:
- TEST-upstream: src/config.ts -> getSecret()
- (existing): src/utils.ts -> hash()
`
	path := filepath.Join(issuesDir, "TEST-abc.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := parseIssueFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ID != "TEST-abc" {
		t.Errorf("expected id=TEST-abc, got %s", data.ID)
	}
	if data.Status != "in_progress" {
		t.Errorf("expected status=in_progress, got %s", data.Status)
	}
	if len(data.Produces) != 2 {
		t.Fatalf("expected 2 produces entries, got %d: %v", len(data.Produces), data.Produces)
	}
	if data.Produces[0] != "src/auth.ts -> generateToken()" {
		t.Errorf("unexpected first produces: %s", data.Produces[0])
	}
	if len(data.BlockedBy) != 2 {
		t.Fatalf("expected 2 blocked_by entries, got %d: %v", len(data.BlockedBy), data.BlockedBy)
	}
	if data.BlockedBy[0] != "TEST-upstream" || data.BlockedBy[1] != "TEST-other" {
		t.Errorf("unexpected blocked_by: %v", data.BlockedBy)
	}
	if len(data.ConsumeRefs) != 1 {
		t.Fatalf("expected 1 consume ref (skipping '(existing)'), got %d: %v", len(data.ConsumeRefs), data.ConsumeRefs)
	}
	if data.ConsumeRefs[0] != "TEST-upstream" {
		t.Errorf("unexpected consume ref: %s", data.ConsumeRefs[0])
	}
}

func TestParseYAMLList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"[A, B, C]", []string{"A", "B", "C"}},
		{"[A]", []string{"A"}},
		{"[]", nil},
		{"not-a-list", nil},
		{"[PROJ-a1b, PROJ-c3d]", []string{"PROJ-a1b", "PROJ-c3d"}},
	}

	for _, tt := range tests {
		got := parseYAMLList(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseYAMLList(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseYAMLList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestExtractConsumeRef(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"PROJ-a1b: src/auth.ts -> generateToken()", "PROJ-a1b"},
		{"(existing): src/utils.ts -> hash()", ""},
		{"(none -- leaf story)", ""},
		{"just-text-no-colon", ""},
		{"PRA-2n7k: conversation_live.ex (assigns-based)", "PRA-2n7k"},
	}

	for _, tt := range tests {
		got := extractConsumeRef(tt.input)
		if got != tt.want {
			t.Errorf("extractConsumeRef(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
