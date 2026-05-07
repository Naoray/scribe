package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"charm.land/huh/v2"
	"github.com/BurntSushi/toml"
	"github.com/mattn/go-isatty"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/agent"
	"github.com/Naoray/scribe/internal/app"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/config"
	gh "github.com/Naoray/scribe/internal/github"
	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/lockfile"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/projectstore"
	"github.com/Naoray/scribe/internal/provider"
	"github.com/Naoray/scribe/internal/reconcile"
	"github.com/Naoray/scribe/internal/snippet"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

const registryMuteAfter = 3

// SyncSteps returns the step list for the sync command.
func SyncSteps() []Step {
	return []Step{
		{"LoadConfig", StepLoadConfig},
		{"LoadState", StepLoadState},
		{"ResolveProjectRoot", StepResolveProjectRoot},
		{"ResolveTeamShareMode", StepResolveTeamShareMode},
		{"CheckConnected", StepCheckConnected},
		{"FilterRegistries", StepFilterRegistries},
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"ResolveKitFilter", StepResolveKitFilter},
		{"ResolveMCPServers", StepResolveMCPServers},
		{"ProjectMCPServers", StepProjectMCPServers},
		{"ProjectSnippets", StepProjectSnippets},
		{"EnsureScribeAgent", StepEnsureScribeAgent},
		{"Adopt", StepAdopt},
		{"ReconcilePre", StepReconcileSystem},
		{"SyncSkills", StepSyncSkills},
		{"ReconcilePost", StepReconcileSystem},
	}
}

// SyncTail returns the shared tail of steps reused by connect and create-registry.
func SyncTail() []Step {
	return []Step{
		{"ResolveFormatter", StepResolveFormatter},
		{"ResolveTools", StepResolveTools},
		{"ResolveProjectRoot", StepResolveProjectRoot},
		{"ResolveTeamShareMode", StepResolveTeamShareMode},
		{"ResolveKitFilter", StepResolveKitFilter},
		{"ResolveMCPServers", StepResolveMCPServers},
		{"ProjectMCPServers", StepProjectMCPServers},
		{"ProjectSnippets", StepProjectSnippets},
		{"EnsureScribeAgent", StepEnsureScribeAgent},
		{"SyncSkills", StepSyncSkills},
		{"ReconcilePost", StepReconcileSystem},
	}
}

func StepReconcileSystem(_ context.Context, b *Bag) error {
	engine := reconcile.Engine{
		Tools:            b.Tools,
		ProjectRoot:      b.ProjectRoot,
		KitFilter:        b.KitFilter,
		KitFilterEnabled: b.KitFilterEnabled,
	}
	summary, actions, err := engine.Run(b.State)
	if err != nil {
		return fmt.Errorf("reconcile system: %w", err)
	}
	for _, action := range actions {
		if action.Kind != reconcile.ActionConflict {
			continue
		}
		for _, conflict := range b.State.Installed[action.Name].Conflicts {
			if conflict.Path == action.Path && conflict.Tool == action.Tool {
				b.Formatter.OnReconcileConflict(action.Name, conflict)
				break
			}
		}
	}
	b.Formatter.OnReconcileComplete(sync.ReconcileCompleteMsg{Summary: summary})
	b.MarkStateDirty()
	return nil
}

func StepResolveProjectRoot(_ context.Context, b *Bag) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	projectFile, err := projectfile.Find(wd)
	if err != nil {
		return err
	}
	if projectFile == "" {
		b.ProjectRoot = ""
		return nil
	}
	b.ProjectRoot = filepath.Dir(projectFile)
	return nil
}

func StepResolveTeamShareMode(_ context.Context, b *Bag) error {
	if b.ProjectRoot == "" {
		b.TeamShareMode = false
		return nil
	}
	_, err := os.Stat(filepath.Join(b.ProjectRoot, ".ai", "scribe.lock"))
	if err == nil {
		b.TeamShareMode = true
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		b.TeamShareMode = false
		return nil
	}
	return err
}

