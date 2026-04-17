package doctor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/skillmd"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/tools"
	"gopkg.in/yaml.v3"
)

type IssueKind string

const (
	IssueCanonicalMetadata IssueKind = "canonical_metadata"
	IssueProjectionDrift   IssueKind = "projection_drift"
)

type Issue struct {
	Skill   string
	Tool    string
	Kind    IssueKind
	Status  string
	Message string
}

type Report struct {
	Issues []Issue
}

func InspectManagedSkills(cfg *config.Config, st *state.State, name string) (Report, error) {
	if st == nil {
		return Report{}, fmt.Errorf("load state: missing")
	}

	names := managedSkillNames(st, name)
	if len(names) == 0 {
		return Report{}, nil
	}

	availableTools := availableToolNames(cfg)

	var issues []Issue
	for _, skillName := range names {
		skill := st.Installed[skillName]

		if skill.IsPackage() {
			continue
		}

		if canonicalIssues, err := inspectCanonicalMetadata(skillName); err != nil {
			return Report{}, err
		} else {
			issues = append(issues, canonicalIssues...)
		}

		if projectionIssue, ok := inspectProjectionDrift(cfg, skillName, skill, availableTools); ok {
			issues = append(issues, projectionIssue)
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Skill != issues[j].Skill {
			return issues[i].Skill < issues[j].Skill
		}
		if issues[i].Kind != issues[j].Kind {
			return issues[i].Kind < issues[j].Kind
		}
		if issues[i].Tool != issues[j].Tool {
			return issues[i].Tool < issues[j].Tool
		}
		return issues[i].Message < issues[j].Message
	})

	return Report{Issues: issues}, nil
}

func managedSkillNames(st *state.State, name string) []string {
	if name != "" {
		if _, ok := st.Installed[name]; !ok {
			return nil
		}
		return []string{name}
	}

	names := make([]string, 0, len(st.Installed))
	for skillName := range st.Installed {
		names = append(names, skillName)
	}
	sort.Strings(names)
	return names
}

func availableToolNames(cfg *config.Config) []string {
	seen := map[string]bool{}
	disabled := map[string]bool{}
	names := make([]string, 0)

	if cfg != nil {
		for _, tc := range cfg.Tools {
			if strings.TrimSpace(tc.Name) == "" {
				continue
			}
			if !tc.Enabled {
				disabled[strings.ToLower(tc.Name)] = true
			}
		}
	}

	for _, tool := range tools.DetectTools() {
		name := tool.Name()
		key := strings.ToLower(name)
		if seen[key] || disabled[key] {
			continue
		}
		seen[key] = true
		names = append(names, name)
	}
	if cfg == nil {
		return names
	}
	for _, tc := range cfg.Tools {
		if !tc.Enabled || strings.TrimSpace(tc.Name) == "" {
			continue
		}
		key := strings.ToLower(tc.Name)
		if seen[key] || disabled[key] {
			continue
		}
		seen[key] = true
		names = append(names, tc.Name)
	}
	return names
}

func inspectCanonicalMetadata(skillName string) ([]Issue, error) {
	canonicalDir, err := storeSkillDir(skillName)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(filepath.Join(canonicalDir, "SKILL.md"))
	if err != nil {
		return []Issue{{
			Skill:   skillName,
			Kind:    IssueCanonicalMetadata,
			Status:  "error",
			Message: fmt.Sprintf("read canonical SKILL.md: %v", err),
		}}, nil
	}

	_, normalized, err := skillmd.Normalize(skillName, content)
	if err != nil {
		return []Issue{{
			Skill:   skillName,
			Kind:    IssueCanonicalMetadata,
			Status:  "error",
			Message: err.Error(),
		}}, nil
	}

	if bytes.Equal(content, normalized) {
		return nil, nil
	}

	message := "SKILL.md needs canonical normalization"
	hasDescription, descErr := canonicalFrontmatterHasDescription(content)
	if descErr != nil {
		return []Issue{{
			Skill:   skillName,
			Kind:    IssueCanonicalMetadata,
			Status:  "error",
			Message: descErr.Error(),
		}}, nil
	}
	if !hasDescription {
		message = "SKILL.md is missing a description"
	}

	return []Issue{{
		Skill:   skillName,
		Kind:    IssueCanonicalMetadata,
		Status:  "warn",
		Message: message,
	}}, nil
}

