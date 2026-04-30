# Troubleshooting

When something looks off, start with `scribe doctor`. It compares the canonical store under `~/.scribe/skills/` against the tool-facing projections (e.g. `<project>/.claude/skills/`, `~/.codex/skills/`) and reports anything inconsistent.

## `scribe doctor`

```bash
scribe doctor                          # check everything, no writes
scribe doctor --skill recap            # check a single skill
scribe doctor --json                   # machine-readable report
scribe doctor --fix                    # apply safe normalization + reproject affected skills
scribe doctor --skill recap --fix      # repair just one skill
```

Live `scribe doctor --json` excerpt:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": {
    "fix": false,
    "summary": null,
    "issues_sample": [
      {
        "skill": "add-init",
        "tool": "cursor",
        "kind": "projection_drift",
        "status": "warn",
        "message": "unexpected managed projection at ...; missing managed projection for cursor at ..."
      }
    ]
  },
  "meta": { "duration_ms": 25, "bootstrap_ms": 4, "command": "scribe doctor", "scribe_version": "dev" }
}
```

`kind` values you'll see today:

- `projection_drift` — a tool-facing copy is missing, in the wrong place, or no longer matches the canonical store
- `metadata_normalization` — frontmatter in the canonical `SKILL.md` deviates from the canonical shape (whitespace, ordering, frontmatter casing)
- `opaque_tool` — a registered tool reports as opaque, so projection comparison is intentionally skipped (informational, not a real problem)

`scribe doctor` v1 does not attempt to rewrite mixed package layouts for Codex. It focuses on canonical-metadata health plus projection repair.

## Common situations

### "I edited a skill in `~/.claude/skills/` and now sync says it differs"

When a tool-local copy diverges from the canonical store, `scribe sync` preserves the local content and prints a repair hint instead of overwriting:

```
conflict: recap in codex differs from managed copy
run `scribe skill repair recap --tool codex` to resolve
```

Pick a side:

```bash
scribe skill repair recap --tool codex            # accept canonical, overwrite local
scribe resolve recap --theirs                     # accept local, overwrite canonical
scribe resolve recap --ours                       # accept canonical (same as repair)
```

### "A skill I uninstalled keeps coming back"

`scribe remove <skill>` records a deny-list entry in `~/.scribe/config.yaml` that survives across syncs. If a skill keeps reappearing, check that the remove command actually ran (`scribe config | jq '.deny'` in a future migrated version, or grep the config) — and confirm you removed via scribe rather than just `rm`-ing the symlink.

### "scribe list shows everything as `(local)`"

That means scribe adopted a bunch of hand-rolled skills it wasn't expecting. Either:

- Run `scribe config adoption --mode off` to stop auto-adoption, then decide which skills you actually want adopted, or
- Let it stand — adopted skills are first-class managed entries; they just have no upstream.

See [`adoption.md`](adoption.md) for the full adoption story.

### "Codex truncated all my skill descriptions"

Symptom of issue [#114](https://github.com/Naoray/scribe/issues/114) — Codex's 5440-byte description budget overflowed. The kits/snippets work introduces a real refusal at 100% of budget. If you're still on global-projection compat mode, switch to project-local projection by adding a `.scribe.yaml` with the kits you actually need. See [`projects-and-kits.md`](projects-and-kits.md).

### "Private repos don't authenticate"

Auth fallback chain:

1. `gh auth token` (preferred)
2. `GITHUB_TOKEN` environment variable
3. `~/.scribe/config.yaml` token field
4. unauthenticated (public repos only)

Run `gh auth login` if scribe can't reach a private registry. Scribe does not store tokens of its own.

### "I don't know which commands support `--json`"

```bash
scribe schema --all --json | jq 'keys'
```

Anything not in that list is still pre-envelope and rejects `--json` with `JSON_NOT_SUPPORTED`. Inspect `data.error.remediation` for the migration todo.

## Where things live

```
~/.scribe/
  config.yaml              # connected registries, tool settings, adoption mode, deny-list
  state.json               # what's installed, last sync time, projection map
  skills/                  # canonical skill store (symlinked by tools)
  kits/                    # user-defined kit YAMLs
  packages/                # registry packages cloned for catalog metadata
```

If you want to start over, deleting `~/.scribe/` is safe (you'll lose deny-list and projections) — your hand-rolled skills in `~/.claude/skills/` etc. are still there because adoption only ever symlinked them.

## Where to look next

- Daily commands: [`commands.md`](commands.md)
- Projecting skills onto the right projects: [`projects-and-kits.md`](projects-and-kits.md)
- Adopting hand-rolled skills: [`adoption.md`](adoption.md)
- Envelope shape, exit codes, schema introspection: [`json-envelope.md`](json-envelope.md)
