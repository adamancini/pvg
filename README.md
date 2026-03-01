# pvg

[![CI](https://github.com/paivot-ai/pvg/actions/workflows/ci.yml/badge.svg)](https://github.com/paivot-ai/pvg/actions/workflows/ci.yml)
[![Release](https://github.com/paivot-ai/pvg/actions/workflows/release.yml/badge.svg)](https://github.com/paivot-ai/pvg/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/paivot-ai/pvg)](https://goreportcard.com/report/github.com/paivot-ai/pvg)

Deterministic CLI for the [paivot-graph](https://github.com/paivot-ai/paivot-graph) Claude Code plugin. Replaces shell scripts with a single Go binary that handles vault governance, session lifecycle hooks, scope guards, execution loop management, and dispatcher mode.

```
pvg hook session-start       # Load vault context at session start
pvg guard                    # PreToolUse scope guard (reads JSON from stdin)
pvg seed [--force]           # Seed vault with agent prompts and conventions
pvg loop setup --all         # Start unattended execution loop
pvg version                  # Print version
```

## Why pvg exists

paivot-graph is a Claude Code plugin that turns an Obsidian vault into a persistent knowledge layer for AI agents. Early versions used shell scripts for hooks, guards, and vault seeding. As the plugin grew, the scripts became fragile -- quoting issues, inconsistent error handling, and no way to test edge cases deterministically.

pvg consolidates all of this into a single Go binary:

- **Scope guard** -- Blocks direct writes to protected vault directories (methodology/, conventions/, decisions/, etc.), enforcing the proposal workflow. Allows `_inbox/` writes and all `vlt` commands.
- **Session lifecycle** -- Loads vault context at session start, saves knowledge before compaction and stop, logs session end.
- **Dispatcher mode** -- Tracks BLT agent (BA, Designer, Architect) lifecycle so the guard can distinguish agent writes from orchestrator writes.
- **Execution loop** -- Manages unattended story execution with configurable iteration limits and automatic blocking detection.
- **Vault seeding** -- Writes agent prompts and behavioral notes to the Obsidian vault under an exclusive `vlt` lock to prevent concurrent write corruption.
- **FSM governance** -- Enforces `nd` state machine transitions (stories must follow ready -> in_progress -> delivered -> accepted).

## Installation

### Pre-built binaries

Download from [Releases](https://github.com/paivot-ai/pvg/releases):

```bash
# macOS (Apple Silicon)
gh release download -R paivot-ai/pvg -p '*darwin_arm64*' -D /tmp
tar xzf /tmp/pvg_*_darwin_arm64.tar.gz -C ~/go/bin

# macOS (Intel)
gh release download -R paivot-ai/pvg -p '*darwin_amd64*' -D /tmp
tar xzf /tmp/pvg_*_darwin_amd64.tar.gz -C ~/go/bin

# Linux (amd64)
gh release download -R paivot-ai/pvg -p '*linux_amd64*' -D /tmp
tar xzf /tmp/pvg_*_linux_amd64.tar.gz -C ~/go/bin
```

### From source (requires Go 1.24+)

```bash
git clone https://github.com/paivot-ai/pvg.git
cd pvg
make build     # produces ./pvg binary
make install   # installs to $GOPATH/bin
```

## Command reference

### Lifecycle hooks

Called by Claude Code via `hooks.json`. Each reads JSON from stdin and writes structured output to stdout.

| Command | Hook event | Description |
|---------|-----------|-------------|
| `pvg hook session-start` | SessionStart | Load vault context, project knowledge, operating mode |
| `pvg hook pre-compact` | PreCompact | Save decisions, patterns, and debug insights before compaction |
| `pvg hook stop` | Stop | Capture session knowledge before ending |
| `pvg hook session-end` | SessionEnd | Log session end, clean up dispatcher state |
| `pvg hook user-prompt` | UserPromptSubmit | Auto-detect and manage dispatcher mode |
| `pvg hook subagent-start` | SubagentStart | Track BLT agent activation |
| `pvg hook subagent-stop` | SubagentStop | Track BLT agent deactivation |

### Scope guard

```bash
echo '{"tool_name":"Edit","tool_input":{"file_path":"/path/to/file"}}' | pvg guard
```

Exit codes:
- `0` -- Operation allowed
- `2` -- Operation blocked (protected vault path or governance violation)

Two protection layers:
1. **System vault** -- Protects methodology/, conventions/, decisions/, patterns/, debug/, concepts/, projects/, people/. Allows `_inbox/` and `_templates/`.
2. **Project vault** -- Protects `.vault/knowledge/` files. Allows `.settings.yaml`.

All `vlt` commands are always allowed (they use advisory locking for concurrent safety).

### Execution loop

```bash
pvg loop setup --all                    # Run all ready stories
pvg loop setup --epic PROJ-a1b          # Target a specific epic
pvg loop setup --all --max 25           # Limit iterations
pvg loop status                         # Show loop state
pvg loop cancel                         # Cancel active loop
```

### Dispatcher mode

```bash
pvg dispatcher on       # Enable (orchestrator becomes coordinator-only)
pvg dispatcher off      # Disable
pvg dispatcher status   # Show state and active BLT agents
```

### Vault seeding

```bash
pvg seed              # Bootstrap vault notes (skip if exists)
pvg seed --force      # Overwrite all vault notes with latest content
```

Seeds agent prompts (8 agents), skill content, and behavioral notes (Session Operating Mode, Pre-Compact Checklist, Stop Capture Checklist) into the Obsidian vault.

### Settings

```bash
pvg settings                         # Show all settings
pvg settings stack_detection=true    # Set a value
```

### Other

| Command | Description |
|---------|-------------|
| `pvg version` | Print version |
| `pvg help` | Show usage |

## Architecture

```
cmd/pvg/
  main.go              CLI entry point, argument parsing, command dispatch

internal/
  dispatcher/          Dispatcher mode state management (BLT agent tracking)
  governance/          Vault seeding with vlt lock
  guard/               Scope guard (system vault, project vault, dispatcher, FSM)
  lifecycle/           Session hooks (start, pre-compact, stop, end, user-prompt, subagent)
  loop/                Execution loop state machine (setup, evaluate, cancel)
  settings/            Project settings (YAML read/write)
  vaultcfg/            Vault discovery and configuration
```

### Dependencies

| Dependency | Purpose |
|-----------|---------|
| [vlt](https://github.com/RamXX/vlt) | Obsidian vault operations (library import) |

## Development

```bash
make build    # compile
make test     # run tests (verbose)
make vet      # run go vet
make install  # install to $GOPATH/bin
make clean    # remove build artifacts
```

### Running tests

```bash
go test -v ./...                    # verbose output
go test -cover ./...                # with coverage
go test -run TestCheckFilePath ./.. # run specific test
```

All tests use `t.TempDir()` for isolated environments. No mocks in integration tests.

## Releasing

Tag and push:

```bash
git tag v1.18.0
git push origin v1.18.0
```

The [release workflow](.github/workflows/release.yml) runs tests, then uses GoReleaser to produce binaries for darwin/linux x amd64/arm64.

## License

Copyright 2025 Ramiro Salas. All rights reserved.
