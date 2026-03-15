package ndvault

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_UsesPaivotVaultOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom-paivot-vault")
	if err := os.Setenv("PAIVOT_ND_VAULT", override); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Unsetenv("PAIVOT_ND_VAULT")
	}()

	got, err := Resolve("/does/not/matter")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if got != override {
		t.Fatalf("Resolve() = %q, want %q", got, override)
	}
}

func TestEnsure_InitializesMissingVault(t *testing.T) {
	projectRoot, sharedVault := setupSharedWorktree(t)

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		if name != "nd" {
			t.Fatalf("unexpected command %q", name)
		}
		if len(args) != 3 || args[0] != "init" || args[1] != "--vault" || args[2] != sharedVault {
			t.Fatalf("unexpected nd args: %v", args)
		}
		if err := os.WriteFile(filepath.Join(sharedVault, ".nd.yaml"), []byte("vault: ok\n"), 0o644); err != nil {
			t.Fatalf("write .nd.yaml: %v", err)
		}
		return exec.Command("sh", "-c", "exit 0")
	}

	got, err := Ensure(projectRoot)
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if got != sharedVault {
		t.Fatalf("Ensure() = %q, want %q", got, sharedVault)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 nd init call, got %d", len(calls))
	}
}

func TestEnsure_SkipsInitWhenVaultAlreadyConfigured(t *testing.T) {
	projectRoot, sharedVault := setupSharedWorktree(t)
	if err := os.WriteFile(filepath.Join(sharedVault, ".nd.yaml"), []byte("vault: ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("unexpected exec %s %v", name, args)
		return nil
	}

	got, err := Ensure(projectRoot)
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if got != sharedVault {
		t.Fatalf("Ensure() = %q, want %q", got, sharedVault)
	}
}

func TestEnsure_PropagatesInitFailure(t *testing.T) {
	projectRoot, _ := setupSharedWorktree(t)

	oldExec := execCommand
	defer func() { execCommand = oldExec }()

	execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("sh", "-c", fmt.Sprintf("echo 'boom' >&2; exit 3"))
	}

	_, err := Ensure(projectRoot)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Ensure() error = %v, want stderr included", err)
	}
}
