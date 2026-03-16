// Package lifecycle implements memory interception hooks for syncing Claude's
// native memory operations with the vault.
package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paivot-ai/vlt"

	"github.com/paivot-ai/pvg/internal/vaultcfg"
)

// memoryToolInput matches Claude Code's hook input for Read/Write/Edit tools.
type memoryToolInput struct {
	ToolName  string            `json:"tool_name"`
	ToolInput memoryToolContent `json:"tool_input"`
	CWD       string            `json:"cwd"`
}

// memoryToolContent contains the actual tool parameters.
type memoryToolContent struct {
	FilePath  string `json:"file_path"`
	Content   string `json:"content"`
	NewString string `json:"new_string"`
}

// hookOutput is the structured response to Claude Code.
type hookOutput struct {
	SystemMessage string `json:"systemMessage"`
}

// MemoryRead intercepts Read operations on memory files and supplements with vault knowledge.
// Reads JSON from stdin, outputs structured response to stdout. Always exits 0 (fail-open).
func MemoryRead() error {
	return handleMemoryOperation("Read", func(input *memoryToolInput, vaultClient *vlt.Vault, projectName string) (string, error) {
		// Search vault for project-related knowledge
		searchQuery := projectName
		if strings.ContainsAny(projectName, " \t\"") {
			searchQuery = `"` + strings.ReplaceAll(projectName, `"`, `\"`) + `"`
		}

		results, err := vaultClient.Search(vlt.SearchOptions{Query: searchQuery})
		if err != nil || len(results) == 0 {
			// No vault results -- memory read proceeds without supplementation
			return "", nil
		}

		// Format search results as system message
		msg := fmt.Sprintf("[VAULT MEMORY] Vault knowledge related to project %q:\n\n", projectName)
		for _, r := range results {
			msg += fmt.Sprintf("- %s (%s)\n", r.Title, r.RelPath)
		}
		msg += "\nConsider this alongside the memory file contents."
		return msg, nil
	})
}

// MemoryWrite intercepts Write operations and mirrors full content to vault.
// Reads JSON from stdin, outputs structured response to stdout. Always exits 0 (fail-open).
func MemoryWrite() error {
	return handleMemoryOperation("Write", func(input *memoryToolInput, vaultClient *vlt.Vault, projectName string) (string, error) {
		content := input.ToolInput.Content
		if content == "" {
			return "", nil // nothing to mirror
		}

		mirrorNote := projectName + "-memory"
		bodyContent, fullContent := buildMemoryMirrorContent(projectName, content)

		// Try to write to existing note first (update mode)
		result, err := vaultClient.Read(mirrorNote, "")
		if err == nil && result.Content != "" {
			// Note exists -- replace body while preserving frontmatter.
			if err := vaultClient.Write(mirrorNote, bodyContent, true); err != nil {
				// Fail-open: log and continue
				fmt.Fprintf(os.Stderr, "pvg hook memory-write: failed to update vault note: %v\n", err)
			}
		} else {
			// Note doesn't exist -- create it
			if err := vaultClient.Create(mirrorNote, "_inbox/"+mirrorNote+".md", fullContent, true, true); err != nil {
				// Fail-open: log and continue
				fmt.Fprintf(os.Stderr, "pvg hook memory-write: failed to create vault note: %v\n", err)
			}
		}

		return "[VAULT MEMORY] Memory content mirrored to vault.", nil
	})
}