// ResolveKitFilter resolves the kit-scoped skill set for the current working
// directory. Returns the allowed skill names and whether a project file was
// found. All errors are non-fatal; a missing or malformed project file returns
// (nil, false) so callers fall back to global behavior.
func ResolveKitFilter(st *state.State) (filter []string, enabled bool) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, false
	}
	projectPath, err := projectfile.Find(wd)
	if err != nil || projectPath == "" {
		return nil, false
	}
	pf, err := projectfile.Load(projectPath)
	if err != nil {
		return nil, false
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return nil, false
	}
	kits, err := kit.LoadAll(filepath.Join(scribeDir, "kits"))
	if err != nil {
		return nil, false
	}
	names := make([]string, 0, len(st.Installed))
	for name, installed := range st.Installed {
		if installed.Kind == state.KindPackage {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	resolved, err := kit.Resolve(pf, kits, names)
	if err != nil {
		return nil, false
	}
	return resolved, true
}

// StepResolveKitFilter loads the project's .scribe.yaml and resolves its kit
// references against the user's kit library, leaving the resolved skill names
// on b.KitFilter. All errors are non-fatal: a missing or malformed project
// file leaves b.KitFilter nil so the syncer applies no kit filtering and
// behaves like legacy global sync.
func StepResolveKitFilter(_ context.Context, b *Bag) error {
	if b.ProjectRoot == "" || b.State == nil {
		return nil
	}
	b.KitFilter, b.KitFilterEnabled = ResolveKitFilter(b.State)
	return nil
}

// ResolveProjectMCPServers resolves project- and kit-declared MCP server names for the
// current project. All errors are non-fatal; a missing or malformed project
// file returns (nil, false) so callers do not write runtime settings yet.
func ResolveProjectMCPServers() (servers []string, enabled bool) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, false
	}
	projectPath, err := projectfile.Find(wd)
	if err != nil || projectPath == "" {
		return nil, false
	}
	pf, err := projectfile.Load(projectPath)
	if err != nil {
		return nil, false
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return nil, false
	}
	kits, err := kit.LoadAll(filepath.Join(scribeDir, "kits"))
	if err != nil {
		return nil, false
	}
	resolved, err := kit.ResolveMCPServers(pf, kits)
	if err != nil {
		return nil, false
	}
	return resolved, true
}

// StepResolveMCPServers loads the project's .scribe.yaml and resolves
// project MCP server names into read-only workflow state. It does not write
// any agent runtime settings.
func StepResolveMCPServers(_ context.Context, b *Bag) error {
	if b.ProjectRoot == "" {
		return nil
	}
	b.ProjectMCPServers, b.ProjectMCPServersEnabled = ResolveProjectMCPServers()
	return nil
}

// StepProjectMCPServers writes project-resolved MCP server selections into each
// active tool's project config. Server definitions remain in .mcp.json for
// Claude, and are copied from .mcp.json for Codex and Cursor project configs.
func StepProjectMCPServers(_ context.Context, b *Bag) error {
	if b.ProjectRoot == "" || !b.ProjectMCPServersEnabled {
		return nil
	}
	var definitions map[string]map[string]any
	if hasTool(b.Tools, "codex") || hasTool(b.Tools, "cursor") {
		var err error
		definitions, err = loadProjectMCPDefinitions(b.ProjectRoot, b.ProjectMCPServers)
		if err != nil {
			if b.TeamShareMode {
				fmt.Fprintf(os.Stderr, "scribe: team-share warning: skipping MCP projection: %v\n", err)
				return nil
			}
			return err
		}
	}
	if hasTool(b.Tools, "claude") {
		if err := projectClaudeMCPServers(b.ProjectRoot, b.ProjectMCPServers); err != nil {
			return fmt.Errorf("project claude MCP servers: %w", err)
		}
	}
	if hasTool(b.Tools, "codex") || hasTool(b.Tools, "cursor") {
		if hasTool(b.Tools, "codex") {
			if err := projectCodexMCPServers(b.ProjectRoot, b.ProjectMCPServers, definitions); err != nil {
				return fmt.Errorf("project codex MCP servers: %w", err)
			}
		}
		if hasTool(b.Tools, "cursor") {
			if err := projectCursorMCPServers(b.ProjectRoot, b.ProjectMCPServers, definitions); err != nil {
				return fmt.Errorf("project cursor MCP servers: %w", err)
			}
		}
	}
	return nil
}

