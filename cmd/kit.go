package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/cli/fields"
	"github.com/Naoray/scribe/internal/cli/output"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/registry"
)

type kitCreateOptions struct {
	description string
	skills      []string
	mcpServers  []string
	registry    string
	force       bool
	json        bool
}

type kitListOptions struct {
	remote   bool
	registry string
}

type kitCreateOutput struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	SkillsCount     int    `json:"skills_count"`
	MCPServersCount int    `json:"mcp_servers_count"`
}

type kitListOutput struct {
	Kits any `json:"kits"`
}

type kitListItem struct {
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	SkillsCount      int      `json:"skills_count"`
	Skills           []string `json:"skills,omitempty"`
	Registry         string   `json:"registry,omitempty"`
	Path             string   `json:"path,omitempty"`
	Author           string   `json:"author,omitempty"`
	Remote           bool     `json:"remote,omitempty"`
	InstalledLocally bool     `json:"installed_locally,omitempty"`
}

type kitShowOutput struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Skills      []string         `json:"skills"`
	Source      *kitSourceOutput `json:"source,omitempty"`
	Registry    string           `json:"registry,omitempty"`
	Refs        []kitRefOutput   `json:"refs,omitempty"`
}

type kitSourceOutput struct {
	Registry string `json:"registry"`
	Rev      string `json:"rev,omitempty"`
}

