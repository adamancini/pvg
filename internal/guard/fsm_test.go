package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// --- Config parsing ---

func TestParseWorkflowConfig_Disabled(t *testing.T) {
	wc := ParseWorkflowConfig(map[string]string{})
	if wc.Enabled {
		t.Error("expected disabled when key missing")
	}
	wc = ParseWorkflowConfig(map[string]string{"workflow.fsm": "false"})
	if wc.Enabled {
		t.Error("expected disabled for 'false'")
	}
}

func TestParseWorkflowConfig_Enabled(t *testing.T) {
	wc := ParseWorkflowConfig(map[string]string{
		"workflow.fsm":      "true",
		"workflow.sequence": "open,in_progress,delivered,review,closed",
	})
	if !wc.Enabled {
		t.Error("expected enabled")
	}
	if len(wc.Sequence) != 5 {
		t.Errorf("expected 5 statuses, got %d", len(wc.Sequence))
	}
}

func TestParseWorkflowConfig_ExitRules(t *testing.T) {
	wc := ParseWorkflowConfig(map[string]string{
		"workflow.fsm":        "true",
		"workflow.exit_rules": "blocked:open,in_progress;rejected:in_progress",
	})
	if len(wc.ExitRules) != 2 {
		t.Fatalf("expected 2 exit rules, got %d", len(wc.ExitRules))
	}
	if targets := wc.ExitRules["blocked"]; len(targets) != 2 {
		t.Errorf("expected 2 targets for blocked, got %d", len(targets))
	}
	if targets := wc.ExitRules["rejected"]; len(targets) != 1 || targets[0] != "in_progress" {
		t.Errorf("unexpected targets for rejected: %v", targets)
	}
}

func TestParseWorkflowConfig_EmptySequence(t *testing.T) {
	wc := ParseWorkflowConfig(map[string]string{
		"workflow.fsm":      "true",
		"workflow.sequence": "",
	})
	if len(wc.Sequence) != 0 {
		t.Errorf("expected empty sequence, got %d", len(wc.Sequence))
	}
}

func TestParseWorkflowConfig_MalformedExitRules(t *testing.T) {
	wc := ParseWorkflowConfig(map[string]string{
		"workflow.fsm":        "true",
		"workflow.exit_rules": "malformed;also-bad;:;good:open",
	})
	// Only "good:open" should parse
	if len(wc.ExitRules) != 1 {
		t.Errorf("expected 1 exit rule, got %d: %v", len(wc.ExitRules), wc.ExitRules)
	}
}

// --- Transition validation ---

func defaultWC() WorkflowConfig {
	return ParseWorkflowConfig(map[string]string{
		"workflow.fsm":        "true",
		"workflow.sequence":   "open,in_progress,delivered,review,closed",
		"workflow.exit_rules": "blocked:open,in_progress;rejected:in_progress",
	})
}

func TestValidateTransition_ForwardOneStep(t *testing.T) {
	wc := defaultWC()
	tests := []struct {
		from, to string
	}{
		{"open", "in_progress"},
		{"in_progress", "delivered"},
		{"delivered", "review"},
		{"review", "closed"},
	}
	for _, tt := range tests {
		r := ValidateTransition(wc, "TEST-a1b2", tt.from, tt.to)
		if !r.Allowed {
			t.Errorf("%s -> %s: expected allowed, got blocked: %s", tt.from, tt.to, r.Reason)
		}
	}
}

func TestValidateTransition_ForwardSkipBlocked(t *testing.T) {
	wc := defaultWC()
	tests := []struct {
		from, to string
	}{
		{"open", "delivered"},
		{"open", "closed"},
		{"in_progress", "review"},
		{"in_progress", "closed"},
		{"delivered", "closed"},
	}
	for _, tt := range tests {
		r := ValidateTransition(wc, "TEST-a1b2", tt.from, tt.to)
		if r.Allowed {
			t.Errorf("%s -> %s: expected blocked, got allowed", tt.from, tt.to)
		}
	}
}