// MemoryEdit intercepts Edit operations and appends deltas to vault.
// Reads JSON from stdin, outputs structured response to stdout. Always exits 0 (fail-open).
func MemoryEdit() error {
	return handleMemoryOperation("Edit", func(input *memoryToolInput, vaultClient *vlt.Vault, projectName string) (string, error) {
		newString := input.ToolInput.NewString
		if newString == "" {
			return "", nil // nothing to append
		}

		mirrorNote := projectName + "-memory"
		now := time.Now()
		dateStr := now.Format("2006-01-02")
		timeStr := now.Format("15:04")

		// Format delta with timestamp
		delta := fmt.Sprintf("\n## Memory edit (%s %s)\n\n%s", dateStr, timeStr, newString)

		// Try to append to existing note
		result, err := vaultClient.Read(mirrorNote, "")
		if err == nil && result.Content != "" {
			// Note exists -- append delta
			if err := vaultClient.Append(mirrorNote, delta, true); err != nil {
				// Fail-open: log and continue
				fmt.Fprintf(os.Stderr, "pvg hook memory-edit: failed to append to vault note: %v\n", err)
			}
		} else {
			// Note doesn't exist -- create it with initial delta
			timestamp := now.Format("2006-01-02")
			frontmatter := fmt.Sprintf(`---
type: project
project: %s
status: active
created: %s
---

# %s Memory Mirror

Auto-synced from Claude native memory.

`, projectName, timestamp, projectName)

			fullContent := frontmatter + delta
			if err := vaultClient.Create(mirrorNote, "_inbox/"+mirrorNote+".md", fullContent, true, true); err != nil {
				// Fail-open: log and continue
				fmt.Fprintf(os.Stderr, "pvg hook memory-edit: failed to create vault note: %v\n", err)
			}
		}

		return "[VAULT MEMORY] Memory edit mirrored to vault.", nil
	})
}

// handleMemoryOperation is the common handler for all three memory operations.
// It:
//   - Parses JSON from stdin
//   - Fast-exits if not a memory path
//   - Opens vault (fail-open on vault unavailability)
//   - Detects project name
//   - Calls the operation-specific handler
//   - Outputs structured response to stdout
//   - Always exits 0
func handleMemoryOperation(expectedTool string, handler func(*memoryToolInput, *vlt.Vault, string) (string, error)) error {
	// Parse hook input
	var input memoryToolInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		// If parsing fails, exit gracefully (fail-open)
		return nil
	}

	// Fast-exit for non-memory paths
	if !isMemoryPath(input.ToolInput.FilePath) {
		return nil
	}

	// Get project name from CWD
	if input.CWD == "" {
		input.CWD, _ = os.Getwd()
	}
	projectName := detectProject(input.CWD)

	// Open vault (fail-open on vault unavailability)
	vaultClient, err := vaultcfg.OpenVault()
	if err != nil {
		// Vault not available -- allow operation to proceed without vault mirroring
		return nil
	}
	unlock := func() {}
	if lock, lockErr := vlt.LockVault(vaultClient.Dir(), expectedTool != "Read"); lockErr == nil {
		unlock = lock
	} else {
		fmt.Fprintf(os.Stderr, "pvg hook memory-%s: cannot lock vault: %v\n", strings.ToLower(expectedTool), lockErr)
	}
	defer unlock()

	// Call operation-specific handler
	msg, err := handler(&input, vaultClient, projectName)
	if err != nil {
		// Log error but continue (fail-open)
		fmt.Fprintf(os.Stderr, "pvg hook memory-%s: %v\n", strings.ToLower(expectedTool), err)
	}

	// Output response if there's a message to relay
	if msg != "" {
		response := hookOutput{SystemMessage: msg}
		if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
			fmt.Fprintf(os.Stderr, "pvg hook memory: failed to encode response: %v\n", err)
		}
	}

	return nil
}

func buildMemoryMirrorContent(projectName, content string) (bodyContent, fullContent string) {
	timestamp := time.Now().Format("2006-01-02")
	bodyContent = fmt.Sprintf(`# %s Memory Mirror

Auto-synced from Claude native memory.

%s`, projectName, content)
	fullContent = fmt.Sprintf(`---
type: project
project: %s
status: active
created: %s
---

%s`, projectName, timestamp, bodyContent)
	return bodyContent, fullContent
}

// isMemoryPath checks if a file path looks like a Claude memory file.
// Matches patterns like ~/.claude/*/memory/*
func isMemoryPath(filePath string) bool {
	// Expand ~ if present
	if strings.HasPrefix(filePath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			filePath = filepath.Join(home, filePath[1:])
		}
	}

	// Check if path contains .claude and memory
	return strings.Contains(filePath, ".claude") && strings.Contains(filePath, "memory")
}
