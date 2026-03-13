package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paivot-ai/pvg/internal/loop"
)

func TestBuildContinuationPrompt_WaitLike_NoDeliveries(t *testing.T) {
	state := &loop.State{Mode: "all"}
	decision := &loop.StopDecision{
		NewIteration: 3,
		Reason:       "Waiting for in-progress work to complete",
	}
	wc := &loop.WorkCounts{InProgress: 4}

	prompt := BuildContinuationPrompt(state, decision, "unlimited", wc)

	if !strings.Contains(prompt, "Wait for completions") {
		t.Error("expected wait instruction for wait-like scenario")
	}
	if !strings.Contains(prompt, "Do NOT produce explanatory output") {
		t.Error("expected silence instruction when no deliveries")
	}
	if strings.Contains(prompt, "Continue:") {
		t.Error("should not include full continuation prompt for wait-like")
	}
}

func TestBuildContinuationPrompt_WaitLike_WithDeliveries(t *testing.T) {
	state := &loop.State{Mode: "all"}
	decision := &loop.StopDecision{
		NewIteration: 5,
		Reason:       "Waiting for in-progress work to complete",
	}
	wc := &loop.WorkCounts{InProgress: 2, Delivered: 3}

	prompt := BuildContinuationPrompt(state, decision, "50", wc)

	if !strings.Contains(prompt, "3 delivered stories await PM review") {
		t.Error("expected delivery info when deliveries exist")
	}
	if strings.Contains(prompt, "Do NOT produce explanatory output") {
		t.Error("should not silence when there are deliveries to act on")
	}
}

func TestBuildContinuationPrompt_Actionable_ReadyOnly(t *testing.T) {
	state := &loop.State{Mode: "all"}
	decision := &loop.StopDecision{
		NewIteration: 2,
		Reason:       "Actionable work remains",
	}
	wc := &loop.WorkCounts{Ready: 3}

	prompt := BuildContinuationPrompt(state, decision, "50", wc)

	if !strings.Contains(prompt, "Continue:") {
		t.Error("expected continuation prompt for actionable work")
	}
	if !strings.Contains(prompt, "3 ready") {
		t.Error("expected ready count")
	}
	if !strings.Contains(prompt, "wait silently") {
		t.Error("expected 'wait silently' fallback for already-spawned agents")
	}
}

func TestBuildContinuationPrompt_Actionable_WithDeliveries(t *testing.T) {
	state := &loop.State{Mode: "all"}
	decision := &loop.StopDecision{
		NewIteration: 4,
		Reason:       "Actionable work remains",
	}
	wc := &loop.WorkCounts{Ready: 2, Delivered: 1, InProgress: 1}

	prompt := BuildContinuationPrompt(state, decision, "50", wc)

	if !strings.Contains(prompt, "1 delivered") {
		t.Error("expected delivered count")
	}
	if !strings.Contains(prompt, "2 ready") {
		t.Error("expected ready count")
	}
}

func TestBuildContinuationPrompt_EpicScope(t *testing.T) {
	state := &loop.State{Mode: "epic", TargetEpic: "PROJ-a1b"}
	decision := &loop.StopDecision{
		NewIteration: 1,
		Reason:       "Actionable work remains",
	}
	wc := &loop.WorkCounts{Ready: 1}

	prompt := BuildContinuationPrompt(state, decision, "10", wc)

	if !strings.Contains(prompt, "Priority epic: PROJ-a1b") {
		t.Error("expected epic scope in prompt")
	}
	if strings.Contains(prompt, "epic PROJ-a1b only") {
		t.Error("epic prompt should not terminate the loop at epic boundaries")
	}
}

func TestBuildContinuationPrompt_Header(t *testing.T) {
	state := &loop.State{Mode: "all"}
	decision := &loop.StopDecision{
		NewIteration: 7,
		Reason:       "Actionable work remains",
	}
	wc := &loop.WorkCounts{Ready: 1, Delivered: 2, InProgress: 3, Blocked: 4, Other: 5}

	prompt := BuildContinuationPrompt(state, decision, "20", wc)

	if !strings.Contains(prompt, "[LOOP] Iteration 7/20") {
		t.Error("expected header with iteration info")
	}
	if !strings.Contains(prompt, "Ready: 1") {
		t.Error("expected Ready count in header")
	}
	if !strings.Contains(prompt, "Delivered: 2") {
		t.Error("expected Delivered count in header")
	}
	if !strings.Contains(prompt, "In-progress: 3") {
		t.Error("expected In-progress count in header")
	}
	if !strings.Contains(prompt, "Blocked: 4") {
		t.Error("expected Blocked count in header")
	}
	if !strings.Contains(prompt, "Other: 5") {
		t.Error("expected Other count in header")
	}
}

func TestCheckLoop_PreservesStateWhenNDQueryFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".vault"), 0755); err != nil {
		t.Fatal(err)
	}

	state := loop.NewState("all", "", 50)
	state.Iteration = 7
	state.ConsecutiveWaits = 2
	state.WaitIterations = 2
	if err := loop.WriteState(dir, state); err != nil {
		t.Fatalf("WriteState() error: %v", err)
	}

	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	ndPath := filepath.Join(binDir, "nd")
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(ndPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := checkLoop(dir); err != nil {
		t.Fatalf("checkLoop() error: %v", err)
	}

	preserved, err := loop.ReadState(dir)
	if err != nil {
		t.Fatalf("expected loop state to remain after nd query failure: %v", err)
	}
	if preserved.Iteration != state.Iteration {
		t.Errorf("expected iteration %d preserved, got %d", state.Iteration, preserved.Iteration)
	}
	if preserved.ConsecutiveWaits != state.ConsecutiveWaits {
		t.Errorf("expected consecutive waits %d preserved, got %d", state.ConsecutiveWaits, preserved.ConsecutiveWaits)
	}
	if preserved.WaitIterations != state.WaitIterations {
		t.Errorf("expected wait iterations %d preserved, got %d", state.WaitIterations, preserved.WaitIterations)
	}
}
