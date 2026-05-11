package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/source"
)

const (
	RegistryTypeGitHub    = "github"    // kind: legacy-migrated registry (no type info at migration time)
	RegistryTypeTeam      = "team"      // kind: org/team registry with scribe.yaml
	RegistryTypeCommunity = "community" // kind: community registry (marketplace or tree scan)

	RegistryVisibilityPublic  = "public"
	RegistryVisibilityPrivate = "private"
	RegistryVisibilityUnknown = "unknown"
)

// RegistryConfig describes a connected skill registry.
type RegistryConfig struct {
	ID         string             `yaml:"id,omitempty"`
	Repo       string             `yaml:"repo,omitempty"`
	Enabled    bool               `yaml:"enabled"`
	Builtin    bool               `yaml:"builtin,omitempty"`
	Type       string             `yaml:"type,omitempty"`
	Visibility string             `yaml:"visibility,omitempty"`
	Writable   bool               `yaml:"writable,omitempty"`
	Source     *source.SourceSpec `yaml:"source,omitempty"`
}

type RegistrySource struct {
	ID       string
	Config   RegistryConfig
	Source   source.SourceSpec
	Identity source.SourceIdentity
}

// ToolConfig describes an AI tool target for skill installation.
type ToolConfig struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	Type      string `yaml:"type,omitempty"`      // "builtin" or "custom"
	Detect    string `yaml:"detect,omitempty"`    // shell command used to detect custom tools
	Install   string `yaml:"install,omitempty"`   // shell command template for custom tools
	Uninstall string `yaml:"uninstall,omitempty"` // shell command template for custom tools
	Path      string `yaml:"path,omitempty"`      // optional installed-path template for custom tools
}

// AdoptionConfig holds settings for local skill adoption scanning.
type AdoptionConfig struct {
	Mode  string   `yaml:"mode,omitempty"`  // "auto" | "prompt" | "off"
	Paths []string `yaml:"paths,omitempty"` // optional extra dirs; builtins always included
}

// ScribeAgentConfig controls embedded bootstrap behavior.
type ScribeAgentConfig struct {
	Enabled    bool `yaml:"enabled"`
	enabledSet bool `yaml:"-"`
}

func (c *ScribeAgentConfig) UnmarshalYAML(value *yaml.Node) error {
	type rawScribeAgentConfig struct {
		Enabled *bool `yaml:"enabled"`
	}
	var raw rawScribeAgentConfig
	if err := value.Decode(&raw); err != nil {
		return err
	}
	c.Enabled = true
	if raw.Enabled != nil {
		c.Enabled = *raw.Enabled
		c.enabledSet = true
	}
	return nil
}

// Config holds user preferences from ~/.scribe/config.yaml.
type Config struct {
	Registries      []RegistryConfig  `yaml:"registries,omitempty"`
	Token           string            `yaml:"token,omitempty"`
	Tools           []ToolConfig      `yaml:"tools,omitempty"`
	Editor          string            `yaml:"editor,omitempty"`
	Adoption        AdoptionConfig    `yaml:"adoption,omitempty"`
	ScribeAgent     ScribeAgentConfig `yaml:"scribe_agent"`
	BuiltinsVersion int               `yaml:"builtins_version,omitempty"`
	LazyGitHub      bool              `yaml:"-"`
}

// TeamRepos returns the list of enabled legacy registry repo strings.
// Backward-compatible helper for GitHub-only code that previously used Config.TeamRepos.
func (c *Config) TeamRepos() []string {
	var repos []string
	for _, r := range c.Registries {
		if r.Enabled {
			if repo := strings.TrimSpace(r.Repo); repo != "" {
				repos = append(repos, repo)
			}
		}
	}
	return repos
}

// AddRegistry adds or updates a registry in the config.
func (c *Config) AddRegistry(rc RegistryConfig) {
	rc.Normalize()
	for i := range c.Registries {
		if registriesMatch(c.Registries[i], rc) {
			c.Registries[i] = rc
			return
		}
	}
	c.Registries = append(c.Registries, rc)
}

