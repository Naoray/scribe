package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/app"
	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/cli/fields"
	"github.com/Naoray/scribe/internal/cli/output"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/manifest"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/registry"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/workflow"
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

type kitInstallOptions struct {
	alias         string
	noDeps        bool
	noInteraction bool
	json          bool
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

type kitInstallOutput struct {
	Name              string         `json:"name"`
	Registry          string         `json:"registry"`
	Path              string         `json:"path"`
	Rev               string         `json:"rev"`
	SkillsInstalled   []string       `json:"skills_installed,omitempty"`
	MissingRefs       []kitRefOutput `json:"missing_refs,omitempty"`
	MissingRegistries []string       `json:"missing_registries,omitempty"`
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
var runKitInstallDepsFn = runKitInstallDeps
var confirmKitMissingRegistriesFn = confirmKitMissingRegistries
var remoteKitRevFn = func(ctx context.Context, client *gh.Client, registryRepo string) (string, error) {
	owner, repo, err := manifest.ParseOwnerRepo(registryRepo)
	if err != nil {
		return "", err
	}
	sha, err := client.LatestCommitSHA(ctx, owner, repo, "main")
	if err != nil {
		return "HEAD", nil
	}
	return sha, nil
}

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
	cmd.AddCommand(newKitInstallCommand())
	cmd.AddCommand(newKitSyncCommand())
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

func newKitInstallCommand() *cobra.Command {
	opts := &kitInstallOptions{}
	cmd := &cobra.Command{
		Use:   "install <registry>:<kit>",
		Short: "Install a kit from a connected registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.json = jsonFlagPassed(cmd)
			opts.noInteraction = noInteractionFlagPassed(cmd)
			return runKitInstall(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.alias, "alias", "", "Install incoming kit under this local name")
	cmd.Flags().BoolVar(&opts.noDeps, "no-deps", false, "Install kit body without installing referenced skills")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output machine-readable JSON")
	addNoInteractionFlag(cmd, "Disable interactive prompts", false)
	return markJSONSupported(cmd)
}

func newKitSyncCommand() *cobra.Command {
	opts := &kitInstallOptions{}
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Refresh installed registry kits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.json = jsonFlagPassed(cmd)
			opts.noInteraction = noInteractionFlagPassed(cmd)
			return runKitSync(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.noDeps, "no-deps", false, "Refresh kit bodies without installing referenced skills")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Output machine-readable JSON")
	addNoInteractionFlag(cmd, "Disable interactive prompts", false)
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

func runKitInstall(cmd *cobra.Command, ref string, opts *kitInstallOptions) error {
	if opts == nil {
		opts = &kitInstallOptions{}
	}
	registryRepo, kitName, err := parseSkillRef(ref)
	if err != nil {
		return err
	}
	if opts.alias != "" {
		if err := validateKitName(opts.alias); err != nil {
			return err
		}
	}
	localName := kitName
	if opts.alias != "" {
		localName = opts.alias
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
	if k.Name == "" {
		k.Name = kitName
	}
	k.Name = localName
	rev, err := remoteKitRevFn(cmd.Context(), client, registryRepo)
	if err != nil {
		return err
	}
	if k.Source == nil {
		k.Source = &kit.Source{}
	}
	k.Source.Registry = registryRepo
	k.Source.Rev = rev

	depsByRegistry, missingRefs, missingRegistries, err := installableKitRefs(k, registryRepo, cfg)
	if err != nil {
		return err
	}
	if len(missingRegistries) > 0 {
		if opts.noInteraction {
			out := kitInstallOutput{Name: localName, Registry: registryRepo, Rev: rev, MissingRefs: missingRefs, MissingRegistries: missingRegistries}
			if opts.json {
				_ = renderMutatorEnvelope(cmd, out, envelope.StatusPartialSuccess)
			}
			return clierrors.Wrap(clierrors.ErrPartialSuccess, "KIT_MISSING_REGISTRIES", clierrors.ExitPartial,
				clierrors.WithMessage("kit references registries that are not connected"),
				clierrors.WithRemediation("Run `scribe registry connect <owner/repo>` for each missing registry, then retry."),
				clierrors.WithRendered(opts.json),
			)
		}
		if err := confirmKitMissingRegistriesFn(cmd, cfg, missingRegistries); err != nil {
			return err
		}
		depsByRegistry, missingRefs, missingRegistries, err = installableKitRefs(k, registryRepo, cfg)
		if err != nil {
			return err
		}
		if len(missingRegistries) > 0 {
			return clierrors.Wrap(fmt.Errorf("registries still missing: %s", strings.Join(missingRegistries, ", ")), "KIT_MISSING_REGISTRIES", clierrors.ExitPartial)
		}
	}
	installedSkills := flattenKitDeps(depsByRegistry)
	if !opts.noDeps {
		if err := runKitInstallDepsFn(cmd, factory, depsByRegistry); err != nil {
			return err
		}
	}

	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return fmt.Errorf("resolve scribe dir: %w", err)
	}
	kitPath := filepath.Join(scribeDir, "kits", localName+".yaml")
	if err := kit.Save(kitPath, k); err != nil {
		return err
	}
	contentHash, err := hashKitFile(localName, kitPath)
	if err != nil {
		return err
	}
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	recordInstalledKit(st, localName, registryRepo, rev, contentHash, k.Skills)
	if err := st.Save(); err != nil {
		return err
	}

	out := kitInstallOutput{
		Name:              localName,
		Registry:          registryRepo,
		Path:              kitPath,
		Rev:               rev,
		SkillsInstalled:   installedSkills,
		MissingRefs:       missingRefs,
		MissingRegistries: missingRegistries,
	}
	if opts.json {
		r := jsonRendererForCommand(cmd, true)
		if err := r.Result(out); err != nil {
			return err
		}
		return r.Flush()
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Installed kit %s from %s\n", localName, registryRepo)
	if len(installedSkills) > 0 && !opts.noDeps {
		fmt.Fprintf(cmd.OutOrStdout(), "Installed skills: %s\n", strings.Join(installedSkills, ", "))
	}
	for _, missing := range missingRefs {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: deferred %s (%s)\n", missing.Raw, missing.Reason)
	}
	return nil
}

type kitInstallDep struct {
	Name   string
	Alias  string
	Source string
}

func installableKitRefs(k *kit.Kit, registryRepo string, cfg *config.Config) (map[string][]kitInstallDep, []kitRefOutput, []string, error) {
	deps := map[string][]kitInstallDep{}
	var missing []kitRefOutput
	seen := map[string]bool{}
	missingRegistrySet := map[string]bool{}
	for _, raw := range k.Skills {
		ref, err := kit.ParseSkillRef(raw, registryRepo)
		if err != nil {
			missing = append(missing, kitRefOutput{Raw: raw, Reason: err.Error()})
			continue
		}
		if ref.Local {
			missing = append(missing, kitRefOutput{Raw: raw, Skill: ref.Skill, Origin: string(kit.OriginLocal), Local: true, Reason: "local_ref_forbidden"})
			continue
		}
		origin := string(kit.OriginSameRegistry)
		if ref.Registry != registryRepo {
			origin = string(kit.OriginCrossRegistry)
		}
		if !registryConnected(cfg, ref.Registry) {
			missingRegistrySet[ref.Registry] = true
			missing = append(missing, kitRefOutput{
				Raw:       raw,
				Skill:     ref.Skill,
				Origin:    origin,
				Registry:  ref.Registry,
				Connected: false,
				Glob:      ref.Glob,
				Reason:    "registry_not_connected",
			})
			continue
		}
		if ref.Glob {
			missing = append(missing, kitRefOutput{Raw: raw, Skill: ref.Skill, Origin: origin, Registry: ref.Registry, Connected: true, Glob: true, Reason: "glob_deferred"})
			continue
		}
		source := ""
		if ref.Source.Host != "" {
			source = ref.Source.String()
		}
		key := ref.Registry + ":" + ref.Skill + ":" + source
		if !seen[key] {
			seen[key] = true
			deps[ref.Registry] = append(deps[ref.Registry], kitInstallDep{Name: ref.Skill, Alias: k.SkillAliases[raw], Source: source})
		}
	}
	for registryName := range deps {
		sort.Slice(deps[registryName], func(i, j int) bool { return deps[registryName][i].Name < deps[registryName][j].Name })
	}
	missingRegistries := make([]string, 0, len(missingRegistrySet))
	for registryName := range missingRegistrySet {
		missingRegistries = append(missingRegistries, registryName)
	}
	sort.Strings(missingRegistries)
	return deps, missing, missingRegistries, nil
}

func runKitInstallDeps(cmd *cobra.Command, factory *app.Factory, depsByRegistry map[string][]kitInstallDep) error {
	registries := make([]string, 0, len(depsByRegistry))
	for registryRepo := range depsByRegistry {
		registries = append(registries, registryRepo)
	}
	sort.Strings(registries)
	for _, registryRepo := range registries {
		deps := depsByRegistry[registryRepo]
		if len(deps) == 0 {
			continue
		}
		skillNames := make([]string, 0, len(deps))
		aliases := map[string]string{}
		pinnedSources := map[string]string{}
		for _, dep := range deps {
			skillNames = append(skillNames, dep.Name)
			if dep.Alias != "" {
				aliases[dep.Name] = dep.Alias
			}
			if dep.Source != "" {
				pinnedSources[dep.Name] = dep.Source
			}
		}
		bag := &workflow.Bag{
			Args:               skillNames,
			RepoFlag:           registryRepo,
			Factory:            factory,
			FilterRegistries:   filterRegistries,
			SkillAliases:       aliases,
			PinnedSkillSources: pinnedSources,
		}
		if err := workflow.Run(cmd.Context(), workflow.InstallSteps(), bag); err != nil {
			return handleNameConflictError(cmd, err)
		}
		if err := saveWorkflowState(bag); err != nil {
			return err
		}
	}
	return nil
}

func runKitSync(cmd *cobra.Command, opts *kitInstallOptions) error {
	if opts == nil {
		opts = &kitInstallOptions{}
	}
	factory := commandFactory()
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	client, err := factory.Client()
	if err != nil {
		return fmt.Errorf("load github client: %w", err)
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return fmt.Errorf("resolve scribe dir: %w", err)
	}

	names := make([]string, 0, len(st.Kits))
	for name := range st.Kits {
		names = append(names, name)
	}
	sort.Strings(names)
	var outputs []kitInstallOutput
	for _, localName := range names {
		installed := st.Kits[localName]
		registryRepo := installed.SourceRegistry
		if registryRepo == "" {
			registryRepo = installed.Source
		}
		if registryRepo == "" {
			continue
		}
		if !registryConnected(cfg, registryRepo) {
			return clierrors.Wrap(fmt.Errorf("registry %q is not connected", registryRepo), "REGISTRY_NOT_CONNECTED", clierrors.ExitNotFound,
				clierrors.WithResource(registryRepo),
				clierrors.WithRemediation("Run `scribe registry connect "+registryRepo+"` first."),
			)
		}
		remoteName := installed.Name
		if remoteName == "" {
			remoteName = localName
		}
		entry, err := findRemoteKitFn(cmd.Context(), client, registryRepo, remoteName)
		if err != nil {
			return clierrors.Wrap(err, "KIT_NOT_FOUND", clierrors.ExitNotFound,
				clierrors.WithResource(registryRepo+":"+remoteName),
				clierrors.WithRemediation("Run `scribe kit list --remote --registry "+registryRepo+"`."),
			)
		}
		k, err := fetchRemoteKitFn(cmd.Context(), client, registryRepo, entry)
		if err != nil {
			return err
		}
		k.Name = localName
		rev, err := remoteKitRevFn(cmd.Context(), client, registryRepo)
		if err != nil {
			return err
		}
		if k.Source == nil {
			k.Source = &kit.Source{}
		}
		k.Source.Registry = registryRepo
		k.Source.Rev = rev
		depsByRegistry, missingRefs, missingRegistries, err := installableKitRefs(k, registryRepo, cfg)
		if err != nil {
			return err
		}
		if len(missingRegistries) > 0 {
			if opts.noInteraction {
				out := kitInstallOutput{Name: localName, Registry: registryRepo, Rev: rev, MissingRefs: missingRefs, MissingRegistries: missingRegistries}
				if opts.json {
					_ = renderMutatorEnvelope(cmd, out, envelope.StatusPartialSuccess)
				}
				return clierrors.Wrap(clierrors.ErrPartialSuccess, "KIT_MISSING_REGISTRIES", clierrors.ExitPartial,
					clierrors.WithMessage("kit references registries that are not connected"),
					clierrors.WithRendered(opts.json),
				)
			}
			if err := confirmKitMissingRegistriesFn(cmd, cfg, missingRegistries); err != nil {
				return err
			}
			depsByRegistry, missingRefs, missingRegistries, err = installableKitRefs(k, registryRepo, cfg)
			if err != nil {
				return err
			}
		}
		kitPath := filepath.Join(scribeDir, "kits", localName+".yaml")
		if err := ensureKitFileUnmodified(localName, kitPath, installed.ContentHash); err != nil {
			return err
		}
		installedSkills := flattenKitDeps(depsByRegistry)
		if !opts.noDeps {
			if err := runKitInstallDepsFn(cmd, factory, depsByRegistry); err != nil {
				return err
			}
		}
		if err := kit.Save(kitPath, k); err != nil {
			return err
		}
		contentHash, err := hashKitFile(localName, kitPath)
		if err != nil {
			return err
		}
		recordInstalledKit(st, localName, registryRepo, rev, contentHash, k.Skills)
		outputs = append(outputs, kitInstallOutput{
			Name:              localName,
			Registry:          registryRepo,
			Path:              kitPath,
			Rev:               rev,
			SkillsInstalled:   installedSkills,
			MissingRefs:       missingRefs,
			MissingRegistries: missingRegistries,
		})
	}
	if err := st.Save(); err != nil {
		return err
	}
	if opts.json {
		return renderMutatorEnvelope(cmd, map[string]any{"kits": outputs}, envelope.StatusOK)
	}
	for _, out := range outputs {
		fmt.Fprintf(cmd.OutOrStdout(), "Synced kit %s from %s\n", out.Name, out.Registry)
	}
	return nil
}

func flattenKitDeps(depsByRegistry map[string][]kitInstallDep) []string {
	var installed []string
	for registryRepo, deps := range depsByRegistry {
		for _, dep := range deps {
			name := dep.Name
			if dep.Alias != "" {
				name = dep.Alias
			}
			installed = append(installed, registryRepo+":"+name)
		}
	}
	sort.Strings(installed)
	return installed
}

func confirmKitMissingRegistries(cmd *cobra.Command, cfg *config.Config, registries []string) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "Kit references unconnected registries: %s\nConnect them now? [y/N] ", strings.Join(registries, ", "))
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return clierrors.Wrap(clierrors.ErrPartialSuccess, "KIT_MISSING_REGISTRIES", clierrors.ExitPartial,
			clierrors.WithMessage("kit references registries that are not connected"),
			clierrors.WithRemediation("Run `scribe registry connect <owner/repo>` for each missing registry, then retry."),
		)
	}
	for _, repo := range registries {
		cfg.AddRegistry(config.RegistryConfig{Repo: repo, Enabled: true, Type: config.RegistryTypeGitHub})
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func recordInstalledKit(st *state.State, name, registryRepo, rev, contentHash string, skills []string) {
	if st.Kits == nil {
		st.Kits = map[string]state.InstalledKit{}
	}
	st.Kits[name] = state.InstalledKit{
		Name:           name,
		SourceRegistry: registryRepo,
		Rev:            rev,
		ContentHash:    contentHash,
		InstalledAt:    time.Now().UTC(),
		Source:         registryRepo,
		Version:        rev,
		Skills:         append([]string(nil), skills...),
	}
}

func hashKitFile(name, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return lockfile.HashFiles([]lockfile.File{{Path: name + ".yaml", Content: data}})
}

func ensureKitFileUnmodified(name, path, expectedHash string) error {
	if expectedHash == "" {
		return nil
	}
	currentHash, err := hashKitFile(name, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if currentHash == expectedHash {
		return nil
	}
	return clierrors.Wrap(fmt.Errorf("kit %q has local edits at %s", name, path), "KIT_LOCAL_EDIT_CONFLICT", clierrors.ExitConflict,
		clierrors.WithResource(path),
		clierrors.WithRemediation("Move or restore the local kit file before running `scribe kit sync` again."),
	)
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
