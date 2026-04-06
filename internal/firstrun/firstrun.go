package firstrun

import (
	"errors"
	"io/fs"
	"os"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/paths"
)

// builtinRepos are well-known community registries auto-added during first run.
var builtinRepos = []string{
	"anthropic/skills",
	"openai/codex-skills",
	"expo/skills",
}

// BuiltinRegistries returns RegistryConfig entries for built-in registries.
func BuiltinRegistries() []config.RegistryConfig {
	registries := make([]config.RegistryConfig, len(builtinRepos))
	for i, repo := range builtinRepos {
		registries[i] = config.RegistryConfig{
			Repo:    repo,
			Enabled: true,
			Type:    config.RegistryTypeCommunity,
			Builtin: true,
		}
	}
	return registries
}

// IsFirstRun returns true if no config file exists yet.
func IsFirstRun() bool {
	path, err := paths.ConfigYAMLPath()
	if err != nil {
		return true
	}
	_, err = os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		// Also check legacy TOML path.
		tomlPath, tomlErr := paths.ConfigPath()
		if tomlErr != nil {
			return true
		}
		_, tomlErr = os.Stat(tomlPath)
		return errors.Is(tomlErr, fs.ErrNotExist)
	}
	return false
}

// ApplyBuiltins adds built-in registries to the config if not already present.
func ApplyBuiltins(cfg *config.Config) {
	for _, builtin := range BuiltinRegistries() {
		if cfg.FindRegistry(builtin.Repo) == nil {
			cfg.AddRegistry(builtin)
		}
	}
}
