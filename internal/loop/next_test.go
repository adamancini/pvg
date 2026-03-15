package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateNext_PrioritizesDeliveredInPriorityEpic(t *testing.T) {
	withStubbedND(t, map[string]string{
		"ready --json":                               `[]`,
		"list --status in_progress --json":           `[]`,
		"list --status open --label rejected --json": `[]`,
		"blocked --json":                             `[]`,
		"list --status !closed --json":               `[{"ID":"PROJ-epic-story","Status":"in_progress","Parent":"PROJ-epic","Labels":["delivered"]}]`,
		"list --status in_progress --label delivered --sort priority --json":                    `[{"ID":"PROJ-backlog","Title":"Global delivery","Status":"in_progress","Labels":["delivered"]}]`,
		"list --status open --label rejected --sort priority --json":                            `[]`,
		"ready --sort priority --json":                                                          `[{"ID":"PROJ-ready","Title":"Ready story","Status":"ready","Labels":[]}]`,
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[{"ID":"PROJ-epic-story","Title":"Epic delivery","Status":"in_progress","Parent":"PROJ-epic","Labels":["delivered"]}]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
	})

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}

	if result.Decision != "act" {
		t.Fatalf("expected act decision, got %s", result.Decision)
	}
	if result.Next == nil || result.Next.StoryID != "PROJ-epic-story" {
		t.Fatalf("expected epic story to be selected, got %#v", result.Next)
	}
	if result.Next.Scope != "priority_epic" {
		t.Fatalf("expected priority_epic scope, got %#v", result.Next)
	}
}

func TestEvaluateNext_FallsBackToBacklogWhenPriorityEpicEmpty(t *testing.T) {
	withStubbedND(t, map[string]string{
		"ready --json":                               `[{"ID":"PROJ-backlog","Status":"ready"}]`,
		"list --status in_progress --json":           `[]`,
		"list --status open --label rejected --json": `[]`,
		"blocked --json":                             `[]`,
		"list --status !closed --json":               `[{"ID":"PROJ-backlog","Status":"ready"}]`,
		"list --status in_progress --label delivered --sort priority --json":                    `[]`,
		"list --status open --label rejected --sort priority --json":                            `[]`,
		"ready --sort priority --json":                                                          `[{"ID":"PROJ-backlog","Title":"Backlog story","Status":"ready","Labels":[]}]`,
		"list --status in_progress --label delivered --sort priority --json --parent PROJ-epic": `[]`,
		"list --status open --label rejected --sort priority --json --parent PROJ-epic":         `[]`,
		"ready --sort priority --json --parent PROJ-epic":                                       `[]`,
	})

	result, err := EvaluateNext(t.TempDir(), "epic", "PROJ-epic")
	if err != nil {
		t.Fatalf("EvaluateNext() error: %v", err)
	}

	if result.Next == nil || result.Next.StoryID != "PROJ-backlog" {
		t.Fatalf("expected backlog story, got %#v", result.Next)
	}
	if result.Next.Scope != "backlog" {
		t.Fatalf("expected backlog scope, got %#v", result.Next)
	}
}

func TestEvaluateNext_PrefersRejectedBeforeReadyAndExcludesRejectedFromReadyCount(t *testing.T) {
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
	if result.Counts.Rejected != 1 {
		t.Fatalf("expected rejected count 1, got %+v", result.Counts)
	}
}

func TestEvaluateNext_HardTDDReadyStartsInRedPhase(t *testing.T) {
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

func TestEvaluateNext_WaitsWhenOnlyInProgressRemains(t *testing.T) {
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
