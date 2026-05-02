package projectmigrate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Naoray/scribe/internal/budget"
	"github.com/Naoray/scribe/internal/projectfile"
	"github.com/Naoray/scribe/internal/state"
)

// ProjectChange describes the .scribe.yaml update for one selected project.
type ProjectChange struct {
	Project        string                   `json:"project"`
	File           string                   `json:"file"`
	AddedSkills    []string                 `json:"added_skills"`
	Skills         []string                 `json:"skills"`
	Changed        bool                     `json:"changed"`
	BudgetPerAgent map[string]budget.Result `json:"budget_per_agent,omitempty"`
}

// MigrationPlan is the complete set of writes/removals for the migration.
type MigrationPlan struct {
	DryRun        bool            `json:"dry_run"`
	GlobalLinks   []GlobalSymlink `json:"global_links"`
	ProjectFiles  []ProjectChange `json:"project_files"`
	RemovedLinks  []GlobalSymlink `json:"removed_links"`
	SkippedLinks  []GlobalSymlink `json:"skipped_links,omitempty"`
	SelectedPaths []string        `json:"selected_paths"`
}

// MigrationResult summarizes applied work.
type MigrationResult struct {
	DryRun                    bool               `json:"dry_run"`
	FoundGlobalLinks          int                `json:"found_global_links"`
	FoundSkills               int                `json:"found_skills"`
	SelectedProjects          int                `json:"selected_projects"`
	PlannedProjectFileWrites  int                `json:"planned_project_file_writes"`
	PlannedGlobalLinkRemovals int                `json:"planned_global_link_removals"`
	WroteProjectFiles         int                `json:"wrote_project_files"`
	RemovedGlobalLinks        int                `json:"removed_global_links"`
	SkippedGlobalLinks        int                `json:"skipped_global_links"`
	ProjectFiles              []ProjectChange    `json:"project_files"`
	RemovedLinks              []GlobalSymlink    `json:"removed_links"`
	SkippedLinks              []GlobalSymlink    `json:"skipped_links,omitempty"`
	CandidateProjects         []ProjectCandidate `json:"candidate_projects"`
}

// BuildPlan creates an idempotent migration plan for selected projects.
func BuildPlan(discovery Discovery, selectedProjects []string, dryRun bool, force ...bool) (MigrationPlan, error) {
	skills := discovery.Skills
	if len(skills) == 0 {
		return MigrationPlan{DryRun: dryRun, GlobalLinks: discovery.GlobalSymlinks}, nil
	}
	forceBudget := len(force) > 0 && force[0]

	selected := normalizeSelectedProjects(selectedProjects)
	projectFiles := make([]ProjectChange, 0, len(selected))
	for _, project := range selected {
		change, err := prepareProjectChange(project, skills)
		if err != nil {
			return MigrationPlan{}, err
		}
		budgetPerAgent, err := BudgetForProjectChange(change, discovery.GlobalSymlinks)
		if err != nil {
			return MigrationPlan{}, err
		}
		change.BudgetPerAgent = budgetPerAgent
		if !dryRun && !forceBudget {
			for agent, result := range budgetPerAgent {
				if result.Status == budget.StatusRefuse {
					return MigrationPlan{}, fmt.Errorf("project %s exceeds %s budget by %d bytes; pass --force to proceed", change.Project, agent, result.Used-result.Limit)
				}
			}
		}
		projectFiles = append(projectFiles, change)
	}

	return MigrationPlan{
		DryRun:        dryRun,
		GlobalLinks:   append([]GlobalSymlink(nil), discovery.GlobalSymlinks...),
		ProjectFiles:  projectFiles,
		RemovedLinks:  append([]GlobalSymlink(nil), discovery.GlobalSymlinks...),
		SelectedPaths: selected,
	}, nil
}

// Apply executes a migration plan. Dry-run plans return the same summary
// without mutating the filesystem.
func Apply(plan MigrationPlan, candidates []ProjectCandidate) (MigrationResult, error) {
	result := MigrationResult{
		DryRun:                    plan.DryRun,
		FoundGlobalLinks:          len(plan.GlobalLinks),
		FoundSkills:               len(uniqueSkills(plan.GlobalLinks)),
		SelectedProjects:          len(plan.ProjectFiles),
		PlannedGlobalLinkRemovals: len(plan.RemovedLinks),
		ProjectFiles:              append([]ProjectChange(nil), plan.ProjectFiles...),
		CandidateProjects:         append([]ProjectCandidate(nil), candidates...),
	}
	for _, change := range plan.ProjectFiles {
		if change.Changed {
			result.PlannedProjectFileWrites++
		}
	}

	if plan.DryRun {
		result.RemovedLinks = append([]GlobalSymlink(nil), plan.RemovedLinks...)
		return result, nil
	}

	snapshot := ""
	if len(plan.GlobalLinks) > 0 || len(plan.ProjectFiles) > 0 {
		captured, err := captureSnapshot(Discovery{
			GlobalSymlinks: append([]GlobalSymlink(nil), plan.GlobalLinks...),
			Projects:       append([]ProjectCandidate(nil), candidates...),
			Skills:         uniqueSkills(plan.GlobalLinks),
		}, plan)
		if err != nil {
			return result, err
		}
		snapshot, err = WriteSnapshot(captured)
		if err != nil {
			return result, err
		}
	}

	for _, change := range plan.ProjectFiles {
		if !change.Changed {
			continue
		}
		if err := writeProjectChange(change); err != nil {
			deleteSnapshot(snapshot)
			return result, err
		}
		result.WroteProjectFiles++
	}

	for _, link := range plan.RemovedLinks {
		removed, err := removeGlobalSymlink(link)
		if err != nil {
			deleteSnapshot(snapshot)
			return result, err
		}
		if removed {
			result.RemovedGlobalLinks++
			result.RemovedLinks = append(result.RemovedLinks, link)
		} else {
			result.SkippedGlobalLinks++
			result.SkippedLinks = append(result.SkippedLinks, link)
		}
	}

	if err := recordMigrationProjections(plan); err != nil {
		deleteSnapshot(snapshot)
		return result, err
	}

	return result, nil
}

