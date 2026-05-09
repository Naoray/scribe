package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/doctor"
	"github.com/Naoray/scribe/internal/logo"
	"github.com/Naoray/scribe/internal/skillmd"
	"github.com/Naoray/scribe/internal/state"
	"github.com/Naoray/scribe/internal/sync"
	"github.com/Naoray/scribe/internal/tools"
)

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Inspect managed skill health",
		Long: `Inspect managed skill health and report canonical metadata or projection drift.

Use --json for machine-readable output.

Examples:
  scribe doctor
  scribe doctor --skill recap
  scribe doctor --fix
  scribe doctor --json`,
		Args: cobra.NoArgs,
		RunE: runDoctor,
	}
	cmd.Flags().Bool("fix", false, "Normalize canonical skill metadata and repair affected projections")
	cmd.Flags().String("skill", "", "Inspect a single managed skill")
	return markJSONSupported(cmd)
}

type doctorIssueJSON struct {
	Skill         string                  `json:"skill"`
	Tool          string                  `json:"tool,omitempty"`
	Kind          string                  `json:"kind"`
	Status        string                  `json:"status"`
	Message       string                  `json:"message"`
	BudgetUsed    int                     `json:"budget_used,omitempty"`
	BudgetLimit   int                     `json:"budget_limit,omitempty"`
	BudgetPercent int                     `json:"budget_percent,omitempty"`
	LargestSkills []doctorSkillBudgetJSON `json:"largest_skills,omitempty"`
}

type doctorReportJSON struct {
	Skill  string            `json:"skill,omitempty"`
	Fix    bool              `json:"fix"`
	Issues []doctorIssueJSON `json:"issues"`
}

type doctorSkillBudgetJSON struct {
	Skill string `json:"skill"`
	Bytes int    `json:"bytes"`
}

type doctorFixResult struct {
	Name             string   `json:"name"`
	UpdatedCanonical bool     `json:"updated_canonical"`
	RepairedTools    []string `json:"repaired_tools,omitempty"`
}

var (
	doctorTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	doctorOKStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	doctorErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	doctorDimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#A3A3A3"))
	doctorMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	doctorBoldStyle  = lipgloss.NewStyle().Bold(true)
)

type doctorSkillSnapshot struct {
	Name         string
	Installed    state.InstalledSkill
	SkillContent []byte
	BaseContent  []byte
	Paths        []pathSnapshot
}

type pathSnapshot struct {
	Path       string
	Kind       string
	Mode       os.FileMode
	Data       []byte
	LinkTarget string
	BackupDir  string
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	fixFlag, _ := cmd.Flags().GetBool("fix")
	skillFlag, _ := cmd.Flags().GetString("skill")
	jsonFlag := jsonFlagPassed(cmd)

	if fixFlag && jsonFlag {
		err := fmt.Errorf("doctor: --fix cannot be combined with --json")
		return clierrors.Wrap(err, "USAGE_FLAG_CONFLICT", clierrors.ExitUsage,
			clierrors.WithRemediation("Run `scribe doctor --fix` for repairs or `scribe doctor --json` for inspection."),
		)
	}

	factory := newCommandFactory()

	cfg, err := factory.Config()
	if err != nil {
		return clierrors.Wrap(fmt.Errorf("load config: %w", err), "CONFIG_LOAD_FAILED", clierrors.ExitValid,
			clierrors.WithRemediation("Check ~/.scribe/config.yaml for invalid YAML or schema drift."),
		)
	}

	st, err := factory.State()
	if err != nil {
		return clierrors.Wrap(fmt.Errorf("load state: %w", err), "STATE_LOAD_FAILED", clierrors.ExitValid,
			clierrors.WithRemediation("Check ~/.scribe/state.yaml for invalid YAML or schema drift."),
		)
	}

	if skillFlag != "" {
		if _, ok := st.Installed[skillFlag]; !ok {
			err := fmt.Errorf("doctor: skill %q is not installed", skillFlag)
			return clierrors.Wrap(err, "SKILL_NOT_FOUND", clierrors.ExitNotFound,
				clierrors.WithResource(skillFlag),
				clierrors.WithRemediation("Run `scribe list` to see installed skills."),
			)
		}
	}

	report, err := doctor.InspectManagedSkills(cfg, st, skillFlag)
	if err != nil {
		return clierrors.Wrap(fmt.Errorf("inspect managed skills: %w", err), "DOCTOR_INSPECT_FAILED", clierrors.ExitValid,
			clierrors.WithRemediation("Run `scribe doctor` without filters to inspect all managed skills."),
		)
	}

	if fixFlag {
		results, err := applyDoctorFixes(cfg, st, skillFlag, report)
		if err != nil {
			return err
		}
		return writeDoctorFixText(cmd.OutOrStdout(), skillFlag, results)
	}

	if jsonFlag {
		r := jsonRendererForCommand(cmd, jsonFlag)
		if err := r.Result(buildDoctorReportJSON(skillFlag, report)); err != nil {
			return err
		}
		return r.Flush()
	}
	out := cmd.OutOrStdout()
	if isatty.IsTerminal(os.Stdout.Fd()) {
		width, _, _ := term.GetSize(int(os.Stdout.Fd()))
		if width <= 0 {
			width = 80
		}
		logo.Render(out, Version, width)
	}
	return writeDoctorText(out, skillFlag, report)
}

