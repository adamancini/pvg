// Package lint provides deterministic backlog quality checks that run
// before Anchor review. These are structural gates, not advisory.
package lint

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ArtifactEntry represents one PRODUCES declaration in a story.
type ArtifactEntry struct {
	Artifact string `json:"artifact"`
	StoryID  string `json:"story_id"`
	Status   string `json:"status"`
}

// Collision represents an artifact claimed by multiple stories.
type Collision struct {
	Artifact string          `json:"artifact"`
	Stories  []ArtifactEntry `json:"stories"`
}

// LintResult holds the results of all lint checks.
type LintResult struct {
	Collisions []Collision `json:"collisions"`
	Total      int         `json:"total_artifacts"`
	Stories    int         `json:"stories_scanned"`
	Passed     bool        `json:"passed"`
}

// IssueData holds parsed fields from an nd issue file.
type IssueData struct {
	ID          string
	Status      string
	Produces    []string
	BlockedBy   []string // from blocked_by and was_blocked_by frontmatter
	ConsumeRefs []string // upstream story IDs from CONSUMES entries
}

// CheckArtifactCollisions scans all non-closed issues in the nd vault for
// PRODUCES blocks and reports any artifact claimed by more than one story,
// excluding sequential modification chains (where one story is blocked_by
// or CONSUMES from the other).
func CheckArtifactCollisions(vaultDir string) (LintResult, error) {
	issuesDir := filepath.Join(vaultDir, "issues")
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return LintResult{Passed: true}, nil
		}
		return LintResult{}, fmt.Errorf("read issues dir: %w", err)
	}

	// artifact path -> list of entries
	artifactMap := make(map[string][]ArtifactEntry)
	// story ID -> parsed issue data (for chain detection)
	issueMap := make(map[string]IssueData)
	storiesScanned := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(issuesDir, entry.Name())
		data, err := parseIssueFile(path)
		if err != nil {
			continue // skip unparseable files
		}

		// Skip closed issues and epics (no PRODUCES)
		if strings.EqualFold(data.Status, "closed") {
			continue
		}

		storiesScanned++
		issueMap[data.ID] = data

		for _, artifact := range data.Produces {
			normalized := normalizeArtifact(artifact)
			if normalized == "" {
				continue
			}
			artifactMap[normalized] = append(artifactMap[normalized], ArtifactEntry{
				Artifact: artifact,
				StoryID:  data.ID,
				Status:   data.Status,
			})
		}
	}

	var collisions []Collision
	for _, entries := range artifactMap {
		// Only flag when DIFFERENT stories produce the same file.
		// One story producing multiple exports from the same file is fine.
		uniqueStories := make(map[string]bool)
		for _, e := range entries {
			uniqueStories[e.StoryID] = true
		}
		if len(uniqueStories) <= 1 {
			continue
		}

		// Check if all producing stories form a dependency chain.
		// If Story B is blocked_by Story A or CONSUMES from Story A,
		// they can both PRODUCE the same file (sequential modification).
		storyIDs := make([]string, 0, len(uniqueStories))
		for id := range uniqueStories {
			storyIDs = append(storyIDs, id)
		}

		if allChained(storyIDs, issueMap) {
			continue // sequential modification chain, not a collision
		}

		collisions = append(collisions, Collision{
			Artifact: entries[0].Artifact,
			Stories:  dedupByStory(entries),
		})
	}

	return LintResult{
		Collisions: collisions,
		Total:      len(artifactMap),
		Stories:    storiesScanned,
		Passed:     len(collisions) == 0,
	}, nil
}

// allChained returns true if every pair of stories can be ordered by
// transitive dependency (blocked_by or CONSUMES), meaning they form
// a sequential modification chain on the same file.
func allChained(storyIDs []string, allIssues map[string]IssueData) bool {
	// Compute transitive reachability for each story in the group,
	// walking the full dependency graph (not just the artifact group).
	reachable := make(map[string]map[string]bool, len(storyIDs))
	for _, id := range storyIDs {
		reachable[id] = transitiveReach(id, allIssues)
	}

	// Every pair must be ordered: one transitively depends on the other.
	for i := 0; i < len(storyIDs); i++ {
		for j := i + 1; j < len(storyIDs); j++ {
			a, b := storyIDs[i], storyIDs[j]
			if !reachable[a][b] && !reachable[b][a] {
				return false
			}
		}
	}
	return true
}

// transitiveReach returns the set of all story IDs that id transitively
// depends on (via blocked_by, was_blocked_by, or CONSUMES references).
func transitiveReach(id string, allIssues map[string]IssueData) map[string]bool {
	reached := make(map[string]bool)
	var visit func(string)
	visit = func(current string) {
		data, ok := allIssues[current]
		if !ok {
			return
		}
		for _, dep := range data.BlockedBy {
			if !reached[dep] {
				reached[dep] = true
				visit(dep)
			}
		}
		for _, ref := range data.ConsumeRefs {
			if !reached[ref] {
				reached[ref] = true
				visit(ref)
			}
		}
	}
	visit(id)
	return reached
}

