package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paivot-ai/pvg/internal/ndvault"
	"github.com/paivot-ai/pvg/internal/settings"
)

// WorkflowConfig holds parsed FSM settings.
type WorkflowConfig struct {
	Enabled   bool
	Sequence  []string
	ExitRules map[string][]string // source status -> allowed targets
}

// ParseWorkflowConfig extracts FSM config from a settings map.
func ParseWorkflowConfig(s map[string]string) WorkflowConfig {
	wc := WorkflowConfig{
		ExitRules: make(map[string][]string),
	}

	wc.Enabled = s["workflow.fsm"] == "true"

	seq := s["workflow.sequence"]
	if seq == "" {
		seq = settings.Default("workflow.sequence")
	}
	if seq != "" {
		for _, status := range strings.Split(seq, ",") {
			if t := strings.TrimSpace(status); t != "" {
				wc.Sequence = append(wc.Sequence, t)
			}
		}
	}

	rules := s["workflow.exit_rules"]
	if rules == "" {
		rules = settings.Default("workflow.exit_rules")
	}
	if rules != "" {
		// Format: "blocked:open,in_progress;rejected:in_progress"
		for _, rule := range strings.Split(rules, ";") {
			parts := strings.SplitN(rule, ":", 2)
			if len(parts) != 2 {
				continue
			}
			source := strings.TrimSpace(parts[0])
			if source == "" {
				continue
			}
			var targets []string
			for _, t := range strings.Split(parts[1], ",") {
				if trimmed := strings.TrimSpace(t); trimmed != "" {
					targets = append(targets, trimmed)
				}
			}
			if len(targets) > 0 {
				wc.ExitRules[source] = targets
			}
		}
	}

	return wc
}

// ValidateTransition checks if a status transition is allowed.
func ValidateTransition(wc WorkflowConfig, issueID, currentStatus, newStatus string) Result {
	// Same status is always a no-op
	if currentStatus == newStatus {
		return Result{Allowed: true}
	}

	// Exit rules: if source status has an exit rule, target must be in its allowed list
	if targets, ok := wc.ExitRules[currentStatus]; ok {
		for _, t := range targets {
			if t == newStatus {
				return Result{Allowed: true}
			}
		}
		return Result{
			Allowed: false,
			Reason: fmt.Sprintf(
				"FSM: cannot transition %s from '%s' to '%s'.\nAllowed next: %s.\nExit rule for '%s' restricts transitions.",
				issueID, currentStatus, newStatus, formatList(targets), currentStatus),
		}
	}

	// Find positions in sequence
	currentIdx := indexOf(wc.Sequence, currentStatus)
	newIdx := indexOf(wc.Sequence, newStatus)

	// Off-sequence: if either status is not in the sequence, allow (escape hatch)
	if currentIdx == -1 || newIdx == -1 {
		return Result{Allowed: true}
	}

	// Backward: any earlier step is always allowed
	if newIdx < currentIdx {
		return Result{Allowed: true}
	}

	// Forward: exactly one step
	if newIdx == currentIdx+1 {
		return Result{Allowed: true}
	}

	// Forward skip: blocked
	var allowed []string
	if currentIdx+1 < len(wc.Sequence) {
		allowed = []string{wc.Sequence[currentIdx+1]}
	}
	// Backward steps are also valid
	for i := 0; i < currentIdx; i++ {
		allowed = append(allowed, wc.Sequence[i])
	}
	return Result{
		Allowed: false,
		Reason: fmt.Sprintf(
			"FSM: cannot transition %s from '%s' to '%s'.\nAllowed next: %s.\nSequence: %s",
			issueID, currentStatus, newStatus, formatList(allowed),
			strings.Join(wc.Sequence, " -> ")),
	}
}

// ndUpdateRe matches: nd [global-flags] update <id> --status=<val> or --status <val>
// Global flags like --vault <val> may appear between nd and the subcommand.
var ndUpdateRe = regexp.MustCompile(`(?:^|[;&|]\s*)(?:\S*/)?nd\s+(?:--\S+\s+\S+\s+)*update\s+(\S+)\s+.*?--status[= ](\S+)`)

// ndCloseRe matches: nd [global-flags] close <id> [<id>...]
var ndCloseRe = regexp.MustCompile(`(?:^|[;&|]\s*)(?:\S*/)?nd\s+(?:--\S+\s+\S+\s+)*close\s+(.+?)(?:\s*[;&|]|$)`)

// ndLabelsAddRe matches: nd [global-flags] labels add <id> <label> [label...]
var ndLabelsAddRe = regexp.MustCompile(`(?:^|[;&|]\s*)(?:\S*/)?nd\s+(?:--\S+\s+\S+\s+)*labels\s+add\s+(\S+)\s+(.+?)(?:\s*[;&|]|$)`)

// ndUpdateAddLabelRe matches: nd [global-flags] update <id> ... --add-label=<label> or --add-label <label>
var ndUpdateAddLabelRe = regexp.MustCompile(`(?:^|[;&|]\s*)(?:\S*/)?nd\s+(?:--\S+\s+\S+\s+)*update\s+(\S+)\s+.*?--add-label(?:=| )(\S+)`)

