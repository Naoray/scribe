package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func newRestoreCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <skill> <revision>",
		Short: "Restore a previous version of a skill",
		Long: `Restore a skill to a previous version snapshot.

The revision can be specified as a number (e.g. "2") or with the rev- prefix
(e.g. "rev-2"). The current content is snapshotted before restoring.

Example:
  scribe restore cleanup 2
  scribe restore cleanup rev-2`,
		Args: cobra.ExactArgs(2),
		RunE: runRestore,
	}
}

func runRestore(cmd *cobra.Command, args []string) error {
	skillName := args[0]
	revArg := args[1]
	factory := newCommandFactory()

	// Parse revision number: accept "rev-2" or just "2".
	revStr := strings.TrimPrefix(revArg, "rev-")
	targetRev, err := strconv.Atoi(revStr)
	if err != nil {
		return fmt.Errorf("invalid revision %q: expected a number or rev-N format", revArg)
	}
	if targetRev <= 0 {
		return fmt.Errorf("invalid revision %d: must be a positive number", targetRev)
	}

	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	skill, ok := st.Installed[skillName]
	if !ok {
		return fmt.Errorf("%q is not installed", skillName)
	}

	storeDir, err := tools.StoreDir()
	if err != nil {
		return fmt.Errorf("resolve store dir: %w", err)
	}
	skillDir := filepath.Join(storeDir, skillName)

	// Find the requested version snapshot.
	versionPath := filepath.Join(skillDir, "versions", fmt.Sprintf("rev-%d.md", targetRev))
	restoreContent, err := os.ReadFile(versionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("version rev-%d not found for %q", targetRev, skillName)
		}
		return fmt.Errorf("read version: %w", err)
	}

	// Snapshot current SKILL.md before overwriting.
	if err := sync.SnapshotVersion(skillDir, skill.Revision); err != nil {
		return fmt.Errorf("snapshot current version: %w", err)
	}

	// Write restored content to SKILL.md.
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, restoreContent, 0o644); err != nil {
		return fmt.Errorf("write restored skill: %w", err)
	}

	// Bump revision (forward operation).
	newRevision := skill.Revision + 1
	skill.Revision = newRevision
	skill.InstalledHash = sync.ComputeFileHash(restoreContent)
	st.Installed[skillName] = skill

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Restored rev %d as rev %d. This skill will be preserved during future syncs.\n", targetRev, newRevision)
	return nil
}
