package tools

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const toolGemini = "gemini"

// GeminiTool delegates skill lifecycle management to the native Gemini CLI.
type GeminiTool struct{}

func (t GeminiTool) Name() string { return toolGemini }

func (t GeminiTool) Detect() bool {
	_, err := exec.LookPath(toolGemini)
	return err == nil
}

func (t GeminiTool) Install(skillName, canonicalDir string) ([]string, error) {
	cmd := exec.Command(toolGemini, "skills", "link", canonicalDir, "--scope", "user", "--consent")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gemini skills link %q: %w%s", skillName, err, formatCommandOutput(out))
	}
	return []string{fmt.Sprintf("gemini:user:%s", skillName)}, nil
}

// SkillPath is not applicable for GeminiTool — Gemini manages skill paths
// internally via its CLI and does not expose a predictable filesystem location.
func (t GeminiTool) SkillPath(skillName string) (string, error) {
	return "", fmt.Errorf("gemini: skill path not available (managed by gemini CLI)")
}

// CanonicalTarget returns ok=false because Gemini owns its skill directory
// through the CLI; reconcile has no filesystem projection to inspect.
func (t GeminiTool) CanonicalTarget(_ string) (string, bool) {
	return "", false
}

func (t GeminiTool) Uninstall(skillName string) error {
	// Fail loudly when the gemini CLI is missing from PATH. Silently returning
	// nil would leave Gemini's side of the install in place while Scribe drops
	// its state entry — the user never learns cleanup was skipped.
	if _, err := exec.LookPath(toolGemini); err != nil {
		return fmt.Errorf("gemini CLI not found in PATH — skill %q may still be linked; run `gemini skills uninstall %s --scope user` manually", skillName, skillName)
	}
	cmd := exec.Command(toolGemini, "skills", "uninstall", skillName, "--scope", "user")
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(trimmed), "not found") {
			return nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 0 {
			return nil
		}
		return fmt.Errorf("gemini skills uninstall %q: %w%s", skillName, err, formatCommandOutput(out))
	}
	return nil
}

func formatCommandOutput(out []byte) string {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return ""
	}
	return ": " + trimmed
}
