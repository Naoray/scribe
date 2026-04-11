package firstrun

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/Naoray/scribe/internal/adopt"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/paths"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
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
		Emit:  func(msg any) {
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
func ApplyBuiltins(cfg *config.Config) {
	for _, builtin := range BuiltinRegistries() {
		if cfg.FindRegistry(builtin.Repo) == nil {
			cfg.AddRegistry(builtin)
		}
	}
}
