package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/targets"
	"github.com/Naoray/scribe/internal/workflow"
)

var syncJSON bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local skills to match team loadout",
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncJSON, "json", false, "Output machine-readable JSON (for CI/agents)")
	syncCmd.Flags().StringVar(&registryFlag, "registry", "", "Sync only this registry (owner/repo or repo name)")
	syncCmd.Flags().Bool("all", false, "Sync all registries (default behavior)")
	syncCmd.Flags().MarkHidden("all")
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := state.Load()
	if err != nil {
		return err
	}

	if len(cfg.TeamRepos) == 0 {
		return fmt.Errorf("not connected — run `scribe connect <owner/repo>` first")
	}

	// Migrate legacy state (no Registries field) on first multi-registry run.
	st.MigrateRegistries(cfg.TeamRepos[0])

	repos, err := filterRegistries(registryFlag, cfg.TeamRepos)
	if err != nil {
		return err
	}

	client := gh.NewClient(cfg.Token)
	tgts := []targets.Target{targets.ClaudeTarget{}, targets.CursorTarget{}}

	useJSON := syncJSON || !isatty.IsTerminal(os.Stdout.Fd())
	multiRegistry := len(cfg.TeamRepos) > 1
	fmtr := workflow.NewFormatter(useJSON, multiRegistry)

	resolved := map[string]sync.SkillStatus{}

	syncer := &sync.Syncer{
		Client:  client,
		Targets: tgts,
	}

	for _, teamRepo := range repos {
		clear(resolved)

		syncer.Emit = func(msg any) {
			switch m := msg.(type) {
			case sync.SkillResolvedMsg:
				resolved[m.Name] = m.SkillStatus
				fmtr.OnSkillResolved(m.Name, m.SkillStatus)
			case sync.SkillSkippedMsg:
				fmtr.OnSkillSkipped(m.Name, resolved[m.Name])
			case sync.SkillDownloadingMsg:
				fmtr.OnSkillDownloading(m.Name)
			case sync.SkillInstalledMsg:
				fmtr.OnSkillInstalled(m.Name, m.Version, m.Updated)
			case sync.SkillErrorMsg:
				fmtr.OnSkillError(m.Name, m.Err)
			case sync.SyncCompleteMsg:
				fmtr.OnSyncComplete(m)
			}
		}

		fmtr.OnRegistryStart(teamRepo)

		if err := syncer.Run(context.Background(), teamRepo, st); err != nil {
			return err
		}

		// Track which registry each synced skill belongs to.
		for name := range resolved {
			st.AddRegistry(name, teamRepo)
		}
		if err := st.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	return fmtr.Flush()
}
