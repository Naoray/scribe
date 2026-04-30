package cmd

import (
	"encoding/json"
	"errors"
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
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

// errAmbiguousSkill is returned when a bare skill name matches multiple installed keys.
var errAmbiguousSkill = errors.New("ambiguous skill")

func newRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <skill>",
		Short: "Uninstall a skill from the local machine",
		Long: `Remove a skill by name or full key (e.g. "deploy" or "Artistfy-hq/deploy").

If the bare name is ambiguous across registries, an interactive picker is shown
(or an error in non-TTY mode). Removing a registry-managed skill records that
intent so future syncs keep it removed until you install it again.`,
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
	factory := newCommandFactory()

	// Load state.
	st, err := factory.State()
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
		if isTTY && !useJSON && errors.Is(err, errAmbiguousSkill) {
			key, err = pickRemoveTarget(input, installedKeys)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	installed := st.Installed[key]

	// Load config for tool resolution.
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	managedBy := registriesFromSources(installed.Sources)

	if len(managedBy) > 0 && !useJSON {
		fmt.Fprintf(os.Stderr, "⚠  %s is managed by %s — future syncs will keep it removed until you install it again\n",
			key, strings.Join(managedBy, ", "))
	}

	// Confirmation prompt.
	if !yes {
		if isTTY && !useJSON {
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

	var errs []string

	// Packages self-manage: best-effort uninstall command, then drop the
	// package dir. Skip tool uninstallers and projection cleanup because
	// packages were never projected in the first place.
	if installed.Kind == state.KindPackage {
		if uninstallErrs := runPackageUninstall(cmd, key, installed); len(uninstallErrs) > 0 {
			errs = append(errs, uninstallErrs...)
		}
		pkgsDir, pdErr := tools.PackagesDir()
		if pdErr == nil {
			pkgDir := filepath.Join(pkgsDir, key)
			if err := os.RemoveAll(pkgDir); err != nil {
				errs = append(errs, fmt.Sprintf("package store: %v", err))
			}
		}
	} else {
		// Uninstall from every tool that originally installed the skill, even if
		// that tool is now disabled or missing from config. Otherwise disabling a
		// tool after install would orphan its side of the skill on disk.
		for _, name := range installed.Tools {
			tool, err := tools.ResolveByName(cfg, name)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if err := tool.Uninstall(key); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
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

		// Remove only the projections Scribe still believes it manages.
		managedPaths := installed.ManagedPaths
		if len(managedPaths) == 0 {
			managedPaths = installed.Paths
		}
		for _, p := range managedPaths {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("managed path %s: %v", p, err))
			}
		}
	}

	// Remove from state, record registry removal intent, and save.
	st.RecordRemovedByUser(key, installed.Sources)
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
		return fmt.Errorf("removed %s with %d warning(s)", key, len(errs))
	}

	fmt.Fprintf(os.Stderr, "Removed %s\n", key)
	return nil
}

func registriesFromSources(sources []state.SkillSource) []string {
	seen := map[string]bool{}
	var registries []string
	for _, src := range sources {
		if src.Registry == "" || seen[src.Registry] {
			continue
		}
		seen[src.Registry] = true
		registries = append(registries, src.Registry)
	}
	sort.Strings(registries)
	return registries
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
	matches := findBareNameMatches(input, installedKeys)

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("%q is not installed", input)
	default:
		return "", fmt.Errorf("%w %q — matches: %s\nSpecify the full name, e.g.: scribe remove %s",
			errAmbiguousSkill, input, strings.Join(matches, ", "), matches[0])
	}
}

// pickRemoveTarget shows an interactive picker for ambiguous skill names.
func pickRemoveTarget(input string, installedKeys []string) (string, error) {
	matches := findBareNameMatches(input, installedKeys)

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

// findBareNameMatches returns all installed keys whose bare name (after the slash)
// matches input case-insensitively.
func findBareNameMatches(input string, keys []string) []string {
	var matches []string
	for _, k := range keys {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) == 2 && strings.EqualFold(parts[1], input) {
			matches = append(matches, k)
		}
	}
	return matches
}

// runPackageUninstall executes the package's declared uninstall command, if
// any. Resolution mirrors the install side: scribe.yaml's install.uninstall
// field, else an uninstall.sh at the package root. Best-effort — non-zero
// exit is returned as a warning, never fatal.
func runPackageUninstall(cmd *cobra.Command, name string, installed state.InstalledSkill) []string {
	pkgsDir, err := tools.PackagesDir()
	if err != nil {
		return []string{fmt.Sprintf("packages dir: %v", err)}
	}
	pkgDir := filepath.Join(pkgsDir, name)

	// Best-effort: skip silently if the dir is already gone.
	if _, err := os.Stat(pkgDir); err != nil {
		return nil
	}

	// Prefer scribe.yaml → install.uninstall; fall back to uninstall.sh.
	var uninstallCmd string
	if plan, err := sync.ResolvePackageInstall(pkgDir); err == nil && plan.Uninstall != "" {
		uninstallCmd = plan.Uninstall
	} else if _, err := os.Stat(filepath.Join(pkgDir, "uninstall.sh")); err == nil {
		uninstallCmd = "sh uninstall.sh"
	}
	if uninstallCmd == "" {
		return nil
	}

	exec := &sync.ShellExecutor{}
	_, stderr, err := sync.RunPackageCommand(cmd.Context(), exec, pkgDir, uninstallCmd, 0)
	if err != nil {
		warn := fmt.Sprintf("uninstall command: %v", err)
		if stderr != "" {
			warn += " (" + strings.TrimSpace(stderr) + ")"
		}
		return []string{warn}
	}
	return nil
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
