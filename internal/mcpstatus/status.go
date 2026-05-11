package mcpstatus

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/Naoray/scribe/internal/kit"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/projectfile"
)

const (
	ClientStateConfigured = "configured"
	ClientStateMissing    = "missing"
	ClientStateUnknown    = "unknown"

	DriftDeclaredMissing      = "declared_missing"
	DriftConfiguredUndeclared = "configured_undeclared"
	DriftConfigMismatch       = "config_mismatch"
	DriftUnknownClientState   = "unknown_client_state"
)

type Report struct {
	ProjectRoot  string   `json:"project_root"`
	ManifestPath string   `json:"manifest_path,omitempty"`
	Definitions  string   `json:"definitions_path,omitempty"`
	Declarations []string `json:"declarations"`
	Servers      []Server `json:"servers"`
	Clients      []Client `json:"clients"`
	Drift        []Drift  `json:"drift,omitempty"`
	Summary      Summary  `json:"summary"`
}

type Server struct {
	Name     string   `json:"name"`
	Declared bool     `json:"declared"`
	Defined  bool     `json:"defined"`
	Clients  []string `json:"clients"`
	Drift    []Drift  `json:"drift,omitempty"`
}

type Client struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	State     string   `json:"state"`
	Projected []string `json:"projected"`
	Drift     []Drift  `json:"drift,omitempty"`
}

