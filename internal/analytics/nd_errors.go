package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// NdError captures a single nd command error for analysis.
type NdError struct {
	Timestamp string `json:"timestamp"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Stderr    string `json:"stderr"`
	Pattern   string `json:"pattern"` // Category: "invalid_flag", "duplicate_label", "already_closed", etc.
}

// LogNdError logs an nd command error to the JSONL file for later analysis.
// The log file is stored at .vault/.guard/nd_errors.jsonl relative to projectRoot.
// This allows us to batch collect nd hallucinations and make informed decisions about
// which nd enhancements would provide the most value.
func LogNdError(projectRoot, command string, exitCode int, stderr string) error {
	if projectRoot == "" {
		return fmt.Errorf("projectRoot is required")
	}

	// Determine the log directory
	logDir := filepath.Join(projectRoot, ".vault", ".guard")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		// Silent failure -- we don't want telemetry to break the user's workflow
		return nil
	}

	logFile := filepath.Join(logDir, "nd_errors.jsonl")

	// Detect the error pattern
	pattern := detectPattern(command, stderr)

	entry := NdError{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Command:   command,
		ExitCode:  exitCode,
		Stderr:    stderr,
		Pattern:   pattern,
	}

	// Append to JSONL (newline-delimited JSON)
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Silent failure
		return nil
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return nil
	}

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		return nil
	}

	return nil
}

// detectPattern categorizes the nd error for batch analysis.
func detectPattern(command, stderr string) string {
	// Pattern detection -- update as new hallucinations are discovered
	switch {
	case containsText(stderr, "invalid flag") || containsText(command, "--status"):
		return "invalid_flag"
	case containsText(stderr, "already exists"):
		return "duplicate_label"
	case containsText(stderr, "already closed"):
		return "already_closed"
	case containsText(stderr, "not found"):
		return "not_found"
	case containsText(stderr, "is not ready"):
		return "not_ready"
	default:
		return "unknown"
	}
}

func containsText(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr))
}
