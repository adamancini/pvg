package rtm

import (
	"os"
	"path/filepath"
	"testing"
)

func setupProject(t *testing.T) (projectRoot, vaultDir string) {
	t.Helper()
	root := t.TempDir()
	vault := filepath.Join(root, ".vault")
	issues := filepath.Join(vault, "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, vault
}

func writeDoc(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeStory(t *testing.T, vaultDir, filename, content string) {
	t.Helper()
	issuesDir := filepath.Join(vaultDir, "issues")
	if err := os.WriteFile(filepath.Join(issuesDir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCoverage_AllCovered(t *testing.T) {
	root, vault := setupProject(t)

	writeDoc(t, root, "BUSINESS.md", `# Business Requirements
- [NEW] Channel-aware routing policies for SMS and email
- [EXPANDED] Multi-tenant support with organization scoping
`)

	writeStory(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
# Implement channel-aware routing policies

Implement routing policies that support SMS and email channels.
Multi-tenant with organization scoping included.
`)

	result, err := CheckCoverage(root, vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected passed, got %d uncovered", result.Uncovered)
	}
	if result.Total != 2 {
		t.Fatalf("expected 2 requirements, got %d", result.Total)
	}
}

func TestCheckCoverage_Uncovered(t *testing.T) {
	root, vault := setupProject(t)

	writeDoc(t, root, "BUSINESS.md", `# Business Requirements
- [NEW] Channel-aware routing policies
- [NEW] Webhook delivery with retry backoff
- [CRITICAL] HIPAA compliance audit trail
`)

	writeStory(t, vault, "PROJ-a1b.md", `---
id: PROJ-a1b
status: open
type: task
---
# Implement channel-aware routing

Routing policies for channels.
`)

	result, err := CheckCoverage(root, vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected failure -- 2 requirements should be uncovered")
	}
	if result.Uncovered < 1 {
		t.Fatalf("expected at least 1 uncovered, got %d", result.Uncovered)
	}
}

func TestCheckCoverage_NoDocs(t *testing.T) {
	root, vault := setupProject(t)

	result, err := CheckCoverage(root, vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Fatal("expected passed when no D&F docs exist")
	}
	if result.Total != 0 {
		t.Fatalf("expected 0 requirements, got %d", result.Total)
	}
}

func TestCheckCoverage_SkipsClosedStories(t *testing.T) {
	root, vault := setupProject(t)

	writeDoc(t, root, "BUSINESS.md", `- [NEW] Webhook delivery with retry`)

	writeStory(t, vault, "PROJ-old.md", `---
id: PROJ-old
status: closed
type: task
---
# Webhook delivery with retry backoff
`)

	result, err := CheckCoverage(root, vault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Fatal("expected failure -- closed story should not count as coverage")
	}
}

func TestExtractRequirements_TaggedLines(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "test.md", `# Test
- [NEW] Feature alpha
- [EXPANDED] Feature beta with more details
- Regular line without tags
- [CRITICAL] Security requirement
`)

	reqs, err := extractRequirements(filepath.Join(root, "test.md"), "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reqs) != 3 {
		t.Fatalf("expected 3 tagged requirements, got %d", len(reqs))
	}
	if reqs[0].Tag != "[NEW]" {
		t.Errorf("expected [NEW] tag, got %s", reqs[0].Tag)
	}
	if reqs[2].Tag != "[CRITICAL]" {
		t.Errorf("expected [CRITICAL] tag, got %s", reqs[2].Tag)
	}
}

func TestExtractRequirements_SkipsFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "test.md", `---
status: active
tags: [NEW]
---
# Real Content
- [NEW] Actual requirement
`)

	reqs, err := extractRequirements(filepath.Join(root, "test.md"), "test.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 requirement (frontmatter skipped), got %d", len(reqs))
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		input    string
		wantMin  int
		contains string
	}{
		{"[NEW] Channel-aware routing policies for SMS and email", 3, "routing"},
		{"[EXPANDED] Multi-tenant support", 2, "tenant"},
		{"[CRITICAL] HIPAA compliance audit trail", 3, "hipaa"},
	}

	for _, tt := range tests {
		kws := extractKeywords(tt.input)
		if len(kws) < tt.wantMin {
			t.Errorf("extractKeywords(%q): got %d keywords, want >= %d: %v", tt.input, len(kws), tt.wantMin, kws)
		}
		found := false
		for _, kw := range kws {
			if kw == tt.contains {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("extractKeywords(%q): expected keyword %q in %v", tt.input, tt.contains, kws)
		}
	}
}

func TestFormatText_Passed(t *testing.T) {
	r := RTMResult{Passed: true, Total: 5, Covered: 5, Stories: 10}
	text := FormatText(r)
	if !containsStr(text, "PASSED") {
		t.Error("expected PASSED in output")
	}
}

func TestFormatText_Failed(t *testing.T) {
	r := RTMResult{
		Passed:    false,
		Total:     3,
		Covered:   1,
		Uncovered: 2,
		Stories:   5,
		Requirements: []Requirement{
			{Tag: "[NEW]", Text: "covered", Source: "BUSINESS.md", Line: 5, Covered: true},
			{Tag: "[NEW]", Text: "missing feature", Source: "BUSINESS.md", Line: 10, Covered: false},
			{Tag: "[CRITICAL]", Text: "security gap", Source: "DESIGN.md", Line: 3, Covered: false},
		},
	}
	text := FormatText(r)
	if !containsStr(text, "FAILED") {
		t.Error("expected FAILED in output")
	}
	if !containsStr(text, "missing feature") {
		t.Error("expected uncovered requirement in output")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
