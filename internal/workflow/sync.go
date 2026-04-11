package workflow

import (
	"context"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/reconcile"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// SyncSteps returns the step list for the sync command.
func SyncSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"CheckConnected", StepCheckConnected},
		{"FilterRegistries", StepFilterRegistries},
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"Adopt", StepAdopt},
		{"ReconcilePre", StepReconcileSystem},
		{"SyncSkills", StepSyncSkills},
		{"ReconcilePost", StepReconcileSystem},
	}
}

// SyncTail returns the shared tail of steps reused by connect and create-registry.
func SyncTail() []Step {
	return []Step{
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"SyncSkills", StepSyncSkills},
		{"ReconcilePost", StepReconcileSystem},
	}
}

func StepReconcileSystem(_ context.Context, b *Bag) error {
	engine := reconcile.Engine{Tools: b.Tools}
	summary, actions, err := engine.Run(b.State)
	if err != nil {
		return fmt.Errorf("reconcile system: %w", err)
	}
	for _, action := range actions {
		if action.Kind != reconcile.ActionConflict {
			continue
		}
		for _, conflict := range b.State.Installed[action.Name].Conflicts {
			if conflict.Path == action.Path && conflict.Tool == action.Tool {
				b.Formatter.OnReconcileConflict(action.Name, conflict)
				break
			}
		}
	}
	b.Formatter.OnReconcileComplete(sync.ReconcileCompleteMsg{Summary: summary})
	return b.State.Save()
}

func StepLoadConfig(ctx context.Context, b *Bag) error {
	if b.Config == nil {
		cfg, err := loadConfig(b.Factory)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		b.Config = cfg
	}

	if b.Client == nil {
		if b.Factory != nil {
			client, err := b.Factory.Client()
			if err != nil {
				return fmt.Errorf("load github client: %w", err)
			}
			b.Client = client
		} else {
			b.Client = gh.NewClient(ctx, b.Config.Token)
		}
	}

	if b.Provider == nil {
		if b.Factory != nil {
			p, err := b.Factory.Provider()
			if err != nil {
				return fmt.Errorf("load provider: %w", err)
			}
			b.Provider = p
		} else {
			b.Provider = provider.NewGitHubProvider(provider.WrapGitHubClient(b.Client))
		}
	}

	return nil
}

func StepLoadState(_ context.Context, b *Bag) error {
	if b.State == nil {
		st, err := loadState(b.Factory)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		b.State = st
	}
	return nil
}

func loadConfig(factory *app.Factory) (*config.Config, error) {
	if factory != nil {
		return factory.Config()
	}
	return config.Load()
}

func loadState(factory *app.Factory) (*state.State, error) {
	if factory != nil {
		return factory.State()
	}
	return state.Load()
}

func StepCheckConnected(_ context.Context, b *Bag) error {
	if len(b.Config.TeamRepos()) == 0 {
		return fmt.Errorf("not connected — run `scribe connect <owner/repo>` first")
	}
	return nil
}

func StepFilterRegistries(_ context.Context, b *Bag) error {
	if b.FilterRegistries != nil {
		repos, err := b.FilterRegistries(b.RepoFlag, b.Config.TeamRepos())
		if err != nil {
			return err
		}
		b.Repos = repos
	} else {
		b.Repos = b.Config.TeamRepos()
	}
	return nil
}

// StepResolveFormatter constructs the Formatter once. Idempotent — if
// bag.Formatter is already set (e.g. by a parent workflow), it skips.
// Must run after StepFilterRegistries so b.Repos reflects the actual set.
func StepResolveFormatter(_ context.Context, b *Bag) error {
	if b.Formatter != nil {
		return nil
	}
	useJSON := b.JSONFlag || !isatty.IsTerminal(os.Stdout.Fd())
	multiRegistry := len(b.Repos) > 1
	b.Formatter = NewFormatter(useJSON, multiRegistry)
	return nil
}

func StepResolveTools(_ context.Context, b *Bag) error {
	if b.Tools == nil {
		resolved, err := tools.ResolveActive(b.Config)
		if err != nil {
			return fmt.Errorf("resolve tools: %w", err)
		}
		b.Tools = resolved
	}
	return nil
}