type Drift struct {
	Kind    string `json:"kind"`
	Client  string `json:"client,omitempty"`
	Server  string `json:"server,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type Summary struct {
	Declared int `json:"declared"`
	Defined  int `json:"defined"`
	Clients  int `json:"clients"`
	Drift    int `json:"drift"`
}

type InspectOptions struct {
	WorkDir string
}

func Inspect(opts InspectOptions) (Report, error) {
	workDir := opts.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return Report{}, fmt.Errorf("get working directory: %w", err)
		}
	}
	projectRoot, manifestPath, declarations, err := resolveDeclarations(workDir)
	if err != nil {
		return Report{}, err
	}
	definitionsPath := filepath.Join(projectRoot, ".mcp.json")
	definitions, definitionsExists, err := readDefinitions(definitionsPath)
	if err != nil {
		return Report{}, err
	}
	if !definitionsExists {
		definitionsPath = ""
	}

	declared := stringSet(declarations)
	clients := []Client{
		inspectClaude(projectRoot, declared),
		inspectCodex(projectRoot, declared, definitions),
		inspectCursor(projectRoot, declared, definitions),
	}

	projectedByServer := map[string][]string{}
	allServers := map[string]struct{}{}
	for name := range declared {
		allServers[name] = struct{}{}
	}
	for name := range definitions {
		allServers[name] = struct{}{}
	}
	var drift []Drift
	configuredClients := 0
	for i := range clients {
		if clients[i].State == ClientStateConfigured {
			configuredClients++
		}
		for _, server := range clients[i].Projected {
			allServers[server] = struct{}{}
			projectedByServer[server] = append(projectedByServer[server], clients[i].Name)
		}
		drift = append(drift, clients[i].Drift...)
	}
	sortDrift(drift)

	servers := make([]Server, 0, len(allServers))
	for name := range allServers {
		serverDrift := filterDriftByServer(drift, name)
		clients := sortedStrings(projectedByServer[name])
		_, isDeclared := declared[name]
		_, isDefined := definitions[name]
		servers = append(servers, Server{
			Name:     name,
			Declared: isDeclared,
			Defined:  isDefined,
			Clients:  clients,
			Drift:    serverDrift,
		})
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Name < servers[j].Name })

	definedCount := 0
	for _, server := range servers {
		if server.Defined {
			definedCount++
		}
	}

	return Report{
		ProjectRoot:  projectRoot,
		ManifestPath: manifestPath,
		Definitions:  definitionsPath,
		Declarations: declarations,
		Servers:      servers,
		Clients:      clients,
		Drift:        drift,
		Summary: Summary{
			Declared: len(declarations),
			Defined:  definedCount,
			Clients:  configuredClients,
			Drift:    len(drift),
		},
	}, nil
}

func resolveDeclarations(workDir string) (projectRoot string, manifestPath string, declarations []string, err error) {
	manifestPath, err = findProjectManifest(workDir)
	if err != nil {
		return "", "", nil, err
	}
	if manifestPath == "" {
		abs, err := filepath.Abs(workDir)
		if err != nil {
			return "", "", nil, fmt.Errorf("resolve working directory: %w", err)
		}
		return abs, "", []string{}, nil
	}
	pf, err := projectfile.Load(manifestPath)
	if err != nil {
		return "", "", nil, err
	}
	scribeDir, err := paths.ScribeDir()
	if err != nil {
		return "", "", nil, err
	}
	kits, err := kit.LoadAll(filepath.Join(scribeDir, "kits"))
	if err != nil {
		return "", "", nil, err
	}
	declarations, err = kit.ResolveMCPServers(pf, kits)
	if err != nil {
		return "", "", nil, err
	}
	return filepath.Dir(manifestPath), manifestPath, declarations, nil
}

func findProjectManifest(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve start dir: %w", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("stat start dir: %w", err)
	}
	if !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for {
		candidate := filepath.Join(dir, projectfile.Filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("stat project file: %w", err)
		}

		if isGitRoot(dir) {
			return "", nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func isGitRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	return false
}

type projectMCPFile struct {
	MCPServers map[string]map[string]any `json:"mcpServers"`
}

func readDefinitions(path string) (map[string]map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]map[string]any{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	var file projectMCPFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if file.MCPServers == nil {
		file.MCPServers = map[string]map[string]any{}
	}
	return file.MCPServers, true, nil
}

func inspectClaude(projectRoot string, declared map[string]struct{}) Client {
	path := filepath.Join(projectRoot, ".claude", "settings.json")
	client := Client{Name: "claude", Path: path, State: ClientStateMissing, Projected: []string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return client
	}
	if err != nil {
		return unknownClient(client, fmt.Sprintf("read %s: %v", path, err))
	}
	var config struct {
		EnableAllProjectMCPServers bool     `json:"enableAllProjectMcpServers"`
		EnabledMCPJSONServers      []string `json:"enabledMcpjsonServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return unknownClient(client, fmt.Sprintf("parse %s: %v", path, err))
	}
	client.State = ClientStateConfigured
	client.Projected = sortedStrings(config.EnabledMCPJSONServers)
	if config.EnableAllProjectMCPServers {
		client.State = ClientStateUnknown
		client.Drift = append(client.Drift, Drift{
			Kind:    DriftUnknownClientState,
			Client:  client.Name,
			Path:    client.Path,
			Message: "claude enables all project MCP servers, so per-server projection cannot be determined",
		})
		return client
	}
	client.Drift = declaredConfigDrift(client.Name, client.Path, declared, stringSet(client.Projected), nil, nil)
	return client
}

func inspectCodex(projectRoot string, declared map[string]struct{}, definitions map[string]map[string]any) Client {
	path := filepath.Join(projectRoot, ".codex", "config.toml")
	client := Client{Name: "codex", Path: path, State: ClientStateMissing, Projected: []string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return client
	}
	if err != nil {
		return unknownClient(client, fmt.Sprintf("read %s: %v", path, err))
	}
	var config struct {
		MCPServers map[string]map[string]any `toml:"mcp_servers"`
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := toml.Unmarshal(data, &config); err != nil {
			return unknownClient(client, fmt.Sprintf("parse %s: %v", path, err))
		}
	}
	if config.MCPServers == nil {
		config.MCPServers = map[string]map[string]any{}
	}
	client.State = ClientStateConfigured
	client.Projected = sortedStrings(mapKeys(config.MCPServers))
	client.Drift = declaredConfigDrift(client.Name, client.Path, declared, stringSet(client.Projected), definitions, codexDefinition)
	return client
}

