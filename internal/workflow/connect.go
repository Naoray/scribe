package workflow

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/registryindex"
	"github.com/Naoray/scribe/internal/source"
)

// ConnectSteps returns the step list for the connect command.
// It saves the registry config and shows available skills — it does NOT
// auto-install anything. Users install skills explicitly with `scribe add`.
func ConnectSteps() []Step {
	steps := append([]Step{}, connectBaseSteps(true)...)
	return append(steps, Step{Name: "ShowAvailable", Fn: StepShowAvailableSkills})
}

// ConnectInstallAllSteps returns the connect path that immediately installs
// every discovered skill from the just-connected registry.
func ConnectInstallAllSteps() []Step {
	steps := append([]Step{}, connectBaseSteps(true)...)
	return append(steps, connectInstallAllTail()...)
}

// ConnectInstallAllTail returns the connect + install-all path starting at
// ResolveFormatter, for callers that already loaded config/client state.
func ConnectInstallAllTail() []Step {
	steps := append([]Step{}, connectBaseSteps(false)...)
	return append(steps, connectInstallAllTail()...)
}

func connectBaseSteps(loadConfig bool) []Step {
	steps := make([]Step, 0, 7)
	if loadConfig {
		steps = append(steps, Step{Name: "LoadConfig", Fn: StepLoadConfig})
	}
	steps = append(steps,
		Step{Name: "ResolveFormatter", Fn: StepResolveFormatter},
		Step{Name: "DedupCheck", Fn: StepDedupCheck},
		Step{Name: "FetchManifest", Fn: StepFetchManifest},
		Step{Name: "ValidateManifest", Fn: StepValidateManifest},
		Step{Name: "InferRegistryType", Fn: StepInferRegistryType},
		Step{Name: "SaveConfig", Fn: StepSaveConfig},
		Step{Name: "IndexPublicRegistry", Fn: StepIndexPublicRegistry},
	)
	return steps
}

func connectInstallAllTail() []Step {
	return []Step{
		{Name: "LoadState", Fn: StepLoadState},
		{Name: "SetSingleRepo", Fn: StepSetSingleRepo},
		{Name: "ResolveTools", Fn: StepResolveTools},
		{Name: "ResolveProjectRoot", Fn: StepResolveProjectRoot},
		{Name: "ResolveKitFilter", Fn: StepResolveKitFilter},
		{Name: "SyncSkills", Fn: StepConnectSyncError},
	}
}

