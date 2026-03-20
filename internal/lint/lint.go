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

// CheckArtifactCollisions scans all non-closed issues in the nd vault for
// PRODUCES blocks and reports any artifact claimed by more than one story.
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
	storiesScanned := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(issuesDir, entry.Name())
		id, status, produces, err := parseIssueFile(path)
		if err != nil {
			continue // skip unparseable files
		}

		// Skip closed issues and epics (no PRODUCES)
		if strings.EqualFold(status, "closed") {
			continue
		}

		storiesScanned++

		for _, artifact := range produces {
			normalized := normalizeArtifact(artifact)
			if normalized == "" {
				continue
			}
			artifactMap[normalized] = append(artifactMap[normalized], ArtifactEntry{
				Artifact: artifact,
				StoryID:  id,
				Status:   status,
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
		if len(uniqueStories) > 1 {
			collisions = append(collisions, Collision{
				Artifact: entries[0].Artifact,
				Stories:  dedupByStory(entries),
			})
		}
	}

	return LintResult{
		Collisions: collisions,
		Total:      len(artifactMap),
		Stories:    storiesScanned,
		Passed:     len(collisions) == 0,
	}, nil
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
// status, and PRODUCES entries from it.
func parseIssueFile(path string) (id, status string, produces []string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	frontmatterDone := false
	inProducesBlock := false

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
						id = kv[1]
					case "status":
						status = kv[1]
					}
				}
			}
			continue
		}

		// Parse body for PRODUCES blocks
		trimmed := strings.TrimSpace(line)

		if strings.EqualFold(trimmed, "PRODUCES:") || strings.HasPrefix(strings.ToUpper(trimmed), "PRODUCES:") {
			inProducesBlock = true
			continue
		}

		// End of PRODUCES block: blank line, another section header, or CONSUMES
		if inProducesBlock {
			if trimmed == "" ||
				strings.HasPrefix(strings.ToUpper(trimmed), "CONSUMES:") ||
				(len(trimmed) > 0 && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, " ")) {
				inProducesBlock = false
				continue
			}

			if strings.HasPrefix(trimmed, "-") {
				entry := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
				if entry != "" {
					produces = append(produces, entry)
				}
			}
		}
	}

	// Fallback: derive ID from filename
	if id == "" {
		base := filepath.Base(path)
		id = strings.TrimSuffix(base, ".md")
	}

	return id, status, produces, scanner.Err()
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
