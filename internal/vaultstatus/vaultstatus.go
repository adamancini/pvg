// Package vaultstatus reports on the health and contents of the vault.
package vaultstatus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Status represents the health of a vault area.
type Status string

const (
	StatusOK    Status = "ok"
	StatusWarn  Status = "warn"
	StatusError Status = "error"
)

// Area describes one section of the vault (conventions, concepts, etc.).
type Area struct {
	Name     string
	Path     string
	Status   Status
	Notes    int
	Message  string
	Modified time.Time
}

// Report is the complete vault status.
type Report struct {
	VaultName  string
	VaultDir   string
	Areas      []Area
	Orphans    int
	TotalNotes int
	LastSync   time.Time
	Healthy    bool
	Messages   []string
}

// FormatText returns a human-readable report.
func FormatText(r Report) string {
	var sb strings.Builder

	sb.WriteString("Vault Status Report\n")
	sb.WriteString("===================\n\n")

	fmt.Fprintf(&sb, "Vault:     %s\n", r.VaultName)
	fmt.Fprintf(&sb, "Path:      %s\n", r.VaultDir)
	fmt.Fprintf(&sb, "Healthy:   %v\n", r.Healthy)
	if !r.LastSync.IsZero() {
		fmt.Fprintf(&sb, "Last sync: %s\n", r.LastSync.Format(time.RFC3339))
	} else {
		sb.WriteString("Last sync: (unknown)\n")
	}
	fmt.Fprintf(&sb, "\nTotal notes: %d\n", r.TotalNotes)
	if r.Orphans > 0 {
		fmt.Fprintf(&sb, "Orphan notes: %d\n", r.Orphans)
	}

	if len(r.Areas) > 0 {
		sb.WriteString("\nAreas:\n")
		for _, a := range r.Areas {
			statusMarker := "✓"
			if a.Status == StatusWarn {
				statusMarker = "⚠"
			} else if a.Status == StatusError {
				statusMarker = "✗"
			}
			fmt.Fprintf(&sb, "  %s %-20s %3d notes", statusMarker, a.Name+":", a.Notes)
			if !a.Modified.IsZero() {
				fmt.Fprintf(&sb, " (updated %s)", humanDuration(time.Since(a.Modified)))
			}
			if a.Message != "" {
				fmt.Fprintf(&sb, " — %s", a.Message)
			}
			sb.WriteString("\n")
		}
	}

	if len(r.Messages) > 0 {
		sb.WriteString("\nNotes:\n")
		for _, m := range r.Messages {
			fmt.Fprintf(&sb, "  • %s\n", m)
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// Analyze scans the vault directory and builds a Report.
func Analyze(vaultDir, vaultName string) Report {
	var r Report
	r.VaultName = vaultName
	r.VaultDir = vaultDir
	r.Healthy = true

	if vaultDir == "" {
		r.Healthy = false
		r.Messages = append(r.Messages, "Vault directory is empty")
		return r
	}

	info, err := os.Stat(vaultDir)
	if err != nil {
		r.Healthy = false
		r.Messages = append(r.Messages, fmt.Sprintf("Cannot access vault: %v", err))
		return r
	}
	if !info.IsDir() {
		r.Healthy = false
		r.Messages = append(r.Messages, "Vault path is not a directory")
		return r
	}

	// Known areas to scan
	areas := []struct {
		name string
		dir  string
	}{
		{"conventions", "conventions"},
		{"concepts", "concepts"},
		{"decisions", "decisions"},
		{"patterns", "patterns"},
		{"debug", "debug"},
		{"projects", "projects"},
		{"methodology", "methodology"},
		{"_inbox", "_inbox"},
	}

	for _, area := range areas {
		a := scanArea(filepath.Join(vaultDir, area.dir))
		a.Name = area.name
		r.Areas = append(r.Areas, a)
		r.TotalNotes += a.Notes
		if a.Status == StatusError {
			r.Healthy = false
		}
	}

	// Count orphan notes (notes not in known areas)
	var orphanCount int
	_ = filepath.Walk(vaultDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(vaultDir, path)
		// Skip dotfiles, trash, and known areas
		if strings.HasPrefix(rel, ".") || strings.HasPrefix(rel, "_trash") {
			return nil
		}
		inKnownArea := false
		for _, area := range areas {
			if strings.HasPrefix(rel, area.dir+string(filepath.Separator)) {
				inKnownArea = true
				break
			}
		}
		if !inKnownArea {
			orphanCount++
		}
		return nil
	})
	r.Orphans = orphanCount
	if orphanCount > 0 {
		r.Messages = append(r.Messages, fmt.Sprintf("%d notes outside known areas (may need triage)", orphanCount))
	}

	// Check for .nd.yaml (nd vault marker)
	ndYaml := filepath.Join(vaultDir, ".nd.yaml")
	if _, err := os.Stat(ndYaml); err != nil {
		r.Messages = append(r.Messages, "No .nd.yaml found — not an nd vault")
	} else {
		r.Messages = append(r.Messages, "nd vault configured (.nd.yaml present)")
	}

	// Check for project knowledge settings
	settingsPath := filepath.Join(vaultDir, "knowledge", ".settings.yaml")
	if _, err := os.Stat(settingsPath); err == nil {
		r.Messages = append(r.Messages, "Project knowledge settings found")
	}

	// Find most recent modification across all notes
	var latest time.Time
	_ = filepath.Walk(vaultDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".md" && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	if !latest.IsZero() {
		r.LastSync = latest
	}

	return r
}

func scanArea(dir string) Area {
	a := Area{Path: dir, Status: StatusOK}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			a.Status = StatusWarn
			a.Message = "directory does not exist"
			return a
		}
		a.Status = StatusError
		a.Message = fmt.Sprintf("cannot read: %v", err)
		return a
	}
	if !info.IsDir() {
		a.Status = StatusError
		a.Message = "not a directory"
		return a
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		a.Status = StatusError
		a.Message = fmt.Sprintf("cannot list: %v", err)
		return a
	}

	var count int
	var latest time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			// Recurse into subdirectories
			sub := scanArea(filepath.Join(dir, entry.Name()))
			count += sub.Notes
			if sub.Modified.After(latest) {
				latest = sub.Modified
			}
			continue
		}
		if filepath.Ext(entry.Name()) == ".md" {
			count++
			info, err := entry.Info()
			if err == nil && info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		}
	}

	a.Notes = count
	a.Modified = latest
	if count == 0 {
		a.Message = "empty"
	}
	return a
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	if d < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return fmt.Sprintf("%dw ago", int(d.Hours()/24/7))
}
