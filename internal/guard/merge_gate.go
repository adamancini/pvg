package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paivot-ai/pvg/internal/dispatcher"
	"github.com/paivot-ai/pvg/internal/loop"
)

// storyMergeRe matches: git merge [flags] origin/story/STORY_ID or story/STORY_ID
// Captures the story ID from the branch name.
var storyMergeRe = regexp.MustCompile(`git\s+merge\s+[^;|&]*?(?:origin/)?story/(\S+)`)

const mergeGateBlockMsg = "BLOCKED: Cannot merge story branch before PM-Acceptor completion.\n\n" +
	"Story %s must be both labeled 'accepted' and closed in nd before merge.\n" +
	"The merge gate requires PM-Acceptor to fully finish review first.\n\n" +
	"Workflow: Developer marks delivered -> PM-Acceptor reviews -> PM-Acceptor adds 'accepted' label -> PM-Acceptor closes story -> Dispatcher merges.\n\n" +
	"To proceed:\n" +
	"  1. Ensure the story has the 'delivered' label\n" +
	"  2. Spawn paivot-graph:pm agent to review the story\n" +
	"  3. PM-Acceptor will add 'accepted' and close the story\n" +
	"  4. Then merge the story branch"

const projectSettingsPath = ".vault/knowledge/.settings.yaml"

// CheckMergeGate blocks git merge of story branches when the story has not been
// fully accepted by PM-Acceptor. Active for Paivot-managed repos, not just when
// dispatcher mode is currently enabled.
//
// Compatibility behavior:
// - If the repo does not look Paivot-managed, allow.
// - If the story issue cannot be found, allow.
// - Once an nd issue is found for the story, require accepted label + closed status.
func CheckMergeGate(projectRoot, command string) Result {
	if projectRoot == "" || command == "" {
		return Result{Allowed: true}
	}

	if !mergeGateEnabled(projectRoot) {
		return Result{Allowed: true}
	}

	matches := storyMergeRe.FindStringSubmatch(command)
	if len(matches) < 2 {
		return Result{Allowed: true}
	}

	storyID := matches[1]
	// Strip quotes if present
	storyID = strings.Trim(storyID, `"'`)

	// Remove any trailing flags (e.g., -m "message" part captured by \S+)
	// The story ID should be alphanumeric with hyphens only
	if idx := strings.IndexAny(storyID, " \t"); idx >= 0 {
		storyID = storyID[:idx]
	}

	labels := ReadIssueLabels(projectRoot, storyID)
	if labels == nil {
		// No matching nd issue: likely not a Paivot-managed story branch.
		return Result{Allowed: true}
	}

	hasAccepted := false
	for _, label := range labels {
		if label == "accepted" {
			hasAccepted = true
			break
		}
	}

	if !hasAccepted {
		return Result{
			Allowed: false,
			Reason:  fmt.Sprintf(mergeGateBlockMsg, storyID),
		}
	}

	if status := ReadIssueStatus(projectRoot, storyID); status != "closed" {
		return Result{
			Allowed: false,
			Reason:  fmt.Sprintf(mergeGateBlockMsg, storyID),
		}
	}

	return Result{
		Allowed: true,
	}
}

func mergeGateEnabled(projectRoot string) bool {
	if loop.IsActive(projectRoot) {
		return true
	}

	state, err := dispatcher.ReadState(projectRoot)
	if err == nil && state.Enabled {
		return true
	}

	_, err = os.Stat(filepath.Join(projectRoot, projectSettingsPath))
	return err == nil
}

// ReadIssueLabels reads labels from an nd issue's frontmatter.
// Returns nil on any error (fail-open). Returns empty slice if no labels.
func ReadIssueLabels(projectRoot, issueID string) []string {
	path := filepath.Join(projectRoot, ".vault", "issues", issueID+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return nil
	}
	end := strings.Index(content[3:], "---")
	if end == -1 {
		return nil
	}
	frontmatter := content[3 : 3+end]

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "labels:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "labels:"))
		return parseYAMLArray(value)
	}
	return []string{}
}

// parseYAMLArray parses a YAML inline array like [a, b, c] into a string slice.
func parseYAMLArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "[]" {
		return []string{}
	}

	// Strip brackets
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")

	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		// Strip quotes
		item = strings.Trim(item, `"'`)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}
