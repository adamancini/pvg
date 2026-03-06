// pvg is the paivot-graph CLI -- a deterministic replacement for shell hooks
// and scripts. It uses vlt as a library for all vault operations, encoding
// scope guards, proposal workflow, and session lifecycle in Go.
//
// This replaces: vault-scope-guard.sh, vault-session-start.sh,
// vault-pre-compact.sh, vault-stop.sh, vault-session-end.sh, seed-vault.sh
//
// Usage:
//
//	pvg hook session-start       # SessionStart hook
//	pvg hook pre-compact         # PreCompact hook
//	pvg hook stop                # Stop hook
//	pvg hook session-end         # SessionEnd hook
//	pvg guard                    # PreToolUse scope guard (stdin: JSON)
//	pvg seed [--force]           # Seed vault with agent prompts
//	pvg settings [key=value]     # View/set project settings
//	pvg loop setup [--all|--epic EPIC_ID] [--max-iterations|--max N]
//	pvg loop cancel              # Cancel active loop
//	pvg loop status              # Show loop state
//	pvg loop snapshot            # Checkpoint agent/worktree state
//	pvg loop recover             # Clean up after context loss
//	pvg version                  # Print version
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/paivot-ai/pvg/internal/dispatcher"
	"github.com/paivot-ai/pvg/internal/governance"
	"github.com/paivot-ai/pvg/internal/guard"
	"github.com/paivot-ai/pvg/internal/lifecycle"
	"github.com/paivot-ai/pvg/internal/loop"
	"github.com/paivot-ai/pvg/internal/settings"
	"github.com/paivot-ai/pvg/internal/vaultcfg"
)

// Set at build time via -ldflags "-X main.version=..."
// Falls back to VCS info from go build metadata when not set.
var version = ""

func resolvedVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		var vcsRev, vcsTime, vcsDirty string
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				vcsRev = s.Value
			case "vcs.time":
				vcsTime = s.Value
			case "vcs.modified":
				if s.Value == "true" {
					vcsDirty = "-dirty"
				}
			}
		}
		if vcsRev != "" {
			short := vcsRev
			if len(short) > 7 {
				short = short[:7]
			}
			v := "dev-" + short + vcsDirty
			if vcsTime != "" {
				v += " (" + vcsTime + ")"
			}
			return v
		}
	}
	return "dev"
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "hook":
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "pvg hook: missing subcommand (session-start, pre-compact, stop, session-end)")
			os.Exit(1)
		}
		err = runHook(args[0])
	case "guard":
		err = runGuard()
	case "seed":
		force := len(args) > 0 && args[0] == "--force"
		err = runSeed(force)
	case "loop":
		err = runLoop(args)
	case "dispatcher":
		err = runDispatcher(args)
	case "settings":
		err = settings.Run(args)
	case "version", "--version", "-V":
		fmt.Printf("pvg %s\n", resolvedVersion())
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "pvg: unknown command %q\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "pvg %s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `pvg -- paivot-graph CLI

Commands:
  hook session-start     SessionStart lifecycle hook
  hook pre-compact       PreCompact lifecycle hook
  hook stop              Stop lifecycle hook
  hook session-end       SessionEnd lifecycle hook
  hook user-prompt       UserPromptSubmit hook (auto-detect dispatcher mode)
  hook subagent-start    SubagentStart hook (BLT agent tracking)
  hook subagent-stop     SubagentStop hook (BLT agent tracking)
  guard                  PreToolUse scope guard (reads JSON from stdin)
  loop setup [flags]     Start an execution loop (--all, --epic ID, --max[-iterations] N)
  loop cancel            Cancel active execution loop
  loop status            Show execution loop state
  loop snapshot          Checkpoint active agent/worktree state
  loop recover           Clean up after context loss
  dispatcher on|off|status  Manage dispatcher mode
  seed [--force]         Seed vault with agent prompts and conventions
  settings [key=value]   View or set project settings
  version                Print version
  help                   Show this help`)
}