// StepProjectClaudeMCPServers writes project-resolved MCP server approvals into
// shared project Claude settings. Server definitions remain in .mcp.json.
func StepProjectClaudeMCPServers(_ context.Context, b *Bag) error {
	if b.ProjectRoot == "" || !b.ProjectMCPServersEnabled || !hasTool(b.Tools, "claude") {
		return nil
	}
	if err := projectClaudeMCPServers(b.ProjectRoot, b.ProjectMCPServers); err != nil {
		return fmt.Errorf("project claude MCP servers: %w", err)
	}
	return nil
}

func StepProjectSnippets(_ context.Context, b *Bag) error {
	if b.ProjectRoot == "" {
		return nil
	}
	projectPath := filepath.Join(b.ProjectRoot, projectfile.Filename)
	pf, err := projectfile.Load(projectPath)
	if err != nil {
		return fmt.Errorf("load project snippets: %w", err)
	}
	var snippets []snippet.Snippet
	if len(pf.Snippets) > 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("home dir: %w", err)
		}
		snippets, err = snippet.LoadProject(snippet.Dir(home), pf.Snippets)
		if err != nil {
			if b.TeamShareMode {
				fmt.Fprintf(os.Stderr, "scribe: team-share warning: skipping missing snippets: %v\n", err)
				snippets = nil
			} else {
				return err
			}
		}
	}
	legacyPaths, err := removeLegacyCursorSnippets(b.ProjectRoot, snippets, b.State)
	if err != nil {
		return err
	}
	toolNames := make([]string, 0, len(b.Tools))
	for _, tool := range b.Tools {
		toolNames = append(toolNames, tool.Name())
	}
	paths, err := snippet.Project(b.ProjectRoot, snippets, toolNames)
	if err != nil {
		return err
	}
	if b.State != nil {
		if b.State.Snippets == nil {
			b.State.Snippets = map[string]state.InstalledSnippet{}
		}
		for _, sn := range snippets {
			next := state.InstalledSnippet{
				Source:  sn.Path,
				Targets: append([]string(nil), sn.Targets...),
			}
			if !installedSnippetEqual(b.State.Snippets[sn.Name], next) {
				b.State.Snippets[sn.Name] = next
				b.MarkStateDirty()
			}
		}
		if len(paths) > 0 || len(legacyPaths) > 0 {
			b.MarkStateDirty()
		}
	}
	b.ProjectSnippets = append(b.ProjectSnippets[:0], pf.Snippets...)
	return nil
}

func installedSnippetEqual(a, b state.InstalledSnippet) bool {
	if a.Source != b.Source || a.Version != b.Version || len(a.Targets) != len(b.Targets) {
		return false
	}
	for i := range a.Targets {
		if a.Targets[i] != b.Targets[i] {
			return false
		}
	}
	return true
}