func applyDoctorFixes(cfg *config.Config, st *state.State, skillName string, report doctor.Report) ([]doctorFixResult, error) {
	results := make([]doctorFixResult, 0)
	applied := make([]doctorSkillSnapshot, 0)
	for _, name := range doctorSkillNames(st, skillName) {
		snapshot, err := snapshotDoctorSkill(cfg, st, name)
		if err != nil {
			if rollbackErr := rollbackDoctorSnapshots(st, applied); rollbackErr != nil {
				return nil, fmt.Errorf("%w (rollback: %v)", err, rollbackErr)
			}
			return nil, err
		}
		result, changed, err := applyDoctorFix(cfg, st, name, report)
		if err != nil {
			if rollbackErr := rollbackDoctorSnapshots(st, append(applied, snapshot)); rollbackErr != nil {
				return nil, fmt.Errorf("doctor: fix %s: %w (rollback: %v)", name, err, rollbackErr)
			}
			return nil, fmt.Errorf("doctor: fix %s: %w", name, err)
		}
		if changed {
			applied = append(applied, snapshot)
			results = append(results, result)
		}
	}
	return results, nil
}

func applyDoctorFix(cfg *config.Config, st *state.State, name string, report doctor.Report) (doctorFixResult, bool, error) {
	canonicalDir := filepath.Join(mustStoreDir(), name)
	skillPath := filepath.Join(canonicalDir, "SKILL.md")

	content, err := os.ReadFile(skillPath)
	if err != nil {
		return doctorFixResult{}, false, fmt.Errorf("read canonical SKILL.md: %w", err)
	}

	_, normalized, err := skillmd.Normalize(name, content)
	if err != nil {
		return doctorFixResult{}, false, err
	}

	needsProjectionRepair := doctorReportHasProjectionIssue(report, name)
	updatedCanonical := !bytes.Equal(content, normalized)
	if !updatedCanonical && !needsProjectionRepair {
		return doctorFixResult{}, false, nil
	}

	workingState := cloneState(st)
	result := doctorFixResult{Name: name}
	if updatedCanonical {
		if err := tools.WriteCanonicalSkill(canonicalDir, normalized); err != nil {
			return doctorFixResult{}, false, err
		}

		installed := workingState.Installed[name]
		installed.InstalledHash = sync.ComputeFileHash(normalized)
		workingState.Installed[name] = installed
		result.UpdatedCanonical = true
		needsProjectionRepair = true
	}

	if needsProjectionRepair {
		effectiveTools, err := doctorEffectiveTools(cfg, workingState.Installed[name])
		if err != nil {
			return doctorFixResult{}, false, err
		}
		if len(effectiveTools) > 0 {
			repairResult, err := repairSkillProjections(cfg, workingState, name)
			if err != nil {
				return doctorFixResult{}, false, err
			}
			result.RepairedTools = append(result.RepairedTools, repairResult.Tools...)
		} else {
			if err := cleanupInactiveSkillProjections(workingState, name); err != nil {
				return doctorFixResult{}, false, err
			}
			if err := workingState.Save(); err != nil {
				return doctorFixResult{}, false, err
			}
			if !result.UpdatedCanonical {
				result.RepairedTools = []string{"cleaned"}
			}
		}
	} else if result.UpdatedCanonical {
		if err := workingState.Save(); err != nil {
			return doctorFixResult{}, false, err
		}
	}

	*st = *workingState
	return result, true, nil
}