func runHook(name string) error {
	switch name {
	case "session-start":
		return lifecycle.SessionStart()
	case "pre-compact":
		return lifecycle.PreCompact()
	case "stop":
		return lifecycle.Stop()
	case "session-end":
		return lifecycle.SessionEnd()
	case "user-prompt":
		return lifecycle.UserPromptSubmit()
	case "subagent-start":
		return lifecycle.SubagentStart()
	case "subagent-stop":
		return lifecycle.SubagentStop()
	default:
		return fmt.Errorf("unknown hook %q", name)
	}
}

func runGuard() error {
	// Parse JSON from stdin -- fail-closed on parse errors to prevent
	// bypasses via malformed input.
	input, err := guard.ParseInput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pvg guard: failed to parse hook input: %v\n", err)
		os.Exit(2)
		return nil // unreachable, for the compiler
	}

	// Determine vault directory -- if vault isn't configured, nothing to
	// protect, so allow. This is intentional: the guard only activates when
	// a vault is present.
	vaultDir, err := vaultcfg.VaultDir()
	if err != nil {
		// No vault configured -- nothing to protect.
		return nil
	}

	// Get project root (CWD) for project vault checks
	cwd, _ := os.Getwd()

	// Check the operation
	result := guard.Check(vaultDir, cwd, input)
	if !result.Allowed {
		fmt.Fprintln(os.Stderr, result.Reason)
		os.Exit(2)
	}

	return nil
}

func runDispatcher(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: pvg dispatcher on|off|status")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	switch args[0] {
	case "on":
		if err := dispatcher.On(cwd); err != nil {
			return fmt.Errorf("enable dispatcher mode: %w", err)
		}
		fmt.Println("Dispatcher mode enabled.")
	case "off":
		if err := dispatcher.Off(cwd); err != nil {
			return fmt.Errorf("disable dispatcher mode: %w", err)
		}
		fmt.Println("Dispatcher mode disabled.")
	case "status":
		dispatcher.Status(cwd)
	default:
		return fmt.Errorf("unknown dispatcher subcommand %q (use on|off|status)", args[0])
	}
	return nil
}

