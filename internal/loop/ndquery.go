package loop

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/paivot-ai/pvg/internal/ndvault"
)

// WorkCounts holds the counts of issues in each state.
type WorkCounts struct {
	Ready      int
	Delivered  int
	InProgress int
	Blocked    int
	Other      int
}

// ndIssue matches the PascalCase JSON output of nd.
type ndIssue struct {
	ID     string   `json:"ID"`
	Status string   `json:"Status"`
	Labels []string `json:"Labels"`
	Type   string   `json:"Type"`
}

var execCommand = exec.Command

// QueryWorkCounts returns work counts across the live backlog.
//
// Even in epic mode, stop decisions stay backlog-wide so the loop can continue
// past a single epic instead of terminating when that epic empties.
func QueryWorkCounts(projectRoot, mode, targetEpic string) (WorkCounts, error) {
	return queryAllCounts(projectRoot)
}

// queryAllCounts uses nd subcommands to gather counts across the whole backlog.
func queryAllCounts(projectRoot string) (WorkCounts, error) {
	var wc WorkCounts
	readyOK := false
	ipOK := false
	blockedOK := false
	readyIssues, err := runND(projectRoot, "ready", "--json")
	if err == nil {
		wc.Ready = len(readyIssues)
		readyOK = true
	}

	// In-progress issues (includes delivered -- we separate below)
	ipIssues, err := runND(projectRoot, "list", "--status", "in_progress", "--json")
	if err == nil {
		ipOK = true
		for _, issue := range ipIssues {
			if hasLabel(issue.Labels, "delivered") {
				wc.Delivered++
			} else {
				wc.InProgress++
			}
		}
	}

	// Blocked issues
	blockedIssues, err := runND(projectRoot, "blocked", "--json")
	if err == nil {
		wc.Blocked = len(blockedIssues)
		blockedOK = true
	}

	allIssues, err := runND(projectRoot, "list", "--status", "!closed", "--json")
	if err == nil && readyOK && ipOK && blockedOK {
		wc.Other = countOtherIssues(readyIssues, ipIssues, blockedIssues, allIssues)
	}

	return wc, nil
}

// queryEpicCounts uses nd children to count work within a specific epic.
func queryEpicCounts(projectRoot, epicID string) (WorkCounts, error) {
	var wc WorkCounts

	issues, err := runND(projectRoot, "children", epicID, "--json")
	if err != nil {
		return wc, fmt.Errorf("query epic children: %w", err)
	}

	for _, issue := range issues {
		switch strings.ToLower(issue.Status) {
		case "ready":
			wc.Ready++
		case "in_progress":
			if hasLabel(issue.Labels, "delivered") {
				wc.Delivered++
			} else {
				wc.InProgress++
			}
		case "blocked":
			wc.Blocked++
		case "closed":
			// done issues are not counted
		default:
			wc.Other++
		}
	}

	return wc, nil
}

func countOtherIssues(readyIssues, ipIssues, blockedIssues, allIssues []ndIssue) int {
	known := make(map[string]bool, len(readyIssues)+len(ipIssues)+len(blockedIssues))
	for _, issue := range readyIssues {
		known[issue.ID] = true
	}
	for _, issue := range ipIssues {
		known[issue.ID] = true
	}
	for _, issue := range blockedIssues {
		known[issue.ID] = true
	}

	other := 0
	for _, issue := range allIssues {
		if issue.ID == "" || known[issue.ID] {
			continue
		}
		other++
	}
	return other
}

// ValidateEpic checks that an epic ID exists and is a valid epic.
func ValidateEpic(projectRoot, epicID string) error {
	issues, err := runND(projectRoot, "show", epicID, "--json")
	if err != nil {
		return fmt.Errorf("epic %s not found: %w", epicID, err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("epic %s not found", epicID)
	}
	issue := issues[0]
	if !strings.EqualFold(issue.Type, "epic") {
		return fmt.Errorf("%s is not an epic (type: %s)", epicID, issue.Type)
	}
	return nil
}

// runND executes an nd command and parses JSON output.
// Returns empty slice (not error) when nd outputs nothing.
func runND(projectRoot string, args ...string) ([]ndIssue, error) {
	vaultDir, err := ndvault.Resolve(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve nd vault: %w", err)
	}

	ndArgs := append([]string{"--vault", vaultDir}, args...)
	cmd := execCommand("nd", ndArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nd %s: %w", strings.Join(ndArgs, " "), err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		return nil, nil
	}

	// nd may return a single object or an array
	var issues []ndIssue
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &issues); err != nil {
			return nil, fmt.Errorf("parse nd output: %w", err)
		}
	} else {
		var single ndIssue
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return nil, fmt.Errorf("parse nd output: %w", err)
		}
		issues = []ndIssue{single}
	}

	return issues, nil
}

// QueryInProgress returns all in-progress issues from nd.
func QueryInProgress(projectRoot string) ([]ndIssue, error) {
	return runND(projectRoot, "list", "--status", "in_progress", "--json")
}

// hasLabel checks if a label exists in a slice (case-insensitive).
func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if strings.EqualFold(l, target) {
			return true
		}
	}
	return false
}