func removeLegacyCursorSnippets(projectRoot string, snippets []snippet.Snippet, st *state.State) ([]string, error) {
	if st == nil || len(st.Snippets) == 0 {
		return nil, nil
	}
	current := map[string]bool{}
	for _, sn := range snippets {
		current[sn.Name] = true
	}
	var paths []string
	for name, installed := range st.Snippets {
		if current[name] || !snippet.TargetsAgent(installed.Targets, "cursor") || installed.Source == "" {
			continue
		}
		data, err := os.ReadFile(installed.Source)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return paths, fmt.Errorf("read stale snippet source %s: %w", installed.Source, err)
		}
		sn, err := snippet.Parse(installed.Source, data)
		if err != nil {
			continue
		}
		path, removed, err := snippet.RemoveLegacyCursorRule(projectRoot, sn)
		if err != nil {
			return paths, err
		}
		if removed {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

type projectMCPFile struct {
	MCPServers map[string]map[string]any `json:"mcpServers"`
}

func loadProjectMCPDefinitions(projectRoot string, servers []string) (map[string]map[string]any, error) {
	if len(servers) == 0 {
		return map[string]map[string]any{}, nil
	}
	path := filepath.Join(projectRoot, ".mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file projectMCPFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	selected := make(map[string]map[string]any, len(servers))
	for _, server := range sortedStrings(servers) {
		def, ok := file.MCPServers[server]
		if !ok {
			return nil, fmt.Errorf("MCP server %q declared by .scribe.yaml/kits but missing from .mcp.json", server)
		}
		selected[server] = cloneMap(def)
	}
	return selected, nil
}

func projectClaudeMCPServers(projectRoot string, servers []string) error {
	settingsDir := filepath.Join(projectRoot, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	settings := map[string]any{}
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}

	resolved := append([]string{}, servers...)
	sort.Strings(resolved)
	settings["enableAllProjectMcpServers"] = false
	settings["enabledMcpjsonServers"] = resolved

	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("create claude settings dir: %w", err)
	}
	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode claude settings: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(settingsDir, ".settings.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp claude settings: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp claude settings: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp claude settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp claude settings: %w", err)
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		return fmt.Errorf("save claude settings: %w", err)
	}
	return nil
}

func projectCodexMCPServers(projectRoot string, servers []string, definitions map[string]map[string]any) error {
	configDir := filepath.Join(projectRoot, ".codex")
	configPath := filepath.Join(configDir, "config.toml")
	sidecarPath := filepath.Join(configDir, "scribe-mcp.json")

	config := map[string]any{}
	data, err := os.ReadFile(configPath)
	if err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse %s: %w", configPath, err)
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", configPath, err)
	}

	mcpServers := map[string]any{}
	if existing, ok := config["mcp_servers"].(map[string]any); ok {
		for name, def := range existing {
			mcpServers[name] = def
		}
	}
	managed, err := readManagedMCPServers(sidecarPath)
	if err != nil {
		return err
	}
	for _, name := range managed {
		delete(mcpServers, name)
	}
	for _, name := range sortedStrings(servers) {
		mcpServers[name] = codexMCPDefinition(definitions[name])
	}
	config["mcp_servers"] = mcpServers

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create codex config dir: %w", err)
	}
	data, err = toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode codex config: %w", err)
	}
	if err := atomicWriteFile(configPath, data, ".config.toml.*.tmp"); err != nil {
		return fmt.Errorf("save codex config: %w", err)
	}
	if err := writeManagedMCPServers(sidecarPath, servers); err != nil {
		return err
	}
	return nil
}

func projectCursorMCPServers(projectRoot string, servers []string, definitions map[string]map[string]any) error {
	configDir := filepath.Join(projectRoot, ".cursor")
	configPath := filepath.Join(configDir, "mcp.json")
	sidecarPath := filepath.Join(configDir, "scribe-mcp.json")

	config := map[string]any{}
	data, err := os.ReadFile(configPath)
	if err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse %s: %w", configPath, err)
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", configPath, err)
	}

	mcpServers := map[string]any{}
	if existing, ok := config["mcpServers"].(map[string]any); ok {
		for name, def := range existing {
			mcpServers[name] = def
		}
	}
	managed, err := readManagedMCPServers(sidecarPath)
	if err != nil {
		return err
	}
	for _, name := range managed {
		delete(mcpServers, name)
	}
	for _, name := range sortedStrings(servers) {
		mcpServers[name] = cloneMap(definitions[name])
	}
	config["mcpServers"] = mcpServers

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create cursor config dir: %w", err)
	}
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cursor config: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWriteFile(configPath, data, ".mcp.json.*.tmp"); err != nil {
		return fmt.Errorf("save cursor config: %w", err)
	}
	if err := writeManagedMCPServers(sidecarPath, servers); err != nil {
		return err
	}
	return nil
}

type managedMCPProjection struct {
	Servers []string `json:"servers"`
}

func readManagedMCPServers(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var projection managedMCPProjection
	if err := json.Unmarshal(data, &projection); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return projection.Servers, nil
}

func writeManagedMCPServers(path string, servers []string) error {
	data, err := json.MarshalIndent(managedMCPProjection{Servers: sortedStrings(servers)}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode managed MCP projection: %w", err)
	}
	data = append(data, '\n')
	if err := atomicWriteFile(path, data, ".scribe-mcp.json.*.tmp"); err != nil {
		return fmt.Errorf("save managed MCP projection: %w", err)
	}
	return nil
}

