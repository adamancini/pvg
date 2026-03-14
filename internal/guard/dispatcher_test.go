package guard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paivot-ai/pvg/internal/dispatcher"
)

func setupDispatcher(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	knowledgeDir := filepath.Join(root, ".vault", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, ".claude", "worktrees", "agent-1")
	if err := os.MkdirAll(worktree, 0755); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.On(root); err != nil {
		t.Fatal(err)
	}
	return root, worktree
}

func TestCheckDispatcher_AllowsWhenNoStateFile(t *testing.T) {
	dir := t.TempDir()
	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: filepath.Join(dir, "BUSINESS.md")},
	}
	result := CheckDispatcher(dir, input)
	if !result.Allowed {
		t.Errorf("expected allowed when no state file, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_AllowsWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	knowledgeDir := filepath.Join(dir, ".vault", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create disabled state
	if err := dispatcher.On(dir); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Off(dir); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: filepath.Join(dir, "BUSINESS.md")},
	}
	// State file removed by Off, so ReadState returns error -> fail-open
	result := CheckDispatcher(dir, input)
	if !result.Allowed {
		t.Errorf("expected allowed when disabled, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_BlocksBUSINESSmd_NoBLTAgent(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: filepath.Join(dir, "BUSINESS.md")},
	}
	result := CheckDispatcher(dir, input)
	if result.Allowed {
		t.Error("expected blocked for BUSINESS.md without BLT agent")
	}
}

func TestCheckDispatcher_BlocksDESIGNmd_NoBLTAgent(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Edit",
		ToolInput: ToolInput{FilePath: "/some/path/DESIGN.md"},
	}
	result := CheckDispatcher(dir, input)
	if result.Allowed {
		t.Error("expected blocked for DESIGN.md without BLT agent")
	}
}

func TestCheckDispatcher_BlocksARCHITECTUREmd_NoBLTAgent(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: "/project/ARCHITECTURE.md"},
	}
	result := CheckDispatcher(dir, input)
	if result.Allowed {
		t.Error("expected blocked for ARCHITECTURE.md without BLT agent")
	}
}

func TestCheckDispatcher_BlocksBUSINESSmd_FromOrchestratorRootEvenWithBLTAgent(t *testing.T) {
	root, worktree := setupDispatcher(t)

	// Track a BLT agent
	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:business-analyst"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: filepath.Join(root, "BUSINESS.md")},
	}
	result := CheckDispatcher(root, input)
	if result.Allowed {
		t.Error("expected orchestrator root write to stay blocked even with active BLT agent")
	}
}

func TestCheckDispatcher_AllowsBUSINESSmd_FromMatchingWorktree(t *testing.T) {
	root, worktree := setupDispatcher(t)

	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:business-analyst"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: filepath.Join(worktree, "BUSINESS.md")},
	}
	result := CheckDispatcher(worktree, input)
	if !result.Allowed {
		t.Errorf("expected matching BLT worktree write allowed, got blocked: %s", result.Reason)
	}

	state, err := dispatcher.ReadState(root)
	if err != nil {
		t.Fatalf("read dispatcher state: %v", err)
	}
	if !dispatcher.HasActiveAgentTypeAtPath(state, "paivot-graph:business-analyst", worktree) {
		t.Fatal("expected tracked worktree to be recognized for active BA")
	}
}

func TestCheckDispatcher_BlocksMismatchedBLTAgent(t *testing.T) {
	_, worktree := setupDispatcher(t)

	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:designer"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: filepath.Join(worktree, "BUSINESS.md")},
	}
	result := CheckDispatcher(worktree, input)
	if result.Allowed {
		t.Error("expected BUSINESS.md write to be blocked for mismatched BLT agent")
	}
}

func TestCheckDispatcher_AllowsNonDFFiles(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: filepath.Join(dir, "main.go")},
	}
	result := CheckDispatcher(dir, input)
	if !result.Allowed {
		t.Errorf("expected allowed for non-D&F file, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_AllowsEmptyProjectRoot(t *testing.T) {
	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: "/some/BUSINESS.md"},
	}
	result := CheckDispatcher("", input)
	if !result.Allowed {
		t.Error("expected allowed with empty project root")
	}
}

