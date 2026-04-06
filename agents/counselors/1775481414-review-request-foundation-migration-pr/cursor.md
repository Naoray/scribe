## Verdict

Foundation migration is **close but not merge-safe yet**. The core namespace mismatch bug appears fixed for the happy path, but there are still **serious upgrade and safety risks**.

## Findings (ordered by severity)

### 1) **P0 Security**: `WriteToStore` can escape `~/.scribe/skills` with absolute skill names

`manifest.Validate()` only checks that `entry.name` is non-empty; it does not restrict path-like values. That name flows into `WriteToStore()`, which blocks `..` but does **not** block absolute paths.

```46:126:internal/manifest/manifest.go
type Entry struct {
	Name        string `yaml:"name"`
	// ...
}

for _, e := range m.Catalog {
	if e.Name == "" {
		return errors.New("catalog entry has empty name")
	}
	// no path-safety validation on e.Name
}
```

```20:38:internal/tools/store.go
func WriteToStore(registrySlug, skillName string, files []SkillFile) (string, error) {
	if strings.Contains(registrySlug, "..") { ... }
	if strings.Contains(skillName, "..") { ... }

	base, err := StoreDir()
	// ...
	skillDir := filepath.Join(base, registrySlug, skillName)
```

If `skillName` is `/tmp/pwn`, `filepath.Join(base, registrySlug, skillName)` resolves outside store.  
**Fix**: enforce `filepath.IsAbs(skillName) == false`, disallow path separators in skill names, and validate manifest entry names with a strict regex.

---

### 2) **P1 Upgrade regression**: old bare symlinks/rules are not migrated or cleaned, leading to duplicate installs

State migration rewrites keys but does not migrate or remove old tool link paths. Sync only relinks on missing/outdated entries; `current` entries are skipped, so old layout artifacts remain indefinitely.

```134:163:internal/state/state.go
for name, raw := range legacy.Installed {
	// ...
	skill := InstalledSkill{
		// ...
		Paths: ls.Paths,
	}
	nsKey := namespaceKey(name, ls.Registries)
	s.Installed[nsKey] = skill
}
```

```154:158:internal/sync/syncer.go
case StatusCurrent, StatusExtra:
	s.emit(SkillSkippedMsg{Name: sk.Name})
```

```210:216:internal/sync/syncer.go
qualifiedName := registrySlug + "/" + sk.Name
for _, t := range s.Tools {
	links, err := t.Install(qualifiedName, canonicalDir)
```

Net effect on upgraded users:
- old `~/.claude/skills/<name>` can coexist with new `~/.claude/skills/<slug>/<name>`
- old Cursor `deploy.mdc` can coexist with new `<slug>-deploy.mdc` (potential duplicate rule application)

**Fix**: one-time migration cleanup pass (remove legacy link locations and/or rewrite paths), plus an integration test for “old state + old links + first sync”.

---

### 3) **P1 Compatibility risk**: namespace matching is case-sensitive and can drift across config/state

You now namespace by slugified registry string. That fixes the prior bug when strings are identical, but there is no canonicalization (e.g., casing). A case-only difference (`ArtistfyHQ/team-skills` vs `artistfyhq/team-skills`) yields mismatch and false missing/extra.

```174:181:internal/state/state.go
if len(registries) > 0 {
	slug := strings.ReplaceAll(registries[0], "/", "-")
	return slug + "/" + name
}
```

```71:77:internal/sync/syncer.go
registrySlug := tools.SlugifyRegistry(teamRepo)
qualifiedName := registrySlug + "/" + entry.Name
installedPtr := lookupInstalled(st, qualifiedName)
```

**Fix**: normalize namespace keys (lowercase owner/repo before slugging) everywhere migration and lookup occur.

---

### 4) **P2 Completeness gap**: new `Config.Tools` model is not actually honored

You introduced tool config, but runtime sync resolution ignores it and uses auto-detect only. `add` even hardcodes both tools regardless of detection/config.

```34:37:internal/config/config.go
Tools      []ToolConfig     `yaml:"tools,omitempty"`
```

```91:95:internal/workflow/sync.go
func StepResolveTools(_ context.Context, b *Bag) error {
	if b.Tools == nil {
		b.Tools = tools.DetectTools()
	}
}
```

```85:86:cmd/add.go
tgts := []tools.Tool{tools.ClaudeTool{}, tools.CursorTool{}}
```

**Fix**: define precedence (`config.tools` override vs autodetect fallback) and apply consistently in `sync` + `add`.

---

## What’s correct (important)

- The key format fix itself looks correct **for same-cased registry strings**: migration uses slugified `registries[0]` and sync lookup uses slugified `teamRepo`.  
- Extra-skill scoping by registry prefix is correctly constrained now.
- File-level traversal protection for per-file paths is decent (`dest` prefix check), but top-level `skillName` validation is still insufficient (finding #1).

## Recommended next actions (before merge)

1. **Block unsafe names at manifest parse time** (strict regex, no separators, no absolute paths).
2. **Harden `WriteToStore`** with explicit absolute-path and separator checks.
3. **Add upgrade migration pass** to clean old bare links/legacy rule filenames.
4. **Canonicalize namespace** (lowercase repo slug) in both migration + lookup.
5. Add one integration test: old pre-migration disk/state -> first sync -> no duplicates + consistent namespaced state.
