package ndvault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_PrefersSharedVaultForPaivotWorktree(t *testing.T) {
	projectRoot, sharedVault := setupPaivotWorktree(t)

	localVault := filepath.Join(projectRoot, ".vault")
	if err := os.MkdirAll(localVault, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(projectRoot)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if got != sharedVault {
		t.Fatalf("Resolve() = %q, want %q", got, sharedVault)
	}
}

func TestResolve_FallsBackToNearestLocalVault(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "repo")
	nested := filepath.Join(projectRoot, "pkg", "service")

	if err := os.MkdirAll(filepath.Join(projectRoot, ".vault"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve(nested)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := filepath.Join(projectRoot, ".vault")
	if resolved, err := filepath.EvalSymlinks(got); err == nil {
		got = resolved
	}
	if resolved, err := filepath.EvalSymlinks(want); err == nil {
		want = resolved
	}
	if got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

func TestResolve_UsesEnvironmentOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom-vault")
	if err := os.Setenv("ND_VAULT_DIR", override); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Unsetenv("ND_VAULT_DIR")
	}()

	got, err := Resolve("/does/not/matter")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if got != override {
		t.Fatalf("Resolve() = %q, want %q", got, override)
	}
}

func setupPaivotWorktree(t *testing.T) (projectRoot, sharedVault string) {
	t.Helper()

	base := t.TempDir()
	projectRoot = filepath.Join(base, "repo")
	gitDir := filepath.Join(base, "gitdir", "worktrees", "story")
	commonDir := filepath.Join(base, "gitdir")
	sharedVault = filepath.Join(commonDir, sharedVaultRelPath)

	if err := os.MkdirAll(filepath.Join(projectRoot, ".vault", "knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".vault", "knowledge", ".settings.yaml"), []byte("workflow.fsm: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sharedVault, 0o755); err != nil {
		t.Fatal(err)
	}

	gitPtr := "gitdir: " + filepath.ToSlash(gitDir) + "\n"
	if err := os.WriteFile(filepath.Join(projectRoot, ".git"), []byte(gitPtr), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return projectRoot, sharedVault
}