func inspectCursor(projectRoot string, declared map[string]struct{}, definitions map[string]map[string]any) Client {
	path := filepath.Join(projectRoot, ".cursor", "mcp.json")
	client := Client{Name: "cursor", Path: path, State: ClientStateMissing, Projected: []string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return client
	}
	if err != nil {
		return unknownClient(client, fmt.Sprintf("read %s: %v", path, err))
	}
	var config struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &config); err != nil {
			return unknownClient(client, fmt.Sprintf("parse %s: %v", path, err))
		}
	}
	if config.MCPServers == nil {
		config.MCPServers = map[string]map[string]any{}
	}
	client.State = ClientStateConfigured
	client.Projected = sortedStrings(mapKeys(config.MCPServers))
	client.Drift = declaredConfigDrift(client.Name, client.Path, declared, stringSet(client.Projected), definitions, cloneDefinition)
	return client
}

func unknownClient(client Client, message string) Client {
	client.State = ClientStateUnknown
	client.Drift = append(client.Drift, Drift{
		Kind:    DriftUnknownClientState,
		Client:  client.Name,
		Path:    client.Path,
		Message: message,
	})
	return client
}

func declaredConfigDrift(client, path string, declared, configured map[string]struct{}, definitions map[string]map[string]any, expected func(map[string]any) map[string]any) []Drift {
	var drift []Drift
	for server := range declared {
		if _, ok := configured[server]; !ok {
			drift = append(drift, Drift{
				Kind:    DriftDeclaredMissing,
				Client:  client,
				Server:  server,
				Path:    path,
				Message: fmt.Sprintf("%s is declared but not projected into %s", server, client),
			})
		}
	}
	for server := range configured {
		if _, ok := declared[server]; !ok {
			drift = append(drift, Drift{
				Kind:    DriftConfiguredUndeclared,
				Client:  client,
				Server:  server,
				Path:    path,
				Message: fmt.Sprintf("%s is configured in %s but not declared", server, client),
			})
		}
	}
	if definitions == nil || expected == nil {
		sortDrift(drift)
		return drift
	}
	for server := range declared {
		actual, ok := configuredDefinition(client, path, server)
		if !ok {
			continue
		}
		definition, ok := definitions[server]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(actual, expected(definition)) {
			drift = append(drift, Drift{
				Kind:    DriftConfigMismatch,
				Client:  client,
				Server:  server,
				Path:    path,
				Message: fmt.Sprintf("%s is projected into %s with a different config", server, client),
			})
		}
	}
	sortDrift(drift)
	return drift
}

func configuredDefinition(client, path, server string) (map[string]any, bool) {
	switch client {
	case "codex":
		var config struct {
			MCPServers map[string]map[string]any `toml:"mcp_servers"`
		}
		data, err := os.ReadFile(path)
		if err != nil || toml.Unmarshal(data, &config) != nil {
			return nil, false
		}
		def, ok := config.MCPServers[server]
		return def, ok
	case "cursor":
		var config struct {
			MCPServers map[string]map[string]any `json:"mcpServers"`
		}
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &config) != nil {
			return nil, false
		}
		def, ok := config.MCPServers[server]
		return def, ok
	default:
		return nil, false
	}
}

func codexDefinition(def map[string]any) map[string]any {
	out := cloneDefinition(def)
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

func cloneDefinition(def map[string]any) map[string]any {
	out := make(map[string]any, len(def))
	for k, v := range def {
		out[k] = v
	}
	return out
}

func filterDriftByServer(drift []Drift, server string) []Drift {
	out := make([]Drift, 0)
	for _, item := range drift {
		if item.Server == server {
			out = append(out, item)
		}
	}
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func sortDrift(drift []Drift) {
	sort.Slice(drift, func(i, j int) bool {
		if drift[i].Client != drift[j].Client {
			return drift[i].Client < drift[j].Client
		}
		if drift[i].Server != drift[j].Server {
			return drift[i].Server < drift[j].Server
		}
		return drift[i].Kind < drift[j].Kind
	})
}
