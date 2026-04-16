package workflow

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
)

// ConnectSteps returns the step list for the connect command.
// It saves the registry config and shows available skills — it does NOT
// auto-install anything. Users install skills explicitly with `scribe add`.
func ConnectSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"ResolveFormatter", StepResolveFormatter},
		{"DedupCheck", StepDedupCheck},
		{"FetchManifest", StepFetchManifest},
		{"ValidateManifest", StepValidateManifest},
		{"InferRegistryType", StepInferRegistryType},
		{"SaveConfig", StepSaveConfig},
		{"ShowAvailable", StepShowAvailableSkills},
	}
}

// ConnectAndSyncTail returns connect + sync steps starting from
// ResolveFormatter, for use by create-registry where the user just
// authored the skills and wants them installed immediately.
func ConnectAndSyncTail() []Step {
	return []Step{
		{"ResolveFormatter", StepResolveFormatter},
		{"DedupCheck", StepDedupCheck},
		{"FetchManifest", StepFetchManifest},
		{"ValidateManifest", StepValidateManifest},
		{"InferRegistryType", StepInferRegistryType},
		{"SaveConfig", StepSaveConfig},
		{"LoadState", StepLoadState},
		{"SetSingleRepo", StepSetSingleRepo},
		{"ResolveTools", StepResolveTools},
		{"SyncSkills", StepConnectSyncError},
	}
}

func StepDedupCheck(_ context.Context, b *Bag) error {
	for _, existing := range b.Config.TeamRepos() {
		if strings.EqualFold(existing, b.RepoArg) {
			b.Formatter.OnConnectDuplicate(existing)
			return errSkip
		}
	}
	return nil
}

func StepFetchManifest(ctx context.Context, b *Bag) error {
	if b.Provider == nil {
		return fmt.Errorf("internal: Provider not set in workflow bag")
	}

	result, err := b.Provider.Discover(ctx, b.RepoArg)
	if err != nil {
		return fmt.Errorf("could not discover skills in %s: %w", b.RepoArg, err)
	}

	// Build a minimal manifest from discovered entries.
	b.manifest = &manifest.Manifest{
		APIVersion: "scribe/v1",
		Kind:       "Registry",
		Catalog:    result.Entries,
	}
	// Only set Team if discovery found an actual team manifest (scribe.yaml/toml).
	if result.IsTeam {
		b.manifest.Team = &manifest.Team{Name: b.RepoArg}
	}
	return nil
}

func StepValidateManifest(_ context.Context, b *Bag) error {
	if b.manifest == nil || len(b.manifest.Catalog) == 0 {
		return fmt.Errorf("%s: no skills discovered — is this a valid skill registry?", b.RepoArg)
	}
	return nil
}

func StepInferRegistryType(ctx context.Context, b *Bag) error {
	regType := config.RegistryTypeCommunity
	if b.manifest.IsRegistry() {
		regType = config.RegistryTypeTeam
	}

	writable := false
	if b.Client != nil {
		owner, repo, err := manifest.ParseOwnerRepo(b.RepoArg)
		if err == nil {
			writable, _ = b.Client.HasPushAccess(ctx, owner, repo)
		}
	}

	b.Config.AddRegistry(config.RegistryConfig{
		Repo:     b.RepoArg,
		Enabled:  true,
		Type:     regType,
		Writable: writable,
	})

	return nil
}

func StepSaveConfig(_ context.Context, b *Bag) error {
	if err := b.Config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	b.Formatter.OnConnectSaved(b.RepoArg)
	return nil
}

// StepSetSingleRepo sets Repos to just the newly connected repo for the sync tail.
// Used by ConnectAndSyncTail (create-registry path).
func StepSetSingleRepo(_ context.Context, b *Bag) error {
	b.Repos = []string{b.RepoArg}
	b.Formatter.OnConnectSyncing()
	return nil
}

// StepShowAvailableSkills prints how many skills the registry offers and
// tells the user how to install them. Used by the plain connect path.
func StepShowAvailableSkills(_ context.Context, b *Bag) error {
	count := 0
	if b.manifest != nil {
		count = len(b.manifest.Catalog)
	}
	b.Formatter.OnConnectAvailable(b.RepoArg, count)
	return nil
}

// StepConnectSyncError handles sync errors gracefully during connect.
// In TTY mode, sync failures are warnings; in non-TTY, they're fatal.
func StepConnectSyncError(ctx context.Context, b *Bag) error {
	// This step wraps SyncSkills with error recovery for connect.
	err := StepSyncSkills(ctx, b)
	if err != nil {
		b.Formatter.OnConnectSyncWarning(b.RepoArg, err)
		if !isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("sync failed: %w", err)
		}
		// Prevent Run() from calling Flush(), which would print a misleading
		// "done: 0 installed..." summary after the warning.
		b.Formatter = nil
		return nil
	}
	return nil
}