// FindRegistry returns the RegistryConfig for a given repo, or nil if not found.
func (c *Config) FindRegistry(repo string) *RegistryConfig {
	for i := range c.Registries {
		if strings.EqualFold(c.Registries[i].SourceSpec().Repo, repo) {
			return &c.Registries[i]
		}
	}
	return nil
}

func (c *Config) FindRegistryByKeyOrRepo(input string) *RegistryConfig {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	for i := range c.Registries {
		rc := &c.Registries[i]
		if strings.EqualFold(rc.ID, input) || strings.EqualFold(rc.Repo, input) {
			return rc
		}
		spec := rc.SourceSpec()
		if strings.EqualFold(spec.Repo, input) || strings.EqualFold(spec.URL, input) || strings.EqualFold(spec.ID, input) {
			return rc
		}
		if canonical, ident, err := source.Canonicalize(spec); err == nil {
			if strings.EqualFold(ident.Key, input) || strings.EqualFold(canonical.URL, input) {
				return rc
			}
		}
	}
	return nil
}

// EnabledRegistries returns all registries that are enabled.
func (c *Config) EnabledRegistries() []RegistryConfig {
	var enabled []RegistryConfig
	for _, rc := range c.Registries {
		if rc.Enabled {
			enabled = append(enabled, rc)
		}
	}
	return enabled
}

func (c *Config) EnabledSources() []RegistrySource {
	var enabled []RegistrySource
	ids := map[string]int{}
	for _, rc := range c.Registries {
		if !rc.Enabled {
			continue
		}
		spec, ident, err := source.Canonicalize(rc.SourceSpec())
		if err != nil {
			continue
		}
		id := registrySourceID(rc, spec, ident)
		key := strings.ToLower(id)
		ids[key]++
		if ids[key] > 1 {
			id = fmt.Sprintf("%s-%d", id, ids[key])
		}
		enabled = append(enabled, RegistrySource{
			ID:       id,
			Config:   rc,
			Source:   spec,
			Identity: ident,
		})
	}
	return enabled
}

// AdoptionMode returns the validated adoption mode, defaulting to "auto".
func (c *Config) AdoptionMode() string {
	switch c.Adoption.Mode {
	case "auto", "prompt", "off":
		return c.Adoption.Mode
	default:
		return "auto"
	}
}

// AdoptionPaths returns the full list of directories to scan for adoptable skills.
// Builtins (~/.claude/skills and ~/.codex/skills) are always first, followed by
// any user-configured paths. Tilde and relative paths are resolved against the
// user's home directory. Paths outside the home directory are rejected.
func (c *Config) AdoptionPaths() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	// Resolve symlinks on home dir so comparisons work on macOS where
	// os.UserHomeDir() returns /Users/alice but /tmp and /var are symlinked
	// under /private/. EvalSymlinks requires the path to exist; home always does.
	resolvedHome := home
	if rh, err := filepath.EvalSymlinks(home); err == nil {
		resolvedHome = rh
	}

	builtins := []string{
		filepath.Join(resolvedHome, ".claude", "skills"),
		filepath.Join(resolvedHome, ".codex", "skills"),
	}

	result := make([]string, 0, len(builtins)+len(c.Adoption.Paths))
	result = append(result, builtins...)

	for _, p := range c.Adoption.Paths {
		var resolved string
		if strings.HasPrefix(p, "~/") {
			resolved = filepath.Join(home, p[2:])
		} else if filepath.IsAbs(p) {
			resolved = p
		} else {
			resolved = filepath.Join(home, p)
		}
		resolved = filepath.Clean(resolved)

		// Attempt to resolve symlinks so that e.g. /private/Users/alice/skills
		// compares correctly against resolvedHome=/private/Users/alice.
		// Adoption paths may not exist yet — that is fine; we handle ErrNotExist
		// by rebasing any home-relative path onto resolvedHome so the boundary
		// check stays consistent even when the path doesn't exist on disk yet.
		if rp, err := filepath.EvalSymlinks(resolved); err == nil {
			resolved = rp
		} else if errors.Is(err, fs.ErrNotExist) {
			// Path doesn't exist yet. If it starts with home, rebase onto
			// resolvedHome so the Rel comparison works correctly (avoids
			// mismatches when home itself is a symlink, e.g. macOS /var →
			// /private/var).
			if rel, relErr := filepath.Rel(home, resolved); relErr == nil && !strings.HasPrefix(rel, "..") {
				resolved = filepath.Join(resolvedHome, rel)
			}
		}
		// For other EvalSymlinks errors (permission denied, etc.) keep cleaned path.

		// Use filepath.Rel rather than strings.HasPrefix so that
		// /Users/alice-other is not mistakenly accepted as inside /Users/alice.
		rel, err := filepath.Rel(resolvedHome, resolved)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("adoption.paths entry %q is outside home", p)
		}

		result = append(result, resolved)
	}

	return result, nil
}