type kitRefOutput struct {
	Raw       string `json:"raw"`
	Skill     string `json:"skill"`
	Origin    string `json:"origin"`
	Registry  string `json:"registry,omitempty"`
	Connected bool   `json:"connected"`
	Glob      bool   `json:"glob,omitempty"`
	Local     bool   `json:"local,omitempty"`
	Source    string `json:"source,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

var listRemoteKitsFn = registry.ListKits
var findRemoteKitFn = registry.FindKit
var fetchRemoteKitFn = registry.FetchKitBody

var kitListFieldSet = fields.FieldSet[kitListItem]{
	"name": func(item kitListItem) any {
		return item.Name
	},
	"description": func(item kitListItem) any {
		return item.Description
	},
	"skills_count": func(item kitListItem) any {
		return item.SkillsCount
	},
	"skills": func(item kitListItem) any {
		return item.Skills
	},
	"registry": func(item kitListItem) any {
		return item.Registry
	},
	"path": func(item kitListItem) any {
		return item.Path
	},
}

func newKitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kit",
		Short: "Manage local skill kits",
	}
	cmd.AddCommand(newKitCreateCommand())
	cmd.AddCommand(newKitListCommand())
	cmd.AddCommand(newKitShowCommand())
	return cmd
}

func newKitCreateCommand() *cobra.Command {
	opts := &kitCreateOptions{}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a local skill kit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.json = jsonFlagPassed(cmd)
			return runKitCreate(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.description, "description", "", "Kit description")
	cmd.Flags().StringSliceVar(&opts.skills, "skills", nil, "Comma-separated skill names")
	cmd.Flags().StringSliceVar(&opts.mcpServers, "mcp-servers", nil, "Comma-separated MCP server names")
	cmd.Flags().StringVar(&opts.registry, "registry", "", "Source registry for this kit")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite an existing kit")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func newKitListCommand() *cobra.Command {
	opts := &kitListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local skill kits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKitListWithOptions(cmd, args, opts)
		},
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().BoolVar(&opts.remote, "remote", false, "List kits from connected registries")
	cmd.Flags().StringVar(&opts.registry, "registry", "", "Limit remote kits to one connected registry")
	output.AttachFieldsFlag(cmd, kitListFieldSet)
	return markJSONSupported(cmd)
}

func newKitShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a local skill kit",
		Args:  cobra.ExactArgs(1),
		RunE:  runKitShow,
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return markJSONSupported(cmd)
}

func runKitCreate(cmd *cobra.Command, name string, opts *kitCreateOptions) error {
	if opts == nil {
		opts = &kitCreateOptions{}
	}

	if err := validateKitName(name); err != nil {
		return err
	}

	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return fmt.Errorf("resolve scribe dir: %w", err)
	}
	kitPath := filepath.Join(scribeDir, "kits", name+".yaml")

	if !opts.force {
		if _, err := os.Stat(kitPath); err == nil {
			return clierrors.Wrap(fmt.Errorf("kit %q already exists", name), "KIT_EXISTS", clierrors.ExitConflict,
				clierrors.WithResource(kitPath),
				clierrors.WithRemediation("Use --force to overwrite the existing kit."),
			)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check kit path: %w", err)
		}
	}

	k := kit.Kit{
		Name:        name,
		Description: opts.description,
		Skills:      opts.skills,
		MCPServers:  opts.mcpServers,
	}
	if opts.registry != "" {
		k.Source = &kit.Source{Registry: opts.registry}
	}

	if err := kit.Save(kitPath, &k); err != nil {
		return err
	}

	out := kitCreateOutput{
		Name:            name,
		Path:            kitPath,
		SkillsCount:     len(opts.skills),
		MCPServersCount: len(opts.mcpServers),
	}
	if opts.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created kit %s at %s with %d skills and %d MCP servers\n", out.Name, out.Path, out.SkillsCount, out.MCPServersCount)
	return nil
}

func runKitList(cmd *cobra.Command, args []string) error {
	return runKitListWithOptions(cmd, args, &kitListOptions{})
}

func runKitListWithOptions(cmd *cobra.Command, args []string, opts *kitListOptions) error {
	if opts == nil {
		opts = &kitListOptions{}
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return fmt.Errorf("resolve scribe dir: %w", err)
	}
	loaded, err := kit.LoadAll(filepath.Join(scribeDir, "kits"))
	if err != nil {
		return err
	}

	items := kitListItems(loaded)
	if opts.remote || opts.registry != "" {
		remote, err := remoteKitListItems(cmd, opts, loaded)
		if err != nil {
			return err
		}
		items = append(items, remote...)
		sort.Slice(items, func(i, j int) bool {
			if items[i].Registry != items[j].Registry {
				return items[i].Registry < items[j].Registry
			}
			return items[i].Name < items[j].Name
		})
	}
	fieldsFlag, _ := cmd.Flags().GetString("fields")
	projected, err := projectKitListItems(items, fieldsFlag)
	if err != nil {
		return err
	}

	if jsonFlagPassed(cmd) {
		r := jsonRendererForCommand(cmd, true)
		if err := r.Result(kitListOutput{Kits: projected}); err != nil {
			return err
		}
		return r.Flush()
	}

	return printKitList(cmd, items, fieldsFlag)
}

func runKitShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	if strings.Contains(name, ":") {
		return runKitShowRemote(cmd, name)
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return fmt.Errorf("resolve scribe dir: %w", err)
	}

	k, err := kit.Load(filepath.Join(scribeDir, "kits", name+".yaml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return clierrors.Wrap(fmt.Errorf("kit %q not found", name), "KIT_NOT_FOUND", clierrors.ExitNotFound,
				clierrors.WithResource(name),
				clierrors.WithRemediation("Run `scribe kit list` to see available kits."),
			)
		}
		return err
	}

	out := kitShowOutputFromKit(k)
	if jsonFlagPassed(cmd) {
		r := jsonRendererForCommand(cmd, true)
		if err := r.Result(out); err != nil {
			return err
		}
		return r.Flush()
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Kit: %s\n", out.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", out.Description)
	fmt.Fprintf(cmd.OutOrStdout(), "Skills (%d): %s\n", len(out.Skills), strings.Join(out.Skills, ", "))
	fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\n", kitSourceLabel(out.Source))
	return nil
}

func remoteKitListItems(cmd *cobra.Command, opts *kitListOptions, local map[string]*kit.Kit) ([]kitListItem, error) {
	factory := commandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	client, err := factory.Client()
	if err != nil {
		return nil, fmt.Errorf("load github client: %w", err)
	}
	repos, err := remoteKitRepos(opts.registry, cfg)
	if err != nil {
		return nil, err
	}

	var items []kitListItem
	for _, repo := range repos {
		kits, err := listRemoteKitsFn(cmd.Context(), client, repo)
		if err != nil {
			return nil, fmt.Errorf("list kits from %s: %w", repo, err)
		}
		for _, remote := range kits {
			_, installed := local[remote.Name]
			items = append(items, kitListItem{
				Name:             remote.Name,
				Description:      remote.Description,
				Registry:         remote.Registry,
				Path:             remote.Path,
				Author:           remote.Author,
				Remote:           true,
				InstalledLocally: installed,
			})
		}
	}
	return items, nil
}

func remoteKitRepos(registryFilter string, cfg *config.Config) ([]string, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if registryFilter == "" {
		return cfg.TeamRepos(), nil
	}
	repo, err := resolveRegistry(registryFilter, cfg.TeamRepos())
	if err != nil {
		return nil, clierrors.Wrap(fmt.Errorf("registry %q is not connected", registryFilter), "REGISTRY_NOT_CONNECTED", clierrors.ExitNotFound,
			clierrors.WithResource(registryFilter),
			clierrors.WithRemediation("Run `scribe registry connect "+registryFilter+"` first."),
		)
	}
	return []string{repo}, nil
}

func runKitShowRemote(cmd *cobra.Command, ref string) error {
	registryRepo, kitName, err := parseSkillRef(ref)
	if err != nil {
		return err
	}
	factory := commandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !registryConnected(cfg, registryRepo) {
		return clierrors.Wrap(fmt.Errorf("registry %q is not connected", registryRepo), "REGISTRY_NOT_CONNECTED", clierrors.ExitNotFound,
			clierrors.WithResource(registryRepo),
			clierrors.WithRemediation("Run `scribe registry connect "+registryRepo+"` first."),
		)
	}
	client, err := factory.Client()
	if err != nil {
		return fmt.Errorf("load github client: %w", err)
	}
	entry, err := findRemoteKitFn(cmd.Context(), client, registryRepo, kitName)
	if err != nil {
		return clierrors.Wrap(err, "KIT_NOT_FOUND", clierrors.ExitNotFound,
			clierrors.WithResource(ref),
			clierrors.WithRemediation("Run `scribe kit list --remote --registry "+registryRepo+"`."),
		)
	}
	k, err := fetchRemoteKitFn(cmd.Context(), client, registryRepo, entry)
	if err != nil {
		return err
	}
	out := kitShowOutputFromKit(k)
	out.Registry = registryRepo
	out.Refs = kitRefOutputs(k.Skills, registryRepo, cfg)

	if jsonFlagPassed(cmd) {
		r := jsonRendererForCommand(cmd, true)
		if err := r.Result(out); err != nil {
			return err
		}
		return r.Flush()
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Kit: %s:%s\n", registryRepo, out.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", out.Description)
	for _, ref := range out.Refs {
		status := "missing"
		if ref.Connected {
			status = "connected"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s, %s)\n", ref.Raw, ref.Origin, status)
	}
	return nil
}

func kitListItems(kits map[string]*kit.Kit) []kitListItem {
	items := make([]kitListItem, 0, len(kits))
	for _, k := range kits {
		if k == nil {
			continue
		}
		items = append(items, kitListItem{
			Name:        k.Name,
			Description: k.Description,
			SkillsCount: len(k.Skills),
			Skills:      append([]string(nil), k.Skills...),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func projectKitListItems(items []kitListItem, fieldsFlag string) (any, error) {
	if strings.TrimSpace(fieldsFlag) == "" {
		return items, nil
	}

	projected := make([]map[string]any, 0, len(items))
	selected := strings.Split(fieldsFlag, ",")
	for _, item := range items {
		row, err := fields.Project(kitListFieldSet, selected, item)
		if err != nil {
			return nil, err
		}
		projected = append(projected, row)
	}
	return projected, nil
}

func printKitList(cmd *cobra.Command, items []kitListItem, fieldsFlag string) error {
	selected := kitListTextFields(fieldsFlag)
	for _, item := range items {
		parts := make([]string, 0, len(selected))
		for _, field := range selected {
			value, err := kitListTextValue(item, field)
			if err != nil {
				return err
			}
			if value != "" {
				parts = append(parts, value)
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), strings.Join(parts, "  "))
	}
	return nil
}

func kitListTextFields(fieldsFlag string) []string {
	if strings.TrimSpace(fieldsFlag) == "" {
		return []string{"name", "description", "skills_count"}
	}
	return strings.Split(fieldsFlag, ",")
}

func kitListTextValue(item kitListItem, field string) (string, error) {
	field = strings.TrimSpace(field)
	switch field {
	case "":
		return "", nil
	case "name":
		return item.Name, nil
	case "description":
		return item.Description, nil
	case "skills_count":
		return fmt.Sprintf("(%d skills)", item.SkillsCount), nil
	case "skills":
		return strings.Join(item.Skills, ", "), nil
	default:
		return "", &clierrors.Error{
			Code:        "USAGE_UNKNOWN_FIELD",
			Message:     "unknown field: " + field,
			Remediation: "scribe schema <command> --fields",
			Exit:        clierrors.ExitUsage,
		}
	}
}

func kitShowOutputFromKit(k *kit.Kit) kitShowOutput {
	out := kitShowOutput{
		Name:        k.Name,
		Description: k.Description,
		Skills:      append([]string(nil), k.Skills...),
	}
	if k.Source != nil {
		out.Source = &kitSourceOutput{
			Registry: k.Source.Registry,
			Rev:      k.Source.Rev,
		}
	}
	return out
}

func kitRefOutputs(rawRefs []string, defaultRegistry string, cfg *config.Config) []kitRefOutput {
	out := make([]kitRefOutput, 0, len(rawRefs))
	for _, raw := range rawRefs {
		ref, err := kit.ParseSkillRef(raw, defaultRegistry)
		if err != nil {
			out = append(out, kitRefOutput{Raw: raw, Reason: err.Error()})
			continue
		}
		origin := string(kit.OriginCrossRegistry)
		if ref.Local {
			origin = string(kit.OriginLocal)
		} else if ref.Registry == defaultRegistry {
			origin = string(kit.OriginSameRegistry)
		}
		connected := ref.Local || ref.Registry == defaultRegistry || registryConnected(cfg, ref.Registry)
		item := kitRefOutput{
			Raw:       raw,
			Skill:     ref.Skill,
			Origin:    origin,
			Registry:  ref.Registry,
			Connected: connected,
			Glob:      ref.Glob,
			Local:     ref.Local,
		}
		if ref.Source.Host != "" {
			item.Source = ref.Source.String()
		}
		if !connected {
			item.Reason = "registry_not_connected"
		}
		out = append(out, item)
	}
	return out
}

func registryConnected(cfg *config.Config, repo string) bool {
	if cfg == nil || repo == "" {
		return false
	}
	rc := cfg.FindRegistry(repo)
	return rc != nil && rc.Enabled
}

func kitSourceLabel(source *kitSourceOutput) string {
	if source == nil || source.Registry == "" {
		return "(local)"
	}
	return source.Registry
}

func validateKitName(name string) error {
	if name == "" || filepath.IsAbs(name) || strings.ContainsRune(name, rune(os.PathSeparator)) || strings.Contains(name, "..") {
		return clierrors.Wrap(fmt.Errorf("invalid kit name %q", name), "KIT_NAME_INVALID", clierrors.ExitValid,
			clierrors.WithResource(name),
			clierrors.WithRemediation("Use a simple kit name without path separators or parent directory segments."),
		)
	}
	return nil
}
