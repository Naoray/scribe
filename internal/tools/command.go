package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

// CommandTool is a configurable tool backed by shell command templates.
type CommandTool struct {
	ToolName         string
	DetectCommand    string
	InstallCommand   string
	UninstallCommand string
	PathTemplate     string
}

func (t CommandTool) Name() string { return t.ToolName }

func (t CommandTool) Detect() bool {
	if strings.TrimSpace(t.DetectCommand) == "" {
		return false
	}
	return runShell(t.DetectCommand) == nil
}

func (t CommandTool) Install(skillName, canonicalDir string) ([]string, error) {
	cmd := renderTemplate(t.InstallCommand, t.ToolName, skillName, canonicalDir)
	if err := runShell(cmd); err != nil {
		return nil, fmt.Errorf("install %s via %s: %w", skillName, t.ToolName, err)
	}

	path := renderPathTemplate(t.PathTemplate, t.ToolName, skillName, canonicalDir)
	return []string{path}, nil
}

// SkillPath returns the path this tool would link for skillName, using PathTemplate.
// Returns an error if no PathTemplate is configured (path is unknown).
func (t CommandTool) SkillPath(skillName string) (string, error) {
	if strings.TrimSpace(t.PathTemplate) == "" {
		return "", fmt.Errorf("command tool %q: no path_template configured", t.ToolName)
	}
	return renderPathTemplate(t.PathTemplate, t.ToolName, skillName, ""), nil
}

// CanonicalTarget returns ok=false for CommandTool — the projection shape is
// defined by user-supplied shell templates and Scribe has no way to know
// which path inside canonicalDir the install step mirrored.
func (t CommandTool) CanonicalTarget(_ string) (string, bool) {
	return "", false
}

func (t CommandTool) Uninstall(skillName string) error {
	cmd := renderTemplate(t.UninstallCommand, t.ToolName, skillName, "")
	if err := runShell(cmd); err != nil {
		return fmt.Errorf("uninstall %s via %s: %w", skillName, t.ToolName, err)
	}
	return nil
}

func runShell(command string) error {
	out, err := exec.Command("sh", "-c", command).CombinedOutput()
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, trimmed)
}

func renderTemplate(template, toolName, skillName, canonicalDir string) string {
	repl := strings.NewReplacer(
		"{{tool_name}}", toolName,
		"{{skill_name}}", skillName,
		"{{canonical_dir}}", canonicalDir,
		"{{skill_dir}}", canonicalDir,
		"{{skill_md}}", strings.TrimSuffix(canonicalDir, "/")+"/SKILL.md",
	)
	return repl.Replace(template)
}

func renderPathTemplate(template, toolName, skillName, canonicalDir string) string {
	if strings.TrimSpace(template) == "" {
		return fmt.Sprintf("tool:%s:%s", toolName, skillName)
	}
	return renderTemplate(template, toolName, skillName, canonicalDir)
}
