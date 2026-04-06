This review evaluates the foundation migration (`feat/foundation-migration`) for the Scribe CLI.

### Executive Summary
The PR represents a significant and necessary architectural improvement. The move to a namespaced state and disk layout resolves long-standing issues with registry-to-registry collisions. The implementation of atomic state/config writes and advisory locking is excellent. However, there is a **functional regression in local skill discovery** that will cause legacy skills to appear duplicated or "ghosted" in the list view, and a potential **loss of user data** in the Cursor rule generation logic.

---

### Critical Issues & Risks

#### 1. Discovery Regression: Legacy Local Skills
The namespacing migration in `state.go` correctly prefixes legacy bare keys with `local/`. However, `internal/discovery/discovery.go` does not account for this when scanning the root of the skill store.

*   **The Bug:** When `OnDisk` finds a directory like `~/.scribe/skills/deploy`, it calls `buildSkill` with `name="deploy"`. `buildSkill` attempts to lookup `st.Installed["deploy"]`, which is now empty (it’s now `local/deploy`).
*   **The Result:** Legacy skills will appear in `scribe list` as unmanaged/unknown. Worse, the cleanup loop at the end of `OnDisk` will find the `local/deploy` entry in state (which wasn't marked as "seen" under that key) and add a *second* entry for the same skill, marked as missing from disk.
*   **Fix:** `buildSkill` and `OnDisk` must be aware that skills found in the root of the store directory are implicitly in the `local/` namespace.

#### 2. Data Loss: Cursor MDC Overwrite
In `internal/tools/cursor.go`, the `Install` method unconditionally overwrites `.cursor.mdc` in the canonical skill store:

```go
mdcPath := filepath.Join(canonicalDir, ".cursor.mdc")
mdc := generateMDC(skillMD)
if err := os.WriteFile(mdcPath, mdc, 0o644); err != nil { ... }
```

*   **The Risk:** If a skill author has provided a finely-tuned `.cursor.mdc` in their repository, Scribe will clobber it with a generic version generated from the `SKILL.md` frontmatter.
*   **Recommendation:** Check if `.cursor.mdc` already exists in the `canonicalDir` before overwriting. Only generate it if it’s missing.

#### 3. Security: Tool Path Traversal
While `WriteToStore` validates `sk.Name` for `..` segments, the `Tool` interface implementations (`ClaudeTool` and `CursorTool`) do not validate the `skillName` passed to them.

*   **The Risk:** If a manifest name somehow bypasses earlier validation (or if the tools are used elsewhere), `ClaudeTool` could be tricked into overwriting arbitrary files via symlink:
    *   `link := filepath.Join(skillsDir, skillName)` where `skillName` is `../../.ssh/id_rsa`.
*   **Recommendation:** Add a simple path traversal guard at the start of `ClaudeTool.Install` and `CursorTool.Install`, or rely on a shared validation helper.

---

### Architecture & Correctness

#### Namespacing & State Migration
The "P0 bug" fix (matching the syncer’s lookup key with the state migration key) is **verified as correct**.
*   **Syncer:** Uses `tools.SlugifyRegistry(teamRepo) + "/" + entry.Name`.
*   **Migration:** Uses `strings.ReplaceAll(registries[0], "/", "-") + "/" + name`.
*   Both implementations result in `owner-repo/name`, ensuring that users upgrading will not lose their "installed" status for registry skills.

#### State Concurrency
The introduction of `syscall.Flock` for advisory locking on the state file is a major win. The use of shared locks for `Load` and exclusive locks for `Save` provides robust protection against concurrent CLI invocations without sacrificing read performance.

#### YAML Migration
The TOML to YAML migration for configuration is handled gracefully. The fallback logic in `config.Load()` ensures a seamless upgrade path, and the automatic creation of the YAML version on first run is a good UX choice.

---

### Minor Observations & Technical Debt

*   **Slugify Duplication:** The logic to convert `owner/repo` to `owner-repo` is implemented in `internal/tools/store.go` as `SlugifyRegistry` and duplicated manually in `internal/state/state.go`. This should be consolidated into the `paths` or `tools` package to ensure they never drift.
*   **ShouldInclude Filter:** The filter in `internal/sync/filter.go` is well-implemented. Denying infrastructure files only at the root level is a smart way to prevent `LICENSE` or `.gitignore` from cluttering the agent's context while allowing them to exist in subdirectories if necessary.
*   **Cursor MDC Flattening:** The logic to flatten namespaced rules (e.g., `org-repo-deploy.mdc`) is correct and prevents collisions in the flat `.cursor/rules` directory.

### Final Verdict
**Approve with Changes.** The migration is architecturally sound and fixes critical namespacing issues, but the discovery regression for local skills and the MDC overwrite are blockers for a production release. Once these are addressed, this is a very strong update.
