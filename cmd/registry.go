package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage connected skill registries",
	RunE:  runRegistryList,
	Args:  cobra.NoArgs,
}

func init() {
	registryCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	registryCmd.AddCommand(registryListCmd)
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

