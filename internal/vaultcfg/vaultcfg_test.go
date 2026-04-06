package vaultcfg

import (
	"testing"
)

func TestVaultNameDefault(t *testing.T) {
	t.Setenv("PVG_VAULT", "")
	got := VaultName()
	if got != DefaultVaultName {
		t.Errorf("VaultName() = %q, want %q", got, DefaultVaultName)
	}
}

func TestVaultNameEnvOverride(t *testing.T) {
	t.Setenv("PVG_VAULT", "MyVault")
	got := VaultName()
	if got != "MyVault" {
		t.Errorf("VaultName() = %q, want %q", got, "MyVault")
	}
}

func TestVaultNameAbsolutePath(t *testing.T) {
	t.Setenv("PVG_VAULT", "/tmp/myvault")
	got := VaultName()
	if got != "/tmp/myvault" {
		t.Errorf("VaultName() = %q, want %q", got, "/tmp/myvault")
	}
}

func TestVaultNameTrimsWhitespace(t *testing.T) {
	t.Setenv("PVG_VAULT", "  SpacedVault  ")
	got := VaultName()
	if got != "SpacedVault" {
		t.Errorf("VaultName() = %q, want %q", got, "SpacedVault")
	}
}

func TestVaultNameWhitespaceOnlyFallsBack(t *testing.T) {
	t.Setenv("PVG_VAULT", "   ")
	got := VaultName()
	if got != DefaultVaultName {
		t.Errorf("VaultName() with whitespace-only PVG_VAULT = %q, want %q", got, DefaultVaultName)
	}
}

func TestVaultDirUsesEnvVar(t *testing.T) {
	// Create a temp directory. vlt.OpenByName accepts absolute paths
	// directly (validates the directory exists), so VaultDir() will
	// return this path without needing Obsidian config or iCloud fallback.
	tmp := t.TempDir()
	t.Setenv("PVG_VAULT", tmp)

	dir, err := VaultDir()
	if err != nil {
		t.Fatalf("VaultDir() error: %v", err)
	}
	if dir != tmp {
		t.Errorf("VaultDir() = %q, want %q", dir, tmp)
	}
}