// IsTeam returns whether this is a team registry.
func (rc RegistryConfig) IsTeam() bool {
	return rc.Type == RegistryTypeTeam
}

// IsPublic returns true only for registries verified or migrated as public.
func (rc RegistryConfig) IsPublic() bool {
	return rc.Visibility == RegistryVisibilityPublic
}

func (rc RegistryConfig) SourceSpec() source.SourceSpec {
	var spec source.SourceSpec
	if rc.Source != nil {
		spec = *rc.Source
	} else {
		spec = source.SourceSpec{Type: source.SourceGitHub, Repo: rc.Repo}
	}
	if spec.ID == "" {
		spec.ID = rc.ID
	}
	if spec.Writable == nil && rc.Writable {
		writable := true
		spec.Writable = &writable
	}
	return spec
}

func ParseConfigSource(rc RegistryConfig) (source.SourceSpec, error) {
	spec, _, err := source.Canonicalize(rc.SourceSpec())
	if err != nil {
		return source.SourceSpec{}, err
	}
	return spec, nil
}

func (rc RegistryConfig) Identity() source.SourceIdentity {
	_, ident, err := source.Canonicalize(rc.SourceSpec())
	if err != nil {
		return source.SourceIdentity{}
	}
	return ident
}

// Normalize fills privacy-safe defaults for legacy or incomplete registry rows.
func (rc *RegistryConfig) Normalize() {
	if rc.Visibility == "" {
		rc.Visibility = VisibilityForLegacyType(rc.Type)
		return
	}
	rc.Visibility = NormalizeRegistryVisibility(rc.Visibility)
}

func (c *Config) ValidateSources() error {
	ids := map[string]struct{}{}
	keys := map[string]struct{}{}
	for _, rc := range c.Registries {
		if rc.ID != "" {
			id := strings.ToLower(rc.ID)
			if _, ok := ids[id]; ok {
				return fmt.Errorf("duplicate registry id %q", rc.ID)
			}
			ids[id] = struct{}{}
		}
		if rc.Source == nil && rc.Repo == "" {
			continue
		}
		spec, ident, err := source.Canonicalize(rc.SourceSpec())
		if err != nil {
			return err
		}
		if spec.ID != "" {
			id := strings.ToLower(spec.ID)
			if _, ok := ids[id]; ok && !strings.EqualFold(spec.ID, rc.ID) {
				return fmt.Errorf("duplicate registry id %q", spec.ID)
			}
			ids[id] = struct{}{}
		}
		if rc.Source != nil {
			key := strings.ToLower(ident.Key)
			if _, ok := keys[key]; ok {
				return fmt.Errorf("duplicate registry source key %q", ident.Key)
			}
			keys[key] = struct{}{}
		}
	}
	return nil
}

func NormalizeRegistryVisibility(visibility string) string {
	switch visibility {
	case RegistryVisibilityPublic, RegistryVisibilityPrivate, RegistryVisibilityUnknown:
		return visibility
	default:
		return RegistryVisibilityUnknown
	}
}

