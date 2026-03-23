package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/paivot-ai/vlt"

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

	// 3. Detect project name (with timeout to avoid hanging on slow git)
	project := detectProjectTimeout(input.CWD)
	today := time.Now().Format("2006-01-02")

	// 4. Open vault once (reuse for both link collection and append)
	v, err := vaultcfg.OpenVault()
	if err != nil {
		// No vault -- collect local links only and write via direct file ops
		links := collectLocalLinks(input.CWD, today)
		entry := formatSessionEntry(today, links)
		writeSessionEntryDirect(input.CWD, project, entry)
		return nil
	}

	// 5. Collect links (local only -- skip vault search to stay within timeout)
	links := collectLocalLinks(input.CWD, today)
	entry := formatSessionEntry(today, links)

	// 6. Append to project note
	unlock := func() {}
	if lock, lockErr := vlt.LockVault(v.Dir(), true); lockErr == nil {
		unlock = lock
	}
	defer unlock()
	_ = v.Append(project, entry, false)
	return nil
}

// detectProjectTimeout is like detectProject but with a 3-second timeout
// on the git exec to avoid hanging during session end.
func detectProjectTimeout(cwd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", cwd, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err == nil {
		url := strings.TrimSpace(string(out))
		if url != "" {
			base := filepath.Base(url)
			return strings.TrimSuffix(base, ".git")
		}
	}
	return filepath.Base(cwd)
}

// collectLocalLinks scans project-local .vault/knowledge/ for files modified
// today. Unlike collectSessionLinks, it skips the global vault search to stay
// well within the hook timeout budget.
func collectLocalLinks(cwd, today string) []string {
	var titles []string
	knowledgeDir := filepath.Join(cwd, ".vault", "knowledge")
	if info, err := os.Stat(knowledgeDir); err == nil && info.IsDir() {
		_ = filepath.Walk(knowledgeDir, func(path string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || !strings.HasSuffix(fi.Name(), ".md") {
				return nil
			}
			if fi.ModTime().Format("2006-01-02") == today {
				titles = append(titles, strings.TrimSuffix(fi.Name(), ".md"))
			}
			return nil
		})
	}
	return titles
}

// writeSessionEntryDirect appends to the project note via direct file ops
// (no vault dependency). Used as fallback when the vault is unavailable.
func writeSessionEntryDirect(cwd, project, entry string) {
	vaultDir, err := vaultcfg.VaultDir()
	if err != nil {
		return
	}
	for _, c := range []string{
		filepath.Join(vaultDir, "projects", project+".md"),
		filepath.Join(vaultDir, project+".md"),
	} {
		if _, serr := os.Stat(c); serr == nil {
			if f, ferr := os.OpenFile(c, os.O_APPEND|os.O_WRONLY, 0644); ferr == nil {
				_, _ = f.WriteString(entry)
				_ = f.Close()
			}
			return
		}
	}
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
