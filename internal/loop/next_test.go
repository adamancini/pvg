package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Epic mode: containment tests
// ---------------------------------------------------------------------------

func TestEvaluateNext_EpicMode_ActsOnDeliveredInEpic(t *testing.T) {
	withStubbedND(t, epicModeStubs(map[string]string{
		// Epic has a delivered story
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[{"ID":"PROJ-s1","Title":"Epic delivery","Status":"in_progress","Parent":"PROJ-epic","Labels":["delivered"]}]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
		// Epic children: one delivered
		"children PROJ-epic --json": `[{"ID":"PROJ-s1","Status":"in_progress","Labels":["delivered"]}]`,
	}))

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Decision != "act" {
		t.Fatalf("expected act, got %s: %s", result.Decision, result.Reason)
	}
	if result.Next == nil || result.Next.StoryID != "PROJ-s1" {
		t.Fatalf("expected PROJ-s1, got %#v", result.Next)
	}
	if result.Next.Scope != "epic" {
		t.Fatalf("expected epic scope, got %s", result.Next.Scope)
	}
}

func TestEvaluateNext_EpicMode_DoesNotFallThroughToGlobal(t *testing.T) {
	withStubbedND(t, epicModeStubs(map[string]string{
		// Epic has NO actionable work
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
		// Epic children: one in-progress (agents working)
		"children PROJ-epic --json": `[{"ID":"PROJ-s1","Status":"in_progress","Labels":[]}]`,
	}))

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	// Must NOT pick from global backlog -- must wait within the epic.
	if result.Decision != "wait" {
		t.Fatalf("expected wait (containment), got %s: %s", result.Decision, result.Reason)
	}
	if result.Next != nil {
		t.Fatalf("expected no action (containment), got %#v", result.Next)
	}
}

func TestEvaluateNext_EpicMode_EpicCompleteWhenAllClosed(t *testing.T) {
	withStubbedND(t, epicModeStubs(map[string]string{
		// Epic has no actionable work
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
		// Epic children: all closed (empty result)
		"children PROJ-epic --json": `[]`,
		// AutoSelectEpic: another epic exists
		"list --type epic --status !closed --sort priority --json":                            `[{"ID":"PROJ-epic","Type":"epic"},{"ID":"PROJ-e2","Title":"Next Epic","Type":"epic"}]`,
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-e2": `[]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-e2":         `[]`,
		"ready --sort priority --json --parent PROJ-e2":                                       `[{"ID":"PROJ-s2","Title":"Story Two","Status":"ready"}]`,
	}))

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Decision != "epic_complete" {
		t.Fatalf("expected epic_complete, got %s: %s", result.Decision, result.Reason)
	}
	if result.NextEpic != "PROJ-e2" {
		t.Fatalf("expected rotation to PROJ-e2, got %s", result.NextEpic)
	}
}

func TestEvaluateNext_EpicMode_EpicCompleteLastEpic(t *testing.T) {
	withStubbedND(t, epicModeStubs(map[string]string{
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
		"children PROJ-epic --json": `[]`,
		// No other epics
		"list --type epic --status !closed --sort priority --json": `[{"ID":"PROJ-epic","Type":"epic"}]`,
		// PROJ-epic is excluded by AutoSelectEpic, so no match
	}))

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Decision != "epic_complete" {
		t.Fatalf("expected epic_complete, got %s: %s", result.Decision, result.Reason)
	}
	if result.NextEpic != "" {
		t.Fatalf("expected empty NextEpic (last epic), got %s", result.NextEpic)
	}
}

func TestEvaluateNext_EpicMode_EpicBlockedWhenOnlyBlocked(t *testing.T) {
	withStubbedND(t, epicModeStubs(map[string]string{
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
		"children PROJ-epic --json": `[{"ID":"PROJ-s1","Status":"blocked","Labels":[]}]`,
	}))

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Decision != "epic_blocked" {
		t.Fatalf("expected epic_blocked, got %s: %s", result.Decision, result.Reason)
	}
}

func TestEvaluateNext_EpicMode_WaitsOnDeliveredInEpicCounts(t *testing.T) {
	// Edge case: queryQueues finds no delivered (because nd query timing),
	// but epicCounts shows delivered. Should wait, not fall through.
	withStubbedND(t, epicModeStubs(map[string]string{
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
		// But children shows delivered (race-safe: epicCounts catches it)
		"children PROJ-epic --json": `[{"ID":"PROJ-s1","Status":"in_progress","Labels":["delivered"]},{"ID":"PROJ-s2","Status":"closed","Labels":[]}]`,
	}))

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Decision != "wait" {
		t.Fatalf("expected wait (delivered in epic counts), got %s: %s", result.Decision, result.Reason)
	}
}

// ---------------------------------------------------------------------------
// All mode: legacy behavior (unchanged)
// ---------------------------------------------------------------------------

