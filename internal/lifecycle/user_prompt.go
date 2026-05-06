package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paivot-ai/pvg/internal/dispatcher"
	"github.com/paivot-ai/pvg/internal/sessionname"
)

// userPromptInput matches the JSON Claude Code sends to UserPromptSubmit hooks.
type userPromptInput struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id"`
}

// triggerPhrases are case-insensitive phrases that activate dispatcher mode.
var triggerPhrases = []string{
	"use paivot",
	"paivot this",
	"run paivot",
	"engage paivot",
	"with paivot",
}

// renameSuggestionContext is the one-shot rename guidance injected on the first
// UserPromptSubmit for a given session_id when no dispatcher activation occurs.
// Project name is derived from cwd basename; the model is expected to pick a
// high-level workstream name and call `/rename "<project>: <workstream>"`.
func renameSuggestionContext(project string) string {
	return "SESSION NAMING: Consider calling `/rename \"" + project + ": <workstream>\"` if the session scope is now clear. " +
		"Workstream should be high-level (e.g., 'session naming UX'), not story-specific. " +
		"This is one-shot; you will not see this reminder again for this session."
}

// dispatcherActivationContext is the full context injected when dispatcher mode
// is first activated by a trigger phrase. Includes rename guidance if this is the
// first prompt for the session.
func buildDispatcherActivationContext(project string, includeRename bool) string {
	ctx := "DISPATCHER MODE ACTIVE. You are a coordinator only. " +
		"Do NOT write D&F files or mutate the nd backlog directly; those are structurally guarded. source code and tests must also be produced by the appropriate agent rather than by you. " +
		"Spawn the appropriate agent for any production work. " +
		"BLT QUESTIONING PROTOCOL: When a BLT agent (BA, Designer, Architect) returns output, " +
		"check for a QUESTIONS_FOR_USER block BEFORE checking for a document. " +
		"The agent's first output in any D&F engagement MUST be questions, not a document. " +
		"If the agent produced a document on its first turn without any questioning round, " +
		"this is a protocol violation -- re-spawn the agent with an explicit reminder to ask questions first. " +
		"See [[Session Naming]] for the project: workstream convention."
	if includeRename {
		ctx += " " + renameSuggestionContext(project)
	}
	return ctx
}

// dispatcherReminderContext is the concise nudge injected on every prompt when
// dispatcher mode is already active. This survives context compaction by being
// re-injected continuously rather than relying on the original activation
// message persisting in compressed context.
const dispatcherReminderContext = "DISPATCHER MODE REMINDER: You are a coordinator, NOT a producer. " +
	"Do NOT write BUSINESS.md, DESIGN.md, or ARCHITECTURE.md yourself, and do NOT mutate nd directly; the guard will block those. source code and test files must still be delegated to the appropriate agent. " +
	"Spawn the appropriate agent for any production work. " +
	"If you are about to write a file that an agent should produce, STOP and spawn the agent instead."

// UserPromptSubmit detects Paivot trigger phrases in user prompts and
// auto-enables dispatcher mode. When dispatcher mode is already active,
// injects a concise reminder on every prompt to prevent post-compaction drift.
// Also emits a one-shot rename suggestion on the first prompt per session.
func UserPromptSubmit() error {
	var input userPromptInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return nil // fail-open
	}

	cwd, _ := os.Getwd()
	if cwd == "" {
		return nil
	}
	project := filepath.Base(cwd)

	needsRename := sessionname.ShouldSuggest(cwd, input.SessionID)
	var contextToEmit string
	var markRename bool

	// Path 1: Trigger phrase -- activate dispatcher mode
	if containsTriggerPhrase(input.Prompt) {
		if err := dispatcher.On(cwd); err != nil {
			fmt.Fprintf(os.Stderr, "pvg: failed to enable dispatcher mode: %v\n", err)
			return nil
		}
		contextToEmit = buildDispatcherActivationContext(project, needsRename)
		markRename = needsRename
		if markRename {
			if err := sessionname.MarkSuggested(cwd, input.SessionID); err != nil {
				fmt.Fprintf(os.Stderr, "pvg: failed to mark rename suggested: %v\n", err)
			}
		}
		return emitDispatcherContext(contextToEmit)
	}

	// Path 2: No trigger phrase, but dispatcher mode already active -- reinforce.
	state, err := dispatcher.ReadState(cwd)
	if err == nil && state.Enabled {
		contextToEmit = dispatcherReminderContext
		// Rename suggestion is not injected during dispatcher reminders;
		// it was (or will be) handled on the first prompt for the session.
		return emitDispatcherContext(contextToEmit)
	}

	// Path 3: Not in dispatcher mode -- standalone rename suggestion on first prompt
	if needsRename {
		contextToEmit = renameSuggestionContext(project)
		markRename = true
	} else {
		return nil // nothing to emit
	}

	if markRename {
		if err := sessionname.MarkSuggested(cwd, input.SessionID); err != nil {
			fmt.Fprintf(os.Stderr, "pvg: failed to mark rename suggested: %v\n", err)
			// continue emitting anyway; state failure is non-blocking
		}
	}
	return emitDispatcherContext(contextToEmit)
}

// emitDispatcherContext outputs a UserPromptSubmit hook response with the
// given context string as additionalContext.
func emitDispatcherContext(context string) error {
	resp := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": context,
		},
	}
	return json.NewEncoder(os.Stdout).Encode(resp)
}

// negationPrefixes are words that negate the trigger when they appear
// immediately before the trigger phrase.
var negationPrefixes = []string{
	"don't ", "dont ", "do not ", "not ", "no ", "without ",
	"never ", "stop ", "disable ", "skip ",
}

// containsTriggerPhrase checks if the prompt contains any Paivot trigger phrase,
// excluding negated forms like "don't use paivot" or "not paivot".
func containsTriggerPhrase(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, phrase := range triggerPhrases {
		idx := strings.Index(lower, phrase)
		if idx < 0 {
			continue
		}

		// Check if the phrase is preceded by a negation word.
		prefix := lower[:idx]
		negated := false
		for _, neg := range negationPrefixes {
			if strings.HasSuffix(prefix, neg) {
				negated = true
				break
			}
		}
		if !negated {
			return true
		}
	}
	return false
}
