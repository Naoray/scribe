---
name: scribe-agent
description: Use when the user wants to install, list, sync, remove, or manage AI coding-agent skills on this machine. Scribe manages a canonical skill store and links skills into Claude Code, Cursor, Codex, and other supported tools.
---

# scribe-agent

## First-run bootstrap (do this before anything else)

Before running any command in this skill, check that `scribe` is installed and that this skill is registered locally. If either is missing, bootstrap them now.

Step 1 — check for the binary:

```bash
scribe --version
```

If the command succeeds, skip to Step 3. If it fails with "command not found" (or similar), run Step 2.

Step 2 — install scribe. Prefer install methods that land the binary on `PATH` so future sessions can call `scribe` directly. Pick the first option that fits the machine; stop on the first success:

```bash
# macOS Homebrew (preferred on macOS — installs to a PATH directory)
brew install Naoray/tap/scribe

# macOS Apple Silicon binary (installs to /usr/local/bin)
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_darwin_arm64.tar.gz | tar xz && sudo mv scribe /usr/local/bin/

# macOS Intel binary
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_darwin_amd64.tar.gz | tar xz && sudo mv scribe /usr/local/bin/

# Linux amd64 binary
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_linux_amd64.tar.gz | tar xz && sudo mv scribe /usr/local/bin/

# Linux arm64 binary
curl -L https://github.com/Naoray/scribe/releases/latest/download/scribe_linux_arm64.tar.gz | tar xz && sudo mv scribe /usr/local/bin/

# Windows amd64 (PowerShell) — downloads to $env:USERPROFILE\bin and adds it to your PATH
powershell -Command "
  \$dest = \"\$env:USERPROFILE\bin\";
  New-Item -ItemType Directory -Force -Path \$dest | Out-Null;
  Invoke-WebRequest -Uri 'https://github.com/Naoray/scribe/releases/latest/download/scribe_windows_amd64.zip' -OutFile \"\$env:TEMP\scribe.zip\";
  Expand-Archive -Path \"\$env:TEMP\scribe.zip\" -DestinationPath \$dest -Force;
  \$current = [Environment]::GetEnvironmentVariable('PATH','User');
  if (\$current -notlike \"*\$dest*\") {
    [Environment]::SetEnvironmentVariable('PATH', \"\$dest;\$current\", 'User')
  }
"

# Go toolchain — LAST RESORT: installs to $(go env GOBIN) or ~/go/bin, which is
# often not on PATH. Only use if none of the above work.
go install github.com/Naoray/scribe/cmd/scribe@latest
```

After install, verify `scribe` is reachable via PATH:

```bash
command -v scribe && scribe --version
```

If `command -v scribe` prints nothing but the binary exists (e.g. at `~/go/bin/scribe` or `$(go env GOBIN)/scribe`), the install directory is not on PATH. Fix it before continuing:

1. Detect the install dir: `GOBIN=$(go env GOBIN); [ -z "$GOBIN" ] && GOBIN="$(go env GOPATH)/bin"; echo "$GOBIN"`
2. Append to the user's shell rc (`~/.zshrc` for zsh, `~/.bashrc` for bash): `export PATH="$GOBIN:$PATH"`
3. Tell the user the line you added and which file, and that they need to open a new shell (or `source` the file) for it to take effect.
4. For the rest of this bootstrap, call scribe by absolute path (`"$GOBIN"/scribe ...`).

If all install options fail, stop and tell the user which one errored and why.

Step 3 — make sure this skill is registered locally so it survives future sessions:

```bash
scribe list --json
```

If the output already contains an entry with `"name": "scribe-agent"`, you're done — continue with the user's actual request.

If it does not, install this skill now. First trigger first-run so the `Naoray/scribe` built-in registry is wired up, then install:

```bash
scribe status --json
scribe add Naoray/scribe:scribe-agent --yes --json
scribe list --json
```

Confirm the final `scribe list --json` includes `scribe-agent`. If anything in this bootstrap fails, report the failing command and its stderr verbatim — do not guess around it.

Once bootstrap succeeds, proceed with the user's request using the command reference below.

## What scribe does

Scribe manages local coding-agent skills.
It stores canonical copies in `~/.scribe/skills/` and links them into supported tool directories.
Use it for installs, updates, removal, adoption of unmanaged local skills, and structured inspection.

## Trigger phrases to commands