func TestValidateTransition_BackwardAny(t *testing.T) {
	wc := defaultWC()
	tests := []struct {
		from, to string
	}{
		{"closed", "review"},
		{"closed", "open"},
		{"review", "delivered"},
		{"review", "open"},
		{"delivered", "open"},
		{"in_progress", "open"},
	}
	for _, tt := range tests {
		r := ValidateTransition(wc, "TEST-a1b2", tt.from, tt.to)
		if !r.Allowed {
			t.Errorf("%s -> %s: expected allowed (backward), got blocked: %s", tt.from, tt.to, r.Reason)
		}
	}
}

func TestValidateTransition_SameStatusNoOp(t *testing.T) {
	wc := defaultWC()
	r := ValidateTransition(wc, "TEST-a1b2", "open", "open")
	if !r.Allowed {
		t.Errorf("same status should be no-op, got blocked: %s", r.Reason)
	}
}

func TestValidateTransition_ExitRules(t *testing.T) {
	wc := defaultWC()

	// blocked -> open: allowed by exit rule
	r := ValidateTransition(wc, "TEST-a1b2", "blocked", "open")
	if !r.Allowed {
		t.Errorf("blocked -> open: expected allowed by exit rule, got blocked: %s", r.Reason)
	}
	// blocked -> in_progress: allowed by exit rule
	r = ValidateTransition(wc, "TEST-a1b2", "blocked", "in_progress")
	if !r.Allowed {
		t.Errorf("blocked -> in_progress: expected allowed by exit rule, got blocked: %s", r.Reason)
	}
	// blocked -> closed: NOT in exit rule
	r = ValidateTransition(wc, "TEST-a1b2", "blocked", "closed")
	if r.Allowed {
		t.Error("blocked -> closed: expected blocked by exit rule, got allowed")
	}
	// rejected -> in_progress: allowed by exit rule
	r = ValidateTransition(wc, "TEST-a1b2", "rejected", "in_progress")
	if !r.Allowed {
		t.Errorf("rejected -> in_progress: expected allowed by exit rule, got blocked: %s", r.Reason)
	}
	// rejected -> closed: NOT in exit rule
	r = ValidateTransition(wc, "TEST-a1b2", "rejected", "closed")
	if r.Allowed {
		t.Error("rejected -> closed: expected blocked by exit rule, got allowed")
	}
}

func TestValidateTransition_OffSequence(t *testing.T) {
	wc := defaultWC()
	// "custom_status" is not in the sequence -- should be unrestricted
	r := ValidateTransition(wc, "TEST-a1b2", "open", "custom_status")
	if !r.Allowed {
		t.Errorf("off-sequence target should be allowed, got blocked: %s", r.Reason)
	}
	r = ValidateTransition(wc, "TEST-a1b2", "custom_status", "closed")
	if !r.Allowed {
		t.Errorf("off-sequence source should be allowed, got blocked: %s", r.Reason)
	}
}

// --- nd command parsing ---