// StepAdopt runs skill adoption as a prelude before registry sync.
// Adoption errors are non-fatal — they are reported through Formatter and sync continues.
func StepAdopt(_ context.Context, b *Bag) error {
	mode := b.Config.AdoptionMode()
	if mode == "off" {
		return nil
	}

	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	if mode == "prompt" {
		if !isTTY || b.JSONFlag {
			b.Formatter.OnAdoptionSkipped(
				`adoption mode is "prompt" but stdin is not a terminal — skipping adoption; run "scribe adopt --yes" or set adoption.mode to auto/off`,
			)
		} else {
			b.Formatter.OnAdoptionSkipped(`prompt mode — run 'scribe adopt' to review candidates`)
		}
		return nil
	}

	candidates, conflicts, err := adopt.FindCandidates(b.State, b.Config.Adoption)
	if err != nil {
		b.Formatter.OnAdoptionSkipped(fmt.Sprintf("adoption scan failed: %v", err))
		return nil
	}

	if len(conflicts) > 0 {
		b.Formatter.OnAdoptionConflictsDeferred(len(conflicts))
	}

	if len(candidates) == 0 {
		return nil
	}

	b.Formatter.OnAdoptionStarted(len(candidates))

	adopter := &adopt.Adopter{
		State: b.State,
		Tools: b.Tools,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case adopt.AdoptedMsg:
				b.Formatter.OnAdopted(m.Name, m.Tools)
			case adopt.AdoptErrorMsg:
				b.Formatter.OnAdoptionError(m.Name, m.Err)
			case adopt.AdoptCompleteMsg:
				b.Formatter.OnAdoptionComplete(m.Adopted, m.Skipped, m.Failed)
			}
		},
	}

	adopter.Apply(candidates) // errors routed through Emit; never abort sync

	return nil
}

func StepSyncSkills(ctx context.Context, b *Bag) error {
	resolved := map[string]sync.SkillStatus{}

	syncer := &sync.Syncer{
		Client:   sync.WrapGitHubClient(b.Client),
		Provider: b.Provider,
		Tools:    b.Tools,
		Executor: &sync.ShellExecutor{},
		TrustAll: b.TrustAllFlag,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case sync.SkillResolvedMsg:
				resolved[m.Name] = m.SkillStatus
				b.Formatter.OnSkillResolved(m.Name, m.SkillStatus)
			case sync.SkillSkippedMsg:
				b.Formatter.OnSkillSkipped(m.Name, resolved[m.Name])
			case sync.SkillDownloadingMsg:
				b.Formatter.OnSkillDownloading(m.Name)
			case sync.SkillInstalledMsg:
				b.Formatter.OnSkillInstalled(m.Name, m.Updated)
			case sync.SkillErrorMsg:
				b.Formatter.OnSkillError(m.Name, m.Err)
			case sync.LegacyFormatMsg:
				b.Formatter.OnLegacyFormat(m.Repo)
			case sync.SyncCompleteMsg:
				b.Formatter.OnSyncComplete(m)

			// Package events
			case sync.PackageInstallPromptMsg:
				b.Formatter.OnPackageInstallPrompt(m.Name, m.Command, m.Source)
			case sync.PackageApprovedMsg:
				b.Formatter.OnPackageApproved(m.Name)
			case sync.PackageDeniedMsg:
				b.Formatter.OnPackageDenied(m.Name)
			case sync.PackageSkippedMsg:
				b.Formatter.OnPackageSkipped(m.Name, m.Reason)
			case sync.PackageInstallingMsg:
				b.Formatter.OnPackageInstalling(m.Name)
			case sync.PackageInstalledMsg:
				b.Formatter.OnPackageInstalled(m.Name)
			case sync.PackageUpdateMsg:
				b.Formatter.OnPackageUpdating(m.Name)
			case sync.PackageUpdatedMsg:
				b.Formatter.OnPackageUpdated(m.Name)
			case sync.PackageErrorMsg:
				b.Formatter.OnPackageError(m.Name, m.Err, m.Stderr)
			case sync.PackageHashMismatchMsg:
				b.Formatter.OnPackageHashMismatch(m.Name, m.OldCommand, m.NewCommand, m.Source)
			}
		},
	}

	// Set interactive approval when in TTY mode.
	// Skip in JSON mode — machine output cannot be interleaved with a blocking prompt.
	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	if isTTY && !b.TrustAllFlag && !b.JSONFlag {
		syncer.ApprovalFunc = func(name, command, source string) bool {
			var approved bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Package %q wants to run a shell command", name)).
				Description(fmt.Sprintf("source:  %s\ncommand: %s", source, command)).
				Affirmative("Approve").
				Negative("Deny").
				Value(&approved).
				Run()
			if err != nil {
				return false
			}
			return approved
		}
	}

	for _, teamRepo := range b.Repos {
		clear(resolved)
		b.Formatter.OnRegistryStart(teamRepo)

		if err := syncer.Run(ctx, teamRepo, b.State); err != nil {
			return err
		}

		if err := b.State.Save(); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}

	return nil
}
