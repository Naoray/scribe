package state

// EffectiveTools returns the tool names a skill should be installed into,
// given the currently available (globally enabled) tool names.
//
// Inherit mode (default): union of available tools — the skill tracks global
// settings. Pinned mode: intersection of the skill's Tools with availability,
// preserving the user's chosen order.
//
// Packages (Type == "package") bypass per-skill routing and always receive the
// full available list. This mirrors the existing behavior where packages install
// independently of tool-facing symlinks.
func (s InstalledSkill) EffectiveTools(available []string) []string {
	if s.IsPackage() {
		return append([]string(nil), available...)
	}
	if s.ToolsMode != ToolsModePinned {
		return append([]string(nil), available...)
	}
	return intersectTools(s.Tools, available)
}

// EffectiveToolsForProject returns the tool names a skill should be installed
// into for a specific project scope.
//
// Project projections are more specific than the legacy/global Tools pin. This
// lets one project's projected tool set survive unrelated per-skill trimming in
// another scope.
func (s InstalledSkill) EffectiveToolsForProject(available []string, projectRoot string) []string {
	if s.IsPackage() {
		return append([]string(nil), available...)
	}
	for _, projection := range s.Projections {
		if projection.Project == projectRoot {
			return excludeTools(intersectTools(projection.Tools, available), projection.ExcludedTools)
		}
	}
	return s.EffectiveTools(available)
}

// NormalizeToolSelection dedupes a user-provided tool list while preserving
// order. Used by `scribe skill edit --tools` and --add.
func NormalizeToolSelection(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func intersectTools(selected, available []string) []string {
	availSet := make(map[string]bool, len(available))
	for _, a := range available {
		availSet[a] = true
	}
	out := make([]string, 0, len(selected))
	for _, t := range selected {
		if availSet[t] {
			out = append(out, t)
		}
	}
	return out
}

func excludeTools(selected, excluded []string) []string {
	if len(selected) == 0 || len(excluded) == 0 {
		return selected
	}
	drop := make(map[string]bool, len(excluded))
	for _, tool := range excluded {
		drop[tool] = true
	}
	out := selected[:0]
	for _, tool := range selected {
		if !drop[tool] {
			out = append(out, tool)
		}
	}
	return out
}