func deleteSnapshot(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

func recordMigrationProjections(plan MigrationPlan) error {
	if len(plan.ProjectFiles) == 0 || len(plan.GlobalLinks) == 0 {
		return nil
	}
	st, err := state.Load()
	if err != nil {
		return err
	}
	if !applyMigrationProjections(st, plan, false) {
		return nil
	}
	return st.Save()
}

func applyMigrationProjections(st *state.State, plan MigrationPlan, clearLegacy bool) bool {
	changed := false
	toolsBySkill := toolsBySkill(plan.GlobalLinks)
	for skill, tools := range toolsBySkill {
		installed, ok := st.Installed[skill]
		if !ok {
			continue
		}
		if clearLegacy {
			installed.Projections = removeLegacyProjectionTools(installed.Projections, tools)
		}
		for _, change := range plan.ProjectFiles {
			installed.Projections = mergeMigrationProjection(installed.Projections, change.Project, tools)
		}
		st.Installed[skill] = installed
		changed = true
	}
	return changed
}

func toolsBySkill(links []GlobalSymlink) map[string][]string {
	bySkill := map[string][]string{}
	for _, link := range links {
		if !containsString(bySkill[link.Skill], link.Tool) {
			bySkill[link.Skill] = append(bySkill[link.Skill], link.Tool)
		}
	}
	for skill := range bySkill {
		sort.Strings(bySkill[skill])
	}
	return bySkill
}

func mergeMigrationProjection(projections []state.ProjectionEntry, project string, tools []string) []state.ProjectionEntry {
	next := state.ProjectionEntry{
		Project: project,
		Tools:   append([]string(nil), tools...),
		Source:  state.SourceMigration,
	}
	for i, projection := range projections {
		if projection.Project == project {
			projections[i] = next
			return projections
		}
	}
	return append(projections, next)
}

func removeLegacyProjectionTools(projections []state.ProjectionEntry, tools []string) []state.ProjectionEntry {
	out := projections[:0]
	for _, projection := range projections {
		if projection.Project != "" {
			out = append(out, projection)
			continue
		}
		remaining := removeStrings(projection.Tools, tools)
		if len(remaining) == 0 {
			continue
		}
		projection.Tools = remaining
		out = append(out, projection)
	}
	return out
}

func prepareProjectChange(project string, skills []string) (ProjectChange, error) {
	abs, err := filepath.Abs(project)
	if err != nil {
		return ProjectChange{}, fmt.Errorf("resolve project path: %w", err)
	}
	file := filepath.Join(abs, projectfile.Filename)
	pf, err := projectfile.Load(file)
	if err != nil {
		return ProjectChange{}, err
	}

	addSet := map[string]bool{}
	for _, skill := range pf.Add {
		addSet[skill] = true
	}
	removeSet := map[string]bool{}
	for _, skill := range pf.Remove {
		removeSet[skill] = true
	}

	var added []string
	changed := false
	for _, skill := range skills {
		if !addSet[skill] {
			addSet[skill] = true
			added = append(added, skill)
			changed = true
		}
		if removeSet[skill] {
			delete(removeSet, skill)
			changed = true
		}
	}

	nextAdd := sortedKeys(addSet)
	nextRemove := sortedKeys(removeSet)
	if !sameStrings(pf.Add, nextAdd) || !sameStrings(pf.Remove, nextRemove) {
		changed = true
	}

	return ProjectChange{
		Project:     abs,
		File:        file,
		AddedSkills: added,
		Skills:      nextAdd,
		Changed:     changed,
	}, nil
}

func writeProjectChange(change ProjectChange) error {
	pf, err := projectfile.Load(change.File)
	if err != nil {
		return err
	}
	addSet := map[string]bool{}
	for _, skill := range pf.Add {
		addSet[skill] = true
	}
	removeSet := map[string]bool{}
	for _, skill := range pf.Remove {
		removeSet[skill] = true
	}
	for _, skill := range change.Skills {
		addSet[skill] = true
		delete(removeSet, skill)
	}
	pf.Add = sortedKeys(addSet)
	pf.Remove = sortedKeys(removeSet)
	return projectfile.Save(change.File, pf)
}

func removeGlobalSymlink(link GlobalSymlink) (bool, error) {
	info, err := os.Lstat(link.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat global link %s: %w", link.Path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	if err := os.Remove(link.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("remove global link %s: %w", link.Path, err)
	}
	return true, nil
}

func normalizeSelectedProjects(projects []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, project := range projects {
		if project == "" {
			continue
		}
		abs, err := filepath.Abs(project)
		if err != nil {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func removeStrings(values []string, remove []string) []string {
	removeSet := map[string]bool{}
	for _, value := range remove {
		removeSet[value] = true
	}
	out := values[:0]
	for _, value := range values {
		if !removeSet[value] {
			out = append(out, value)
		}
	}
	return out
}
