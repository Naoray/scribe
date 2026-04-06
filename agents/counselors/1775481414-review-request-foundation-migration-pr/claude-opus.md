I now have a thorough understanding of the full changeset. Here's my review.

---

# Foundation Migration Review

## Verdict: **Ship-ready with two issues to address**

The migration is well-structured, well-tested, and handles the critical namespace consistency correctly. The P0 fix is verified correct. I found one **real bug**, one **upgrade risk**, and a few **minor observations**.

---

## P0 Namespace Key Fix — Verified Correct

The critical question: do the keys the syncer writes match what Diff reads back?

- **Syncer writes**: `registrySlug + "/" + sk.Name` where `registrySlug = tools.SlugifyRegistry(teamRepo)` → `strings.ReplaceAll(repo, "/", "-")` (`syncer.go:210,237`)
- **Diff reads**: `registrySlug + "/" + entry.Name` using the same `SlugifyRegistry` (`syncer.go:71,76`)
- **State migration**: `strings.ReplaceAll(registries[0], "/", "-")` (`state.go:179`) — identical logic
- **Extra-skills scoping**: `strings.HasPrefix(name, registrySlug+"/")` (`syncer.go:107`) — matches

All four paths produce the same slug format. The fix is correct.

---

## BUG: Path traversal guard has a gap for file paths that equal the skill directory

`store.go:47`:
```go
if !strings.HasPrefix(filepath.Clean(dest), filepath.Clean(skillDir)+string(filepath.Separator)) && filepath.Clean(dest) != filepath.Clean(skillDir) {
```

The second condition (`filepath.Clean(dest) != filepath.Clean(skillDir)`) means a file with `Path: ""` or `Path: "."` would pass the check and attempt to write *to the skill directory itself* as a file. This would fail with a confusing error (`os.WriteFile` on a directory), but the guard should reject it cleanly. More importantly, a `Path` of just `"/"` after `filepath.Join` and `Clean` could produce unexpected results on edge-case platforms.

**Recommendation**: Add an explicit check for empty/dot paths before the traversal guard:

```go
if f.Path == "" || f.Path == "." {
    return "", fmt.Errorf("invalid file path: empty")
}
```

This is low severity — the data comes from GitHub API responses which won't produce these paths — but the guard claims to be defense-in-depth and should actually be thorough.

---

## UPGRADE RISK: State migration depends on `registries` field that may not exist

The `namespaceKey` function in `state.go:174` relies on `legacyInstalledSkill.Registries` to determine the correct namespace prefix. If a user's existing `state.json` has bare keys like `"deploy"` but the `registries` field was never populated (because it was added in a version between the old and new format), those skills get namespaced as `local/deploy` instead of `ArtistfyHQ-team-skills/deploy`.

**Impact**: On first sync after upgrade:
1. Old skills become orphaned under `local/` in state
2. Syncer looks for `ArtistfyHQ-team-skills/deploy`, finds nothing → re-installs everything
3. `scribe list` shows both `local/deploy` (extra) and `ArtistfyHQ-team-skills/deploy` (current)

This isn't data loss — it's a cosmetic annoyance and one extra sync cycle. But the orphaned `local/` entries will linger in state forever unless manually cleaned.

**Recommendation**: Either:
- (a) Accept this as expected behavior and document it in release notes ("first sync after upgrade may re-install skills"), or
- (b) Add a heuristic: if a bare key's `source` field contains a registry slug (e.g. `"github:ArtistfyHQ/team-skills@v1.0.0"`), extract the namespace from the source URL instead of relying on `registries`. Something like:

```go
if len(registries) == 0 && strings.HasPrefix(source, "github:") {
    // extract owner/repo from source string
}
```

I'd lean toward (a) for now — keep it simple, note it in the changelog.

---

## Config migration is solid

- YAML-first with TOML fallback: correct priority order (`config.go:73-116`)
- `team_repo` (singular) → `team_repos` (plural) handled (`config.go:120-123`)
- TOML preserved as backup after migration
- YAML wins when both exist — tested (`config_test.go:252-280`)
- Auto-migration on `Load()` is transparent to callers
- `AddRegistry` deduplication is correct (`config.go:52-63`)

No issues.

---

## File filter is well-scoped

`filter.go` only denies infrastructure files at the root level (`filepath.Dir(path) == "."`), so a skill with `lib/LICENSE` would correctly pass through. The deny list covers common cases. Case-insensitive matching via `strings.ToLower` is correct.

One minor gap: `README.md` passes through, which is fine — some skills include README as documentation. But `CHANGELOG.md`, `CONTRIBUTING.md`, and `CODE_OF_CONDUCT.md` also leak through if present at repo root. These are rarely useful in a skill directory. Consider adding them to the deny list, or accept the current scope as intentional.

---

## Tool interface migration (targets → tools) is clean

- `Tool` interface adds `Detect()` and `Uninstall()` — good for future `scribe uninstall`
- `DetectTools()` filters at runtime — only creates symlinks for installed tools
- `ClaudeTool.Uninstall` cleans up empty parent dirs for namespaced symlinks (`claude.go:54-57`)
- `CursorTool` flattens namespaced names to avoid subdirectories in `.cursor/rules/` (`cursor.go:59`)
- `replaceSymlink` is Remove-then-Symlink — acceptable non-atomic for CLI

The `Detect()` implementations check for the `.claude` and `.cursor` directories. This is reasonable but note: a user who installs Cursor *after* running `scribe sync` won't get Cursor links until the next sync. This is expected behavior, just calling it out.

---

## Discovery adapts correctly to namespaced layout

`discovery.go:88-129` handles both flat (`~/.scribe/skills/<name>/`) and namespaced (`~/.scribe/skills/<registrySlug>/<name>/`) directories. The `isSkillDir` heuristic (check for `SKILL.md`) is the right approach — registry-slug directories won't have `SKILL.md` at their root.

One edge case: if someone manually creates a `SKILL.md` inside a registry-slug directory (not in a subdirectory), it would be treated as a flat skill named after the registry slug. This is unlikely enough to ignore.

---

## Locking strategy

State uses `flock` with shared locks for reads and exclusive locks for writes (`state.go:247-267`). This protects against concurrent `scribe sync` runs (e.g., from multiple terminals). The lock file is separate from the state file (`.lock` suffix), which is correct — you don't want the lock file to be atomically replaced.

Note: `syscall.Flock` is POSIX-only. This will fail on Windows. If Windows support matters, this needs a conditional build tag or a cross-platform lock library. For a macOS/Linux CLI tool, this is fine.

---

## Summary

| Area | Verdict |
|------|---------|
| Namespace key consistency (P0) | **Correct** — all paths produce identical slugs |
| Config TOML→YAML migration | **Correct** — well-tested, idempotent |
| State flattening + key namespacing | **Correct** with one upgrade risk (see above) |
| targets→tools rename | **Clean** — minimal interface, good additions |
| Disk layout namespacing | **Correct** — path traversal guard has minor gap |
| File filter | **Correct** — scoped appropriately |
| Test coverage | **Good** — migration edge cases covered |
| Backward compatibility | **Good** — migrations are transparent on Load() |

**Ship it** with a changelog note about the first-sync-after-upgrade behavior, and optionally tighten the path traversal guard.