func inspectProjectionDrift(cfg *config.Config, skillName string, skill state.InstalledSkill, availableTools []string) (Issue, bool) {
	expectedTools := skill.EffectiveTools(availableTools)
	if skill.IsPackage() {
		return Issue{}, false
	}

	type expectedProjection struct {
		Tool   string
		Target string
	}

	expectedPaths := make(map[string]expectedProjection, len(expectedTools))
	opaquePaths := make(map[string]bool, len(expectedTools))
	canonicalDir, err := storeSkillDir(skillName)
	if err != nil {
		return Issue{
			Skill:   skillName,
			Kind:    IssueProjectionDrift,
			Status:  "error",
			Message: fmt.Sprintf("resolve canonical dir: %v", err),
		}, true
	}

	for _, toolName := range expectedTools {
		tool, err := tools.ResolveByName(cfg, toolName)
		if err != nil {
			return Issue{
				Skill:   skillName,
				Tool:    toolName,
				Kind:    IssueProjectionDrift,
				Status:  "error",
				Message: fmt.Sprintf("resolve tool %q: %v", toolName, err),
			}, true
		}
		path, err := tool.SkillPath(skillName)
		if err != nil {
			return Issue{
				Skill:   skillName,
				Tool:    toolName,
				Kind:    IssueProjectionDrift,
				Status:  "error",
				Message: fmt.Sprintf("resolve projection path for %q: %v", toolName, err),
			}, true
		}
		target, inspectable := tool.CanonicalTarget(canonicalDir)
		if !inspectable {
			opaquePaths[path] = true
			continue
		}
		expectedPaths[path] = expectedProjection{Tool: toolName, Target: target}
	}

	actualPaths := projectionPaths(skill)
	actualSet := make(map[string]bool, len(actualPaths))
	for _, path := range actualPaths {
		if path == "" {
			continue
		}
		actualSet[path] = true
	}

	var details []string
	primaryTool := ""

	for _, conflict := range skill.Conflicts {
		if opaquePaths[conflict.Path] {
			continue
		}
		details = append(details, fmt.Sprintf("%s projection at %s is conflicted", conflict.Tool, conflict.Path))
		if primaryTool == "" && conflict.Tool != "" {
			primaryTool = conflict.Tool
		}
	}

	for _, path := range actualPaths {
		if path == "" {
			continue
		}
		if _, ok := expectedPaths[path]; ok {
			continue
		}
		if opaquePaths[path] {
			continue
		}
		toolName := inferToolName(path, cfg, skillName)
		details = append(details, fmt.Sprintf("unexpected managed projection %s at %s", toolName, path))
		if primaryTool == "" {
			primaryTool = toolName
		}
	}

	for path, expected := range expectedPaths {
		if !actualSet[path] {
			details = append(details, fmt.Sprintf("missing managed projection for %s at %s", expected.Tool, path))
			if primaryTool == "" {
				primaryTool = expected.Tool
			}
			continue
		}
		if !pathPointsToCanonical(path, expected.Target) {
			details = append(details, fmt.Sprintf("%s projection at %s does not point to the canonical target", expected.Tool, path))
			if primaryTool == "" {
				primaryTool = expected.Tool
			}
		}
	}

	if len(details) == 0 {
		return Issue{}, false
	}

	return Issue{
		Skill:   skillName,
		Tool:    primaryTool,
		Kind:    IssueProjectionDrift,
		Status:  "warn",
		Message: strings.Join(details, "; "),
	}, true
}

func projectionPaths(skill state.InstalledSkill) []string {
	if len(skill.ManagedPaths) > 0 {
		return append([]string(nil), skill.ManagedPaths...)
	}
	return append([]string(nil), skill.Paths...)
}

func inferToolName(path string, cfg *config.Config, skillName string) string {
	for _, toolName := range availableToolNames(cfg) {
		tool, err := tools.ResolveByName(cfg, toolName)
		if err != nil {
			continue
		}
		toolPath, err := tool.SkillPath(skillName)
		if err == nil && toolPath == path {
			return tool.Name()
		}
	}
	return ""
}

func pathPointsToCanonical(path, canonicalDir string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	canonicalResolved, err := filepath.EvalSymlinks(canonicalDir)
	if err != nil {
		canonicalResolved = canonicalDir
	}
	return resolved == canonicalResolved
}

func storeSkillDir(skillName string) (string, error) {
	base, err := tools.StoreDir()
	if err != nil {
		return "", fmt.Errorf("resolve store dir: %w", err)
	}
	return filepath.Join(base, skillName), nil
}

func canonicalFrontmatterHasDescription(content []byte) (bool, error) {
	type frontmatter struct {
		Description string `yaml:"description"`
	}

	normalized := normalizeLineEndings(content)
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return false, nil
	}

	lines := bytes.Split(normalized, []byte("\n"))
	if len(lines) < 2 {
		return false, fmt.Errorf("parse frontmatter: unterminated frontmatter")
	}

	var fmLines [][]byte
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(string(lines[i])) == "---" {
			end = i
			break
		}
		fmLines = append(fmLines, lines[i])
	}
	if end < 0 {
		return false, fmt.Errorf("parse frontmatter: unterminated frontmatter")
	}

	var fm frontmatter
	if err := yaml.Unmarshal(bytes.Join(fmLines, []byte("\n")), &fm); err != nil {
		return false, fmt.Errorf("parse frontmatter: %w", err)
	}
	return strings.TrimSpace(fm.Description) != "", nil
}

func normalizeLineEndings(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}
