package governance

import (
	"fmt"
	"os"
	"path/filepath"
)

// BaselineDir returns the directory where seed baselines are stored.
func BaselineDir(vaultDir string) string {
	return filepath.Join(vaultDir, ".seed-baselines")
}

// ReadBaseline reads the baseline content for a given relative path.
// Returns the content as a string. Returns a wrapped os.ErrNotExist if the
// baseline file does not exist.
func ReadBaseline(baseDir, relPath string) (string, error) {
	fullPath := filepath.Join(baseDir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("baseline not found for %s: %w", relPath, os.ErrNotExist)
		}
		return "", fmt.Errorf("reading baseline %s: %w", relPath, err)
	}
	return string(data), nil
}

// WriteBaseline writes the baseline content for a given relative path,
// creating parent directories as needed.
func WriteBaseline(baseDir, relPath, content string) error {
	fullPath := filepath.Join(baseDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("creating baseline directory for %s: %w", relPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing baseline %s: %w", relPath, err)
	}
	return nil
}