func doctorSkillNames(st *state.State, skillName string) []string {
	if skillName != "" {
		return []string{skillName}
	}

	names := make([]string, 0, len(st.Installed))
	for name, installed := range st.Installed {
		if installed.IsPackage() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func doctorReportHasProjectionIssue(report doctor.Report, name string) bool {
	for _, issue := range report.Issues {
		if issue.Skill == name && issue.Kind == doctor.IssueProjectionDrift {
			return true
		}
	}
	return false
}

func doctorEffectiveTools(cfg *config.Config, installed state.InstalledSkill) ([]string, error) {
	statuses, err := tools.ResolveStatuses(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve tools: %w", err)
	}
	return installed.EffectiveTools(availableToolNames(statuses)), nil
}

func cleanupInactiveSkillProjections(st *state.State, name string) error {
	if st == nil {
		return fmt.Errorf("load state: missing")
	}

	installed, ok := st.Installed[name]
	if !ok {
		return fmt.Errorf("skill %q is not installed", name)
	}

	pathSet := make(map[string]bool)
	for _, path := range doctorProjectionPaths(installed) {
		if strings.TrimSpace(path) != "" {
			pathSet[path] = true
		}
	}
	for _, conflict := range installed.Conflicts {
		if strings.TrimSpace(conflict.Path) != "" {
			pathSet[conflict.Path] = true
		}
	}

	for path := range pathSet {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove stale projection %s: %w", path, err)
		}
	}

	installed.ManagedPaths = nil
	installed.Paths = nil
	installed.Conflicts = nil
	st.Installed[name] = installed
	return nil
}

func doctorProjectionPaths(installed state.InstalledSkill) []string {
	if len(installed.ManagedPaths) > 0 {
		return append([]string(nil), installed.ManagedPaths...)
	}
	return append([]string(nil), installed.Paths...)
}

func cloneState(src *state.State) *state.State {
	if src == nil {
		return nil
	}

	cloned := &state.State{
		SchemaVersion:    src.SchemaVersion,
		LastSync:         src.LastSync,
		Installed:        make(map[string]state.InstalledSkill, len(src.Installed)),
		Migrations:       make(map[string]bool, len(src.Migrations)),
		RegistryFailures: make(map[string]state.RegistryFailure, len(src.RegistryFailures)),
	}
	for key, value := range src.Migrations {
		cloned.Migrations[key] = value
	}
	for key, value := range src.RegistryFailures {
		cloned.RegistryFailures[key] = value
	}
	for key, installed := range src.Installed {
		cloned.Installed[key] = cloneInstalledSkill(installed)
	}
	return cloned
}

func cloneInstalledSkill(src state.InstalledSkill) state.InstalledSkill {
	dst := src
	dst.Sources = append([]state.SkillSource(nil), src.Sources...)
	dst.Tools = append([]string(nil), src.Tools...)
	dst.Paths = append([]string(nil), src.Paths...)
	dst.ManagedPaths = append([]string(nil), src.ManagedPaths...)
	dst.Conflicts = append([]state.ProjectionConflict(nil), src.Conflicts...)
	return dst
}

func snapshotDoctorSkill(cfg *config.Config, st *state.State, name string) (doctorSkillSnapshot, error) {
	installed, ok := st.Installed[name]
	if !ok {
		return doctorSkillSnapshot{}, fmt.Errorf("skill %q is not installed", name)
	}

	canonicalDir := filepath.Join(mustStoreDir(), name)
	skillContent, err := os.ReadFile(filepath.Join(canonicalDir, "SKILL.md"))
	if err != nil {
		return doctorSkillSnapshot{}, fmt.Errorf("read canonical SKILL.md: %w", err)
	}
	baseContent, err := os.ReadFile(filepath.Join(canonicalDir, ".scribe-base.md"))
	if err != nil {
		return doctorSkillSnapshot{}, fmt.Errorf("read canonical .scribe-base.md: %w", err)
	}

	pathSet := make(map[string]bool)
	for _, path := range doctorProjectionPaths(installed) {
		if strings.TrimSpace(path) != "" {
			pathSet[path] = true
		}
	}
	for _, conflict := range installed.Conflicts {
		if strings.TrimSpace(conflict.Path) != "" {
			pathSet[conflict.Path] = true
		}
	}
	if effectiveTools, err := doctorEffectiveTools(cfg, installed); err == nil {
		for _, toolName := range effectiveTools {
			tool, err := tools.ResolveByName(cfg, toolName)
			if err != nil {
				continue
			}
			path, err := tool.SkillPath(name, "")
			if err != nil || strings.TrimSpace(path) == "" {
				continue
			}
			pathSet[path] = true
		}
	}

	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	snapshots := make([]pathSnapshot, 0, len(paths))
	for _, path := range paths {
		snapshot, err := snapshotPath(path)
		if err != nil {
			return doctorSkillSnapshot{}, err
		}
		snapshots = append(snapshots, snapshot)
	}

	return doctorSkillSnapshot{
		Name:         name,
		Installed:    cloneInstalledSkill(installed),
		SkillContent: skillContent,
		BaseContent:  baseContent,
		Paths:        snapshots,
	}, nil
}

func rollbackDoctorSnapshots(st *state.State, snapshots []doctorSkillSnapshot) error {
	if st == nil {
		return fmt.Errorf("load state: missing")
	}

	var restoreErrs []string
	for i := len(snapshots) - 1; i >= 0; i-- {
		snapshot := snapshots[i]
		canonicalDir := filepath.Join(mustStoreDir(), snapshot.Name)
		if err := os.WriteFile(filepath.Join(canonicalDir, "SKILL.md"), snapshot.SkillContent, 0o644); err != nil {
			restoreErrs = append(restoreErrs, fmt.Sprintf("%s canonical skill: %v", snapshot.Name, err))
		}
		if err := os.WriteFile(filepath.Join(canonicalDir, ".scribe-base.md"), snapshot.BaseContent, 0o644); err != nil {
			restoreErrs = append(restoreErrs, fmt.Sprintf("%s canonical base: %v", snapshot.Name, err))
		}
		for _, path := range snapshot.Paths {
			if err := restorePath(path); err != nil {
				restoreErrs = append(restoreErrs, fmt.Sprintf("%s path %s: %v", snapshot.Name, path.Path, err))
			}
		}
		st.Installed[snapshot.Name] = cloneInstalledSkill(snapshot.Installed)
	}
	if err := st.Save(); err != nil {
		restoreErrs = append(restoreErrs, fmt.Sprintf("save state: %v", err))
	}
	for _, snapshot := range snapshots {
		for _, path := range snapshot.Paths {
			if path.BackupDir != "" {
				_ = os.RemoveAll(path.BackupDir)
			}
		}
	}
	if len(restoreErrs) > 0 {
		return fmt.Errorf("%s", strings.Join(restoreErrs, "; "))
	}
	return nil
}

func snapshotPath(path string) (pathSnapshot, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return pathSnapshot{Path: path, Kind: "absent"}, nil
	}
	if err != nil {
		return pathSnapshot{}, fmt.Errorf("stat %s: %w", path, err)
	}

	snapshot := pathSnapshot{Path: path, Mode: info.Mode()}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return pathSnapshot{}, fmt.Errorf("readlink %s: %w", path, err)
		}
		snapshot.Kind = "symlink"
		snapshot.LinkTarget = target
	case info.IsDir():
		backupDir, err := os.MkdirTemp("", "doctor-path-*")
		if err != nil {
			return pathSnapshot{}, fmt.Errorf("create path snapshot for %s: %w", path, err)
		}
		backupPath := filepath.Join(backupDir, "contents")
		if err := copyPath(path, backupPath); err != nil {
			_ = os.RemoveAll(backupDir)
			return pathSnapshot{}, err
		}
		snapshot.Kind = "dir"
		snapshot.BackupDir = backupPath
	case info.Mode().IsRegular():
		data, err := os.ReadFile(path)
		if err != nil {
			return pathSnapshot{}, fmt.Errorf("read %s: %w", path, err)
		}
		snapshot.Kind = "file"
		snapshot.Data = data
	default:
		return pathSnapshot{}, fmt.Errorf("unsupported projection type at %s", path)
	}
	return snapshot, nil
}

