package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveProjectRoot_ClaudeWorktreesConvention(t *testing.T) {
	// Create a temp dir that mimics the Paivot worktree layout:
	//   /tmp/xxx/.git/              (marks project root)
	//   /tmp/xxx/.claude/worktrees/dev-STORY-a1b/  (worktree dir)
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	wtPath := filepath.Join(root, ".claude", "worktrees", "dev-STORY-a1b")
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveProjectRoot(wtPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != root {
		t.Errorf("ResolveProjectRoot(%q) = %q, want %q", wtPath, got, root)
	}
}

func TestResolveProjectRoot_NoConvention_NoGit(t *testing.T) {
	// A random path with no .claude/worktrees/ and no git -- should fail.
	dir := t.TempDir()
	_, err := ResolveProjectRoot(dir)
	if err == nil {
		t.Error("expected error for path with no convention and no git, got nil")
	}
}

func TestResolveProjectRoot_NestedConvention(t *testing.T) {
	// Ensure it picks the right root even with nested .claude dirs.
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Deeply nested worktree path
	wtPath := filepath.Join(root, ".claude", "worktrees", "dev-PRA-nqys")
	if err := os.MkdirAll(wtPath, 0755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveProjectRoot(wtPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestSafeRemove_NonexistentWorktree(t *testing.T) {
	// SafeRemove on a path that doesn't exist and has no .claude convention.
	result := SafeRemove("/nonexistent/path/to/worktree")
	if result.Removed {
		t.Error("expected Removed=false for nonexistent path")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for nonexistent path")
	}
}

func TestSafeRemove_StaleWorktree(t *testing.T) {
	// If the worktree directory is gone but metadata remains, SafeRemove
	// should fall back to prune. We test the ResolveProjectRoot part here
	// since full git worktree integration requires a real git repo.
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Worktree path follows convention but directory does NOT exist.
	wtPath := filepath.Join(root, ".claude", "worktrees", "dev-GONE")

	resolved, err := ResolveProjectRoot(wtPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != root {
		t.Errorf("got %q, want %q", resolved, root)
	}
}

func TestRemoveResult_FormatText(t *testing.T) {
	r := RemoveResult{
		Removed:      true,
		WorktreePath: "/repo/.claude/worktrees/dev-X",
		ProjectRoot:  "/repo",
		Pruned:       true,
	}
	text := r.FormatText()
	if text == "" {
		t.Error("expected non-empty text")
	}
	if !contains(text, "Removed") {
		t.Errorf("expected 'Removed' in text, got %q", text)
	}
	if !contains(text, "[pruned]") {
		t.Errorf("expected '[pruned]' in text, got %q", text)
	}
}

func TestRemoveResult_FormatText_Error(t *testing.T) {
	r := RemoveResult{
		Error: "something went wrong",
	}
	text := r.FormatText()
	if !contains(text, "FAIL") {
		t.Errorf("expected 'FAIL' in error text, got %q", text)
	}
}

func TestRemoveResult_FormatJSON(t *testing.T) {
	r := RemoveResult{
		Removed:      true,
		WorktreePath: "/repo/.claude/worktrees/dev-X",
		ProjectRoot:  "/repo",
	}
	j := r.FormatJSON()
	if j == "" {
		t.Error("expected non-empty JSON")
	}
	if !contains(j, `"removed": true`) {
		t.Errorf("expected removed:true in JSON, got %s", j)
	}
}

// Integration test: only runs when a real git repo is available.
func TestSafeRemove_Integration(t *testing.T) {
	// Create a real git repo with a worktree.
	root := t.TempDir()

	// Restore original execCommand after test
	origExec := execCommand
	t.Cleanup(func() { execCommand = origExec })

	// Init git repo
	cmd := exec.Command("git", "init", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init failed (git not available?): %s", out)
	}

	// Need at least one commit for worktrees to work.
	cmd = exec.Command("git", "-C", root, "commit", "--allow-empty", "-m", "init")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git commit failed: %s", out)
	}

	// Create a branch for the worktree
	cmd = exec.Command("git", "-C", root, "branch", "story/TEST-123")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch failed: %s", out)
	}

	// Create the worktree directory structure (Paivot convention)
	wtDir := filepath.Join(root, ".claude", "worktrees")
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		t.Fatal(err)
	}

	wtPath := filepath.Join(wtDir, "dev-TEST-123")
	cmd = exec.Command("git", "-C", root, "worktree", "add", wtPath, "story/TEST-123")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add failed: %s", out)
	}

	// Verify the worktree exists
	if !isDir(wtPath) {
		t.Fatalf("worktree dir does not exist at %s", wtPath)
	}

	// SafeRemove should work even if we cd somewhere else.
	result := SafeRemove(wtPath)
	if result.Error != "" {
		t.Fatalf("SafeRemove error: %s", result.Error)
	}
	if !result.Removed {
		t.Error("expected Removed=true")
	}
	if result.ProjectRoot != root {
		t.Errorf("ProjectRoot = %q, want %q", result.ProjectRoot, root)
	}

	// Verify the worktree is gone
	if isDir(wtPath) {
		t.Error("worktree dir still exists after removal")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
