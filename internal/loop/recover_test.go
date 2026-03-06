package loop

import (
	"testing"
)

func TestEvaluateRecover_EmptyConfig(t *testing.T) {
	plan := EvaluateRecover(RecoverConfig{})
	if len(plan.Actions) != 0 {
		t.Errorf("expected no actions for empty config, got %d", len(plan.Actions))
	}
	if plan.Summary.WorktreesRemoved != 0 {
		t.Errorf("expected 0 worktrees removed, got %d", plan.Summary.WorktreesRemoved)
	}
}

func TestEvaluateRecover_OrphanedStory(t *testing.T) {
	cfg := RecoverConfig{
		SnapshotStories: []SnapshotEntry{
			{
				StoryID:      "PROJ-a1b",
				NDStatus:     "in_progress",
				NDLabels:     nil,
				WorktreePath: "/project/.claude/worktrees/agent-a1b",
				BranchName:   "worktree-agent-a1b",
			},
		},
		InProgressIssues: []ndIssue{
			{ID: "PROJ-a1b", Status: "in_progress", Labels: nil},
		},
	}

	plan := EvaluateRecover(cfg)

	// Expect: remove worktree + delete branch + reset story
	kinds := actionKinds(plan.Actions)
	assertContains(t, kinds, ActionRemoveWorktree, "remove_worktree")
	assertContains(t, kinds, ActionDeleteBranch, "delete_branch")
	assertContains(t, kinds, ActionResetStory, "reset_story")
	assertNotContains(t, kinds, ActionNoteDelivered, "note_delivered")

	if plan.Summary.WorktreesRemoved != 1 {
		t.Errorf("expected 1 worktree removed, got %d", plan.Summary.WorktreesRemoved)
	}
	if plan.Summary.BranchesDeleted != 1 {
		t.Errorf("expected 1 branch deleted, got %d", plan.Summary.BranchesDeleted)
	}
	if plan.Summary.StoriesReset != 1 {
		t.Errorf("expected 1 story reset, got %d", plan.Summary.StoriesReset)
	}
}

func TestEvaluateRecover_DeliveredStory(t *testing.T) {
	cfg := RecoverConfig{
		SnapshotStories: []SnapshotEntry{
			{
				StoryID:      "PROJ-c3d",
				NDStatus:     "in_progress",
				NDLabels:     []string{"delivered"},
				WorktreePath: "/project/.claude/worktrees/agent-c3d",
				BranchName:   "worktree-agent-c3d",
			},
		},
		InProgressIssues: []ndIssue{
			{ID: "PROJ-c3d", Status: "in_progress", Labels: []string{"delivered"}},
		},
	}

	plan := EvaluateRecover(cfg)

	kinds := actionKinds(plan.Actions)
	assertContains(t, kinds, ActionRemoveWorktree, "remove_worktree")
	assertContains(t, kinds, ActionDeleteBranch, "delete_branch")
	assertContains(t, kinds, ActionNoteDelivered, "note_delivered")
	assertNotContains(t, kinds, ActionResetStory, "reset_story")

	if plan.Summary.StoriesDelivered != 1 {
		t.Errorf("expected 1 story delivered, got %d", plan.Summary.StoriesDelivered)
	}
	if plan.Summary.StoriesReset != 0 {
		t.Errorf("expected 0 stories reset, got %d", plan.Summary.StoriesReset)
	}
}

func TestEvaluateRecover_StoryClosedSinceSnapshot(t *testing.T) {
	cfg := RecoverConfig{
		SnapshotStories: []SnapshotEntry{
			{
				StoryID:      "PROJ-e5f",
				NDStatus:     "in_progress",
				WorktreePath: "/project/.claude/worktrees/agent-e5f",
				BranchName:   "worktree-agent-e5f",
			},
		},
		// Story no longer in-progress in nd (was closed)
		InProgressIssues: nil,
	}

	plan := EvaluateRecover(cfg)

	kinds := actionKinds(plan.Actions)
	assertContains(t, kinds, ActionRemoveWorktree, "remove_worktree")
	assertContains(t, kinds, ActionDeleteBranch, "delete_branch")
	// No nd mutation since story is no longer in-progress
	assertNotContains(t, kinds, ActionResetStory, "reset_story")
	assertNotContains(t, kinds, ActionNoteDelivered, "note_delivered")
}

func TestEvaluateRecover_OrphanWorktree(t *testing.T) {
	cfg := RecoverConfig{
		// No snapshot stories
		CurrentWorktrees: []Worktree{
			{
				Path:   "/project/.claude/worktrees/orphan-123",
				Branch: "worktree-orphan-123",
			},
		},
	}

	plan := EvaluateRecover(cfg)

	kinds := actionKinds(plan.Actions)
	assertContains(t, kinds, ActionRemoveWorktree, "remove_worktree")
	assertContains(t, kinds, ActionDeleteBranch, "delete_branch")

	if plan.Summary.OrphanWorktrees != 1 {
		t.Errorf("expected 1 orphan worktree, got %d", plan.Summary.OrphanWorktrees)
	}
}