func restorePath(snapshot pathSnapshot) error {
	if err := os.RemoveAll(snapshot.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	switch snapshot.Kind {
	case "absent":
		return nil
	case "symlink":
		if err := os.MkdirAll(filepath.Dir(snapshot.Path), 0o755); err != nil {
			return err
		}
		return os.Symlink(snapshot.LinkTarget, snapshot.Path)
	case "file":
		if err := os.MkdirAll(filepath.Dir(snapshot.Path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(snapshot.Path, snapshot.Data, snapshot.Mode.Perm())
	case "dir":
		return copyPath(snapshot.BackupDir, snapshot.Path)
	default:
		return fmt.Errorf("unknown path snapshot kind %q", snapshot.Kind)
	}
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("readlink %s: %w", src, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("read dir %s: %w", src, err)
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	case info.Mode().IsRegular():
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", src, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, info.Mode().Perm())
	default:
		return fmt.Errorf("unsupported path type at %s", src)
	}
}

func writeDoctorJSON(w io.Writer, skill string, report doctor.Report) error {
	out := buildDoctorReportJSON(skill, report)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func buildDoctorReportJSON(skill string, report doctor.Report) doctorReportJSON {
	out := doctorReportJSON{
		Skill:  skill,
		Fix:    false,
		Issues: make([]doctorIssueJSON, 0, len(report.Issues)),
	}
	for _, issue := range report.Issues {
		largest := make([]doctorSkillBudgetJSON, 0, len(issue.LargestSkills))
		for _, item := range issue.LargestSkills {
			largest = append(largest, doctorSkillBudgetJSON{
				Skill: item.Skill,
				Bytes: item.Bytes,
			})
		}
		out.Issues = append(out.Issues, doctorIssueJSON{
			Skill:         issue.Skill,
			Tool:          issue.Tool,
			Kind:          string(issue.Kind),
			Status:        issue.Status,
			Message:       issue.Message,
			BudgetUsed:    issue.BudgetUsed,
			BudgetLimit:   issue.BudgetLimit,
			BudgetPercent: issue.BudgetPercent,
			LargestSkills: largest,
		})
	}
	return out
}

func writeDoctorText(w io.Writer, skill string, report doctor.Report) error {
	tty := writerIsTerminal(w)
	var buf strings.Builder
	if err := writeDoctorTextWithOptions(&buf, skill, report, tty); err != nil {
		return err
	}
	out := buf.String()
	if !tty {
		out = stripANSI(out)
	}
	_, err := io.WriteString(w, out)
	return err
}

func writeGlobalListingBudgetIssue(w io.Writer, issue doctor.Issue) error {
	agent := doctorAgentLabel(issue.Tool)
	if _, err := fmt.Fprintf(w, "- %s skill-listing budget at %d/%d bytes (%d%%) [%s]\n", agent, issue.BudgetUsed, issue.BudgetLimit, issue.BudgetPercent, issue.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "    %s may truncate skill descriptions in this session.\n", agent); err != nil {
		return err
	}
	if len(issue.LargestSkills) > 0 {
		if _, err := fmt.Fprintln(w, "    Largest contributors:"); err != nil {
			return err
		}
		for _, item := range issue.LargestSkills {
			if _, err := fmt.Fprintf(w, "      - %s (%d bytes)\n", item.Skill, item.Bytes); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w, "    Action: scribe remove <name>  # or raise skillListingBudgetFraction in ~/.claude/settings.json")
	return err
}

func doctorAgentLabel(tool string) string {
	switch strings.ToLower(tool) {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "":
		return "Agent"
	default:
		return strings.ToUpper(tool[:1]) + tool[1:]
	}
}

func writeDoctorFixText(w io.Writer, skill string, results []doctorFixResult) error {
	tty := writerIsTerminal(w)
	var buf strings.Builder
	if err := writeDoctorFixTextStyled(&buf, skill, results); err != nil {
		return err
	}
	out := buf.String()
	if !tty {
		out = stripANSI(out)
	}
	_, err := io.WriteString(w, out)
	return err
}

func writeDoctorFixTextStyled(w io.Writer, skill string, results []doctorFixResult) error {
	if len(results) == 0 {
		_, err := fmt.Fprintln(w, doctorOKStyle.Render("✓")+" "+doctorDimStyle.Render("No managed skill issues found."))
		return err
	}

	if skill != "" {
		if _, err := fmt.Fprintln(w, doctorTitleStyle.Render(fmt.Sprintf("Repaired managed skill %s:", skill))); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, doctorTitleStyle.Render("Repaired managed skills:")); err != nil {
			return err
		}
	}

	for _, result := range results {
		if result.UpdatedCanonical {
			if _, err := fmt.Fprintf(w, "  %s %s normalized canonical SKILL.md\n", doctorOKStyle.Render("✓"), doctorBoldStyle.Render(result.Name)); err != nil {
				return err
			}
		}
		if len(result.RepairedTools) > 0 {
			if _, err := fmt.Fprintf(w, "  %s %s repaired projections (%s)\n", doctorOKStyle.Render("✓"), doctorBoldStyle.Render(result.Name), doctorDimStyle.Render(strings.Join(result.RepairedTools, ", "))); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(w, doctorDimStyle.Render(fmt.Sprintf("Repaired %d skills.", len(results))))
	return err
}

func writeDoctorTextWithOptions(w io.Writer, skill string, report doctor.Report, truncate bool) error {
	if len(report.Issues) == 0 {
		_, err := fmt.Fprintln(w, doctorOKStyle.Render("✓")+" "+doctorDimStyle.Render("No managed skill issues found."))
		return err
	}

	if skill != "" {
		if _, err := fmt.Fprintln(w, doctorTitleStyle.Render(fmt.Sprintf("scribe doctor — %d issues for %s", len(report.Issues), skill))); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, doctorTitleStyle.Render(fmt.Sprintf("scribe doctor — %d issues across %d skills", len(report.Issues), countDoctorIssueSkills(report.Issues)))); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	groups := groupDoctorIssuesByKind(report.Issues)
	for gi, group := range groups {
		if gi > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, doctorBoldStyle.Render(fmt.Sprintf("%s (%d)", prettyDoctorKind(group.Kind), group.Count))); err != nil {
			return err
		}
		if group.Kind == doctor.IssueGlobalListingBudgetOverflow {
			for _, issue := range group.Issues {
				if err := writeGlobalListingBudgetIssue(w, issue); err != nil {
					return err
				}
			}
			continue
		}
		rows := group.Rows
		remaining := 0
		if truncate && len(rows) > 10 {
			remaining = len(rows) - 10
			rows = rows[:10]
		}
		for _, row := range rows {
			if _, err := fmt.Fprintln(w, renderDoctorIssueRow(row)); err != nil {
				return err
			}
		}
		if remaining > 0 {
			if _, err := fmt.Fprintf(w, "  %s\n", doctorDimStyle.Render(fmt.Sprintf("… %d more  (run with --skill <name> or --json for full list)", remaining))); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(w, "\n"+doctorDimStyle.Render("Run `scribe doctor --fix` to repair drift and normalize canonical metadata."))
	return err
}

type doctorIssueGroup struct {
	Kind   doctor.IssueKind
	Count  int
	Issues []doctor.Issue
	Rows   []doctorIssueRow
}

type doctorIssueRow struct {
	Kind    doctor.IssueKind
	Skill   string
	Tools   []string
	Status  string
	Message string
}

func groupDoctorIssuesByKind(issues []doctor.Issue) []doctorIssueGroup {
	byKind := map[doctor.IssueKind][]doctor.Issue{}
	for _, issue := range issues {
		byKind[issue.Kind] = append(byKind[issue.Kind], issue)
	}

	groups := make([]doctorIssueGroup, 0, len(byKind))
	for kind, kindIssues := range byKind {
		groups = append(groups, doctorIssueGroup{
			Kind:   kind,
			Count:  len(kindIssues),
			Issues: kindIssues,
			Rows:   foldDoctorIssueRows(kind, kindIssues),
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return prettyDoctorKind(groups[i].Kind) < prettyDoctorKind(groups[j].Kind)
	})
	return groups
}

func foldDoctorIssueRows(kind doctor.IssueKind, issues []doctor.Issue) []doctorIssueRow {
	bySkill := map[string][]doctor.Issue{}
	for _, issue := range issues {
		bySkill[issue.Skill] = append(bySkill[issue.Skill], issue)
	}
	skills := make([]string, 0, len(bySkill))
	for skill := range bySkill {
		skills = append(skills, skill)
	}
	sort.Strings(skills)

	rows := make([]doctorIssueRow, 0, len(skills))
	for _, skill := range skills {
		skillIssues := bySkill[skill]
		sort.SliceStable(skillIssues, func(i, j int) bool {
			if skillIssues[i].Tool != skillIssues[j].Tool {
				return skillIssues[i].Tool < skillIssues[j].Tool
			}
			return skillIssues[i].Message < skillIssues[j].Message
		})
		switch kind {
		case doctor.IssueMigrationBudgetOverflow:
			rows = append(rows, doctorIssueRow{
				Kind:    kind,
				Skill:   tildePath(skill),
				Status:  foldedDoctorStatus(skillIssues),
				Message: strings.Join(migrationBudgetParts(skillIssues), doctorDimStyle.Render(" · ")),
			})
		default:
			rows = append(rows, doctorIssueRow{
				Kind:    kind,
				Skill:   tildePath(skill),
				Tools:   issueTools(skillIssues),
				Status:  foldedDoctorStatus(skillIssues),
				Message: summarizeDoctorIssueMessage(kind, skillIssues),
			})
		}
	}
	return rows
}

func renderDoctorIssueRow(row doctorIssueRow) string {
	status := renderDoctorStatus(row.Status)
	switch row.Kind {
	case doctor.IssueMigrationBudgetOverflow:
		return "  " + renderDoctorColumn(row.Skill, 42, doctorDimStyle) + " " + status + " " + row.Message
	default:
		toolLabel := strings.Join(row.Tools, ", ")
		if toolLabel == "" {
			toolLabel = "-"
		}
		return "  " + renderDoctorColumn(row.Skill, 16, doctorBoldStyle) + " " + renderDoctorColumn(toolLabel, 8, doctorMutedStyle) + " " + status + " " + row.Message
	}
}

func renderDoctorColumn(value string, width int, style lipgloss.Style) string {
	if len(value) < width {
		value += strings.Repeat(" ", width-len(value))
	}
	return style.Render(value)
}

func renderDoctorStatus(status string) string {
	if status == "" {
		status = "warn"
	}
	label := "[" + status + "]"
	if len(label) < 7 {
		label += strings.Repeat(" ", 7-len(label))
	}
	switch status {
	case "error":
		return doctorErrorStyle.Render(label)
	default:
		return doctorMutedStyle.Render(label)
	}
}

func foldedDoctorStatus(issues []doctor.Issue) string {
	status := ""
	for _, issue := range issues {
		switch issue.Status {
		case "error":
			return "error"
		case "warn":
			status = "warn"
		default:
			if status == "" {
				status = issue.Status
			}
		}
	}
	return status
}

func migrationBudgetParts(issues []doctor.Issue) []string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		label := issue.Tool
		if label == "" {
			label = "tool"
		}
		parts = append(parts, fmt.Sprintf("%s +%s", label, formatBytes(extractByteCount(issue.Message))))
	}
	return parts
}

func issueTools(issues []doctor.Issue) []string {
	seen := map[string]bool{}
	var tools []string
	for _, issue := range issues {
		if issue.Tool == "" || seen[issue.Tool] {
			continue
		}
		seen[issue.Tool] = true
		tools = append(tools, issue.Tool)
	}
	sort.Strings(tools)
	return tools
}

func summarizeDoctorIssueMessage(kind doctor.IssueKind, issues []doctor.Issue) string {
	seen := map[string]bool{}
	var parts []string
	for _, issue := range issues {
		for _, part := range strings.Split(issue.Message, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			part = summarizeDoctorMessagePart(kind, part)
			if !seen[part] {
				seen[part] = true
				parts = append(parts, part)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, doctorDimStyle.Render(" · "))
}

func summarizeDoctorMessagePart(kind doctor.IssueKind, part string) string {
	part = tildePath(part)
	if kind != doctor.IssueProjectionDrift {
		return part
	}
	switch {
	case strings.HasPrefix(part, "unexpected managed projection "):
		if path, ok := suffixAfter(part, " at "); ok {
			return "unexpected projection at " + path
		}
	case strings.HasPrefix(part, "missing managed projection for "):
		if path, ok := suffixAfter(part, " at "); ok {
			return "missing projection at " + path
		}
	case strings.Contains(part, " projection at ") && strings.HasSuffix(part, " is conflicted"):
		if path, ok := between(part, " projection at ", " is conflicted"); ok {
			return "conflicted projection at " + path
		}
	case strings.Contains(part, " projection at ") && strings.HasSuffix(part, " does not point to the canonical target"):
		if path, ok := between(part, " projection at ", " does not point to the canonical target"); ok {
			return "projection target mismatch at " + path
		}
	}
	return part
}

func suffixAfter(s, marker string) (string, bool) {
	i := strings.LastIndex(s, marker)
	if i < 0 {
		return "", false
	}
	return strings.TrimSpace(s[i+len(marker):]), true
}

func between(s, start, end string) (string, bool) {
	i := strings.Index(s, start)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:j]), true
}

func prettyDoctorKind(kind doctor.IssueKind) string {
	text := strings.ReplaceAll(string(kind), "_", " ")
	if text == "" {
		return ""
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

func countDoctorIssueSkills(issues []doctor.Issue) int {
	seen := map[string]bool{}
	for _, issue := range issues {
		seen[issue.Skill] = true
	}
	return len(seen)
}

func extractByteCount(message string) int64 {
	fields := strings.Fields(message)
	for i, field := range fields {
		if field == "bytes" && i > 0 {
			n, err := strconv.ParseInt(fields[i-1], 10, 64)
			if err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

func formatBytes(n int64) string {
	const kb = 1024
	const mb = 1024 * kb
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	default:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	}
}

func tildePath(s string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return s
	}
	return strings.ReplaceAll(s, home, "~")
}

type fdWriter interface {
	Fd() uintptr
}

func writerIsTerminal(w io.Writer) bool {
	f, ok := w.(fdWriter)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			if i+1 < len(s) && s[i+1] == '[' {
				i += 2
				for i < len(s) && (s[i] < '@' || s[i] > '~') {
					i++
				}
			} else if i+1 < len(s) {
				i++
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
