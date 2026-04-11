package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func newResolveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <skill>",
		Short: "Resolve a merge conflict in a skill",
		Long: `Resolve a merge conflict by choosing which version to keep.

  --ours    Keep your local version (discard upstream changes)
  --theirs  Accept the upstream version (discard local changes)

Exactly one of --ours or --theirs must be specified.`,
		Args: cobra.ExactArgs(1),
		RunE: runResolve,
	}

	cmd.Flags().Bool("ours", false, "Keep the local version")
	cmd.Flags().Bool("theirs", false, "Accept the upstream version")

	return cmd
}

func runResolve(cmd *cobra.Command, args []string) error {
	skillName := args[0]
	ours, _ := cmd.Flags().GetBool("ours")
	theirs, _ := cmd.Flags().GetBool("theirs")

	if ours == theirs {
		return fmt.Errorf("specify exactly one of --ours or --theirs")
	}

	st, err := state.Load()
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

	var content []byte
	theirsPath := filepath.Join(skillDir, ".scribe-theirs.md")

	if ours {
		// --ours: restore from the latest version snapshot (what we had before the merge).
		versions, err := sync.ListVersions(skillDir)
		if err != nil {
			return fmt.Errorf("list versions: %w", err)
		}
		if len(versions) == 0 {
			return fmt.Errorf("no version snapshots available for %q", skillName)
		}
		latest := versions[len(versions)-1]
		content, err = os.ReadFile(latest.Path)
		if err != nil {
			return fmt.Errorf("read version snapshot: %w", err)
		}
	} else {
		// --theirs: use the upstream sidecar persisted by ThreeWayMerge on conflict.
		content, err = os.ReadFile(theirsPath)
		if err != nil {
			return fmt.Errorf("read upstream sidecar: %w", err)
		}
	}

	// Write resolved content to SKILL.md.
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, content, 0o644); err != nil {
		return fmt.Errorf("write resolved skill: %w", err)
	}

	// Advance the merge base to the resolved content so the next sync starts
	// from a clean 3-way merge baseline, and drop the conflict sidecar.
	basePath := filepath.Join(skillDir, ".scribe-base.md")
	if err := os.WriteFile(basePath, content, 0o644); err != nil {
		return fmt.Errorf("update merge base: %w", err)
	}
	_ = os.Remove(theirsPath)

	// Update state.
	skill.InstalledHash = sync.ComputeFileHash(content)
	skill.Revision++
	st.Installed[skillName] = skill

	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	side := "local"
	if theirs {
		side = "upstream"
	}
	fmt.Fprintf(os.Stderr, "Resolved %s → kept %s version (rev %d)\n", skillName, side, skill.Revision)
	return nil
}
