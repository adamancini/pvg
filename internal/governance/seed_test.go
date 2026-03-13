package governance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgentSrc_PrefersLocalPluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	localAgents := filepath.Join(pluginDir, "agents")
	if err := os.MkdirAll(localAgents, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := resolveAgentSrc(pluginDir)
	if err != nil {
		t.Fatalf("resolveAgentSrc() error: %v", err)
	}
	if got != localAgents {
		t.Fatalf("resolveAgentSrc() = %q, want %q", got, localAgents)
	}
}

func TestResolveAgentSrc_UsesExplicitOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom-agents")
	if err := os.Setenv("AGENT_SRC", override); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("AGENT_SRC") }()

	got, err := resolveAgentSrc("")
	if err != nil {
		t.Fatalf("resolveAgentSrc() error: %v", err)
	}
	if got != override {
		t.Fatalf("resolveAgentSrc() = %q, want %q", got, override)
	}
}
