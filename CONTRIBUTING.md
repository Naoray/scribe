# Contributing to Scribe

Thanks for your interest in scribe — a local skill manager for AI coding agents. This guide covers how to file issues, submit pull requests, set up a local development environment, follow scribe's JSON envelope conventions, and add a new skill registry.

For a high-level tour of the codebase, read [`AGENTS.md`](AGENTS.md). For the agent-facing contract, read [`docs/json-envelope.md`](docs/json-envelope.md).

## Filing issues

Open issues at <https://github.com/Naoray/scribe/issues>. Before filing, search existing issues — many features and known bugs are already tracked.

A useful issue includes:

- **Scribe version** — output of `scribe --version`.
- **Operating system** — macOS or Linux distribution and version.
- **What you expected to happen** — one or two sentences.
- **What actually happened** — exact error message or unexpected output. Paste the JSON envelope when applicable (`scribe <command> --json`); the `error.code`, `error.message`, and `meta` fields make a report much easier to triage.
- **Reproduction steps** — minimal commands that reliably reproduce the problem.

For bugs that depend on local state (registries, adopted skills, project config), `scribe doctor --json` is a good diagnostic snapshot to attach.

Feature requests are welcome. Frame them around the agent or human workflow you want to enable, not the implementation. The maintainers will discuss design before code lands.

## Pull request workflow

Scribe follows trunk-based development. The default branch is `main`; releases are tagged from `main`.

1. **Open or claim an issue first** for non-trivial changes. This avoids duplicate work and gives the maintainers a chance to flag scope or design concerns early. Drive-by typo fixes, doc tweaks, and obvious bugs do not need a prior issue.
2. **Fork and branch.** Create a feature branch from an up-to-date `main`. Use a short, descriptive name (`fix/list-tui-tools-drop`, `feat/registry-resync-flag`).
3. **Keep PRs focused.** One logical change per PR. Mixing a refactor with a behavior change makes review and rollback harder. If you find unrelated cleanup along the way, split it into a follow-up PR.
4. **Match the existing patterns.** Cobra commands follow the `newXCommand` / `runX` shape. Migrated commands emit the JSON envelope (see below). Tests sit next to the code they cover.
5. **Run the checks.** Before pushing:
   - `go build ./...`
   - `go test ./...`
   - `gofmt -w` on any files you touched (CI fails on unformatted code).
6. **Write a useful commit history.** Short imperative subjects, one logical change per commit. Examples from `git log`: `fix: preserve tools on list TUI updates`, `perf: lazy-init command dependencies`. Avoid mixed refactors in a single commit.
7. **Open the PR.** Describe the user-visible change, note any migration impact or risk, and include terminal output or a screenshot when changing TUI behavior. Link the issue you are closing with `Closes #N`.
8. **Respond to review.** Push fix-up commits to the same branch — do not force-push during review unless asked. The maintainer will squash-merge to keep `main` clean.

For larger changes (new commands, new envelope fields, breaking flag changes), draft a short design note in the PR description before writing code. A discussion of the public contract is cheaper than rewriting an implementation.

## Local development

### Prerequisites

- Go 1.22 or newer (`go version`).
- `git`.
- `gh` CLI is recommended for registry commands and authenticated GitHub access; not strictly required for public repos.
- macOS or Linux. Windows is not supported.

### Build and run

```bash
git clone https://github.com/Naoray/scribe
cd scribe
go build ./cmd/scribe        # builds the `scribe` binary in the working directory
go run ./cmd/scribe --help   # run without installing
```

For a quick smoke test of non-TTY output:

```bash
go run ./cmd/scribe list --json
```

### Testing

```bash
go test ./...                                            # full suite
go test ./internal/workflow ./internal/sync ./cmd        # focused pass for command + sync changes
go test -run TestNewSyncCommand ./cmd                    # single test
go test -race ./...                                      # race detector before shipping concurrent code
```

Write table-driven tests where practical. Cover behavior changes, not just happy paths. TUI and workflow regressions should include command-path tests when possible.

### Linting and formatting

Scribe relies on standard Go tooling:

```bash
gofmt -w .
go vet ./...
```

Keep code `gofmt`-clean, prefer small functions, and use explicit error handling over panic. Package names stay lowercase; file names are descriptive (`list_tui.go`, `syncer.go`).

### Working with local state

Scribe writes to `~/.scribe/`, `~/.claude/`, `~/.codex/`, `~/.cursor/`, and `~/.gemini/` on a real install. Tests must not touch your home directory — use the test helpers in `internal/state` and `internal/paths` that point at temporary directories. When running scribe manually for debugging, prefer a throwaway project directory.

## JSON envelope conventions