func StepDedupCheck(_ context.Context, b *Bag) error {
	targets := []string{b.RepoArg, b.SourceKey, b.SourceID}
	for _, src := range b.Config.EnabledSources() {
		existing := registryDisplayForWorkflow(src)
		for _, target := range targets {
			if target != "" && strings.EqualFold(existing, target) {
				b.Formatter.OnConnectDuplicate(existing)
				return errSkip
			}
			if target != "" && strings.EqualFold(src.Identity.Key, target) {
				b.Formatter.OnConnectDuplicate(existing)
				return errSkip
			}
			if target != "" && strings.EqualFold(src.ID, target) {
				b.Formatter.OnConnectDuplicate(existing)
				return errSkip
			}
		}
		if b.SourceKey != "" && strings.EqualFold(src.Identity.Key, b.SourceKey) {
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

	var result *provider.DiscoverResult
	var err error
	if b.SourceArg.Type != "" {
		sourceProvider, ok := b.Provider.(provider.SourceProvider)
		if !ok {
			return fmt.Errorf("internal: Provider does not support sources")
		}
		result, err = sourceProvider.DiscoverSource(ctx, b.SourceArg)
	} else {
		result, err = b.Provider.Discover(ctx, b.RepoArg)
	}
	if err != nil {
		return fmt.Errorf("could not discover skills in %s: %w", workflowSourceDisplay(b), err)
	}

	if result.Manifest != nil {
		b.manifest = result.Manifest
	} else {
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
	visibility := registryVisibility(ctx, b)

	writable := false
	if b.Client != nil && b.RepoArg != "" {
		owner, repo, err := manifest.ParseOwnerRepo(b.RepoArg)
		if err == nil {
			writable, _ = b.Client.HasPushAccess(ctx, owner, repo)
		}
	}

	if b.SourceArg.Type != "" {
		spec := b.SourceArg
		if b.SourceID != "" {
			spec.ID = b.SourceID
		}
		b.Config.AddRegistry(config.RegistryConfig{
			ID:         b.SourceID,
			Enabled:    true,
			Type:       regType,
			Visibility: visibility,
			Writable:   writable,
			Source:     &spec,
		})
	} else {
		b.Config.AddRegistry(config.RegistryConfig{
			Repo:       b.RepoArg,
			Enabled:    true,
			Type:       regType,
			Visibility: visibility,
			Writable:   writable,
		})
	}

	return nil
}

func registryVisibility(ctx context.Context, b *Bag) string {
	if b.Visibility == nil {
		return config.RegistryVisibilityUnknown
	}
	if b.SourceArg.Type != "" && b.SourceArg.Type != source.SourceGitHub {
		return config.RegistryVisibilityUnknown
	}
	owner, repo, err := manifest.ParseOwnerRepo(b.RepoArg)
	if err != nil {
		return config.RegistryVisibilityUnknown
	}
	private, err := b.Visibility.RepositoryIsPrivate(ctx, owner, repo)
	if err != nil {
		return config.RegistryVisibilityUnknown
	}
	if private {
		return config.RegistryVisibilityPrivate
	}
	return config.RegistryVisibilityPublic
}

func StepSaveConfig(_ context.Context, b *Bag) error {
	if err := b.Config.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	b.Formatter.OnConnectSaved(workflowSourceDisplay(b))
	return nil
}

func StepIndexPublicRegistry(ctx context.Context, b *Bag) error {
	if b.SourceArg.Type != "" && (b.SourceArg.Type != source.SourceGitHub || b.SourceArg.Path != "" || b.SourceArg.Ref != "") {
		return nil
	}
	return updateRegistryIndex(ctx, b, b.RepoArg, b.manifest)
}

func updateRegistryIndex(ctx context.Context, b *Bag, repo string, m *manifest.Manifest) error {
	if b == nil || b.Config == nil {
		return nil
	}
	rc := b.Config.FindRegistry(repo)
	if rc == nil {
		return nil
	}
	rc.Normalize()
	if !rc.IsPublic() {
		return nil
	}
	path, err := registryindex.Path()
	if err != nil {
		return err
	}
	entry, err := registryindex.BuildEntry(ctx, *rc, m, b.RegistryIndex)
	if err != nil {
		return fmt.Errorf("update registry index for %s: %w", repo, err)
	}
	if err := registryindex.Upsert(path, entry); err != nil {
		return fmt.Errorf("update registry index for %s: %w", repo, err)
	}
	return nil
}

// StepSetSingleRepo sets Repos to just the newly connected repo for the sync tail.
// Used by the connect install-all path.
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
	b.Formatter.OnConnectAvailable(workflowSourceDisplay(b), count)
	return nil
}

// StepConnectSyncError handles sync errors gracefully during connect.
// In TTY mode, sync failures are warnings; in non-TTY, they're fatal.
func StepConnectSyncError(ctx context.Context, b *Bag) error {
	// This step wraps SyncSkills with error recovery for connect.
	err := StepSyncSkills(ctx, b)
	if err != nil {
		b.Formatter.OnConnectSyncWarning(workflowSourceDisplay(b), err)
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

func workflowSourceDisplay(b *Bag) string {
	if b.SourceID != "" {
		return b.SourceID
	}
	if b.SourceArg.Type != "" {
		if b.SourceKey != "" {
			return b.SourceKey
		}
		if b.SourceArg.Repo != "" {
			return b.SourceArg.Repo
		}
		if b.SourceArg.URL != "" {
			return b.SourceArg.URL
		}
		return b.SourceArg.Path
	}
	return b.RepoArg
}

func registryDisplayForWorkflow(src config.RegistrySource) string {
	if src.ID != "" {
		return src.ID
	}
	if src.Config.Repo != "" {
		return src.Config.Repo
	}
	if src.Identity.Key != "" {
		return src.Identity.Key
	}
	return src.Source.Repo
}
