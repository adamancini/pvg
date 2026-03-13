package loop

// StopConfig holds all inputs needed for the stop decision.
// This is a value struct -- no I/O, no side effects.
type StopConfig struct {
	Active         bool
	Mode           string
	TargetEpic     string
	PersistState   bool
	Iteration      int
	MaxIterations  int // 0 = unlimited
	ConsecWaits    int
	MaxConsecWaits int
	WaitIterations int
	Ready          int
	Delivered      int
	InProgress     int
	Blocked        int
	Other          int
}

// StopDecision is the output of EvaluateStop.
type StopDecision struct {
	Allow          bool   // true = allow session exit
	Reason         string // human-readable explanation
	RemoveState    bool   // true = clean up state file on exit
	NewIteration   int    // updated iteration count
	NewConsecWaits int    // updated consecutive wait count
	NewWaitIters   int    // updated total wait iterations
}

// EvaluateStop is a pure function that decides whether to allow session exit
// or block it (continuing the loop). No I/O -- all context comes from cfg.
func EvaluateStop(cfg StopConfig) StopDecision {
	// Not active -- always allow
	if !cfg.Active {
		return StopDecision{
			Allow:  true,
			Reason: "Loop not active",
		}
	}

	nextIter := cfg.Iteration + 1

	// Max iterations reached
	if cfg.MaxIterations > 0 && nextIter >= cfg.MaxIterations {
		return StopDecision{
			Allow:        true,
			Reason:       "Max iterations reached",
			RemoveState:  true,
			NewIteration: nextIter,
		}
	}

	actionable := cfg.Ready + cfg.Delivered
	total := cfg.Ready + cfg.Delivered + cfg.InProgress + cfg.Blocked + cfg.Other

	// All dev work complete (total==0)
	if total == 0 {
		return StopDecision{
			Allow:        true,
			Reason:       "All work complete",
			RemoveState:  true,
			NewIteration: nextIter,
		}
	}

	// No actionable work remaining (only blocked items)
	if actionable == 0 && cfg.InProgress == 0 {
		if cfg.Other > 0 {
			return StopDecision{
				Allow:        true,
				Reason:       "Non-dispatcher workflow states remain",
				RemoveState:  false,
				NewIteration: nextIter,
			}
		}
		return StopDecision{
			Allow:        true,
			Reason:       "No actionable work remains",
			RemoveState:  true,
			NewIteration: nextIter,
		}
	}

	// Work exists but the dispatcher may be at capacity (agents running,
	// concurrency limits reached). Track consecutive waits uniformly --
	// after MaxConsecWaits iterations of no progress, allow exit.
	// Background agent notifications will resume the loop.
	newConsec := cfg.ConsecWaits + 1
	newWaitIters := cfg.WaitIterations + 1

	if newConsec >= cfg.MaxConsecWaits {
		return StopDecision{
			Allow:          true,
			Reason:         "No progress after consecutive wait iterations",
			RemoveState:    !cfg.PersistState,
			NewIteration:   nextIter,
			NewConsecWaits: 0, // reset budget for next session
			NewWaitIters:   newWaitIters,
		}
	}

	// Determine reason based on what's pending
	reason := "Actionable work remains"
	if cfg.Delivered > 0 && cfg.Ready == 0 && cfg.InProgress == 0 {
		reason = "Delivered stories await PM review"
	} else if cfg.Ready == 0 && cfg.InProgress > 0 {
		reason = "Waiting for in-progress work to complete"
	}

	return StopDecision{
		Allow:          false,
		Reason:         reason,
		NewIteration:   nextIter,
		NewConsecWaits: newConsec,
		NewWaitIters:   newWaitIters,
	}
}