Every migrated command emits a versioned envelope. Read [`docs/json-envelope.md`](docs/json-envelope.md) for the full contract; the highlights for contributors:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": { /* command payload */ },
  "meta": {
    "duration_ms": 12,
    "bootstrap_ms": 3,
    "command": "scribe <name>",
    "scribe_version": "dev"
  }
}
```

When adding or modifying a command, follow these rules:

- **Status values are fixed.** `ok`, `partial_success`, or `error`. Do not invent new ones.
- **Payload lives under `data`.** Never put command output at the top level. `data.summary` carries `{installed, updated, skipped, failed}` for mutators.
- **Errors use the error envelope.** On failure, populate `error.{code, message, retryable, remediation, exit_code}` and exit with the matching code from the table in `docs/json-envelope.md`. `code` is `SCREAMING_SNAKE_CASE`; `remediation` is a short imperative sentence.
- **Exit codes are part of the contract.** `0` success, `2` usage, `3` not found, `5` conflict, `8` validation, `10` partial success, etc. Keep `cmd/exit_codes_test.go` green.
- **Bump `format_version` only on breaking shape changes.** Adding a new optional field under `data` is not breaking; renaming or removing a field is.
- **Ship a JSON Schema.** Migrated commands provide input + output schemas via `scribe schema <command> --json`. Update the schema alongside the implementation; `cmd/schema_test.go` enforces this.
- **Support `--fields`** on read-only tabular commands when the schema declares projection-friendly fields. Unknown fields are silently ignored.
- **Auto-detect non-TTY.** Commands emit JSON automatically when stdout is not a TTY or `CI=true`. Do not gate JSON behind `--json` only.
- **Never write human prose to stdout in JSON mode.** Logs, progress, and warnings go to stderr. The envelope is the entire stdout payload.

If a command genuinely cannot emit JSON yet, return the `JSON_NOT_SUPPORTED` error envelope with exit code `2` and a remediation pointer to the migration tracking issue.

## Adding a registry

A registry is a GitHub repository that ships scribe-compatible skills declared in a top-level `scribe.yaml` manifest. Anyone can host one — your team's private skills repo, a community pack, or a personal collection.

### Create a new registry

The fastest path is the scaffold command:

```bash
scribe registry create
```

It walks you through creating a GitHub repository (private or public), seeding `scribe.yaml`, and pushing the initial commit. Pass flags to skip prompts in scripts:

```bash
scribe registry create --owner my-org --repo my-skills --private --json
```

If you prefer to bootstrap by hand, the manifest looks like this:

```yaml
# scribe.yaml
version: 1
skills:
  - name: review-checklist
    path: skills/review-checklist
    description: Apply the team's review checklist before opening a PR.
```

Each skill lives in its own directory with a `SKILL.md` following the [agentskills.io](https://agentskills.io) format. Anything that works with `skills.sh` or Paks works with scribe.

### Connect to an existing registry

```bash
scribe registry connect my-org/my-skills
scribe sync
```

`registry connect` adds the repo to `~/.scribe/registries.yaml` and fetches its catalog. `sync` then projects the registry's skills into the agents you have detected.

For one-off discovery without committing:

```bash
scribe browse --registry my-org/my-skills
```

### Share a local skill into a registry

```bash
scribe registry add review-checklist
```

This opens a PR against the registry repo with the skill content. You need write access (or fork-and-PR rights) to the registry. See `docs/commands.md` for the full registry command surface — `enable`, `disable`, `forget`, `resync`, `list`, `migrate`.

### Registry conventions

- **One repo, many skills.** Registries are intentionally flat collections — no nested registries.
- **Stable skill names.** Renaming a skill is a breaking change for every consumer; bump the manifest version and document the migration in the registry's `README.md`.
- **`scribe.yaml`, not `scribe.toml`.** The TOML format is deprecated; run `scribe registry migrate` on legacy registries to convert.
- **Public discovery is opt-in.** Scribe does not phone home about registries you connect. A future public-discovery feature will be opt-in per registry.

## Code of conduct

Scribe follows a simple rule: be respectful, assume good faith, and keep technical discussion on the merits. Personal attacks, harassment, or discriminatory language are not welcome. Maintainers may remove comments or block contributors who do not follow this guidance.

## License

By contributing, you agree that your contributions will be licensed under the MIT License that covers the project.

## Where to get help

- [GitHub Discussions](https://github.com/Naoray/scribe/discussions) for design questions and how-do-I requests.
- [GitHub Issues](https://github.com/Naoray/scribe/issues) for bugs and feature requests.
- [`docs/`](docs/) for the user-facing reference, including [`commands.md`](docs/commands.md), [`json-envelope.md`](docs/json-envelope.md), [`projects-and-kits.md`](docs/projects-and-kits.md), [`adoption.md`](docs/adoption.md), and [`troubleshooting.md`](docs/troubleshooting.md).

Thanks for helping make scribe better.
