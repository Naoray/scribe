package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Naoray/scribe/internal/config"
	"github.com/Naoray/scribe/internal/doctor"
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
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

type doctorIssueJSON struct {
	Skill   string `json:"skill"`
	Tool    string `json:"tool,omitempty"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type doctorReportJSON struct {
	Skill  string            `json:"skill,omitempty"`
	Fix    bool              `json:"fix"`
	Issues []doctorIssueJSON `json:"issues"`
}

type doctorFixResult struct {
	Name             string   `json:"name"`
	UpdatedCanonical bool     `json:"updated_canonical"`
	RepairedTools    []string `json:"repaired_tools,omitempty"`
}

type doctorSkillSnapshot struct {
	Name        string
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
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if fixFlag && jsonFlag {
		return fmt.Errorf("doctor: --fix cannot be combined with --json")
	}

	factory := newCommandFactory()

	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	st, err := factory.State()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if skillFlag != "" {
		if _, ok := st.Installed[skillFlag]; !ok {
			return fmt.Errorf("doctor: skill %q is not installed", skillFlag)
		}
	}

	report, err := doctor.InspectManagedSkills(cfg, st, skillFlag)
	if err != nil {
		return fmt.Errorf("inspect managed skills: %w", err)
	}

	if fixFlag {
		results, err := applyDoctorFixes(cfg, st, skillFlag, report)
		if err != nil {
			return err
		}
		return writeDoctorFixText(cmd.OutOrStdout(), skillFlag, results)
	}

	if jsonFlag {
		return writeDoctorJSON(cmd.OutOrStdout(), skillFlag, report)
	}
	return writeDoctorText(cmd.OutOrStdout(), skillFlag, report)
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
			path, err := tool.SkillPath(name)
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
	out := doctorReportJSON{
		Skill:  skill,
		Fix:    false,
		Issues: make([]doctorIssueJSON, 0, len(report.Issues)),
	}
	for _, issue := range report.Issues {
		out.Issues = append(out.Issues, doctorIssueJSON{
			Skill:   issue.Skill,
			Tool:    issue.Tool,
			Kind:    string(issue.Kind),
			Status:  issue.Status,
			Message: issue.Message,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeDoctorText(w io.Writer, skill string, report doctor.Report) error {
	if len(report.Issues) == 0 {
		if skill != "" {
			_, err := fmt.Fprintf(w, "No managed skill issues found for %s.\n", skill)
			return err
		}
		_, err := fmt.Fprintln(w, "No managed skill issues found.")
		return err
	}

	if skill != "" {
		if _, err := fmt.Fprintf(w, "Managed skill issues for %s:\n", skill); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Managed skill issues:"); err != nil {
			return err
		}
	}

	for _, issue := range report.Issues {
		if issue.Tool != "" {
			if _, err := fmt.Fprintf(w, "- %s [%s] %s tool=%s: %s\n", issue.Skill, issue.Status, issue.Kind, issue.Tool, issue.Message); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "- %s [%s] %s: %s\n", issue.Skill, issue.Status, issue.Kind, issue.Message); err != nil {
			return err
		}
	}

	return nil
}

func writeDoctorFixText(w io.Writer, skill string, results []doctorFixResult) error {
	if len(results) == 0 {
		if skill != "" {
			_, err := fmt.Fprintf(w, "No managed skill issues found for %s.\n", skill)
			return err
		}
		_, err := fmt.Fprintln(w, "No managed skill issues found.")
		return err
	}

	if skill != "" {
		if _, err := fmt.Fprintf(w, "Repaired managed skill %s:\n", skill); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Repaired managed skills:"); err != nil {
			return err
		}
	}

	for _, result := range results {
		message := "repaired projections"
		if result.UpdatedCanonical {
			message = "normalized canonical SKILL.md"
		}
		if _, err := fmt.Fprintf(w, "- %s %s", result.Name, message); err != nil {
			return err
		}
		if len(result.RepairedTools) > 0 {
			if _, err := fmt.Fprintf(w, " and repaired tools: %s", strings.Join(result.RepairedTools, ", ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	return nil
}
