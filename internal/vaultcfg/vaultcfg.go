// Package vaultcfg provides shared vault configuration for pvg commands.
package vaultcfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paivot-ai/vlt"
)

// DefaultVaultName is the vault name used when PVG_VAULT is not set.
const DefaultVaultName = "Claude"

// VaultName returns the effective vault name or path.
// It checks PVG_VAULT first, falling back to DefaultVaultName.
func VaultName() string {
	if v := strings.TrimSpace(os.Getenv("PVG_VAULT")); v != "" {
		return v
	}
	return DefaultVaultName
}

// VaultDir returns the vault directory path, opening it via vlt.
func VaultDir() (string, error) {
	name := VaultName()
	v, err := vlt.OpenByName(name)
	if err != nil {
		// Fallback to conventional iCloud path
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", fmt.Errorf("cannot determine vault directory: %w", err)
		}
		dir := filepath.Join(home, "Library", "Mobile Documents", "iCloud~md~obsidian", "Documents", name)
		if _, serr := os.Stat(dir); serr != nil {
			return "", fmt.Errorf("vault not found via vlt or at %s", dir)
		}
		return dir, nil
	}
	return v.Dir(), nil
}

// OpenVault opens the vault via vlt using the effective vault name.
func OpenVault() (*vlt.Vault, error) {
	return vlt.OpenByName(VaultName())
}
