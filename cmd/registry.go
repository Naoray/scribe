package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage connected skill registries",
		RunE:  runRegistryList,
		Args:  cobra.NoArgs,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.AddCommand(newRegistryListCommand())
	cmd.AddCommand(newConnectCommand()) // connect lives under registry
	cmd.AddCommand(newRegistryEnableCommand())
	cmd.AddCommand(newRegistryDisableCommand())
	cmd.AddCommand(newRegistryAddCommand())
	cmd.AddCommand(newRegistryCreateCommand())
	cmd.AddCommand(newRegistryMigrateCommand())
	return cmd
}

// newRegistryMigrateCommand wires `scribe registry migrate` to the existing
// migrate workflow.
func newRegistryMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate [owner/repo]",
		Short: "Convert a scribe.toml registry to scribe.yaml",
		Long: `Fetches the existing scribe.toml from a registry, converts it to the
new scribe.yaml format, and pushes the change as a single commit
(deleting scribe.toml and creating scribe.yaml).`,
		Args: cobra.MaximumNArgs(1),
		RunE: runMigrate,
	}
}

// newRegistryCreateCommand wires `scribe registry create` to the existing
// create-registry workflow.
func newRegistryCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Scaffold a new team skills registry on GitHub",
		Long: `Create a new GitHub repository with a scribe.yaml manifest and connect to it.

Examples:
  scribe registry create                                    # interactive
  scribe registry create -t myteam -o MyOrg                 # flags
  scribe registry create -t myteam -o MyOrg -r skills-repo  # custom repo name`,
		RunE: runCreateRegistry,
	}
	cmd.Flags().StringP("team", "t", "", "Team name")
	cmd.Flags().StringP("owner", "o", "", "GitHub org or username")
	cmd.Flags().StringP("repo", "r", "team-registry", "Repository name")
	cmd.Flags().Bool("private", true, "Create a private repository")
	return cmd
}

// resolveRegistry matches a user-provided registry string against connected repos.
// Accepts full "owner/repo" (case-insensitive) or partial "repo" name if unambiguous.
func resolveRegistry(input string, repos []string) (string, error) {
	// Try exact match first (case-insensitive).
	for _, r := range repos {
		if strings.EqualFold(r, input) {
			return r, nil
		}
	}

	// Try partial match on repo name (the part after the slash).
	var matches []string
	for _, r := range repos {
		parts := strings.SplitN(r, "/", 2)
		if len(parts) == 2 && strings.EqualFold(parts[1], input) {
			matches = append(matches, r)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("not connected to %q — run: scribe connect %s", input, input)
	default:
		return "", fmt.Errorf("ambiguous registry %q — did you mean:\n  %s", input, strings.Join(matches, "\n  "))
	}
}

// filterRegistries returns the subset of repos to operate on, based on the --registry flag.
// If flag is empty, returns all repos. Otherwise resolves and returns a single-element slice.
func filterRegistries(flag string, repos []string) ([]string, error) {
	if flag == "" {
		return repos, nil
	}
	resolved, err := resolveRegistry(flag, repos)
	if err != nil {
		return nil, err
	}
	return []string{resolved}, nil
}
