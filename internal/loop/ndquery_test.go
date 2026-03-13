package loop

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHasLabel_Found(t *testing.T) {
	labels := []string{"bug", "delivered", "urgent"}
	if !hasLabel(labels, "delivered") {
		t.Error("expected to find 'delivered'")
	}
}

func TestHasLabel_CaseInsensitive(t *testing.T) {
	labels := []string{"Bug", "Delivered", "Urgent"}
	if !hasLabel(labels, "delivered") {
		t.Error("expected case-insensitive match for 'delivered'")
	}
}

func TestHasLabel_NotFound(t *testing.T) {
	labels := []string{"bug", "urgent"}
	if hasLabel(labels, "delivered") {
		t.Error("expected not to find 'delivered'")
	}
}

func TestHasLabel_EmptyLabels(t *testing.T) {
	if hasLabel(nil, "delivered") {
		t.Error("expected false for nil labels")
	}
	if hasLabel([]string{}, "delivered") {
		t.Error("expected false for empty labels")
	}
}

func TestNDIssue_JSONParsing_Array(t *testing.T) {
	input := `[
		{"ID": "PROJ-a1b", "Status": "in_progress", "Labels": ["delivered", "bug"], "Type": "story"},
		{"ID": "PROJ-c3d", "Status": "ready", "Labels": [], "Type": "story"}
	]`

	var issues []ndIssue
	if err := json.Unmarshal([]byte(input), &issues); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].ID != "PROJ-a1b" {
		t.Errorf("expected PROJ-a1b, got %s", issues[0].ID)
	}
	if issues[0].Status != "in_progress" {
		t.Errorf("expected in_progress, got %s", issues[0].Status)
	}
	if !hasLabel(issues[0].Labels, "delivered") {
		t.Error("expected delivered label on first issue")
	}
	if issues[1].Type != "story" {
		t.Errorf("expected story type, got %s", issues[1].Type)
	}
}

func TestNDIssue_JSONParsing_Single(t *testing.T) {
	input := `{"ID": "PROJ-x1y", "Status": "ready", "Labels": ["epic"], "Type": "epic"}`

	var issue ndIssue
	if err := json.Unmarshal([]byte(input), &issue); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if issue.ID != "PROJ-x1y" {
		t.Errorf("expected PROJ-x1y, got %s", issue.ID)
	}
	if issue.Type != "epic" {
		t.Errorf("expected epic, got %s", issue.Type)
	}
}

func TestNDIssue_JSONParsing_EmptyLabels(t *testing.T) {
	input := `{"ID": "TEST-001", "Status": "ready", "Labels": null, "Type": "story"}`

	var issue ndIssue
	if err := json.Unmarshal([]byte(input), &issue); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if issue.Labels != nil {
		t.Errorf("expected nil labels, got %v", issue.Labels)
	}
	if hasLabel(issue.Labels, "delivered") {
		t.Error("hasLabel should return false for nil labels")
	}
}

func TestRunND_UsesResolvedVault(t *testing.T) {
	var calls [][]string
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return exec.Command("true")
	}
	defer func() { execCommand = oldExec }()

	override := filepath.Join(t.TempDir(), "shared-vault")
	if err := os.Setenv("ND_VAULT_DIR", override); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("ND_VAULT_DIR") }()

	if _, err := runND(t.TempDir(), "ready", "--json"); err != nil {
		t.Fatalf("runND() error: %v", err)
	}

	want := []string{"nd", "--vault", override, "ready", "--json"}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Fatalf("unexpected nd call: got %#v want %#v", calls, want)
	}
}

func TestQueryWorkCounts_EpicModeStillQueriesWholeBacklog(t *testing.T) {
	var calls [][]string
	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return exec.Command("true")
	}
	defer func() { execCommand = oldExec }()

	override := filepath.Join(t.TempDir(), "shared-vault")
	if err := os.Setenv("ND_VAULT_DIR", override); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("ND_VAULT_DIR") }()

	if _, err := QueryWorkCounts(t.TempDir(), "epic", "PROJ-epic"); err != nil {
		t.Fatalf("QueryWorkCounts() error: %v", err)
	}

	want := [][]string{
		{"nd", "--vault", override, "ready", "--json"},
		{"nd", "--vault", override, "list", "--status", "in_progress", "--json"},
		{"nd", "--vault", override, "blocked", "--json"},
		{"nd", "--vault", override, "list", "--status", "!closed", "--json"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected nd calls:\n got: %#v\nwant: %#v", calls, want)
	}
}

func TestCountOtherIssues(t *testing.T) {
	ready := []ndIssue{{ID: "PROJ-ready"}}
	inProgress := []ndIssue{{ID: "PROJ-dev"}}
	blocked := []ndIssue{{ID: "PROJ-blocked"}}
	all := []ndIssue{
		{ID: "PROJ-ready", Status: "open"},
		{ID: "PROJ-dev", Status: "in_progress"},
		{ID: "PROJ-blocked", Status: "blocked"},
		{ID: "PROJ-review", Status: "review"},
		{ID: "PROJ-qa", Status: "qa"},
	}

	if got := countOtherIssues(ready, inProgress, blocked, all); got != 2 {
		t.Fatalf("expected 2 other issues, got %d", got)
	}
}
