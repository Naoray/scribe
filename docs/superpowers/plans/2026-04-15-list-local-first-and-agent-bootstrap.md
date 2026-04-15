# List Local-First And Agent Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `scribe list` local-first by default, add explicit remote browsing/install surfaces, bootstrap the embedded `scribe-agent` skill without network access, and land the supporting registry cleanup/muting work from the design spec.

**Architecture:** Split local and remote list workflows so local commands avoid GitHub and tool-resolution work entirely, move cross-command side effects out of root pre-run into explicit workflow steps, and keep bootstrap/browse behavior behind narrowly scoped packages and commands. Reuse existing sync/provider/add command seams instead of embedding new network logic inside the list TUI.

**Tech Stack:** Go, Cobra, Bubble Tea, go:embed, existing `internal/workflow`, `internal/provider`, `internal/firstrun`, `internal/state`, and `internal/sync` packages.

---

### Task 1: Local-First List Workflow

**Files:**
- Modify: `internal/workflow/list.go`
- Modify: `internal/workflow/sync.go`
- Modify: `internal/workflow/bag.go`
- Modify: `internal/app/factory.go`
- Modify: `cmd/list.go`
- Modify: `cmd/list_tui.go`
- Test: `internal/workflow/list_test.go`
- Test: `cmd/list_tui_test.go`

- [ ] Add separate local and remote list step constructors and bag flags for lazy GitHub/config handling.
- [ ] Write failing tests proving default list does not resolve tools or remote providers, while `--remote` still does.
- [ ] Implement the minimal loader split and `buildRows` short-circuit for local mode.
- [ ] Update list loading copy and keep JSON local-by-default behavior intact.
- [ ] Run focused tests for workflow and list TUI.

### Task 2: Builtins Banner, Config Scaffold, Registry Migration/Muting

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/firstrun/firstrun.go`
- Modify: `internal/firstrun/firstrun_test.go`
- Modify: `internal/firstrun/firstrun_internal_test.go`
- Modify: `cmd/root.go`
- Modify: `cmd/root_test.go`
- Modify: `internal/state/state.go`
- Modify: `internal/state/state_test.go`
- Modify: `cmd/registry.go`
- Modify: `cmd/registry_enable.go`
- Create: `cmd/registry_forget.go`
- Create: `cmd/registry_forget_test.go`

- [ ] Add `scribe_agent.enabled` config schema with round-trip coverage.
- [ ] Write failing tests for first-run-only banner behavior and JSON cleanliness.
- [ ] Change builtin application to return first-run context, remove `openai/codex-skills`, and record one-shot removal migration state.
- [ ] Add persistent registry failure muting state and commands to forget/resync registries.
- [ ] Run focused tests for config, firstrun, root, registry, and state packages.

### Task 3: Tree Scan Fix

**Files:**
- Modify: `internal/provider/treescan.go`
- Modify: `internal/provider/provider.go` or related provider structs if needed
- Test: `internal/provider/treescan_test.go`
- Test: `internal/provider/github_test.go`

- [ ] Capture the current tree-scan behavior in failing tests using fixtures that model the anthropic root/nested layouts.
- [ ] Implement the narrowest discovery fix needed in `ScanTreeForSkills` / discover path handling.
- [ ] Verify provider discovery tests stay green.

### Task 4: Embedded `scribe-agent` Bootstrap

**Files:**
- Create: `internal/agent/embed.go`
- Create: `internal/agent/bootstrap.go`
- Create: `internal/agent/bootstrap_test.go`
- Add embedded asset under: `internal/agent/scribe_agent/SKILL.md`
- Modify: `internal/state/state.go`
- Modify: `internal/workflow/list.go`
- Modify: `internal/workflow/sync.go`
- Modify: `cmd/upgrade.go`
- Create: `cmd/upgrade_agent.go`
- Create: `cmd/upgrade_agent_test.go`

- [ ] Write failing tests for install, no-op, version mismatch reinstall, opt-out, read-only store, and version-command skip behavior.
- [ ] Add embedded bytes/version constants and bootstrap install logic with `OriginBootstrap`.
- [ ] Wire bootstrap into local/remote list and sync workflows, not root pre-run.
- [ ] Add explicit `scribe upgrade-agent` refresh command.
- [ ] Run focused bootstrap and command tests.

### Task 5: Browse Command And List Command Mode

**Files:**
- Create: `internal/workflow/list_load.go`
- Modify: `cmd/list_tui.go`
- Modify: `cmd/list_tui_test.go`
- Modify: `cmd/add_tui.go`
- Create: `cmd/browse.go`
- Create: `cmd/browse_test.go`
- Modify: `cmd/root.go`

- [ ] Extract list row-loading helpers out of `cmd/list_tui.go` with no behavior change beyond imports.
- [ ] Write failing tests for `scribe browse` JSON/query/install behavior and list `:` command delegation.
- [ ] Implement top-level `browse` command by reusing existing add/browser logic.
- [ ] Implement `:` command prefix in list search, shelling to Cobra commands via `tea.ExecProcess`.
- [ ] Run focused browse/list tests.

### Task 6: Verification

**Files:**
- None

- [ ] Run `go test ./internal/workflow ./internal/provider ./internal/firstrun ./internal/config ./internal/state ./cmd`
- [ ] Run `go test ./...`
- [ ] Run `go build ./...`