// FormatText returns a human-readable lint report.
func FormatText(r LintResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[LINT] Scanned %d stories, %d unique artifacts\n", r.Stories, r.Total)
	if r.Passed {
		b.WriteString("[LINT] PASSED: no artifact collisions found\n")
		return b.String()
	}
	fmt.Fprintf(&b, "[LINT] FAILED: %d artifact collision(s)\n", len(r.Collisions))
	for _, c := range r.Collisions {
		fmt.Fprintf(&b, "\n  Artifact: %s\n", c.Artifact)
		fmt.Fprintf(&b, "  Claimed by:\n")
		for _, s := range c.Stories {
			fmt.Fprintf(&b, "    - %s (%s)\n", s.StoryID, s.Status)
		}
	}
	return b.String()
}

// FormatJSON returns the lint result as indented JSON.
func FormatJSON(r LintResult) (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseIssueFile reads an nd issue markdown file and extracts the ID,
// status, PRODUCES entries, blocked_by dependencies, and CONSUMES refs.
func parseIssueFile(path string) (IssueData, error) {
	var data IssueData

	f, err := os.Open(path)
	if err != nil {
		return data, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false
	inProducesBlock := false
	inConsumesBlock := false

	for scanner.Scan() {
		line := scanner.Text()

		// Parse frontmatter
		if !frontmatterDone {
			if line == "---" {
				if inFrontmatter {
					frontmatterDone = true
				} else {
					inFrontmatter = true
				}
				continue
			}
			if inFrontmatter {
				if kv := parseFrontmatterLine(line); kv != nil {
					switch kv[0] {
					case "id":
						data.ID = kv[1]
					case "status":
						data.Status = kv[1]
					case "blocked_by", "was_blocked_by":
						data.BlockedBy = append(data.BlockedBy, parseYAMLList(kv[1])...)
					}
				}
			}
			continue
		}

		// Parse body for PRODUCES and CONSUMES blocks
		trimmed := strings.TrimSpace(line)

		if strings.EqualFold(trimmed, "PRODUCES:") || strings.HasPrefix(strings.ToUpper(trimmed), "PRODUCES:") {
			inProducesBlock = true
			inConsumesBlock = false
			continue
		}

		if strings.EqualFold(trimmed, "CONSUMES:") || strings.HasPrefix(strings.ToUpper(trimmed), "CONSUMES:") {
			inConsumesBlock = true
			inProducesBlock = false
			continue
		}

		if inProducesBlock {
			if trimmed == "" ||
				(len(trimmed) > 0 && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, " ")) {
				inProducesBlock = false
				continue
			}

			if strings.HasPrefix(trimmed, "-") {
				entry := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
				if entry != "" {
					data.Produces = append(data.Produces, entry)
				}
			}
		}

		if inConsumesBlock {
			if trimmed == "" ||
				(len(trimmed) > 0 && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, " ")) {
				inConsumesBlock = false
				continue
			}

			if strings.HasPrefix(trimmed, "-") {
				entry := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
				if ref := extractConsumeRef(entry); ref != "" {
					data.ConsumeRefs = append(data.ConsumeRefs, ref)
				}
			}
		}
	}

	// Fallback: derive ID from filename
	if data.ID == "" {
		base := filepath.Base(path)
		data.ID = strings.TrimSuffix(base, ".md")
	}

	return data, scanner.Err()
}

// parseFrontmatterLine extracts key-value from "key: value" lines.
func parseFrontmatterLine(line string) []string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	return []string{key, value}
}

// parseYAMLList parses "[item1, item2]" into a string slice.
func parseYAMLList(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil
	}
	s = s[1 : len(s)-1] // strip brackets
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// extractConsumeRef extracts the upstream story ID from a CONSUMES entry.
// Format: "PROJ-abc: file -> function()" returns "PROJ-abc".
// Entries starting with "(" like "(existing)" or "(none)" return "".
func extractConsumeRef(entry string) string {
	entry = strings.TrimSpace(entry)
	if strings.HasPrefix(entry, "(") {
		return "" // skip (existing), (none -- leaf story), etc.
	}
	parts := strings.SplitN(entry, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	ref := strings.TrimSpace(parts[0])
	if ref == "" {
		return ""
	}
	return ref
}

// dedupByStory keeps one entry per story for collision reporting.
func dedupByStory(entries []ArtifactEntry) []ArtifactEntry {
	seen := make(map[string]bool)
	var deduped []ArtifactEntry
	for _, e := range entries {
		if !seen[e.StoryID] {
			seen[e.StoryID] = true
			deduped = append(deduped, e)
		}
	}
	return deduped
}

// normalizeArtifact extracts the file path from a PRODUCES entry,
// stripping the "-> function/type" suffix for collision comparison.
func normalizeArtifact(entry string) string {
	// Format: "src/auth.ts -> generateToken(userId: string): string"
	// We normalize to just the file path for collision detection.
	parts := strings.SplitN(entry, "->", 2)
	return strings.TrimSpace(parts[0])
}