func runLoop(args []string) error {
	if len(args) < 1 {
		loopUsage()
		return fmt.Errorf("missing subcommand")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	switch args[0] {
	case "help", "--help", "-h":
		loopUsage()
		return nil
	case "setup":
		return loopSetup(cwd, args[1:])
	case "cancel":
		return loopCancel(cwd)
	case "status":
		return loopStatus(cwd)
	case "snapshot":
		return loopSnapshot(cwd, args[1:])
	case "recover":
		return loopRecover(cwd, args[1:])
	default:
		loopUsage()
		return fmt.Errorf("unknown loop subcommand %q", args[0])
	}
}

func loopUsage() {
	fmt.Fprintln(os.Stderr, `pvg loop -- execution loop management

Subcommands:
  setup [flags]   Start an execution loop
  cancel          Cancel active execution loop
  status          Show execution loop state
  snapshot        Checkpoint active agent/worktree state
  recover         Clean up after context loss

Setup flags:
  --all                    Run all ready work across all epics
  --epic EPIC_ID           Target a specific epic (or pass EPIC_ID as positional arg)
  --max-iterations N       Max iterations before stopping (default: 50, 0 for unlimited)
  --max N                  Alias for --max-iterations
  --help, -h               Show this help

Snapshot flags:
  --agent ID=TYPE          Agent assignment (repeatable, e.g. --agent PROJ-a1b=developer)
  --json                   Output as JSON

Recover flags:
  --json                   Output as JSON

Examples:
  pvg loop setup --all
  pvg loop setup --epic PROJ-a1b
  pvg loop setup PROJ-a1b --max 10
  pvg loop setup --all --max-iterations 25
  pvg loop snapshot --agent PROJ-a1b=developer
  pvg loop recover`)
}

func loopSetup(cwd string, args []string) error {
	// Parse flags manually (consistent with pvg pattern, no cobra)
	var (
		mode    = ""
		epicID  = ""
		maxIter = 50 // default
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			loopUsage()
			return nil
		case "--all":
			mode = "all"
		case "--epic":
			if i+1 >= len(args) {
				return fmt.Errorf("--epic requires an argument")
			}
			i++
			epicID = args[i]
			mode = "epic"
		case "--max-iterations", "--max":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires an argument", args[i])
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 0 {
				return fmt.Errorf("--max-iterations must be a non-negative integer")
			}
			maxIter = n
		default:
			// Reject unknown flags before positional fallback
			if len(args[i]) > 1 && args[i][0] == '-' {
				loopUsage()
				return fmt.Errorf("unknown flag %q", args[i])
			}
			// Positional argument -- treat as epic ID
			if mode == "" {
				epicID = args[i]
				mode = "epic"
			} else {
				loopUsage()
				return fmt.Errorf("unexpected argument: %s", args[i])
			}
		}
	}

	if mode == "" {
		return fmt.Errorf("specify --all or --epic EPIC_ID (or pass epic ID as positional arg)")
	}

	// Idempotent: if already active, report status and return success
	if loop.IsActive(cwd) {
		fmt.Println("[LOOP] Execution loop already active -- no changes made.")
		return loopStatus(cwd)
	}

	// Validate epic if specified
	if mode == "epic" {
		if err := loop.ValidateEpic(epicID); err != nil {
			return fmt.Errorf("validate epic: %w", err)
		}
	}

	state := loop.NewState(mode, epicID, maxIter)
	if err := loop.WriteState(cwd, state); err != nil {
		return fmt.Errorf("write loop state: %w", err)
	}

	fmt.Println("[LOOP] Execution loop activated.")
	fmt.Printf("  Mode: %s\n", mode)
	if epicID != "" {
		fmt.Printf("  Target: %s\n", epicID)
	}
	if maxIter > 0 {
		fmt.Printf("  Max iterations: %d\n", maxIter)
	} else {
		fmt.Println("  Max iterations: unlimited")
	}
	return nil
}

func loopCancel(cwd string) error {
	if !loop.IsActive(cwd) {
		fmt.Println("[LOOP] No active loop to cancel.")
		return nil
	}

	state, _ := loop.ReadState(cwd)
	if err := loop.RemoveState(cwd); err != nil {
		return fmt.Errorf("remove loop state: %w", err)
	}

	fmt.Println("[LOOP] Execution loop cancelled.")
	if state != nil {
		fmt.Printf("  Completed iterations: %d\n", state.Iteration)
		fmt.Printf("  Wait iterations: %d\n", state.WaitIterations)
	}
	return nil
}

func loopStatus(cwd string) error {
	state, err := loop.ReadState(cwd)
	if err != nil {
		fmt.Println("[LOOP] No active loop.")
		return nil
	}

	if !state.Active {
		fmt.Println("[LOOP] Loop state exists but is inactive.")
		return nil
	}

	fmt.Println("[LOOP] Execution loop active.")
	fmt.Printf("  Mode: %s\n", state.Mode)
	if state.TargetEpic != "" {
		fmt.Printf("  Target: %s\n", state.TargetEpic)
	}
	fmt.Printf("  Iteration: %d", state.Iteration)
	if state.MaxIterations > 0 {
		fmt.Printf(" / %d", state.MaxIterations)
	}
	fmt.Println()
	fmt.Printf("  Consecutive waits: %d / %d\n", state.ConsecutiveWaits, state.MaxConsecutiveWaits)
	fmt.Printf("  Total wait iterations: %d\n", state.WaitIterations)
	fmt.Printf("  Started: %s\n", state.StartedAt)
	return nil
}