// parseNdStatusChange extracts issue IDs and new status from an nd command.
// Returns multiple IDs for "nd close id1 id2 ...".
func parseNdStatusChange(command string) (ids []string, newStatus string, found bool) {
	// Check for nd update with --status
	if matches := ndUpdateRe.FindStringSubmatch(command); len(matches) == 3 {
		id := strings.Trim(matches[1], `"'`)
		status := strings.Trim(matches[2], `"'`)
		return []string{id}, status, true
	}

	// Check for nd close
	if matches := ndCloseRe.FindStringSubmatch(command); len(matches) == 2 {
		raw := strings.TrimSpace(matches[1])
		var closeIDs []string
		for _, part := range strings.Fields(raw) {
			part = strings.Trim(part, `"'`)
			// Skip flags
			if strings.HasPrefix(part, "-") {
				continue
			}
			if part != "" {
				closeIDs = append(closeIDs, part)
			}
		}
		if len(closeIDs) > 0 {
			return closeIDs, "closed", true
		}
	}

	return nil, "", false
}

func parseNdContractLabelAdd(command string) (issueID string, labels []string, found bool) {
	if matches := ndUpdateAddLabelRe.FindStringSubmatch(command); len(matches) == 3 {
		id := strings.Trim(matches[1], `"'`)
		label := strings.Trim(matches[2], `"'`)
		if id != "" && label != "" {
			return id, strings.Split(label, ","), true
		}
	}

	if matches := ndLabelsAddRe.FindStringSubmatch(command); len(matches) == 3 {
		id := strings.Trim(matches[1], `"'`)
		if id == "" {
			return "", nil, false
		}
		for _, part := range strings.Fields(matches[2]) {
			label := strings.Trim(part, `"'`)
			if label != "" && !strings.HasPrefix(label, "-") {
				labels = append(labels, label)
			}
		}
		if len(labels) > 0 {
			return id, labels, true
		}
	}

	return "", nil, false
}

// ReadIssueStatus reads the status from an nd issue's frontmatter.
// Returns "" on any error (fail-open).
func ReadIssueStatus(projectRoot, issueID string) string {
	vaultDir, err := ndvault.Resolve(projectRoot)
	if err != nil {
		return ""
	}
	path := filepath.Join(vaultDir, "issues", issueID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	content := string(data)
	// Look for frontmatter between --- markers
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return ""
	}
	frontmatter := content[3 : 3+end]

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "status:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "status:"))
		}
	}
	return ""
}

const settingsPath = ".vault/knowledge/.settings.yaml"

// CheckFSM is the entry point for FSM validation, called from guard.Check().
func CheckFSM(projectRoot, command string) Result {
	if projectRoot == "" {
		return Result{Allowed: true}
	}

	path := filepath.Join(projectRoot, settingsPath)
	s := settings.LoadFile(path)

	wc := ParseWorkflowConfig(s)
	if !wc.Enabled {
		return Result{Allowed: true}
	}

	ids, newStatus, found := parseNdStatusChange(command)
	if found {
		for _, id := range ids {
			currentStatus := ReadIssueStatus(projectRoot, id)
			if currentStatus == "" {
				// Fail-open: can't read current status
				continue
			}
			if r := ValidateTransition(wc, id, currentStatus, newStatus); !r.Allowed {
				return r
			}
		}
	}

	if issueID, labels, found := parseNdContractLabelAdd(command); found {
		currentStatus := ReadIssueStatus(projectRoot, issueID)
		if currentStatus == "" {
			return Result{Allowed: true}
		}
		for _, label := range labels {
			if r := validateContractLabel(issueID, currentStatus, label); !r.Allowed {
				return r
			}
		}
	}

	return Result{Allowed: true}
}

func validateContractLabel(issueID, currentStatus, label string) Result {
	switch label {
	case "delivered":
		if currentStatus != "in_progress" {
			return Result{
				Allowed: false,
				Reason: fmt.Sprintf(
					"Contract: cannot mark %s as delivered while nd status is '%s'.\nRequired nd status: 'in_progress'.",
					issueID, currentStatus),
			}
		}
	case "accepted":
		if currentStatus != "closed" {
			return Result{
				Allowed: false,
				Reason: fmt.Sprintf(
					"Contract: cannot mark %s as accepted while nd status is '%s'.\nRequired nd status: 'closed'.",
					issueID, currentStatus),
			}
		}
	case "rejected":
		if currentStatus != "open" {
			return Result{
				Allowed: false,
				Reason: fmt.Sprintf(
					"Contract: cannot mark %s as rejected while nd status is '%s'.\nRequired nd status: 'open'.",
					issueID, currentStatus),
			}
		}
	}
	return Result{Allowed: true}
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

func formatList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "'" + item + "'"
	}
	return strings.Join(quoted, ", ")
}