func TestCheckDispatcher_BashBlocksRedirectToDFFile(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `cat content.txt > BUSINESS.md`},
	}
	result := CheckDispatcher(dir, input)
	if result.Allowed {
		t.Error("expected blocked for bash redirect to BUSINESS.md")
	}
}

func TestCheckDispatcher_BashAllowsReadFromDFFile(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `cat BUSINESS.md`},
	}
	result := CheckDispatcher(dir, input)
	if !result.Allowed {
		t.Errorf("expected allowed for reading BUSINESS.md, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_BashBlocksCpToDFFile(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `cp /tmp/draft.md DESIGN.md`},
	}
	result := CheckDispatcher(dir, input)
	if result.Allowed {
		t.Error("expected blocked for cp to DESIGN.md")
	}
}

func TestCheckDispatcher_BashAllowsDFWriteWithAgent(t *testing.T) {
	_, worktree := setupDispatcher(t)
	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:architect"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `cat content.txt > ARCHITECTURE.md`},
	}
	result := CheckDispatcher(worktree, input)
	if !result.Allowed {
		t.Errorf("expected allowed with BLT agent, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_BashBlocksDFWriteWithWrongAgent(t *testing.T) {
	_, worktree := setupDispatcher(t)
	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:business-analyst"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `cat content.txt > ARCHITECTURE.md`},
	}
	result := CheckDispatcher(worktree, input)
	if result.Allowed {
		t.Error("expected ARCHITECTURE.md write to be blocked for wrong BLT agent")
	}
}

func TestCheckDispatcher_BashBlocksMutatingNDCommandFromCoordinator(t *testing.T) {
	root, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `nd close PROJ-a1b2`},
	}
	result := CheckDispatcher(root, input)
	if result.Allowed {
		t.Fatal("expected coordinator nd mutation to be blocked in dispatcher mode")
	}
}

func TestCheckDispatcher_BashAllowsMutatingNDCommandFromDeveloperWorktree(t *testing.T) {
	_, worktree := setupDispatcher(t)
	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:developer"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `nd update PROJ-a1b2 --add-label delivered`},
	}
	result := CheckDispatcher(worktree, input)
	if !result.Allowed {
		t.Fatalf("expected developer worktree nd mutation allowed, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_BashAllowsMutatingNDCommandFromPMWorktree(t *testing.T) {
	_, worktree := setupDispatcher(t)
	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:pm"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `nd close PROJ-a1b2`},
	}
	result := CheckDispatcher(worktree, input)
	if !result.Allowed {
		t.Fatalf("expected pm worktree nd close allowed, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_BashAllowsMutatingNDCommandFromSrPMWorktree(t *testing.T) {
	_, worktree := setupDispatcher(t)
	if err := dispatcher.TrackAgent(worktree, "agent-1", "paivot-graph:sr-pm"); err != nil {
		t.Fatal(err)
	}

	input := HookInput{
		ToolName:  "Bash",
		ToolInput: ToolInput{Command: `nd create "Story title"`},
	}
	result := CheckDispatcher(worktree, input)
	if !result.Allowed {
		t.Fatalf("expected sr-pm worktree nd create allowed, got blocked: %s", result.Reason)
	}
}

func TestCheckDispatcher_BlockReasonContainsInstructions(t *testing.T) {
	dir, _ := setupDispatcher(t)
	input := HookInput{
		ToolName:  "Write",
		ToolInput: ToolInput{FilePath: "/project/DESIGN.md"},
	}
	result := CheckDispatcher(dir, input)
	if result.Allowed {
		t.Fatal("expected blocked")
	}
	if result.Reason == "" {
		t.Error("expected non-empty block reason")
	}
	// Check that the message tells the user what to do
	checks := []string{"BLOCKED", "Dispatcher mode", "BLT agents", "designer"}
	for _, check := range checks {
		if !containsStr(result.Reason, check) {
			t.Errorf("block reason missing %q: %s", check, result.Reason)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