func TestEvaluateRecover_StoryWithNoWorktree(t *testing.T) {
	cfg := RecoverConfig{
		SnapshotStories: []SnapshotEntry{
			{
				StoryID:  "PROJ-g7h",
				NDStatus: "in_progress",
				// No worktree path or branch
			},
		},
		InProgressIssues: []ndIssue{
			{ID: "PROJ-g7h", Status: "in_progress"},
		},
	}

	plan := EvaluateRecover(cfg)

	kinds := actionKinds(plan.Actions)
	assertContains(t, kinds, ActionResetStory, "reset_story")
	assertNotContains(t, kinds, ActionRemoveWorktree, "remove_worktree")
	assertNotContains(t, kinds, ActionDeleteBranch, "delete_branch")

	if plan.Summary.WorktreesRemoved != 0 {
		t.Errorf("expected 0 worktrees removed, got %d", plan.Summary.WorktreesRemoved)
	}
}

func TestEvaluateRecover_MixedStories(t *testing.T) {
	cfg := RecoverConfig{
		SnapshotStories: []SnapshotEntry{
			{
				StoryID:      "PROJ-a1b",
				NDStatus:     "in_progress",
				WorktreePath: "/wt/agent-a1b",
				BranchName:   "wt-a1b",
			},
			{
				StoryID:      "PROJ-c3d",
				NDStatus:     "in_progress",
				NDLabels:     []string{"delivered"},
				WorktreePath: "/wt/agent-c3d",
				BranchName:   "wt-c3d",
			},
			{
				StoryID:      "PROJ-e5f",
				NDStatus:     "in_progress",
				WorktreePath: "/wt/agent-e5f",
				BranchName:   "wt-e5f",
			},
		},
		CurrentWorktrees: []Worktree{
			{Path: "/wt/agent-a1b", Branch: "wt-a1b"},
			{Path: "/wt/agent-c3d", Branch: "wt-c3d"},
			{Path: "/wt/agent-e5f", Branch: "wt-e5f"},
			{Path: "/wt/orphan-xyz", Branch: "orphan-xyz"},
		},
		InProgressIssues: []ndIssue{
			{ID: "PROJ-a1b", Status: "in_progress"},
			{ID: "PROJ-c3d", Status: "in_progress", Labels: []string{"delivered"}},
			// PROJ-e5f was closed
		},
	}

	plan := EvaluateRecover(cfg)

	// 3 snapshot worktrees + 1 orphan = 4 worktrees removed
	if plan.Summary.WorktreesRemoved != 4 {
		t.Errorf("expected 4 worktrees removed, got %d", plan.Summary.WorktreesRemoved)
	}
	// 3 snapshot branches + 1 orphan = 4 branches deleted
	if plan.Summary.BranchesDeleted != 4 {
		t.Errorf("expected 4 branches deleted, got %d", plan.Summary.BranchesDeleted)
	}
	// PROJ-a1b reset (in-progress, no delivered label)
	if plan.Summary.StoriesReset != 1 {
		t.Errorf("expected 1 story reset, got %d", plan.Summary.StoriesReset)
	}
	// PROJ-c3d delivered
	if plan.Summary.StoriesDelivered != 1 {
		t.Errorf("expected 1 story delivered, got %d", plan.Summary.StoriesDelivered)
	}
	// 1 orphan
	if plan.Summary.OrphanWorktrees != 1 {
		t.Errorf("expected 1 orphan worktree, got %d", plan.Summary.OrphanWorktrees)
	}
}

func TestIsInProgressInND_Found(t *testing.T) {
	issues := []ndIssue{
		{ID: "PROJ-a1b", Status: "in_progress"},
		{ID: "PROJ-c3d", Status: "ready"},
	}
	if !isInProgressInND("PROJ-a1b", issues) {
		t.Error("expected true for in-progress story")
	}
}

func TestIsInProgressInND_NotFound(t *testing.T) {
	issues := []ndIssue{
		{ID: "PROJ-a1b", Status: "in_progress"},
	}
	if isInProgressInND("PROJ-xxx", issues) {
		t.Error("expected false for missing story")
	}
}

func TestIsInProgressInND_WrongStatus(t *testing.T) {
	issues := []ndIssue{
		{ID: "PROJ-a1b", Status: "closed"},
	}
	if isInProgressInND("PROJ-a1b", issues) {
		t.Error("expected false for closed story")
	}
}

func TestIsInProgressInND_Empty(t *testing.T) {
	if isInProgressInND("PROJ-a1b", nil) {
		t.Error("expected false for nil issues")
	}
}

func TestIsInProgressInND_CaseInsensitive(t *testing.T) {
	issues := []ndIssue{
		{ID: "PROJ-a1b", Status: "In_Progress"},
	}
	if !isInProgressInND("PROJ-a1b", issues) {
		t.Error("expected case-insensitive match")
	}
}

// --- helpers ---

func actionKinds(actions []RecoverAction) []ActionKind {
	var kinds []ActionKind
	for _, a := range actions {
		kinds = append(kinds, a.Kind)
	}
	return kinds
}

func assertContains(t *testing.T, kinds []ActionKind, want ActionKind, label string) {
	t.Helper()
	for _, k := range kinds {
		if k == want {
			return
		}
	}
	t.Errorf("expected actions to contain %s", label)
}

func assertNotContains(t *testing.T, kinds []ActionKind, unwanted ActionKind, label string) {
	t.Helper()
	for _, k := range kinds {
		if k == unwanted {
			t.Errorf("expected actions NOT to contain %s", label)
			return
		}
	}
}
