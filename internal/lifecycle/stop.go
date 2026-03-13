package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/paivot-ai/pvg/internal/loop"
	"github.com/paivot-ai/pvg/internal/settings"
	"github.com/paivot-ai/pvg/internal/vaultcfg"
)

// Stop outputs a knowledge capture reminder when Claude tries to stop.
// If an execution loop is active, it evaluates whether to continue or allow exit.
// Reads the Stop Capture Checklist from the vault or uses a static fallback.
func Stop() error {
	cwd, _ := os.Getwd()

	// Loop check: if active, handle loop logic and return early
	if loop.IsActiveFrom(cwd) {
		return checkLoop(cwd)
	}

	v, err := vaultcfg.OpenVault()
	if err == nil {
		result, rerr := v.Read("Stop Capture Checklist", "")
		if rerr == nil && result.Content != "" {
			fmt.Println("[VAULT] Stop capture check (from vault):")
			fmt.Println()
			fmt.Println(result.Content)
			outputTwoTierReminder()
			return nil
		}
	}

	// Try direct file read
	vaultDir, derr := vaultcfg.VaultDir()
	if derr == nil {
		path := filepath.Join(vaultDir, "conventions", "Stop Capture Checklist.md")
		data, ferr := os.ReadFile(path)
		if ferr == nil && len(data) > 0 {
			fmt.Println("[VAULT] Stop capture check (from vault):")
			fmt.Println()
			fmt.Println(string(data))
			outputTwoTierReminder()
			return nil
		}
	}

	// Static fallback
	fmt.Print(staticStopChecklist())
	outputTwoTierReminder()
	return nil
}

func outputTwoTierReminder() {
	cwd, _ := os.Getwd()
	knowledgeDir := filepath.Join(cwd, ".vault", "knowledge")
	if info, err := os.Stat(knowledgeDir); err == nil && info.IsDir() {
		fmt.Print(`
[VAULT] Remember: save to the right tier.
  - Universal insights -> global vault (_inbox/)
  - Project-specific insights -> .vault/knowledge/ (local)
`)
	}
}

func staticStopChecklist() string {
	return `[VAULT] Stop capture check:

Before ending this session, confirm you have considered each of these:

- [ ] Did you capture any DECISIONS made this session?
- [ ] Did you capture any PATTERNS discovered?
- [ ] Did you capture any DEBUG INSIGHTS?
- [ ] Did you update the PROJECT INDEX NOTE?
- [ ] Did you capture project-specific knowledge to .vault/knowledge/?

If none apply (trivial session), that is fine -- but confirm it was considered.

Use: vlt vault="Claude" create name="<Title>" path="_inbox/<Title>.md" content="..." silent
`
}

