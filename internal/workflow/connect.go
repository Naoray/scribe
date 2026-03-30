package workflow

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"

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
		{"SaveConfig", StepSaveConfig},
		{"LoadState", StepLoadState},
		{"SetSingleRepo", StepSetSingleRepo},
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTargets", StepResolveTargets},
		{"SyncSkills", StepConnectSyncError},
	}
}

// ConnectTail returns the connect steps starting from DedupCheck — for use
// by create-registry when Config and Client are already populated.
func ConnectTail() []Step {
	return ConnectSteps()[1:] // skip LoadConfig
}

func StepDedupCheck(_ context.Context, b *Bag) error {
	for _, existing := range b.Config.TeamRepos {
		if strings.EqualFold(existing, b.RepoArg) {
			fmt.Printf("Already connected to %s\n", existing)
			return errSkip
		}
	}
	return nil
}

func StepFetchManifest(ctx context.Context, b *Bag) error {
	owner, repo, err := parseOwnerRepo(b.RepoArg)
	if err != nil {
		return err
	}

	raw, err := b.Client.FetchFile(ctx, owner, repo, "scribe.toml", "HEAD")
	if err != nil {
		return fmt.Errorf("could not access %s: %w", b.RepoArg, err)
	}

	m, err := manifest.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid scribe.toml in %s: %w", b.RepoArg, err)
	}
	b.manifest = m
	return nil
}

func StepValidateManifest(_ context.Context, b *Bag) error {
	if !b.manifest.IsLoadout() {
		return fmt.Errorf("%s/scribe.toml has no [team] section — is this a skill package?", b.RepoArg)
	}
	return nil
}

func StepSaveConfig(_ context.Context, b *Bag) error {
	b.Config.TeamRepos = append(b.Config.TeamRepos, b.RepoArg)
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
		return nil
	}
	return nil
}

func parseOwnerRepo(s string) (string, string, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo %q: expected owner/repo (e.g. ArtistfyHQ/team-skills)", s)
	}
	return parts[0], parts[1], nil
}
