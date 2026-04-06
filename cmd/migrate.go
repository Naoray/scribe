package cmd

import (
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/migrate"
)

// Styles for migrate output.
var (
	migHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	migNameStyle   = lipgloss.NewStyle().Bold(true)
	migDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	migAuthorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00BFFF"))
	migSourceStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	migDivStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	migOKStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	migBoxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 2).BorderForeground(lipgloss.Color("#7C3AED"))
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
		return fmt.Errorf("load config: %w", err)
	}

	var repo string
	if len(args) > 0 {
		repo = args[0]
	} else if len(cfg.TeamRepos()) == 1 {
		repo = cfg.TeamRepos()[0]
	} else {
		return fmt.Errorf("specify a registry: scribe migrate owner/repo")
	}

	owner, repoName, err := manifest.ParseOwnerRepo(repo)
	if err != nil {
		return fmt.Errorf("parse registry %q: %w", repo, err)
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
		return fmt.Errorf("convert manifest: %w", err)
	}

	encoded, err := converted.Encode()
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	// Show preview — styled for TTY, plain YAML for pipes.
	if isatty.IsTerminal(os.Stdout.Fd()) {
		renderMigratePreview(owner+"/"+repoName, converted)
	} else {
		fmt.Printf("Converted %s/%s:\n\n%s\n", owner, repoName, string(encoded))
	}

	if isatty.IsTerminal(os.Stdin.Fd()) {
		var confirm bool
		err := huh.NewConfirm().
			Title("Push this migration?").
			Affirmative("Push").
			Negative("Cancel").
			Value(&confirm).
			Run()
		if err != nil || !confirm {
			fmt.Println(migDimStyle.Render("Aborted."))
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

	fmt.Println()
	fmt.Println(migOKStyle.Render("✓") + " Migrated " + migNameStyle.Render(owner+"/"+repoName) + " to " + manifest.ManifestFilename)
	return nil
}

func renderMigratePreview(repoSlug string, m *manifest.Manifest) {
	fmt.Println()

	// Header box with registry info.
	var headerLines []string
	headerLines = append(headerLines, migHeaderStyle.Render("Migration Preview"))
	headerLines = append(headerLines, "")
	headerLines = append(headerLines, migDimStyle.Render("repo")+"     "+migNameStyle.Render(repoSlug))
	headerLines = append(headerLines, migDimStyle.Render("format")+"   "+migDimStyle.Render("scribe.toml")+" → "+migNameStyle.Render("scribe.yaml"))
	if m.Team != nil {
		headerLines = append(headerLines, migDimStyle.Render("team")+"     "+m.Team.Name)
	}
	if m.Package != nil {
		headerLines = append(headerLines, migDimStyle.Render("package")+"  "+m.Package.Name)
	}
	headerLines = append(headerLines, migDimStyle.Render("entries")+"  "+fmt.Sprintf("%d", len(m.Catalog)))
	fmt.Println(migBoxStyle.Render(strings.Join(headerLines, "\n")))
	fmt.Println()

	if len(m.Catalog) == 0 {
		fmt.Println(migDimStyle.Render("  No catalog entries."))
		fmt.Println()
		return
	}

	// Calculate column widths for aligned output.
	maxName, maxAuthor := 4, 6 // minimum: "name", "author"
	for _, e := range m.Catalog {
		if w := runewidth.StringWidth(e.Name); w > maxName {
			maxName = w
		}
		author := e.Author
		if author == "" {
			author = "—"
		}
		if w := runewidth.StringWidth(author); w > maxAuthor {
			maxAuthor = w
		}
	}

	// Table header.
	header := fmt.Sprintf("  %s  %s  %s",
		migDimStyle.Render(runewidth.FillRight("name", maxName)),
		migDimStyle.Render(runewidth.FillRight("author", maxAuthor)),
		migDimStyle.Render("source"),
	)
	fmt.Println(header)
	fmt.Println(migDivStyle.Render("  " + strings.Repeat("─", maxName+maxAuthor+40)))

	// Entries.
	for _, e := range m.Catalog {
		name := migNameStyle.Render(runewidth.FillRight(e.Name, maxName))
		author := e.Author
		if author == "" {
			author = "—"
		}
		authorStr := migAuthorStyle.Render(runewidth.FillRight(author, maxAuthor))

		source := migSourceStyle.Render(truncateSource(e.Source, 40))
		fmt.Printf("  %s  %s  %s\n", name, authorStr, source)
	}
	fmt.Println()
}

// truncateSource shortens a source string for display.
func truncateSource(s string, maxWidth int) string {
	if s == "" {
		return "—"
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	return runewidth.Truncate(s, maxWidth, "…")
}
