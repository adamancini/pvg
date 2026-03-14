package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RamXX/vlt"

	"github.com/paivot-ai/pvg/internal/dispatcher"
	"github.com/paivot-ai/pvg/internal/vaultcfg"
)

// SessionEnd appends a session log entry to the project index note.
// Fire-and-forget: always returns nil (never blocks session end).
func SessionEnd() error {
	// 1. Parse hook input
	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		input.CWD, _ = os.Getwd()
	}
	if input.CWD == "" {
		input.CWD, _ = os.Getwd()
	}

	// 2. Clear dispatcher state (prevent stale state across sessions)
	_ = dispatcher.Off(input.CWD)

	// 3. Detect project name
	project := detectProject(input.CWD)
	today := time.Now().Format("2006-01-02")

	// 4. Collect notes created this session
	links := collectSessionLinks(input.CWD, project, today)
	entry := formatSessionEntry(today, links)

	// 5. Try vlt first
	v, err := vaultcfg.OpenVault()
	if err == nil {
		unlock := func() {}
		if lock, lockErr := vlt.LockVault(v.Dir(), true); lockErr == nil {
			unlock = lock
		}
		defer unlock()
		_ = v.Append(project, entry, false)
		return nil
	}

	// 6. Fallback to direct file ops
	vaultDir, derr := vaultcfg.VaultDir()
	if derr != nil {
		return nil // silently skip
	}

	projectNote := ""
	candidates := []string{
		filepath.Join(vaultDir, "projects", project+".md"),
		filepath.Join(vaultDir, project+".md"),
	}
	for _, c := range candidates {
		if _, serr := os.Stat(c); serr == nil {
			projectNote = c
			break
		}
	}

	if projectNote != "" {
		f, ferr := os.OpenFile(projectNote, os.O_APPEND|os.O_WRONLY, 0644)
		if ferr == nil {
			_, _ = f.WriteString(entry)
			_ = f.Close()
		}
	}

	return nil
}

// collectSessionLinks finds vault notes created today for this project.
// Returns a list of note titles suitable for wikilinks.
func collectSessionLinks(cwd, project, today string) []string {
	var titles []string
	seen := map[string]bool{}

	// 1. Scan project vault for files modified today
	knowledgeDir := filepath.Join(cwd, ".vault", "knowledge")
	if info, err := os.Stat(knowledgeDir); err == nil && info.IsDir() {
		_ = filepath.Walk(knowledgeDir, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || !strings.HasSuffix(fi.Name(), ".md") {
				return nil
			}
			if fi.ModTime().Format("2006-01-02") == today {
				title := strings.TrimSuffix(fi.Name(), ".md")
				if !seen[title] {
					seen[title] = true
					titles = append(titles, title)
				}
			}
			return nil
		})
	}

	// 2. Search global vault for notes created today for this project
	v, err := vaultcfg.OpenVault()
	if err != nil {
		return titles
	}
	results, err := v.Search(vlt.SearchOptions{
		Query: buildSessionSearchQuery(project, today),
	})
	if err != nil {
		return titles
	}
	for _, r := range results {
		if !seen[r.Title] {
			seen[r.Title] = true
			titles = append(titles, r.Title)
		}
	}

	return titles
}

func buildSessionSearchQuery(project, today string) string {
	return fmt.Sprintf("[project:%s] [created:%s]", project, today)
}

// formatSessionEntry builds the session log text with optional wikilinks.
func formatSessionEntry(today string, links []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n\n## Session log (%s)\n- Session ended normally\n", today)
	if len(links) > 0 {
		sb.WriteString("- Notes created: ")
		for i, title := range links {
			if i > 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "[[%s]]", title)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