func codexMCPDefinition(def map[string]any) map[string]any {
	out := cloneMap(def)
	if headers, ok := out["headers"]; ok {
		if _, exists := out["http_headers"]; !exists {
			out["http_headers"] = headers
		}
		delete(out, "headers")
	}
	if _, ok := out["enabled"]; !ok {
		out["enabled"] = true
	}
	delete(out, "type")
	return out
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func atomicWriteFile(path string, data []byte, pattern string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s dir: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func hasTool(tools []tools.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name() == name {
			return true
		}
	}
	return false
}

func StepLoadConfig(ctx context.Context, b *Bag) error {
	if b.Config == nil {
		cfg, err := loadConfig(b.Factory)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		b.Config = cfg
	}

	if b.LazyGitHub {
		return nil
	}

	if b.Client == nil {
		if b.Factory != nil {
			client, err := b.Factory.Client()
			if err != nil {
				return fmt.Errorf("load github client: %w", err)
			}
			b.Client = client
		} else {
			b.Client = gh.NewClient(ctx, b.Config.Token)
		}
	}

	if b.Provider == nil {
		if b.Factory != nil {
			p, err := b.Factory.Provider()
			if err != nil {
				return fmt.Errorf("load provider: %w", err)
			}
			b.Provider = p
		} else {
			b.Provider = provider.NewGitHubProvider(provider.WrapGitHubClient(b.Client))
		}
	}

	return nil
}

func StepEnsureScribeAgent(_ context.Context, b *Bag) error {
	if b.Config == nil || b.State == nil {
		return nil
	}

	storeDir, err := tools.StoreDir()
	if err != nil {
		return fmt.Errorf("resolve store dir: %w", err)
	}

	changed, err := agent.EnsureScribeAgent(storeDir, b.State, b.Config)
	if err != nil {
		return fmt.Errorf("ensure scribe: %w", err)
	}
	if !changed {
		return nil
	}
	b.MarkStateDirty()
	return nil
}

func StepLoadState(_ context.Context, b *Bag) error {
	if b.State == nil {
		st, err := loadState(b.Factory)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		b.State = st
	}
	return nil
}

func loadConfig(factory *app.Factory) (*config.Config, error) {
	if factory != nil {
		return factory.Config()
	}
	return config.Load()
}

func loadState(factory *app.Factory) (*state.State, error) {
	if factory != nil {
		return factory.State()
	}
	return state.Load()
}

func StepCheckConnected(_ context.Context, b *Bag) error {
	if b.TeamShareMode {
		return nil
	}
	if len(b.Config.TeamRepos()) == 0 {
		return fmt.Errorf("not connected — run `scribe connect <owner/repo>` first")
	}
	return nil
}

func StepFilterRegistries(_ context.Context, b *Bag) error {
	if b.FilterRegistries != nil {
		repos, err := b.FilterRegistries(b.RepoFlag, b.Config.TeamRepos())
		if err != nil {
			return err
		}
		b.Repos = repos
	} else {
		b.Repos = b.Config.TeamRepos()
	}
	return nil
}

// StepResolveFormatter constructs the Formatter once. Idempotent — if
// bag.Formatter is already set (e.g. by a parent workflow), it skips.
// Must run after StepFilterRegistries so b.Repos reflects the actual set.
func StepResolveFormatter(ctx context.Context, b *Bag) error {
	if b.Formatter != nil {
		return nil
	}
	useJSON := UseJSONOutputForProcess(b.JSONFlag)
	multiRegistry := len(b.Repos) > 1
	b.Formatter = NewFormatterForContext(ctx, useJSON, multiRegistry)
	return nil
}

func StepResolveTools(_ context.Context, b *Bag) error {
	if b.Tools == nil {
		resolved, err := tools.ResolveActive(b.Config)
		if err != nil {
			return fmt.Errorf("resolve tools: %w", err)
		}
		b.Tools = resolved
	}
	return nil
}

// StepAdopt runs skill adoption as a prelude before registry sync.
// Adoption errors are non-fatal — they are reported through Formatter and sync continues.
func StepAdopt(_ context.Context, b *Bag) error {
	mode := b.Config.AdoptionMode()
	if mode == "off" {
		return nil
	}

	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	if mode == "prompt" {
		if !isTTY || b.JSONFlag {
			b.Formatter.OnAdoptionSkipped(
				`adoption mode is "prompt" but stdin is not a terminal — skipping adoption; run "scribe adopt --no-interaction" or set adoption.mode to auto/off`,
			)
		} else {
			b.Formatter.OnAdoptionSkipped(`prompt mode — run 'scribe adopt' to review candidates`)
		}
		return nil
	}

	candidates, conflicts, err := adopt.FindCandidates(b.State, b.Config.Adoption)
	if err != nil {
		b.Formatter.OnAdoptionSkipped(fmt.Sprintf("adoption scan failed: %v", err))
		return nil
	}

	if len(conflicts) > 0 {
		b.Formatter.OnAdoptionConflictsDeferred(len(conflicts))
	}

	if len(candidates) == 0 {
		return nil
	}

	b.Formatter.OnAdoptionStarted(len(candidates))

	adopter := &adopt.Adopter{
		State: b.State,
		Tools: b.Tools,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case adopt.AdoptedMsg:
				b.Formatter.OnAdopted(m.Name, m.Tools)
			case adopt.AdoptErrorMsg:
				b.Formatter.OnAdoptionError(m.Name, m.Err)
			case adopt.AdoptCompleteMsg:
				b.Formatter.OnAdoptionComplete(m.Adopted, m.Skipped, m.Failed)
			}
		},
	}

	adopter.Apply(candidates) // errors routed through Emit; never abort sync

	return nil
}

