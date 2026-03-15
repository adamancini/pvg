package ndvault

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var execCommand = exec.Command

// Ensure resolves the live nd vault for a repo and initializes it if needed.
func Ensure(projectRoot string) (string, error) {
	vaultDir, err := Resolve(projectRoot)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(vaultDir, 0o755); err != nil {
		return "", fmt.Errorf("create nd vault %s: %w", vaultDir, err)
	}

	configPath := filepath.Join(vaultDir, ".nd.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return vaultDir, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", configPath, err)
	}

	cmd := execCommand("nd", "init", "--vault", vaultDir)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("nd init --vault %s: %s", vaultDir, msg)
		}
		return "", fmt.Errorf("nd init --vault %s: %w", vaultDir, err)
	}

	return vaultDir, nil
}
