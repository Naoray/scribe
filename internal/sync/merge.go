package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// MergeResult describes the outcome of a 3-way merge.
type MergeResult int

const (
	MergeClean    MergeResult = iota // clean merge, auto-applied
	MergeConflict                    // has conflict markers
	MergeError                       // git merge-file failed
)

// ThreeWayMerge performs a 3-way merge using git merge-file.
// base = .scribe-base.md (last synced pristine)
// ours = SKILL.md (locally modified)
// theirs = new upstream content
//
// On clean merge: updates SKILL.md in place, then updates .scribe-base.md.
// On conflict: writes conflict markers to SKILL.md, updates .scribe-base.md.
func ThreeWayMerge(skillDir string, upstreamContent []byte) (MergeResult, error) {
	oursPath := filepath.Join(skillDir, "SKILL.md")
	basePath := filepath.Join(skillDir, ".scribe-base.md")

	// Verify both files exist before attempting merge.
	if _, err := os.Stat(oursPath); err != nil {
		return MergeError, fmt.Errorf("local file missing: %w", err)
	}
	if _, err := os.Stat(basePath); err != nil {
		return MergeError, fmt.Errorf("base file missing: %w", err)
	}

	// Write upstream to temp file.
	theirsPath := filepath.Join(skillDir, ".scribe-theirs.md")
	if err := os.WriteFile(theirsPath, upstreamContent, 0o644); err != nil {
		return MergeError, err
	}
	defer os.Remove(theirsPath)

	// git merge-file modifies oursPath in place.
	cmd := exec.Command("git", "merge-file",
		"-L", "local",
		"-L", "base",
		"-L", "upstream",
		oursPath, basePath, theirsPath)

	err := cmd.Run()

	// Update .scribe-base.md to new upstream regardless of merge outcome.
	_ = os.WriteFile(basePath, upstreamContent, 0o644)

	if err == nil {
		return MergeClean, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 1 {
			return MergeConflict, nil
		}
	}

	return MergeError, fmt.Errorf("git merge-file: %w", err)
}

// ComputeFileHash returns SHA-256 hash (first 8 hex chars) of file content.
func ComputeFileHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])[:8]
}

// IsLocallyModified checks if SKILL.md has been modified since last sync.
func IsLocallyModified(skillDir string, installedHash string) bool {
	if installedHash == "" {
		return false
	}
	content, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return false
	}
	return ComputeFileHash(content) != installedHash
}
