package guard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paivot-ai/pvg/internal/dispatcher"
	"github.com/paivot-ai/pvg/internal/loop"
	"github.com/paivot-ai/pvg/internal/ndvault"
)

var gitIntegrationRe = regexp.MustCompile(`(?:^|[;&|]\s*)(?:\S*/)?git\s+(merge|pull|rebase|cherry-pick)\b`)
var storyRefRe = regexp.MustCompile(`(?:^|[\s"'=])(?:refs/(?:remotes/origin|heads)/|origin/)?story/([A-Za-z0-9._-]+)`)

const mergeGateBlockMsg = "BLOCKED: Cannot merge story branch before PM-Acceptor completion.\n\n" +
	"Story %s must be both labeled 'accepted' and closed in nd before merge.\n" +
	"The merge gate requires PM-Acceptor to fully finish review first.\n\n" +
	"Workflow: Developer marks delivered -> PM-Acceptor reviews -> PM-Acceptor closes story -> PM-Acceptor adds 'accepted' label -> Dispatcher merges.\n\n" +
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

	storyIDs := parseStoryRefs(command)
	if len(storyIDs) == 0 {
		return Result{Allowed: true}
	}

	for _, storyID := range storyIDs {
		labels := ReadIssueLabels(projectRoot, storyID)
		if labels == nil {
			// No matching nd issue: likely not a Paivot-managed story branch.
			continue
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

		if targetBranch, ok := currentBranch(projectRoot); ok && !strings.HasPrefix(targetBranch, "epic/") {
			return Result{
				Allowed: false,
				Reason: fmt.Sprintf(
					"BLOCKED: story branches may only merge into epic branches.\n\nCurrent branch: %s\nAttempted story branch: story/%s\n\nCheckout epic/%s before merging the accepted story branch.",
					targetBranch, storyID, storyID,
				),
			}
		}
	}

	return Result{
		Allowed: true,
	}
}

func parseStoryRefs(command string) []string {
	if !gitIntegrationRe.MatchString(command) {
		return nil
	}

	var storyIDs []string
	seen := make(map[string]bool)
	for _, match := range storyRefRe.FindAllStringSubmatch(command, -1) {
		if len(match) < 2 {
			continue
		}
		storyID := strings.TrimSpace(match[1])
		if storyID == "" || seen[storyID] {
			continue
		}
		seen[storyID] = true
		storyIDs = append(storyIDs, storyID)
	}
	return storyIDs
}

func mergeGateEnabled(projectRoot string) bool {
	if isLoopActiveFrom(projectRoot) {
		return true
	}

	state, _, err := dispatcher.ReadStateRoot(projectRoot)
	if err == nil && state.Enabled {
		return true
	}

	_, root, found := findAncestorPath(projectRoot, filepath.Join(".vault", "knowledge", ".settings.yaml"))
	return found && root != ""
}

// ReadIssueLabels reads labels from an nd issue's frontmatter.
// Returns nil on any error (fail-open). Returns empty slice if no labels.
func ReadIssueLabels(projectRoot, issueID string) []string {
	path, err := issuePath(projectRoot, issueID)
	if err != nil {
		return nil
	}
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

func issuePath(projectRoot, issueID string) (string, error) {
	vaultDir, err := ndvault.Resolve(projectRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(vaultDir, "issues", issueID+".md"), nil
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

func currentBranch(projectRoot string) (string, bool) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", false
	}
	return branch, true
}

func isLoopActiveFrom(projectRoot string) bool {
	path, root, found := findAncestorPath(projectRoot, filepath.Join(".vault", loop.StateFileName()))
	if !found || root == "" || path == "" {
		return false
	}
	return loop.IsActive(root)
}

func findAncestorPath(start, rel string) (string, string, bool) {
	dir := filepath.Clean(start)
	for {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", false
}
