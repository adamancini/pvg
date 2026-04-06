package governance

import (
	"fmt"
	"os"
	"os/exec"
)

// Merge3 performs a three-way merge using the system diff3 utility.
// base is the common ancestor (the stored baseline), ours is the new plugin
// content, and theirs is the current vault file (with potential user edits).
// Returns the merged content, whether conflict markers are present, and any error.
func Merge3(base, ours, theirs string) (merged string, hasConflict bool, err error) {
	// Create temp files for base, ours, theirs
	baseFile, err := os.CreateTemp("", "merge-base-*")
	if err != nil {
		return "", false, fmt.Errorf("creating temp base file: %w", err)
	}
	defer os.Remove(baseFile.Name())

	oursFile, err := os.CreateTemp("", "merge-ours-*")
	if err != nil {
		return "", false, fmt.Errorf("creating temp ours file: %w", err)
	}
	defer os.Remove(oursFile.Name())

	theirsFile, err := os.CreateTemp("", "merge-theirs-*")
	if err != nil {
		return "", false, fmt.Errorf("creating temp theirs file: %w", err)
	}
	defer os.Remove(theirsFile.Name())

	// Write content to temp files
	if _, err := baseFile.WriteString(base); err != nil {
		return "", false, fmt.Errorf("writing temp base file: %w", err)
	}
	baseFile.Close()

	if _, err := oursFile.WriteString(ours); err != nil {
		return "", false, fmt.Errorf("writing temp ours file: %w", err)
	}
	oursFile.Close()

	if _, err := theirsFile.WriteString(theirs); err != nil {
		return "", false, fmt.Errorf("writing temp theirs file: %w", err)
	}
	theirsFile.Close()

	// Run diff3 -m theirs base ours
	// diff3 -m merges: file1=theirs (current vault), file2=base (ancestor), file3=ours (new plugin)
	cmd := exec.Command("diff3", "-m", theirsFile.Name(), baseFile.Name(), oursFile.Name())
	output, cmdErr := cmd.Output()

	if cmdErr != nil {
		// Check if diff3 binary was found
		if execErr, ok := cmdErr.(*exec.Error); ok {
			return "", false, fmt.Errorf("diff3 not found: install diffutils or verify PATH: %w", execErr)
		}

		// diff3 exit codes: 0=no conflicts, 1=conflicts, 2=error
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			switch exitErr.ExitCode() {
			case 1:
				// Conflicts exist — output still contains the merged content with markers
				return string(output), true, nil
			default:
				return "", false, fmt.Errorf("diff3 failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
			}
		}
		return "", false, fmt.Errorf("diff3 execution error: %w", cmdErr)
	}

	// Exit code 0: clean merge, no conflicts
	return string(output), false, nil
}
