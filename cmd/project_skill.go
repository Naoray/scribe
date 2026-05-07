package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

type projectSkillOutput struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Origin string `json:"origin"`
}

func newProjectSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage project-authored skills",
	}
	cmd.AddCommand(newProjectSkillCreateCommand(), newProjectSkillClaimCommand())
	return cmd
}

func newProjectSkillCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project-authored skill",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectSkillCreate,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func newProjectSkillClaimCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claim <name>",
		Short: "Mark a local skill as project-authored",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectSkillClaim,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func runProjectSkillCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := validateKitName(name); err != nil {
		return err
	}
	factory := commandFactory()
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if installed, ok := st.Installed[name]; ok && installed.Origin != state.OriginProject {
		return clierrors.Wrap(fmt.Errorf("skill %q already exists with origin %q", name, installed.Origin), "PROJECT_SKILL_EXISTS", clierrors.ExitConflict,
			clierrors.WithRemediation("Use `scribe project skill claim "+name+"` for local skills, or remove/recreate conflicting registry skills."),
		)
	}
	dir, err := tools.StoreDir()
	if err != nil {
		return fmt.Errorf("resolve store dir: %w", err)
	}
	skillDir := filepath.Join(dir, name)
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return fmt.Errorf("create skill dir: %w", err)
		}
		if err := os.WriteFile(skillPath, []byte("# "+name+"\n\n"), 0o644); err != nil {
			return fmt.Errorf("write skill: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("check skill path: %w", err)
	}
	next := st.Installed[name]
	next.Origin = state.OriginProject
	st.RecordInstall(name, next)
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	out := projectSkillOutput{Name: name, Path: skillDir, Origin: string(state.OriginProject)}
	if jsonFlagPassed(cmd) {
		return renderMutatorEnvelope(cmd, out, envelope.StatusOK)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created project skill %s at %s\n", name, skillDir)
	return nil
}

func runProjectSkillClaim(cmd *cobra.Command, args []string) error {
	name := args[0]
	factory := commandFactory()
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	installed, ok := st.Installed[name]
	if !ok {
		return clierrors.Wrap(fmt.Errorf("skill %q is not installed", name), "PROJECT_SKILL_NOT_FOUND", clierrors.ExitNotFound,
			clierrors.WithRemediation("Create it with `scribe project skill create "+name+"` or install/adopt it first."),
		)
	}
	switch installed.Origin {
	case state.OriginProject:
	case state.OriginLocal:
		installed.Origin = state.OriginProject
	default:
		return clierrors.Wrap(fmt.Errorf("skill %q has origin %q", name, installed.Origin), "PROJECT_SKILL_CLAIM_REFUSED", clierrors.ExitConflict,
			clierrors.WithRemediation("Only local skills can be claimed. Registry and bootstrap skills must not be silently detached."),
		)
	}
	st.Installed[name] = installed
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	dir, _ := tools.StoreDir()
	out := projectSkillOutput{Name: name, Path: filepath.Join(dir, name), Origin: string(state.OriginProject)}
	if jsonFlagPassed(cmd) {
		return renderMutatorEnvelope(cmd, out, envelope.StatusOK)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Claimed %s as a project skill\n", name)
	return nil
}