func TestEvaluateNext_AllMode_PrefersRejectedBeforeReady(t *testing.T) {
	withStubbedND(t, map[string]string{
		"ready --json":                               `[{"ID":"PROJ-rework","Status":"ready","Labels":["rejected"]},{"ID":"PROJ-ready","Status":"ready","Labels":[]}]`,
		"list --status in_progress --json":           `[]`,
		"list --status open --label rejected --json": `[{"ID":"PROJ-rework","Status":"open","Labels":["rejected"]}]`,
		"blocked --json":                             `[]`,
		"list --status !closed --json":               `[{"ID":"PROJ-rework","Status":"open","Labels":["rejected"]},{"ID":"PROJ-ready","Status":"ready","Labels":[]}]`,
		"list --status in_progress --label delivered --sort priority --json": `[]`,
		"list --status open --label rejected --sort priority --json":         `[{"ID":"PROJ-rework","Title":"Fix me","Status":"open","Labels":["rejected"]}]`,
		"ready --sort priority --json":                                       `[{"ID":"PROJ-rework","Title":"Fix me","Status":"ready","Labels":["rejected"]},{"ID":"PROJ-ready","Title":"New work","Status":"ready","Labels":[]}]`,
	})

	result, err := EvaluateNext(t.TempDir(), "all", "")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Next == nil || result.Next.Queue != "rejected" {
		t.Fatalf("expected rejected queue to win, got %#v", result.Next)
	}
	if result.Counts.Ready != 1 {
		t.Fatalf("expected ready count to exclude rejected stories, got %+v", result.Counts)
	}
}

func TestEvaluateNext_AllMode_HardTDDReadyStartsInRedPhase(t *testing.T) {
	withStubbedND(t, map[string]string{
		"ready --json":                               `[{"ID":"PROJ-hard","Status":"ready","Labels":["hard-tdd"]}]`,
		"list --status in_progress --json":           `[]`,
		"list --status open --label rejected --json": `[]`,
		"blocked --json":                             `[]`,
		"list --status !closed --json":               `[{"ID":"PROJ-hard","Status":"ready","Labels":["hard-tdd"]}]`,
		"list --status in_progress --label delivered --sort priority --json": `[]`,
		"list --status open --label rejected --sort priority --json":         `[]`,
		"ready --sort priority --json":                                       `[{"ID":"PROJ-hard","Title":"Hard story","Status":"ready","Labels":["hard-tdd"]}]`,
	})

	result, err := EvaluateNext(t.TempDir(), "all", "")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Next == nil || result.Next.Phase != "red" || !result.Next.HardTDD {
		t.Fatalf("expected hard-tdd red phase, got %#v", result.Next)
	}
}

func TestEvaluateNext_AllMode_WaitsWhenOnlyInProgressRemains(t *testing.T) {
	withStubbedND(t, map[string]string{
		"ready --json":                               `[]`,
		"list --status in_progress --json":           `[{"ID":"PROJ-run","Status":"in_progress","Labels":[]}]`,
		"list --status open --label rejected --json": `[]`,
		"blocked --json":                             `[]`,
		"list --status !closed --json":               `[{"ID":"PROJ-run","Status":"in_progress","Labels":[]}]`,
		"list --status in_progress --label delivered --sort priority --json": `[]`,
		"list --status open --label rejected --sort priority --json":         `[]`,
		"ready --sort priority --json":                                       `[]`,
	})

	result, err := EvaluateNext(t.TempDir(), "all", "")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Decision != "wait" {
		t.Fatalf("expected wait, got %+v", result)
	}
}

func TestEvaluateNext_AllMode_CompleteWhenEmpty(t *testing.T) {
	withStubbedND(t, map[string]string{
		"ready --json":                               `[]`,
		"list --status in_progress --json":           `[]`,
		"list --status open --label rejected --json": `[]`,
		"blocked --json":                             `[]`,
		"list --status !closed --json":               `[]`,
		"list --status in_progress --label delivered --sort priority --json": `[]`,
		"list --status open --label rejected --sort priority --json":         `[]`,
		"ready --sort priority --json":                                       `[]`,
	})

	result, err := EvaluateNext(t.TempDir(), "all", "")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}
	if result.Decision != "complete" {
		t.Fatalf("expected complete, got %s: %s", result.Decision, result.Reason)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// epicModeStubs merges epic-specific stubs with the global-count stubs
// that EvaluateNext always queries (for the counts in the result).
func epicModeStubs(epicStubs map[string]string) map[string]string {
	base := map[string]string{
		// Global counts (always queried by QueryWorkCounts)
		"ready --json":                               `[]`,
		"list --status in_progress --json":           `[]`,
		"list --status open --label rejected --json": `[]`,
		"blocked --json":                             `[]`,
		"list --status !closed --json":               `[]`,
	}
	for k, v := range epicStubs {
		base[k] = v
	}
	return base
}

func withStubbedND(t *testing.T, responses map[string]string) {
	t.Helper()

	override := filepath.Join(t.TempDir(), "shared-vault")
	if err := os.Setenv("ND_VAULT_DIR", override); err != nil {
		t.Fatalf("set ND_VAULT_DIR: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("ND_VAULT_DIR")
	})

	oldExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		if name != "nd" {
			return exec.Command(name, args...)
		}

		keyParts := make([]string, 0, len(args))
		skipNext := false
		for i, arg := range args {
			if skipNext {
				skipNext = false
				continue
			}
			if arg == "--vault" && i+1 < len(args) {
				skipNext = true
				continue
			}
			keyParts = append(keyParts, arg)
		}
		key := strings.Join(keyParts, " ")
		response, ok := responses[key]
		if !ok {
			return exec.Command("python3", "-c", "import sys; print(sys.argv[1], file=sys.stderr); sys.exit(1)", "missing stub for "+key)
		}
		return exec.Command("python3", "-c", "import sys; sys.stdout.write(sys.argv[1])", response)
	}
	t.Cleanup(func() {
		execCommand = oldExec
	})
}
