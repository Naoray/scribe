package firstrun

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
)

const removeOpenAICodexMigration = "remove_openai_codex_v1"
const renameBuiltinReposMigration = "rename_builtin_repos_v1"
const removeNaorayScribeMigration = "remove_naoray_scribe_v1"

//go:embed default_registries.yaml
var defaultRegistriesYAML []byte

// defaultRegistriesConfig is the shape of default_registries.yaml.
type defaultRegistriesConfig struct {
	Version    int `yaml:"version"`
	Registries []struct {
		Repo string `yaml:"repo"`
		Type string `yaml:"type"`
	} `yaml:"registries"`
}

// parsedDefaults is populated once at init from the embedded YAML.
var parsedDefaults = func() defaultRegistriesConfig {
	var cfg defaultRegistriesConfig
	if err := yaml.Unmarshal(defaultRegistriesYAML, &cfg); err != nil {
		panic(fmt.Sprintf("firstrun: parse default_registries.yaml: %v", err))
	}
	return cfg
}()

// BuiltinRegistries returns RegistryConfig entries for built-in registries
// as declared in default_registries.yaml.
func BuiltinRegistries() []config.RegistryConfig {
	registries := make([]config.RegistryConfig, len(parsedDefaults.Registries))
	for i, r := range parsedDefaults.Registries {
		registries[i] = config.RegistryConfig{
			Repo:    r.Repo,
			Enabled: true,
			Type:    r.Type,
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

// PromptAdoption runs a one-shot Y/n adoption prompt after first-run setup.
// It ignores cfg.Adoption.Mode — firstrun always prompts regardless of persisted config.
// If there are no candidates, it returns nil and writes nothing.
// Errors from candidate discovery or apply are logged to out (stderr channel) and do not fail firstrun.
func PromptAdoption(cfg *config.Config, st *state.State, toolSet []tools.Tool, in io.Reader, out io.Writer) error {
	candidates, _, err := adopt.FindCandidates(st, cfg.Adoption)
	if err != nil {
		fmt.Fprintf(out, "scribe: adoption scan warning: %v\n", err)
		return nil
	}
	if len(candidates) == 0 {
		return nil
	}

	// Summarise what was found.
	fmt.Fprintf(out, "Scribe found %d unmanaged skill(s) in your tool directories.\n", len(candidates))

	yes := promptYN(in, out, "Adopt them now?", true)
	if !yes {
		fmt.Fprintln(out, "Skipped adoption. Run 'scribe adopt' later to review.")
		return nil
	}

	adopter := &adopt.Adopter{
		State: st,
		Tools: toolSet,
		Emit: func(msg any) {
			switch m := msg.(type) {
			case adopt.AdoptedMsg:
				fmt.Fprintf(out, "  + adopted: %s\n", m.Name)
			case adopt.AdoptErrorMsg:
				fmt.Fprintf(out, "  ! error adopting %s: %v\n", m.Name, m.Err)
			}
		},
	}

	result := adopter.Apply(candidates)
	fmt.Fprintf(out, "Adopted %d skill(s).\n", len(result.Adopted))
	return nil
}

// promptYN writes question to out and reads a Y/n response from in.
// defaultYes controls what an empty/unrecognised response resolves to.
// Returns true for yes, false for no.
func promptYN(in io.Reader, out io.Writer, question string, defaultYes bool) bool {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	fmt.Fprintf(out, "%s %s ", question, hint)

	scanner := bufio.NewScanner(in)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		switch answer {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		default:
			return defaultYes
		}
	}
	return defaultYes
}

// ApplyBuiltins adds built-in registries to the config if not already present.
// firstRun is true only when the config had no prior builtins version.
func ApplyBuiltins(cfg *config.Config) ([]string, bool) {
	firstRun := cfg.BuiltinsVersion == 0
	if cfg.BuiltinsVersion >= parsedDefaults.Version {
		return nil, false
	}

	var added []string
	for _, builtin := range BuiltinRegistries() {
		if cfg.FindRegistry(builtin.Repo) == nil {
			cfg.AddRegistry(builtin)
			added = append(added, builtin.Repo)
		}
	}
	cfg.BuiltinsVersion = parsedDefaults.Version
	return added, firstRun
}

func ApplyBuiltinsRemove(cfg *config.Config, st *state.State, removed []string) ([]string, bool) {
	if st != nil && st.HasMigration(removeOpenAICodexMigration) {
		return nil, false
	}

	removeSet := map[string]bool{}
	for _, repo := range removed {
		removeSet[strings.ToLower(repo)] = true
	}

	kept := cfg.Registries[:0]
	var pruned []string
	for _, rc := range cfg.Registries {
		if removeSet[strings.ToLower(rc.Repo)] {
			pruned = append(pruned, rc.Repo)
			continue
		}
		kept = append(kept, rc)
	}
	cfg.Registries = kept

	if st != nil {
		st.MarkMigration(removeOpenAICodexMigration)
		for _, repo := range pruned {
			st.ClearRegistryFailure(repo)
		}
	}

	return pruned, true
}

// RemoveNaorayScribeRegistry removes the Naoray/scribe built-in registry from the
// config on a one-shot migration. The entry is only removed if it carries
// Builtin: true — registries the user added manually are left untouched.
//
// Background: scribe-agent is now managed by the embedded binary
// (EnsureScribeAgent) rather than a registry sync, so the source repo no
// longer needs to be a connected registry.
func RemoveNaorayScribeRegistry(cfg *config.Config, st *state.State) ([]string, bool) {
	if st != nil && st.HasMigration(removeNaorayScribeMigration) {
		return nil, false
	}

	kept := cfg.Registries[:0]
	var pruned []string
	for _, rc := range cfg.Registries {
		if strings.EqualFold(rc.Repo, "Naoray/scribe") && rc.Builtin {
			pruned = append(pruned, rc.Repo)
			continue
		}
		kept = append(kept, rc)
	}
	cfg.Registries = kept

	if st != nil {
		st.MarkMigration(removeNaorayScribeMigration)
		for _, repo := range pruned {
			st.ClearRegistryFailure(repo)
		}
	}

	return pruned, true
}

func ApplyBuiltinsRename(cfg *config.Config, st *state.State, renamed map[string]string) ([]string, bool) {
	if st != nil && st.HasMigration(renameBuiltinReposMigration) {
		return nil, false
	}

	type renameOp struct {
		from string
		to   string
	}
	var ops []renameOp
	for from, to := range renamed {
		ops = append(ops, renameOp{from: from, to: to})
	}

	var applied []string
	for _, op := range ops {
		src := cfg.FindRegistry(op.from)
		if src == nil {
			continue
		}

		if cfg.FindRegistry(op.to) == nil {
			replacement := *src
			replacement.Repo = op.to
			cfg.AddRegistry(replacement)
		}

		kept := cfg.Registries[:0]
		for _, rc := range cfg.Registries {
			if strings.EqualFold(rc.Repo, op.from) {
				continue
			}
			kept = append(kept, rc)
		}
		cfg.Registries = kept
		applied = append(applied, op.from+" -> "+op.to)
	}

	if st != nil {
		st.MarkMigration(renameBuiltinReposMigration)
		for _, op := range ops {
			st.ClearRegistryFailure(op.from)
		}
	}

	return applied, true
}
