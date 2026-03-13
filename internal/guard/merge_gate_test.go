package guard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paivot-ai/pvg/internal/dispatcher"
	"github.com/paivot-ai/pvg/internal/loop"
)

func setupMergeGate(t *testing.T, storyID, issueContent string) string {
	t.Helper()
	dir, sharedVault := setupPaivotWorktree(t)

	// Enable dispatcher mode
	knowledgeDir := filepath.Join(dir, ".vault", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.On(dir); err != nil {
		t.Fatal(err)
	}

	// Create issue file if content provided
	if storyID != "" && issueContent != "" {
		issuesDir := filepath.Join(sharedVault, "issues")
		if err := os.MkdirAll(issuesDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(issuesDir, storyID+".md"), []byte(issueContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func writeProjectSettings(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, ".vault", "knowledge", ".settings.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stack_detection: false\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func setupPaivotWorktree(t *testing.T) (projectRoot, sharedVault string) {
	t.Helper()

	base := t.TempDir()
	projectRoot = filepath.Join(base, "repo")
	gitDir := filepath.Join(base, "gitdir", "worktrees", "story")
	commonDir := filepath.Join(base, "gitdir")
	sharedVault = filepath.Join(commonDir, "paivot", "nd-vault")

	if err := os.MkdirAll(filepath.Join(projectRoot, ".vault", "knowledge"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sharedVault, 0755); err != nil {
		t.Fatal(err)
	}
	gitPtr := "gitdir: " + filepath.ToSlash(gitDir) + "\n"
	if err := os.WriteFile(filepath.Join(projectRoot, ".git"), []byte(gitPtr), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0644); err != nil {
		t.Fatal(err)
	}

	return projectRoot, sharedVault
}

func TestCheckMergeGate_BlocksWithoutAcceptedLabel(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: [delivered]\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge --no-ff origin/story/PROJ-a1b2 -m \"merge\"")
	if r.Allowed {
		t.Error("expected blocked for story without accepted label")
	}
	if r.Reason == "" {
		t.Error("expected block reason")
	}
}

func TestCheckMergeGate_AllowsWithAcceptedLabel(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: closed\nlabels: [delivered, accepted]\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge --no-ff origin/story/PROJ-a1b2 -m \"merge\"")
	if !r.Allowed {
		t.Errorf("expected allowed with accepted label, got blocked: %s", r.Reason)
	}
}

func TestCheckMergeGate_BlocksAcceptedLabelUntilClosed(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: [delivered, accepted]\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge --no-ff origin/story/PROJ-a1b2 -m \"merge\"")
	if r.Allowed {
		t.Error("expected blocked when story has accepted label but is not closed")
	}
}

func TestCheckMergeGate_AllowsNonStoryMerge(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: []\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge --no-ff origin/epic/PROJ-a1b2")
	if !r.Allowed {
		t.Errorf("expected allowed for epic merge, got blocked: %s", r.Reason)
	}
}

func TestCheckMergeGate_AllowsWhenDispatcherOff(t *testing.T) {
	dir := t.TempDir()
	// No dispatcher state -- should allow
	r := CheckMergeGate(dir, "git merge --no-ff origin/story/PROJ-a1b2")
	if !r.Allowed {
		t.Errorf("expected allowed when dispatcher off, got blocked: %s", r.Reason)
	}
}

func TestCheckMergeGate_EnforcedWithProjectSettings(t *testing.T) {
	dir, sharedVault := setupPaivotWorktree(t)
	writeProjectSettings(t, dir)

	issuesDir := filepath.Join(sharedVault, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: [delivered]\n---\nBody"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-a1b2.md"), []byte(issue), 0644); err != nil {
		t.Fatal(err)
	}

	r := CheckMergeGate(dir, "git merge --no-ff origin/story/PROJ-a1b2 -m \"merge\"")
	if r.Allowed {
		t.Error("expected blocked when project is Paivot-managed via settings file")
	}
}

func TestCheckMergeGate_EnforcedWithActiveLoop(t *testing.T) {
	dir, sharedVault := setupPaivotWorktree(t)

	issuesDir := filepath.Join(sharedVault, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: [delivered]\n---\nBody"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-a1b2.md"), []byte(issue), 0644); err != nil {
		t.Fatal(err)
	}

	state := loop.NewState("all", "", 50)
	if err := loop.WriteState(dir, state); err != nil {
		t.Fatal(err)
	}

	r := CheckMergeGate(dir, "git merge --no-ff origin/story/PROJ-a1b2 -m \"merge\"")
	if r.Allowed {
		t.Error("expected blocked when execution loop is active")
	}
}

func TestCheckMergeGate_FailOpenMissingIssue(t *testing.T) {
	// Paivot-managed but no issue file -- allow to avoid breaking non-Paivot story/*
	// branch conventions that happen to share the same naming scheme.
	dir := setupMergeGate(t, "", "")

	r := CheckMergeGate(dir, "git merge --no-ff origin/story/PROJ-noexist")
	if !r.Allowed {
		t.Errorf("expected fail-open for missing issue, got blocked: %s", r.Reason)
	}
}

func TestCheckMergeGate_EmptyLabels(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: []\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge origin/story/PROJ-a1b2")
	if r.Allowed {
		t.Error("expected blocked for story with empty labels")
	}
}

func TestCheckMergeGate_NoLabelsField(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge origin/story/PROJ-a1b2")
	if r.Allowed {
		t.Error("expected blocked for story with no labels field")
	}
}

func TestCheckMergeGate_StoryBranchWithoutOrigin(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: [delivered]\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge story/PROJ-a1b2")
	if r.Allowed {
		t.Error("expected blocked for local story branch merge without accepted label")
	}
}

func TestCheckMergeGate_RefsRemotesOriginStory(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: [delivered]\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git merge refs/remotes/origin/story/PROJ-a1b2")
	if r.Allowed {
		t.Error("expected blocked for refs/remotes/origin story merge without accepted label")
	}
}

func TestCheckMergeGate_CherryPickStoryRef(t *testing.T) {
	issue := "---\ntitle: Test\nstatus: in_progress\nlabels: [delivered]\n---\nBody"
	dir := setupMergeGate(t, "PROJ-a1b2", issue)

	r := CheckMergeGate(dir, "git cherry-pick origin/story/PROJ-a1b2")
	if r.Allowed {
		t.Error("expected blocked for story cherry-pick without accepted label")
	}
}

func TestCheckMergeGate_EmptyCommand(t *testing.T) {
	r := CheckMergeGate("/some/dir", "")
	if !r.Allowed {
		t.Error("expected allowed for empty command")
	}
}

func TestCheckMergeGate_EmptyProjectRoot(t *testing.T) {
	r := CheckMergeGate("", "git merge origin/story/PROJ-a1b2")
	if !r.Allowed {
		t.Error("expected allowed for empty project root")
	}
}

func TestCheckMergeGate_UsesSharedVaultOverLocalBranchState(t *testing.T) {
	dir, sharedVault := setupPaivotWorktree(t)
	writeProjectSettings(t, dir)

	localIssuesDir := filepath.Join(dir, ".vault", "issues")
	if err := os.MkdirAll(localIssuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	staleLocal := "---\ntitle: Local\nstatus: in_progress\nlabels: [delivered]\n---\nBody"
	if err := os.WriteFile(filepath.Join(localIssuesDir, "PROJ-a1b2.md"), []byte(staleLocal), 0644); err != nil {
		t.Fatal(err)
	}

	sharedIssuesDir := filepath.Join(sharedVault, "issues")
	if err := os.MkdirAll(sharedIssuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	authoritative := "---\ntitle: Shared\nstatus: closed\nlabels: [delivered, accepted]\n---\nBody"
	if err := os.WriteFile(filepath.Join(sharedIssuesDir, "PROJ-a1b2.md"), []byte(authoritative), 0644); err != nil {
		t.Fatal(err)
	}

	r := CheckMergeGate(dir, "git merge origin/story/PROJ-a1b2")
	if !r.Allowed {
		t.Fatalf("expected shared vault state to allow merge, got blocked: %s", r.Reason)
	}
}

// --- ReadIssueLabels tests ---

func TestReadIssueLabels_ValidLabels(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".vault", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Test\nstatus: closed\nlabels: [delivered, accepted, hard-tdd]\n---\nBody"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-a1b2.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	labels := ReadIssueLabels(dir, "PROJ-a1b2")
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d: %v", len(labels), labels)
	}
	if labels[0] != "delivered" || labels[1] != "accepted" || labels[2] != "hard-tdd" {
		t.Errorf("unexpected labels: %v", labels)
	}
}

func TestReadIssueLabels_EmptyArray(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".vault", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Test\nlabels: []\n---\nBody"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-a1b2.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	labels := ReadIssueLabels(dir, "PROJ-a1b2")
	if labels == nil || len(labels) != 0 {
		t.Errorf("expected empty slice, got %v", labels)
	}
}

func TestReadIssueLabels_MissingFile(t *testing.T) {
	dir := t.TempDir()
	labels := ReadIssueLabels(dir, "PROJ-noexist")
	if labels != nil {
		t.Errorf("expected nil for missing file, got %v", labels)
	}
}

func TestReadIssueLabels_NoLabelsField(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".vault", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Test\nstatus: open\n---\nBody"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-a1b2.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	labels := ReadIssueLabels(dir, "PROJ-a1b2")
	if labels == nil || len(labels) != 0 {
		t.Errorf("expected empty slice for no labels field, got %v", labels)
	}
}

// --- parseYAMLArray tests ---

func TestParseYAMLArray_Normal(t *testing.T) {
	result := parseYAMLArray("[a, b, c]")
	if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("unexpected: %v", result)
	}
}

func TestParseYAMLArray_Quoted(t *testing.T) {
	result := parseYAMLArray(`["agents-of-chaos", "channel-mediation"]`)
	if len(result) != 2 || result[0] != "agents-of-chaos" || result[1] != "channel-mediation" {
		t.Errorf("unexpected: %v", result)
	}
}

func TestParseYAMLArray_Empty(t *testing.T) {
	result := parseYAMLArray("[]")
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestParseYAMLArray_Blank(t *testing.T) {
	result := parseYAMLArray("")
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}