func StepSyncSkills(ctx context.Context, b *Bag) error {
	resolved := map[string]sync.SkillStatus{}

	syncer := &sync.Syncer{
		Client:           sync.WrapGitHubClient(b.Client),
		Provider:         b.Provider,
		Tools:            b.Tools,
		Executor:         &sync.ShellExecutor{},
		TrustAll:         b.TrustAllFlag,
		ForceBudget:      b.ForceBudget,
		AliasName:        b.AliasName,
		SkillFilter:      b.SkillFilter,
		KitFilter:        b.KitFilter,
		KitFilterEnabled: b.KitFilterEnabled,
		ProjectRoot:      b.ProjectRoot,
		SkipMissing:      workflowSkipMissing(b),
		Emit: func(msg any) {
			switch m := msg.(type) {
			case sync.SkillResolvedMsg:
				resolved[m.Name] = m.SkillStatus
				b.Formatter.OnSkillResolved(m.Name, m.SkillStatus)
			case sync.SkillSkippedMsg:
				b.Formatter.OnSkillSkipped(m.Name, resolved[m.Name])
			case sync.SkillSkippedByDenyListMsg:
				b.Formatter.OnSkillSkippedByDenyList(m.Name, m.Registry)
			case sync.SkillDownloadingMsg:
				b.Formatter.OnSkillDownloading(m.Name)
			case sync.SkillInstalledMsg:
				b.Formatter.OnSkillInstalled(m.Name, m.Updated)
			case sync.SkillErrorMsg:
				b.Formatter.OnSkillError(m.Name, m.Err)
			case sync.BudgetWarningMsg:
				b.Formatter.OnBudgetWarning(m.Agent, m.Message)
			case sync.NameConflictResolvedMsg:
				b.Formatter.OnNameConflictResolved(m.Conflict, m.Resolution)
			case sync.SkillAdoptionNeededMsg:
				// Handled implicitly by the SkillErrorMsg that follows it.
			case sync.LegacyFormatMsg:
				b.Formatter.OnLegacyFormat(m.Repo)
			case sync.SyncCompleteMsg:
				if m.Failed > 0 {
					b.Partial = true
				}
				b.Formatter.OnSyncComplete(m)

			// Package events
			case sync.PackageInstallPromptMsg:
				b.Formatter.OnPackageInstallPrompt(m.Name, m.Command, m.Source)
			case sync.PackageApprovedMsg:
				b.Formatter.OnPackageApproved(m.Name)
			case sync.PackageDeniedMsg:
				b.Formatter.OnPackageDenied(m.Name)
			case sync.PackageSkippedMsg:
				b.Formatter.OnPackageSkipped(m.Name, m.Reason)
			case sync.PackageInstallingMsg:
				b.Formatter.OnPackageInstalling(m.Name)
			case sync.PackageInstalledMsg:
				b.Formatter.OnPackageInstalled(m.Name)
			case sync.PackageUpdateMsg:
				b.Formatter.OnPackageUpdating(m.Name)
			case sync.PackageUpdatedMsg:
				b.Formatter.OnPackageUpdated(m.Name)
			case sync.PackageErrorMsg:
				b.Formatter.OnPackageError(m.Name, m.Err, m.Stderr)
			case sync.PackageHashMismatchMsg:
				b.Formatter.OnPackageHashMismatch(m.Name, m.OldCommand, m.NewCommand, m.Source)
			}
		},
	}

	// Set interactive approval when in TTY mode.
	// Skip in JSON mode — machine output cannot be interleaved with a blocking prompt.
	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	if isTTY && !b.TrustAllFlag && !b.JSONFlag {
		syncer.ApprovalFunc = func(name, command, source string) bool {
			var approved bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Package %q wants to run a shell command", name)).
				Description(fmt.Sprintf("source:  %s\ncommand: %s", source, command)).
				Affirmative("Approve").
				Negative("Deny").
				Value(&approved).
				Run()
			if err != nil {
				return false
			}
			return approved
		}
	}
	if ConflictModeForProcess(b.JSONFlag) == ConflictModeInteractive && b.AliasName == "" {
		syncer.NameConflictResolver = PromptNameConflictResolution
	}

	if b.TeamShareMode {
		projectLock, err := projectstore.Project(b.ProjectRoot).LoadProjectLockfile()
		if err != nil {
			return err
		}
		if err := validateProjectRegistriesConnected(projectLock, b.Config.TeamRepos()); err != nil {
			return err
		}
		clear(resolved)
		b.Formatter.OnRegistryStart("project")
		if err := syncer.RunProject(ctx, b.State, projectLock); err != nil {
			return err
		}
		return nil
	}

	for _, teamRepo := range b.Repos {
		if b.State.RegistryFailure(teamRepo).Muted {
			continue
		}
		clear(resolved)
		b.Formatter.OnRegistryStart(teamRepo)

		if err := syncer.Run(ctx, teamRepo, b.State); err != nil {
			failure, changed := b.State.RecordRegistryFailure(teamRepo, err, registryMuteAfter)
			if changed {
				b.MarkStateDirty()
			}
			if failure.Muted {
				continue
			}
			return err
		}
		if b.State.ClearRegistryFailure(teamRepo) {
			b.MarkStateDirty()
		}
	}

	return nil
}

