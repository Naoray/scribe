# Adoption — claim skills you already have

If you've been hand-rolling skills in `~/.claude/skills/`, `~/.codex/skills/`, or `~/.cursor/rules/`, scribe can adopt them into the canonical store without moving anything. The original path becomes a symlink to `~/.scribe/skills/<name>` — nothing breaks for the tool, and now scribe manages the file.

## Quick start

```bash
scribe adopt                 # interactive: review conflicts, adopt clean candidates
scribe adopt --yes           # auto-adopt all clean candidates
scribe adopt --dry-run       # preview the plan, no writes
scribe adopt <name>          # adopt a single named skill
scribe adopt --json          # machine-readable plan + result
```

Adopted skills appear with `(local)` in `scribe list`. They have no upstream and stay until you `scribe remove` them.

## Conflict handling

`scribe adopt` distinguishes three cases per candidate:

- **clean** — the file on disk does not collide with anything in the store. Adoption is safe to run unattended.
- **collision** — a managed skill of the same name already exists, with different content. Adoption refuses and shows a diff hint.
- **identical** — already in the store, same content hash. Skipped silently.

Pass `--verbose` (or inspect `data.conflicts` in `--json` mode) to see why anything was deferred.

## Adoption mode in `scribe sync`

`scribe sync` runs adoption as a prelude. Configure how aggressively it adopts via `~/.scribe/config.yaml`:

```yaml
adoption:
  mode: auto       # auto | prompt | off
  paths:           # optional extra dirs beyond the defaults
    - ~/src/my-skills
```

- `auto` (default) — silently adopt clean candidates on every sync
- `prompt` — defer to the dedicated `scribe adopt` command; sync only adopts if you run it explicitly
- `off` — never adopt automatically; unmanaged skills still appear in `scribe list` so you can see what's there

Or manage without touching YAML:

```bash
scribe config adoption                          # show current settings
scribe config adoption --mode off
scribe config adoption --add-path ~/src/my-skills
scribe config adoption --remove-path ~/src/my-skills
```

## What adoption does on disk

For a clean candidate at `~/.claude/skills/my-skill/`:

1. Copy directory contents into `~/.scribe/skills/my-skill/`
2. Compute content hash, write into the canonical store's metadata
3. Replace the original directory with a symlink to the canonical path
4. Record an `origin: local` entry in scribe's state so it shows as `(local)` in `list`

The replacement is a single `os.Remove` + `os.Symlink`, performed only after the canonical copy is in place. Sync's reconcile layer is non-destructive on purpose; the destructive `RemoveAll` lives in adopt where it has explicit user intent.

## Deny-list

When you `scribe remove <skill>`, scribe records a deny-list entry in `~/.scribe/config.yaml`. The skill won't come back on the next sync even if it's still in a connected registry. To un-deny, edit the config or `scribe add` it again explicitly.

## Surface area

- `scribe adopt` — see [`commands.md`](commands.md)
- `scribe schema adopt --json` — input flags and output envelope
- `data.summary` in `--json` mode reports `adopted`, `skipped`, `conflicted` counts
