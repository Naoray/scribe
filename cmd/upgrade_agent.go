package cmd

import (
	"context"
	"errors"
	"fmt"

	gogithub "github.com/google/go-github/v69/github"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/agent"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

var errAuthRequired = errors.New("authentication required")

type upgradeAgentClient interface {
	LatestRelease(ctx context.Context, owner, repo string) (*gogithub.RepositoryRelease, error)
	FetchFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error)
	IsAuthenticated() bool
}

func newUpgradeAgentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade-agent",
		Short: "Refresh the embedded scribe-agent skill from Naoray/scribe",
		Args:  cobra.NoArgs,
		RunE:  runUpgradeAgent,
	}
}

func runUpgradeAgent(cmd *cobra.Command, _ []string) error {
	factory := newCommandFactory()

	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	client, err := factory.Client()
	if err != nil {
		return fmt.Errorf("load github client: %w", err)
	}

	return runUpgradeAgentWithDeps(cmd.Context(), cfg, st, client)
}

func runUpgradeAgentWithDeps(
	ctx context.Context,
	cfg *config.Config,
	st *state.State,
	client upgradeAgentClient,
) error {
	if !client.IsAuthenticated() {
		return fmt.Errorf("%w — run `gh auth login` or set GITHUB_TOKEN", errAuthRequired)
	}

	release, err := client.LatestRelease(ctx, "Naoray", "scribe")
	if err != nil {
		return fmt.Errorf("latest release Naoray/scribe: %w", err)
	}
	tag := release.GetTagName()
	if tag == "" {
		return fmt.Errorf("latest release Naoray/scribe: missing tag")
	}

	tmpl, err := client.FetchFile(ctx, "Naoray", "scribe", "SKILL.md.tmpl", tag)
	if err != nil {
		return fmt.Errorf("fetch scribe-agent template at %s: %w", tag, err)
	}

	storeDir, err := tools.StoreDir()
	if err != nil {
		return fmt.Errorf("resolve store dir: %w", err)
	}

	content, err := agent.RenderSkillTemplate(tmpl, storeDir, st)
	if err != nil {
		return fmt.Errorf("render scribe-agent template: %w", err)
	}

	changed, err := agent.InstallScribeAgent(storeDir, st, content, tag)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

var _ upgradeAgentClient = (*gh.Client)(nil)
