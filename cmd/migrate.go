package cmd

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate [owner/repo]",
	Short: "Convert a scribe.toml registry to scribe.yaml",
	Long: `Fetches the existing scribe.toml from a registry, converts it to the
new scribe.yaml format, and pushes the change as a single commit
(deleting scribe.toml and creating scribe.yaml).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMigrate,
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var repo string
	if len(args) > 0 {
		repo = args[0]
	} else if len(cfg.TeamRepos) == 1 {
		repo = cfg.TeamRepos[0]
	} else {
		return fmt.Errorf("specify a registry: scribe migrate owner/repo")
	}

	owner, repoName, err := manifest.ParseOwnerRepo(repo)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	client := gh.NewClient(ctx, cfg.Token)
	if !client.IsAuthenticated() {
		return fmt.Errorf("authentication required — run `gh auth login` or set GITHUB_TOKEN")
	}

	// Check if scribe.yaml already exists.
	exists, err := client.FileExists(ctx, owner, repoName, manifest.ManifestFilename, "HEAD")
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s/%s already has a %s — nothing to migrate", owner, repoName, manifest.ManifestFilename)
	}

	// Fetch and convert.
	raw, err := client.FetchFile(ctx, owner, repoName, manifest.LegacyManifestFilename, "HEAD")
	if err != nil {
		return fmt.Errorf("fetch scribe.toml: %w", err)
	}

	converted, err := migrate.Convert(raw)
	if err != nil {
		return err
	}

	encoded, err := converted.Encode()
	if err != nil {
		return err
	}

	// Show preview.
	fmt.Printf("Converted %s/%s:\n\n%s\n", owner, repoName, string(encoded))

	if isatty.IsTerminal(os.Stdin.Fd()) {
		fmt.Print("Push this change? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Push: delete scribe.toml, create scribe.yaml.
	files := map[string]string{
		manifest.ManifestFilename:       string(encoded),
		manifest.LegacyManifestFilename: "", // empty string = delete
	}
	err = client.PushFiles(ctx, owner, repoName, files, "migrate: scribe.toml → scribe.yaml")
	if err != nil {
		return fmt.Errorf("push migration: %w", err)
	}

	fmt.Printf("✓ Migrated %s/%s to %s\n", owner, repoName, manifest.ManifestFilename)
	return nil
}