func validateProjectRegistriesConnected(lf *lockfile.ProjectLockfile, connected []string) error {
	if lf == nil {
		return nil
	}
	set := map[string]bool{}
	for _, repo := range connected {
		set[repo] = true
	}
	for _, entry := range lf.Entries {
		if !set[entry.SourceRegistry] {
			return clierrors.Wrap(fmt.Errorf("registry %q is not connected", entry.SourceRegistry), "PROJECT_REGISTRY_NOT_CONNECTED", clierrors.ExitPerm,
				clierrors.WithRemediation("Run `scribe registry connect "+entry.SourceRegistry+"` before `scribe sync`."),
			)
		}
	}
	return nil
}

func workflowSkipMissing(b *Bag) bool {
	if b == nil {
		return true
	}
	if b.InstallAllFlag || len(b.SkillFilter) > 0 {
		return false
	}
	// A project .scribe.yaml is declarative intent. Plain `scribe sync` should
	// converge the kit-resolved project loadout, including missing skills.
	if b.KitFilterEnabled {
		return false
	}
	return true
}

func PromptNameConflictResolution(conflict sync.NameConflict) (sync.NameConflictResolution, error) {
	const (
		adopt = "adopt"
		alias = "alias"
		skip  = "skip"
	)
	choice := adopt
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Skill %q conflicts with an existing local directory", conflict.Name)).
		Description(fmt.Sprintf("%s already exists at %s.", conflict.Tool, conflict.Path)).
		Options(
			huh.NewOption("Adopt existing first", adopt),
			huh.NewOption("Install incoming under a different name", alias),
			huh.NewOption("Skip incoming skill", skip),
		).
		Value(&choice).
		Run()
	if err != nil {
		return sync.NameConflictResolution{}, err
	}

	switch choice {
	case adopt:
		return sync.NameConflictResolution{Action: sync.NameConflictActionAdopt}, nil
	case skip:
		return sync.NameConflictResolution{Action: sync.NameConflictActionSkip}, nil
	case alias:
		aliasName := ""
		if err := huh.NewInput().
			Title("Install incoming skill as").
			Placeholder(conflict.Name + "-registry").
			Value(&aliasName).
			Run(); err != nil {
			return sync.NameConflictResolution{}, err
		}
		return sync.NameConflictResolution{
			Action: sync.NameConflictActionAlias,
			Alias:  aliasName,
		}, nil
	default:
		return sync.NameConflictResolution{Action: sync.NameConflictActionUnresolved}, nil
	}
}
