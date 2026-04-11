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
	if s.Type == "package" {
		return append([]string(nil), available...)
	}
	if s.ToolsMode != ToolsModePinned {
		return append([]string(nil), available...)
	}
	availSet := make(map[string]bool, len(available))
	for _, a := range available {
		availSet[a] = true
	}
	out := make([]string, 0, len(s.Tools))
	for _, t := range s.Tools {
		if availSet[t] {
			out = append(out, t)
		}
	}
	return out
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
