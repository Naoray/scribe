package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

func newRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <skill>",
		Short: "Uninstall a skill from the local machine",
		Long: `Remove a skill by name or full key (e.g. "deploy" or "Artistfy-hq/deploy").

If the bare name is ambiguous across registries, an interactive picker is shown
(or an error in non-TTY mode). Skills managed by a registry will be re-installed
on the next sync unless the registry is disconnected.`,
		Args: cobra.ExactArgs(1),
		RunE: runRemove,
	}
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

// removeResult is the JSON output for `scribe remove`.
type removeResult struct {
	Removed   string   `json:"removed"`
	ManagedBy []string `json:"managed_by,omitempty"`
	Errors    []string `json:"errors,omitempty"`
}

func runRemove(cmd *cobra.Command, args []string) error {
	input := args[0]
	yes, _ := cmd.Flags().GetBool("yes")
	jsonFlag, _ := cmd.Flags().GetBool("json")
	useJSON := jsonFlag || !isatty.IsTerminal(os.Stdout.Fd())
	isTTY := isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())

	// Load state.
	st, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Collect installed keys.
	installedKeys := make([]string, 0, len(st.Installed))
	for k := range st.Installed {
		installedKeys = append(installedKeys, k)
	}
	sort.Strings(installedKeys)

	// Resolve the target.
	key, err := resolveRemoveTarget(input, installedKeys)
	if err != nil {
		// If ambiguous and TTY, show a picker.
		if isTTY && !useJSON && strings.Contains(err.Error(), "ambiguous") {
			key, err = pickRemoveTarget(input, installedKeys)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	installed := st.Installed[key]

	// Check if managed by a registry.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	managedBy := findManagingRegistries(key, cfg.Registries)

	// Warn about registry re-install.
	if len(managedBy) > 0 && !useJSON {
		fmt.Fprintf(os.Stderr, "⚠  %s is managed by %s — it will re-install on next sync\n",
			key, strings.Join(managedBy, ", "))
	}

	// Confirmation prompt.
	if !yes && !useJSON {
		if isTTY {
			var confirm bool
			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Remove %s?", key)).
				Value(&confirm).
				Run(); err != nil {
				return err
			}
			if !confirm {
				fmt.Fprintln(os.Stderr, "Aborted.")
				return nil
			}
		} else {
			return fmt.Errorf("use --yes to confirm removal in non-interactive mode")
		}
	}

	// Uninstall from all tools.
	var errs []string
	detectedTools := tools.DetectTools()
	for _, tool := range detectedTools {
		// Only uninstall from tools that had the skill installed.
		for _, t := range installed.Tools {
			if t == tool.Name() {
				if err := tool.Uninstall(key); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", tool.Name(), err))
				}
				break
			}
		}
	}

	// Remove from canonical store.
	storeDir, err := tools.StoreDir()
	if err == nil {
		skillDir := filepath.Join(storeDir, key)
		if err := os.RemoveAll(skillDir); err != nil {
			errs = append(errs, fmt.Sprintf("store: %v", err))
		}
	}

	// Remove any remaining symlinks from installed.Paths.
	for _, p := range installed.Paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("symlink %s: %v", p, err))
		}
	}

	// Remove from state and save.
	st.Remove(key)
	if err := st.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	// Output.
	if useJSON {
		result := removeResult{
			Removed:   key,
			ManagedBy: managedBy,
			Errors:    errs,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "Removed %s with warnings:\n", key)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  • %s\n", e)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Removed %s\n", key)
	}
	return nil
}

// resolveRemoveTarget matches input against installed skill keys.
// Exact namespaced match → bare name unique match → ambiguous error.
func resolveRemoveTarget(input string, installedKeys []string) (string, error) {
	// Exact match.
	for _, k := range installedKeys {
		if k == input {
			return k, nil
		}
	}

	// Bare name match — look at the part after the slash.
	var matches []string
	for _, k := range installedKeys {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 && parts[1] == input {
			matches = append(matches, k)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("%q is not installed", input)
	default:
		return "", fmt.Errorf("ambiguous skill %q — matches:\n  %s\nSpecify the full key to remove", input, strings.Join(matches, "\n  "))
	}
}

// pickRemoveTarget shows an interactive picker for ambiguous skill names.
func pickRemoveTarget(input string, installedKeys []string) (string, error) {
	var matches []string
	for _, k := range installedKeys {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 && parts[1] == input {
			matches = append(matches, k)
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("%q is not installed", input)
	}

	opts := make([]huh.Option[string], len(matches))
	for i, m := range matches {
		opts[i] = huh.NewOption(m, m)
	}

	var selected string
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Multiple skills match %q — which one?", input)).
		Options(opts...).
		Value(&selected).
		Run()
	if err != nil {
		return "", err
	}
	return selected, nil
}

// findManagingRegistries returns registry repos that manage the given skill key.
// A skill key like "Artistfy-hq/deploy" is managed by a registry whose repo
// slugifies to match the namespace prefix (e.g. "Artistfy/hq" → "Artistfy-hq").
func findManagingRegistries(key string, registries []config.RegistryConfig) []string {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) < 2 {
		return nil
	}
	namespace := parts[0]

	var managed []string
	for _, r := range registries {
		slug := strings.ReplaceAll(r.Repo, "/", "-")
		if slug == namespace {
			managed = append(managed, r.Repo)
		}
	}
	return managed
}
