package agent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/state"
	isync "github.com/Naoray/scribe/internal/sync"
	"gopkg.in/yaml.v3"
)

// EnsureScribeAgent installs or refreshes the embedded scribe-agent skill in
// the canonical store. It never performs network I/O.
func EnsureScribeAgent(store string, st *state.State, cfg *config.Config) (bool, error) {
	if cfg != nil && !cfg.ScribeAgent.Enabled {
		return false, nil
	}
	// Fast path: same binary version already on disk and files intact — skip render.
	// renderScribeAgentForStore uses time.Now() (for ShowDailyUpgradePrompt), so
	// checking the version first prevents spurious writes every 24 h when the
	// cooldown window flips and the rendered content no longer hash-matches.
	// We still verify both files exist and the base file passes frontmatter
	// validation so that corruption or manual edits are repaired.
	if installed, ok := st.Installed["scribe-agent"]; ok &&
		installed.Origin == state.OriginBootstrap &&
		len(installed.Sources) > 0 &&
		installed.Sources[0].Ref == EmbeddedVersion {
		skillDir := filepath.Join(store, "scribe-agent")
		_, skillErr := os.Stat(filepath.Join(skillDir, "SKILL.md"))
		_, claudeErr := os.Stat(filepath.Join(skillDir, "CLAUDE.md"))
		baseContent, baseErr := os.ReadFile(filepath.Join(skillDir, ".scribe-base.md"))
		if skillErr == nil && claudeErr == nil && baseErr == nil && validateSkillContent(baseContent) == nil {
			return false, nil
		}
	}
	content, _, err := renderScribeAgentForStore(store, st)
	if err != nil {
		return false, err
	}
	return InstallScribeAgent(store, st, content, EmbeddedVersion)
}

// InstallScribeAgent validates and installs a scribe-agent skill payload into
// the canonical store and state using the provided version string.
func InstallScribeAgent(store string, st *state.State, content []byte, version string) (bool, error) {
	if err := validateSkillContent(content); err != nil {
		return false, err
	}

	skillDir := filepath.Join(store, "scribe-agent")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	claudePath := filepath.Join(skillDir, "CLAUDE.md")
	basePath := filepath.Join(skillDir, ".scribe-base.md")

	if existingContent, err := os.ReadFile(skillPath); err == nil && skillMatches(existingContent, content) {
		existingClaudeContent, claudeErr := os.ReadFile(claudePath)
		if existingBaseContent, err := os.ReadFile(basePath); err == nil &&
			claudeErr == nil &&
			skillMatches(existingBaseContent, content) &&
			skillMatches(existingClaudeContent, EmbeddedClaudeTemplate) {
			if installed, ok := st.Installed["scribe-agent"]; ok && installed.Origin == state.OriginBootstrap {
				if len(installed.Sources) > 0 && installed.Sources[0].Ref == version {
					return false, nil
				}
			}
		}
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return false, fmt.Errorf("create bootstrap skill dir: %w", err)
	}
	if err := os.WriteFile(skillPath, content, 0o644); err != nil {
		return false, fmt.Errorf("write bootstrap skill: %w", err)
	}
	if err := os.WriteFile(claudePath, EmbeddedClaudeTemplate, 0o644); err != nil {
		return false, fmt.Errorf("write bootstrap claude: %w", err)
	}
	if err := os.WriteFile(basePath, content, 0o644); err != nil {
		return false, fmt.Errorf("write bootstrap base: %w", err)
	}

	existing := st.Installed["scribe-agent"]
	revision := existing.Revision
	if revision == 0 {
		revision = 1
	} else if !sameEmbeddedVersion(existing, version) {
		revision++
	}
	installedAt := existing.InstalledAt
	if installedAt.IsZero() {
		installedAt = time.Now().UTC()
	}

	st.Installed["scribe-agent"] = state.InstalledSkill{
		Revision:      revision,
		InstalledHash: isync.ComputeFileHash(content),
		Sources: []state.SkillSource{{
			Registry:   "Naoray/scribe",
			Ref:        version,
			LastSHA:    shortRef(version),
			LastSynced: time.Now().UTC(),
		}},
		InstalledAt:  installedAt,
		Tools:        existing.Tools,
		ToolsMode:    existing.ToolsMode,
		Paths:        existing.Paths,
		ManagedPaths: existing.ManagedPaths,
		Conflicts:    existing.Conflicts,
		Origin:       state.OriginBootstrap,
	}

	return true, nil
}

func skillMatches(content, want []byte) bool {
	return isync.ComputeFileHash(content) == isync.ComputeFileHash(want)
}

func sameEmbeddedVersion(installed state.InstalledSkill, version string) bool {
	if installed.Origin != state.OriginBootstrap || len(installed.Sources) == 0 {
		return false
	}
	return installed.Sources[0].Ref == version
}

func shortRef(ref string) string {
	if len(ref) <= 8 {
		return ref
	}
	return ref[:8]
}

func validateSkillContent(content []byte) error {
	type frontmatter struct {
		Name string `yaml:"name"`
	}
	var raw frontmatter
	if len(content) == 0 {
		return fmt.Errorf("validate scribe-agent skill: empty content")
	}
	parts := bytes.SplitN(content, []byte("---\n"), 3)
	if len(parts) < 3 {
		return fmt.Errorf("validate scribe-agent skill: missing frontmatter")
	}
	if err := yaml.Unmarshal(parts[1], &raw); err != nil {
		return fmt.Errorf("validate scribe-agent skill: parse frontmatter: %w", err)
	}
	if raw.Name != "scribe-agent" {
		return fmt.Errorf("validate scribe-agent skill: unexpected name %q", raw.Name)
	}
	return nil
}
