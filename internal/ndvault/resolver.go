package ndvault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const sharedVaultRelPath = "paivot/nd-vault"

// Resolve returns the nd vault path for a project root.
//
// In Paivot-managed repos, the live nd vault is branch-independent and lives
// under the repository's git common dir. Non-Paivot repos fall back to the
// nearest local .vault directory.
func Resolve(projectRoot string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("ND_VAULT_DIR")); override != "" {
		return filepath.Clean(override), nil
	}

	if projectRoot == "" {
		return "", fmt.Errorf("project root is required")
	}

	projectRoot = filepath.Clean(projectRoot)

	if isPaivotManaged(projectRoot) {
		commonDir, err := gitCommonDir(projectRoot)
		if err == nil {
			return filepath.Join(commonDir, sharedVaultRelPath), nil
		}
	}

	local, err := nearestLocalVault(projectRoot)
	if err != nil {
		return "", err
	}
	return local, nil
}

func isPaivotManaged(projectRoot string) bool {
	candidates := []string{
		filepath.Join(projectRoot, ".vault", "knowledge", ".settings.yaml"),
		filepath.Join(projectRoot, ".vault", "knowledge"),
		filepath.Join(projectRoot, ".vault", ".dispatcher-state.json"),
		filepath.Join(projectRoot, ".vault", ".piv-loop-state.json"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}

	return false
}

func nearestLocalVault(start string) (string, error) {
	dir := filepath.Clean(start)
	for {
		candidate := filepath.Join(dir, ".vault")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find local .vault from %s", start)
}

func gitCommonDir(start string) (string, error) {
	repoRoot, err := findRepoRoot(start)
	if err != nil {
		return "", err
	}

	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", gitPath, err)
	}

	if info.IsDir() {
		return filepath.Clean(gitPath), nil
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", gitPath, err)
	}

	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("%s does not contain a gitdir pointer", gitPath)
	}

	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	commonDirPath := filepath.Join(gitDir, "commondir")
	if data, err := os.ReadFile(commonDirPath); err == nil {
		commonDir := strings.TrimSpace(string(data))
		if commonDir != "" {
			if !filepath.IsAbs(commonDir) {
				commonDir = filepath.Join(gitDir, commonDir)
			}
			return filepath.Clean(commonDir), nil
		}
	}

	return gitDir, nil
}

func findRepoRoot(start string) (string, error) {
	dir := filepath.Clean(start)
	for {
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find git repo from %s", start)
}