| User says | Run |
| --- | --- |
| install the X skill | `scribe add X --json` |
| install X from owner/repo | `scribe add owner/repo:X --yes --json` |
| what skills are installed | `scribe list --json` |
| what skills are available remotely | `scribe list --remote --json` |
| sync my skills | `scribe sync --json` |
| remove X | `scribe remove X --yes --json` |
| import existing local skills | `scribe adopt --dry-run --json` |
| actually adopt them | `scribe adopt --yes --json` |
| explain what X does | `scribe explain X --json` |
| show scribe status | `scribe status --json` |
| audit managed skill health | `scribe doctor --json` |
| repair managed skill metadata/projections | `scribe doctor --skill <name> --fix` |
| connect a registry | `scribe registry add owner/repo` |

## Non-negotiable rules

1. Always use `--json` for anything you plan to parse.
2. Never run bare `scribe add` in automation.
3. Prefer `owner/repo:skill` for deterministic installs.
4. Use `--yes` for direct installs and removals.
5. Use `scribe adopt --dry-run --json` before `scribe adopt --yes --json`.
6. Do not hand-edit `~/.scribe/state.json`.
7. Do not copy skill files directly into tool directories; use `scribe adopt`.
8. `scribe sync` reconciles registries; it does not install an arbitrary new skill by query.
9. Some failures still return plain stderr plus non-zero exit, not a JSON error envelope.

## JSON shapes

### `scribe list --json`

Top level: array.
Each item may include `name`, `description`, `package`, `revision`, `content_hash`, `targets`, `managed`, `origin`, and `path`.
Fresh-home output is `[]`.

### `scribe list --remote --json`

Top level: object with `registries`.
Each registry has `registry` and `skills`.
Each remote skill may include `name`, `status`, `version`, `loadout_ref`, `maintainer`, and `agents`.

### `scribe add query --json`

Top level: object with `results`.
Each result may include `name`, `registry`, `status`, `version`, `description`, and `author`.
This is search output, not install output.

### `scribe add owner/repo:skill --yes --json`

Top level: object with `installed`.
Each installed item may include `name`, `registry`, `status`, and `error`.
Observed statuses: `installed`, `updated`, `already-installed`, `error`.

### `scribe sync --json`

Top level: object with `registries` and `summary`.
Each registry has `registry` and `skills`.
Each skill result may include `name`, `action`, `status`, `version`, and `error`.
`summary` has `installed`, `updated`, `skipped`, and `failed`.
Observed actions: `installed`, `updated`, `skipped`, `error`, `package_installed`, `package_updated`, `denied`.
An optional top-level `adoption` object may appear.

### `scribe adopt --dry-run --json`

Top level: object with `dry_run`, `adopt`, and `conflicts`.
`adopt` entries may include `name`, `local_path`, `targets`, and `hash`.
`conflicts` entries may include `name`, `managed_hash`, `unmanaged_path`, and `unmanaged_hash`.

### `scribe adopt --yes --json`

Top level: formatter envelope with `registries`, `summary`, and `adoption`.
`adoption` may include `skills`, `conflicts_deferred`, `adopted`, `failed`, and `skipped`.

### `scribe remove skill --yes --json`

Top level: object with `removed`.
Optional fields: `managed_by`, `errors`.

### `scribe explain skill --json`

Top level: object with `name` and `content`.
Optional fields: `description`, `revision`, `targets`, and `path`.
This command only works for installed skills on disk.

### `scribe status --json`

Top level: object with `version`, `registries`, and `installed_count`.
Optional field: `last_sync`.

### `scribe doctor --json`

Top level: object with `issues`.
Optional fields: `skill` and `fix`.
Each issue may include `skill`, `tool`, `kind`, `status`, and `message`.

## Recommended flows

Install a known skill:

```bash
scribe add owner/repo:skill --yes --json
```

Search, then install deterministically:

```bash
scribe add query --json
scribe add owner/repo:skill --yes --json
```

Inspect local state:

```bash
scribe list --json
scribe status --json
```

Reconcile connected registries:

```bash
scribe sync --json
```

Audit and repair managed skill health:

```bash
scribe doctor --json
scribe doctor --fix
scribe doctor --skill recap --fix
```

`scribe doctor` audits managed skills and projection health.
`scribe doctor --fix` applies safe metadata normalization and then repairs affected tool projections.
`scribe doctor` v1 does not attempt to rewrite mixed package layouts for Codex; it focuses on canonical metadata health plus projection repair.

Adopt unmanaged local skills:

```bash
scribe adopt --dry-run --json
scribe adopt --yes --json
```

## Anti-patterns

- Bare `scribe add` in automation.
- Parsing styled terminal output instead of `--json`.
- Using `scribe sync` when you mean “install one skill”.
- Removing files by hand from tool directories.
- Editing `~/.scribe/state.json` directly.
- Assuming every failure returns JSON.

## Fallback rule

If you need a command not listed here, run:

```bash
scribe --help
scribe <subcommand> --help
```

Do not guess flags or JSON fields.
