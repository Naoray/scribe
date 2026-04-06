package workflow

import (
	"context"
	"fmt"
	"os"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/provider"
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
		{"SyncSkills", StepSyncSkills},
	}
}

// SyncTail returns the shared tail of steps reused by connect and create-registry.
func SyncTail() []Step {
	return []Step{
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"SyncSkills", StepSyncSkills},
	}
}

func StepLoadConfig(ctx context.Context, b *Bag) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	b.Config = cfg
	b.Client = gh.NewClient(ctx, cfg.Token)

	// Wrap the GitHub client into a Provider for discovery/fetch.
	b.Provider = provider.NewGitHubProvider(provider.WrapGitHubClient(b.Client))

	return nil
}

func StepLoadState(_ context.Context, b *Bag) error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	b.State = st
	return nil
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
		b.Tools = tools.DetectTools()
	}
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
				b.Formatter.OnSkillInstalled(m.Name, m.Version, m.Updated)
			case sync.SkillErrorMsg:
				b.Formatter.OnSkillError(m.Name, m.Err)
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
	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	if isTTY && !b.TrustAllFlag {
		syncer.ApprovalFunc = func(name, command, source string) bool {
			var approved bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Package %q wants to run:", name)).
				Description(command).
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