func loopSnapshot(cwd string, args []string) error {
	agentAssignments := make(map[string]string)
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			loopUsage()
			return nil
		case "--json":
			jsonOutput = true
		case "--agent":
			if i+1 >= len(args) {
				return fmt.Errorf("--agent requires ID=TYPE argument")
			}
			i++
			parts := strings.SplitN(args[i], "=", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("--agent value must be ID=TYPE (e.g. PROJ-a1b=developer)")
			}
			agentAssignments[parts[0]] = parts[1]
		default:
			if len(args[i]) > 1 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag %q", args[i])
			}
			return fmt.Errorf("unexpected argument: %s", args[i])
		}
	}

	snap, err := loop.BuildSnapshot(cwd, agentAssignments)
	if err != nil {
		return fmt.Errorf("build snapshot: %w", err)
	}

	if err := loop.WriteSnapshot(cwd, snap); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	if jsonOutput {
		data, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal snapshot: %w", err)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("[SNAPSHOT] Saved at %s\n", snap.TakenAt)
		fmt.Printf("  Stories: %d\n", len(snap.Stories))
		for _, s := range snap.Stories {
			agent := s.AgentType
			if agent == "" {
				agent = "-"
			}
			wt := s.WorktreePath
			if wt == "" {
				wt = "(none)"
			}
			fmt.Printf("  %s  agent=%s  worktree=%s\n", s.StoryID, agent, wt)
		}
	}

	return nil
}

func loopRecover(cwd string, args []string) error {
	jsonOutput := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			loopUsage()
			return nil
		case "--json":
			jsonOutput = true
		default:
			if len(args[i]) > 1 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag %q", args[i])
			}
			return fmt.Errorf("unexpected argument: %s", args[i])
		}
	}

	cfg, err := loop.BuildRecoverConfig(cwd)
	if err != nil {
		return fmt.Errorf("build recover config: %w", err)
	}

	plan := loop.EvaluateRecover(cfg)
	execErrors := loop.ExecuteRecover(cwd, plan)

	// Remove snapshot after recovery
	if err := loop.RemoveSnapshot(cwd); err != nil {
		execErrors = append(execErrors, fmt.Sprintf("remove snapshot: %v", err))
	}

	if jsonOutput {
		report := struct {
			Summary loop.RecoverSummary `json:"summary"`
			Actions []loop.RecoverAction `json:"actions"`
			Errors  []string            `json:"errors,omitempty"`
		}{
			Summary: plan.Summary,
			Actions: plan.Actions,
			Errors:  execErrors,
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal report: %w", err)
		}
		fmt.Println(string(data))
	} else {
		fmt.Println("[RECOVER] Recovery complete.")
		fmt.Printf("  Worktrees removed: %d\n", plan.Summary.WorktreesRemoved)
		fmt.Printf("  Branches deleted:  %d\n", plan.Summary.BranchesDeleted)
		fmt.Printf("  Stories reset:     %d\n", plan.Summary.StoriesReset)
		fmt.Printf("  Stories delivered:  %d (needs PM review)\n", plan.Summary.StoriesDelivered)
		fmt.Printf("  Orphan worktrees:  %d\n", plan.Summary.OrphanWorktrees)
		if len(execErrors) > 0 {
			fmt.Printf("  Errors: %d\n", len(execErrors))
			for _, e := range execErrors {
				fmt.Printf("    - %s\n", e)
			}
		}
	}

	return nil
}

func runSeed(force bool) error {
	pluginDir := os.Getenv("CLAUDE_PLUGIN_ROOT")
	if pluginDir == "" {
		// Try to find it relative to the pvg binary
		exe, err := os.Executable()
		if err == nil {
			// bin/pvg -> plugin root is ../
			candidate := filepath.Dir(filepath.Dir(exe))
			if _, serr := os.Stat(filepath.Join(candidate, ".claude-plugin")); serr == nil {
				pluginDir = candidate
			}
		}
	}
	return governance.Seed(force, pluginDir)
}
