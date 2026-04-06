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
// The tail reuses sync steps for the auto-sync after connecting,
// with error recovery (sync failures during connect are warnings in TTY mode).
func ConnectSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"DedupCheck", StepDedupCheck},
		{"FetchManifest", StepFetchManifest},
		{"ValidateManifest", StepValidateManifest},
		{"InferRegistryType", StepInferRegistryType},
		{"SaveConfig", StepSaveConfig},
		{"LoadState", StepLoadState},
		{"SetSingleRepo", StepSetSingleRepo},
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"SyncSkills", StepConnectSyncError},
	}
}

// ConnectTail returns the connect steps starting from DedupCheck — for use
// by create-registry when Config and Client are already populated.
func ConnectTail() []Step {
	return ConnectSteps()[1:] // skip LoadConfig
}

func StepDedupCheck(_ context.Context, b *Bag) error {
	for _, existing := range b.Config.TeamRepos() {
		if strings.EqualFold(existing, b.RepoArg) {
			fmt.Printf("Already connected to %s\n", existing)
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
	regType := "community"
	if b.manifest.IsRegistry() {
		regType = "team"
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
	fmt.Printf("Connected to %s\n", b.RepoArg)
	return nil
}

// StepSetSingleRepo sets Repos to just the newly connected repo for the sync tail.
func StepSetSingleRepo(_ context.Context, b *Bag) error {
	b.Repos = []string{b.RepoArg}
	fmt.Printf("\nsyncing skills...\n\n")
	return nil
}

// StepConnectSyncError handles sync errors gracefully during connect.
// In TTY mode, sync failures are warnings; in non-TTY, they're fatal.
func StepConnectSyncError(ctx context.Context, b *Bag) error {
	// This step wraps SyncSkills with error recovery for connect.
	err := StepSyncSkills(ctx, b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: sync failed for %s: %v\n", b.RepoArg, err)
		fmt.Fprintf(os.Stderr, "run `scribe sync` to retry\n")
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

