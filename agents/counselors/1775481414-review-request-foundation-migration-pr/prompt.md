# Second Opinion Request

## Question
# Review Request: Foundation Migration PR

## Question
Review this foundation migration branch (feat/foundation-migration) for correctness, completeness, and risks. This is a large structural change to a Go CLI tool that rewrites 3 core packages.

## Context

### What Changed (8 commits, 31 files, ~1500 insertions)
This branch implements Plan 1: Foundation -- Config, State, and Tools Migration for the Scribe CLI (a team skill sync tool for AI coding agents).

Six major changes:
1. **Config TOML→YAML migration** — config.go rewritten with new structs (RegistryConfig, ToolConfig), YAML loading, TOML auto-migration on Load()
2. **State flattening** — State struct flattened (no more nested Team.LastSync), InstalledSkill.Targets renamed to Tools, bare keys namespaced by registry slug
3. **targets→tools package rename** — internal/targets/ renamed to internal/tools/, Target interface becomes Tool with added Detect() and Uninstall() methods
4. **Disk layout namespacing** — WriteToStore now takes registrySlug parameter, paths become ~/.scribe/skills/<registrySlug>/<name>/
5. **Sync file filter** — shouldInclude filter denies repo infrastructure files (LICENSE, .gitignore) from skill sync
6. **Review fixes** — namespace key mismatch fix, extra-skills scoping, path traversal guards, tmp cleanup

### Files to Review
@internal/config/config.go
@internal/state/state.go
@internal/tools/tool.go
@internal/tools/claude.go
@internal/tools/cursor.go
@internal/tools/store.go
@internal/sync/syncer.go
@internal/sync/filter.go
@internal/discovery/discovery.go
@internal/workflow/sync.go
@internal/workflow/bag.go
@internal/workflow/list.go
@internal/workflow/connect.go
@internal/workflow/registry_list.go
@internal/paths/paths.go
@cmd/add.go
@cmd/migrate.go

### Recent Changes
Run `git diff origin/main` to see the full diff. Run `git log origin/main..HEAD --oneline` for commit history.

## Instructions
You are providing an independent review. Be critical and thorough.
- Read the referenced files to understand the full context
- Focus on: data migration correctness, backward compatibility, edge cases in the namespace scheme
- Check: does the migration produce keys that match what the syncer looks up? (this was a P0 bug we already fixed — verify the fix is correct)
- Check: are there any remaining inconsistencies between the old and new data models?
- Check: is the path traversal protection sufficient?
- Identify risks for users upgrading from the old format
- Be direct and opinionated — don't hedge

## Instructions
You are providing an independent second opinion. Be critical and thorough.
- Analyze the question in the context provided
- Identify risks, tradeoffs, and blind spots
- Suggest alternatives if you see better approaches
- Be direct and opinionated — don't hedge
- Structure your response with clear headings
- Keep your response focused and actionable