// checkLoop evaluates whether the execution loop should continue or allow exit.
// On block: updates state and emits continuation JSON to stdout.
// On allow: logs reason, removes state if needed.
func checkLoop(cwd string) error {
	state, root, err := loop.ReadStateRoot(cwd)
	if err != nil {
		// State disappeared -- fail open, allow exit
		fmt.Fprintln(os.Stderr, "[LOOP] Could not read loop state, allowing exit")
		return nil
	}

	// Query nd for work counts -- fail open on error
	wc, err := loop.QueryWorkCounts(root, state.Mode, state.TargetEpic)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[LOOP] Could not query nd: %v -- allowing exit with loop state preserved\n", err)
		return nil
	}

	cfg := loop.StopConfig{
		Active:         state.Active,
		Mode:           state.Mode,
		TargetEpic:     state.TargetEpic,
		PersistState:   isLoopPersistEnabled(root),
		Iteration:      state.Iteration,
		MaxIterations:  state.MaxIterations,
		ConsecWaits:    state.ConsecutiveWaits,
		MaxConsecWaits: state.MaxConsecutiveWaits,
		WaitIterations: state.WaitIterations,
		Ready:          wc.Ready,
		Delivered:      wc.Delivered,
		InProgress:     wc.InProgress,
		Blocked:        wc.Blocked,
		Other:          wc.Other,
	}

	decision := loop.EvaluateStop(cfg)

	if decision.Allow {
		fmt.Fprintf(os.Stderr, "[LOOP] %s\n", decision.Reason)
		if decision.RemoveState {
			_ = loop.RemoveState(root)
		} else {
			// Escape valve: keep state active but update counters (reset ConsecWaits).
			// Background agent completions will resume the loop in a new session.
			state.Iteration = decision.NewIteration
			state.ConsecutiveWaits = decision.NewConsecWaits
			state.WaitIterations = decision.NewWaitIters
			_ = loop.WriteState(root, state)
		}
		return nil
	}

	// Block exit: update state and emit continuation JSON
	state.Iteration = decision.NewIteration
	state.ConsecutiveWaits = decision.NewConsecWaits
	state.WaitIterations = decision.NewWaitIters
	if err := loop.WriteState(root, state); err != nil {
		fmt.Fprintf(os.Stderr, "[LOOP] Could not update state: %v -- allowing exit\n", err)
		return nil
	}

	// Build and emit continuation
	maxIterStr := "unlimited"
	if state.MaxIterations > 0 {
		maxIterStr = strconv.Itoa(state.MaxIterations)
	}
	prompt := BuildContinuationPrompt(state, &decision, maxIterStr, &wc)

	continuation := map[string]any{
		"decision": "block",
		"reason":   decision.Reason,
		"options": []map[string]string{
			{"value": prompt},
		},
	}

	data, err := json.Marshal(continuation)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[LOOP] Could not marshal continuation: %v\n", err)
		return nil
	}

	fmt.Println(string(data))
	return nil
}

// isLoopPersistEnabled checks if loop state should persist across sessions.
// Default is false (stale loop state is removed unless explicitly preserved).
func isLoopPersistEnabled(cwd string) bool {
	path := filepath.Join(cwd, ".vault", "knowledge", ".settings.yaml")
	s := settings.LoadFile(path)
	v, ok := s["loop.persist_across_sessions"]
	if !ok {
		return false // default
	}
	return v == "true"
}

// BuildContinuationPrompt creates the prompt for the next loop iteration.
// Context-aware: minimal prompt when waiting for agents, fuller prompt when
// there is actionable work the dispatcher can act on.
func BuildContinuationPrompt(state *loop.State, decision *loop.StopDecision, maxIterStr string, wc *loop.WorkCounts) string {
	header := fmt.Sprintf(
		"[LOOP] Iteration %d/%s | Ready: %d, Delivered: %d, In-progress: %d, Blocked: %d, Other: %d | %s\n",
		decision.NewIteration, maxIterStr,
		wc.Ready, wc.Delivered, wc.InProgress, wc.Blocked, wc.Other,
		decision.Reason,
	)

	// Wait-like: nothing ready to spawn, agents are running
	if wc.Ready == 0 && wc.InProgress > 0 {
		prompt := header
		if wc.Delivered > 0 {
			prompt += fmt.Sprintf("\n%d delivered stories await PM review -- spawn PM-Acceptor.\n", wc.Delivered)
		} else {
			prompt += "\nBackground agents are working. Wait for completions.\nDo NOT produce explanatory output or spawn new agents.\n"
		}
		return prompt
	}

	// Actionable ready work exists
	prompt := header + "\nContinue:\n"
	if wc.Delivered > 0 {
		prompt += fmt.Sprintf("- PM-Acceptor: %d delivered\n", wc.Delivered)
	}
	if wc.Ready > 0 {
		prompt += fmt.Sprintf("- Developer: %d ready\n", wc.Ready)
	}
	prompt += "\nIf agents already cover all ready stories, wait silently.\n"
	prompt += "Concurrency: max 2 dev, max 1 PM, max 3 total. Dispatcher-only.\n"

	if state.Mode == "epic" && state.TargetEpic != "" {
		prompt += fmt.Sprintf("Priority epic: %s. Continue with the rest of the backlog once it is empty.\n", state.TargetEpic)
	}

	return prompt
}