func VisibilityForLegacyType(regType string) string {
	switch regType {
	case RegistryTypeCommunity:
		return RegistryVisibilityPublic
	case RegistryTypeTeam:
		return RegistryVisibilityPrivate
	default:
		return RegistryVisibilityUnknown
	}
}

// legacyTOML is the shadow struct for reading old config.toml files.
type legacyTOML struct {
	TeamRepo  string   `toml:"team_repo"`
	TeamRepos []string `toml:"team_repos"`
	Token     string   `toml:"token"`
}

// Load reads ~/.scribe/config.yaml, falling back to config.toml with auto-migration.
// Priority: config.yaml > config.toml (migrated) > empty config.
func Load() (*Config, error) {
	yamlPath, err := paths.ConfigYAMLPath()
	if err != nil {
		return nil, err
	}

	// Try YAML first.
	data, err := os.ReadFile(yamlPath)
	if err == nil {
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parse config.yaml: %w", err)
		}
		cfg.applyDefaults()
		if err := cfg.ValidateSources(); err != nil {
			return nil, fmt.Errorf("validate config.yaml: %w", err)
		}
		if _, err := cfg.AdoptionPaths(); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	// No YAML -- try TOML migration.
	tomlPath, err := paths.ConfigPath()
	if err != nil {
		return nil, err
	}

	var raw legacyTOML
	if _, err := toml.DecodeFile(tomlPath, &raw); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg := &Config{}
			cfg.applyDefaults()
			return cfg, nil
		}
		return nil, fmt.Errorf("read config.toml: %w", err)
	}

	// Migrate TOML -> Config.
	cfg := migrateFromTOML(raw)
	cfg.applyDefaults()

	// Write YAML (keep TOML as backup).
	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("write migrated config.yaml: %w", err)
	}

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if !c.ScribeAgent.enabledSet {
		c.ScribeAgent.Enabled = true
	}
	for i := range c.Registries {
		c.Registries[i].Normalize()
	}
}

func registriesMatch(a, b RegistryConfig) bool {
	if a.ID != "" && b.ID != "" && strings.EqualFold(a.ID, b.ID) {
		return true
	}
	if a.Repo != "" && b.Repo != "" && strings.EqualFold(a.Repo, b.Repo) {
		return true
	}
	_, aIdent, aErr := source.Canonicalize(a.SourceSpec())
	_, bIdent, bErr := source.Canonicalize(b.SourceSpec())
	return aErr == nil && bErr == nil && strings.EqualFold(aIdent.Key, bIdent.Key)
}

func registrySourceID(rc RegistryConfig, spec source.SourceSpec, ident source.SourceIdentity) string {
	if rc.ID != "" {
		return rc.ID
	}
	if spec.ID != "" {
		return spec.ID
	}
	if rc.Repo != "" {
		return slug(rc.Repo)
	}
	if spec.Repo != "" {
		return slug(spec.Repo)
	}
	if spec.URL != "" {
		return slug(spec.URL)
	}
	return slug(ident.Key)
}

func slug(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	replacer := strings.NewReplacer("://", "-", "/", "-", ":", "-", "@", "-", ".", "-", "_", "-")
	s = replacer.Replace(s)
	s = strings.Trim(s, "-")
	if s == "" {
		return "source"
	}
	return s
}

// migrateFromTOML converts legacy TOML fields to the new Config struct.
func migrateFromTOML(raw legacyTOML) *Config {
	repos := raw.TeamRepos
	if len(repos) == 0 && raw.TeamRepo != "" {
		repos = []string{raw.TeamRepo}
	}

	cfg := &Config{Token: raw.Token}
	for _, repo := range repos {
		cfg.Registries = append(cfg.Registries, RegistryConfig{
			Repo:       repo,
			Enabled:    true,
			Type:       RegistryTypeGitHub,
			Visibility: RegistryVisibilityUnknown,
		})
	}
	return cfg
}

// Save writes the config to ~/.scribe/config.yaml atomically.
func (c *Config) Save() error {
	path, err := paths.ConfigYAMLPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