func TestParseNdStatusChange_UpdateEquals(t *testing.T) {
	ids, status, found := parseNdStatusChange("nd update PROJ-a3f8 --status=in_progress")
	if !found || status != "in_progress" || len(ids) != 1 || ids[0] != "PROJ-a3f8" {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

func TestParseNdStatusChange_UpdateSpace(t *testing.T) {
	ids, status, found := parseNdStatusChange("nd update PROJ-a3f8 --status delivered")
	if !found || status != "delivered" || len(ids) != 1 || ids[0] != "PROJ-a3f8" {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

func TestParseNdStatusChange_Close(t *testing.T) {
	ids, status, found := parseNdStatusChange("nd close PROJ-a3f8")
	if !found || status != "closed" || len(ids) != 1 || ids[0] != "PROJ-a3f8" {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

func TestParseNdStatusChange_CloseMultiple(t *testing.T) {
	ids, status, found := parseNdStatusChange("nd close PROJ-a1b2 PROJ-c3d4 PROJ-e5f6")
	if !found || status != "closed" || len(ids) != 3 {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

func TestParseNdStatusChange_NonStatusUpdate(t *testing.T) {
	_, _, found := parseNdStatusChange("nd update PROJ-a3f8 --title=new-title")
	if found {
		t.Error("non-status update should not be detected")
	}
}

func TestParseNdStatusChange_NonNdCommand(t *testing.T) {
	_, _, found := parseNdStatusChange("git status")
	if found {
		t.Error("non-nd command should not be detected")
	}
}

func TestParseNdStatusChange_FullPath(t *testing.T) {
	ids, status, found := parseNdStatusChange("/usr/local/bin/nd update PROJ-a3f8 --status=closed")
	if !found || status != "closed" || len(ids) != 1 || ids[0] != "PROJ-a3f8" {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

func TestParseNdStatusChange_WithVaultFlag(t *testing.T) {
	ids, status, found := parseNdStatusChange("nd --vault .nd update PROJ-a3f8 --status=delivered")
	if !found || status != "delivered" || len(ids) != 1 || ids[0] != "PROJ-a3f8" {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

func TestParseNdStatusChange_ChainedCommand(t *testing.T) {
	ids, status, found := parseNdStatusChange("echo hello && nd update PROJ-a3f8 --status=in_progress")
	if !found || status != "in_progress" || len(ids) != 1 || ids[0] != "PROJ-a3f8" {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

func TestParseNdStatusChange_SemicolonChain(t *testing.T) {
	ids, status, found := parseNdStatusChange("echo hello; nd close PROJ-a3f8")
	if !found || status != "closed" || len(ids) != 1 || ids[0] != "PROJ-a3f8" {
		t.Errorf("unexpected: ids=%v status=%q found=%v", ids, status, found)
	}
}

// --- Issue status reading ---

func TestReadIssueStatus_ValidFile(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".vault", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Test\nstatus: in_progress\n---\nBody text"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-a1b2.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	status := ReadIssueStatus(dir, "PROJ-a1b2")
	if status != "in_progress" {
		t.Errorf("expected 'in_progress', got %q", status)
	}
}

func TestReadIssueStatus_MissingFile(t *testing.T) {
	dir := t.TempDir()
	status := ReadIssueStatus(dir, "PROJ-noexist")
	if status != "" {
		t.Errorf("expected empty for missing file, got %q", status)
	}
}

func TestReadIssueStatus_MissingDirectory(t *testing.T) {
	status := ReadIssueStatus("/nonexistent/project", "PROJ-a1b2")
	if status != "" {
		t.Errorf("expected empty for missing dir, got %q", status)
	}
}

func TestReadIssueStatus_MalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".vault", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "no frontmatter here\njust text"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-bad.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	status := ReadIssueStatus(dir, "PROJ-bad")
	if status != "" {
		t.Errorf("expected empty for malformed frontmatter, got %q", status)
	}
}

func TestReadIssueStatus_NoStatusField(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, ".vault", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntitle: Test\npriority: high\n---\nBody"
	if err := os.WriteFile(filepath.Join(issuesDir, "PROJ-nostatus.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	status := ReadIssueStatus(dir, "PROJ-nostatus")
	if status != "" {
		t.Errorf("expected empty for missing status field, got %q", status)
	}
}

// --- Integration: CheckFSM end-to-end ---

func setupFSMProject(t *testing.T, fsmEnabled bool, issueID, issueStatus string) string {
	t.Helper()
	dir := t.TempDir()

	// Create settings
	settingsDir := filepath.Join(dir, ".vault", "knowledge")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	enabled := "false"
	if fsmEnabled {
		enabled = "true"
	}
	settingsContent := fmt.Sprintf(
		"workflow.fsm: %s\nworkflow.sequence: open,in_progress,delivered,review,closed\nworkflow.exit_rules: blocked:open,in_progress;rejected:in_progress\n",
		enabled)
	if err := os.WriteFile(filepath.Join(settingsDir, ".settings.yaml"), []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create issue if provided
	if issueID != "" && issueStatus != "" {
		issuesDir := filepath.Join(dir, ".vault", "issues")
		if err := os.MkdirAll(issuesDir, 0755); err != nil {
			t.Fatal(err)
		}
		issueContent := fmt.Sprintf("---\ntitle: Test Issue\nstatus: %s\n---\nBody", issueStatus)
		if err := os.WriteFile(filepath.Join(issuesDir, issueID+".md"), []byte(issueContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestCheckFSM_Disabled(t *testing.T) {
	dir := setupFSMProject(t, false, "PROJ-a1b2", "open")
	r := CheckFSM(dir, "nd update PROJ-a1b2 --status=closed")
	if !r.Allowed {
		t.Errorf("expected allowed when FSM disabled, got blocked: %s", r.Reason)
	}
}

func TestCheckFSM_AllowedTransition(t *testing.T) {
	dir := setupFSMProject(t, true, "PROJ-a1b2", "open")
	r := CheckFSM(dir, "nd update PROJ-a1b2 --status=in_progress")
	if !r.Allowed {
		t.Errorf("expected allowed for open -> in_progress, got blocked: %s", r.Reason)
	}
}

func TestCheckFSM_BlockedTransition(t *testing.T) {
	dir := setupFSMProject(t, true, "PROJ-a1b2", "open")
	r := CheckFSM(dir, "nd update PROJ-a1b2 --status=closed")
	if r.Allowed {
		t.Error("expected blocked for open -> closed, got allowed")
	}
	if r.Reason == "" {
		t.Error("expected error message with reason")
	}
}

func TestCheckFSM_FailOpenMissingFile(t *testing.T) {
	dir := setupFSMProject(t, true, "", "") // no issue file
	r := CheckFSM(dir, "nd update PROJ-noexist --status=closed")
	if !r.Allowed {
		t.Errorf("expected fail-open for missing issue file, got blocked: %s", r.Reason)
	}
}

func TestCheckFSM_FailOpenMissingSettings(t *testing.T) {
	dir := t.TempDir() // no settings at all
	r := CheckFSM(dir, "nd update PROJ-a1b2 --status=closed")
	if !r.Allowed {
		t.Errorf("expected fail-open for missing settings, got blocked: %s", r.Reason)
	}
}

func TestCheckFSM_NonNdCommand(t *testing.T) {
	dir := setupFSMProject(t, true, "PROJ-a1b2", "open")
	r := CheckFSM(dir, "git status")
	if !r.Allowed {
		t.Errorf("expected allowed for non-nd command, got blocked: %s", r.Reason)
	}
}

func TestCheckFSM_CloseBlocked(t *testing.T) {
	dir := setupFSMProject(t, true, "PROJ-a1b2", "in_progress")
	r := CheckFSM(dir, "nd close PROJ-a1b2")
	if r.Allowed {
		t.Error("expected blocked for in_progress -> closed via nd close, got allowed")
	}
}

func TestCheckFSM_CloseAllowedFromReview(t *testing.T) {
	dir := setupFSMProject(t, true, "PROJ-a1b2", "review")
	r := CheckFSM(dir, "nd close PROJ-a1b2")
	if !r.Allowed {
		t.Errorf("expected allowed for review -> closed, got blocked: %s", r.Reason)
	}
}

func TestCheckFSM_EmptyProjectRoot(t *testing.T) {
	r := CheckFSM("", "nd update PROJ-a1b2 --status=closed")
	if !r.Allowed {
		t.Errorf("expected allowed for empty project root, got blocked: %s", r.Reason)
	}
}

func TestCheckFSM_BackwardAllowed(t *testing.T) {
	dir := setupFSMProject(t, true, "PROJ-a1b2", "delivered")
	r := CheckFSM(dir, "nd update PROJ-a1b2 --status=open")
	if !r.Allowed {
		t.Errorf("expected allowed for backward transition, got blocked: %s", r.Reason)
	}
}
