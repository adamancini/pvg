package loop

import "fmt"

type queueSnapshot struct {
	Delivered []ndIssue
	Rejected  []ndIssue
	Ready     []ndIssue
}

// NextAction describes the next deterministic orchestration step for a host platform.
type NextAction struct {
	Kind     string `json:"kind"`
	Role     string `json:"role"`
	StoryID  string `json:"story_id"`
	Story    string `json:"story,omitempty"`
	Queue    string `json:"queue"`
	Scope    string `json:"scope"`
	HardTDD  bool   `json:"hard_tdd"`
	Phase    string `json:"phase,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// NextResult is the host-agnostic orchestration decision derived from nd state.
type NextResult struct {
	Mode        string      `json:"mode"`
	TargetEpic  string      `json:"target_epic,omitempty"`
	ActiveLoop  bool        `json:"active_loop"`
	ScopeSource string      `json:"scope_source,omitempty"`
	Decision    string      `json:"decision"`
	Reason      string      `json:"reason"`
	Counts      WorkCounts  `json:"counts"`
	Next        *NextAction `json:"next,omitempty"`
}

// EvaluateNext selects the next orchestration step without mutating state.
// It respects "priority epic" mode by draining the target epic first, then
// falling back to the rest of the backlog once the target epic is empty.
func EvaluateNext(projectRoot, mode, targetEpic string) (NextResult, error) {
	result := NextResult{
		Mode:       mode,
		TargetEpic: targetEpic,
	}

	if result.Mode == "" {
		result.Mode = "all"
	}

	counts, err := QueryWorkCounts(projectRoot, result.Mode, result.TargetEpic)
	if err != nil {
		return result, err
	}

	globalQueues, err := queryQueues(projectRoot, "")
	if err != nil {
		return result, err
	}
	counts.Delivered = len(globalQueues.Delivered)
	counts.Rejected = len(globalQueues.Rejected)
	counts.Ready = len(globalQueues.Ready)
	result.Counts = counts

	if result.Mode == "epic" && result.TargetEpic != "" {
		priorityQueues, err := queryQueues(projectRoot, result.TargetEpic)
		if err != nil {
			return result, err
		}
		if action := chooseNextAction(priorityQueues, "priority_epic"); action != nil {
			result.Decision = "act"
			result.Reason = fmt.Sprintf("Priority epic %s still has actionable work", result.TargetEpic)
			result.Next = action
			return result, nil
		}
	}

	if action := chooseNextAction(globalQueues, "backlog"); action != nil {
		result.Decision = "act"
		result.Reason = reasonForAction(action)
		result.Next = action
		return result, nil
	}

	total := result.Counts.Ready + result.Counts.Rejected + result.Counts.Delivered +
		result.Counts.InProgress + result.Counts.Blocked + result.Counts.Other

	switch {
	case total == 0:
		result.Decision = "complete"
		result.Reason = "All work complete"
	case result.Counts.InProgress > 0:
		result.Decision = "wait"
		result.Reason = "Only in-progress work remains"
	case result.Counts.Blocked > 0 && result.Counts.Other == 0:
		result.Decision = "blocked"
		result.Reason = "All remaining work is blocked"
	case result.Counts.Other > 0:
		result.Decision = "other"
		result.Reason = "Only non-dispatcher workflow states remain"
	default:
		result.Decision = "wait"
		result.Reason = "No actionable work selected"
	}

	return result, nil
}

func queryQueues(projectRoot, parent string) (queueSnapshot, error) {
	var (
		snapshot queueSnapshot
		filters  []string
	)

	if parent != "" {
		filters = append(filters, "--parent", parent)
	}

	delivered, err := QueryDelivered(projectRoot, filters...)
	if err != nil {
		return snapshot, fmt.Errorf("query delivered queue: %w", err)
	}
	rejected, err := QueryRejected(projectRoot, filters...)
	if err != nil {
		return snapshot, fmt.Errorf("query rejected queue: %w", err)
	}
	ready, err := QueryReady(projectRoot, filters...)
	if err != nil {
		return snapshot, fmt.Errorf("query ready queue: %w", err)
	}

	snapshot.Delivered = delivered
	snapshot.Rejected = rejected
	snapshot.Ready = ready
	return snapshot, nil
}

func chooseNextAction(queues queueSnapshot, scope string) *NextAction {
	if len(queues.Delivered) > 0 {
		issue := queues.Delivered[0]
		return &NextAction{
			Kind:     "pm_review",
			Role:     "pm_acceptor",
			StoryID:  issue.ID,
			Story:    issue.Title,
			Queue:    "delivered",
			Scope:    scope,
			HardTDD:  hasLabel(issue.Labels, "hard-tdd"),
			Priority: "1",
		}
	}

	if len(queues.Rejected) > 0 {
		issue := queues.Rejected[0]
		phase := "normal"
		if hasLabel(issue.Labels, "hard-tdd") {
			phase = "rework"
		}
		return &NextAction{
			Kind:     "developer_rework",
			Role:     "developer",
			StoryID:  issue.ID,
			Story:    issue.Title,
			Queue:    "rejected",
			Scope:    scope,
			HardTDD:  hasLabel(issue.Labels, "hard-tdd"),
			Phase:    phase,
			Priority: "2",
		}
	}

	if len(queues.Ready) > 0 {
		issue := queues.Ready[0]
		phase := "normal"
		if hasLabel(issue.Labels, "hard-tdd") {
			phase = "red"
		}
		return &NextAction{
			Kind:     "developer_new",
			Role:     "developer",
			StoryID:  issue.ID,
			Story:    issue.Title,
			Queue:    "ready",
			Scope:    scope,
			HardTDD:  hasLabel(issue.Labels, "hard-tdd"),
			Phase:    phase,
			Priority: "3",
		}
	}

	return nil
}

func reasonForAction(action *NextAction) string {
	switch action.Queue {
	case "delivered":
		return "Delivered work needs PM review before new execution"
	case "rejected":
		return "Rejected work must be repaired before new ready work"
	case "ready":
		return "Ready work is available for developer execution"
	default:
		return "Actionable work remains"
	}
}
