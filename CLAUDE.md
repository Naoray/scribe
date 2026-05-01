# Scribe Agent Contract

Scribe is an agent-first CLI for managing local AI coding agent skills. Prefer JSON mode for automation and inspect command schemas when composing calls.

## JSON Envelope

Migrated commands emit a versioned envelope on stdout:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": {},
  "meta": {
    "duration_ms": 12,
    "bootstrap_ms": 3,
    "command": "scribe list",
    "scribe_version": "dev"
  }
}
```

Errors use the same envelope shape on stderr. Payload keys that were previously top-level now live under `data`.

## Exit Codes

| Code | Meaning |
|---:|---|
| 0 | success |
| 1 | general operational failure |
| 2 | usage or invalid flags |
| 3 | requested command, registry, skill, or schema was not found |
| 4 | permission or authentication failure |
| 5 | conflict requiring user action |
| 6 | network or remote service failure |
| 7 | temporarily unavailable local dependency |
| 8 | validation failure |
| 9 | user canceled |
| 10 | partial success; inspect `data.summary.failed` or item-level errors |

## Prompting Tips

Use `scribe schema <command> --json` before calling an unfamiliar command. Use `--fields` only on commands whose schema lists the flag. When updating scripts from pre-envelope output, change `jq '.foo'` to `jq '.data.foo'`.

Examples:

```bash
scribe list --json --fields name,status
scribe schema sync --json
scribe sync --json | jq '.data.summary'
scribe adopt --dry-run --json | jq '.data.conflicts'
```

## Commands

| Command | Flags | Output schema |
|---|---|---|
| `scribe add` | --alias, --force, --json, --registry, --yes | yes |
| `scribe adopt` | --dry-run, --json, --verbose, --yes | yes |
| `scribe browse` | --install, --json, --query, --registry, --yes | no |
| `scribe check` | --json | yes |
| `scribe config adoption` | --add-path, --json, --mode, --remove-path | no |
| `scribe config set editor` | --json | no |
| `scribe config set` | --json | no |
| `scribe config` | --json | no |
| `scribe create registry` | --json, --owner, --private, --repo, --team | no |
| `scribe create` | --json | no |
| `scribe doctor` | --fix, --json, --skill | yes |
| `scribe explain` | --json, --raw | yes |
| `scribe guide` | --json | yes |
| `scribe init` | --force, --json | yes |
| `scribe install` | --alias, --all, --force, --json, --registry | no |
| `scribe list` | --fields, --json, --registry, --remote | yes |
| `scribe migrate global-to-projects` | --dry-run, --json, --project | no |
| `scribe migrate` | --json | no |
| `scribe push` | --json | yes |
| `scribe registry add` | --install, --json, --registry, --yes | no |
| `scribe registry connect` | --install-all, --json | yes |
| `scribe registry create` | --json, --owner, --private, --repo, --team | no |
| `scribe registry disable` | --json | no |
| `scribe registry enable` | --json | no |
| `scribe registry forget` | --json | no |
| `scribe registry list` | --json | no |
| `scribe registry migrate` | --json | no |
| `scribe registry resync` | --json | no |
| `scribe registry` | --json | no |
| `scribe remove` | --json, --yes | no |
| `scribe resolve` | --json, --ours, --theirs | no |
| `scribe restore` | --json | no |
| `scribe schema` | --all, --json, --markdown | no |
| `scribe show` | --json | no |
| `scribe skill edit` | --add, --inherit, --json, --pin, --remove, --tools | no |
| `scribe skill repair` | --from, --json, --tool | no |
| `scribe skill tools` | --disable, --enable, --json, --reset | no |
| `scribe skill` | --json | no |
| `scribe status` | --json | yes |
| `scribe sync` | --alias, --all, --force, --json, --registry, --trust-all | yes |
| `scribe tools add` | --detect, --install, --json, --path, --uninstall | no |
| `scribe tools disable` | --json | no |
| `scribe tools enable` | --json | no |
| `scribe tools` | --json | no |
| `scribe update` | --apply, --json | yes |
| `scribe upgrade-agent` | --json | no |
| `scribe upgrade` | --check, --json | no |
| `scribe` |  | no |


## Spec Deviations

- Use `--fields name,version` as a separate flag, not overloaded `--json name,version`; this preserves boolean `--json=true` compatibility.
- `meta.duration_ms` measures leaf `RunE` execution. `meta.bootstrap_ms` covers first-run, store migration, builtins apply, and embedded-agent refresh work before the leaf command runs.
- Some wave-3 commands still reject `--json` with `JSON_NOT_SUPPORTED`; inspect `scribe schema --all --json` for migrated commands.
