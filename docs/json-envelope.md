# JSON envelope and agent-first contract

Scribe targets AI coding agents as a first-class consumer. Every migrated command emits a versioned envelope on stdout (or stderr on error) so an agent can parse the result without scraping human-formatted text.

## Envelope shape

```json
{
  "status": "ok",
  "format_version": "1",
  "data": { /* command payload */ },
  "meta": {
    "duration_ms": 12,
    "bootstrap_ms": 3,
    "command": "scribe sync",
    "scribe_version": "dev"
  }
}
```

Status values:

- `ok` — fully succeeded
- `partial_success` — some items failed; inspect `data.summary.failed` or item-level errors. Exit code is `10`.
- `error` — full failure. Body lives under `error.{code,message,retryable,remediation,exit_code}` instead of `data`.

`format_version` lets parsers reject envelopes they don't understand. The current schema is `"1"`. Breaking shape changes will bump it.

`meta.duration_ms` measures leaf `RunE` execution. `meta.bootstrap_ms` covers first-run, store migration, builtins apply, and embedded-agent refresh work that ran before the leaf command.

## Detection

Scribe auto-detects non-TTY environments. When stdout is not a TTY (piped, redirected, captured by a subprocess), commands emit JSON by default. Pass `--json` to force it even in interactive shells. `CI=true` also forces JSON.

```bash
scribe list                           # JSON when piped, TUI on TTY
scribe list --json                    # always JSON
CI=true scribe list                   # always JSON
```

## Live examples

`scribe list --json` (skill array trimmed for brevity):

```json
{
  "status": "ok",
  "format_version": "1",
  "data": {
    "packages": [
      { "name": "superpowers", "revision": 1, "path": "/Users/me/.scribe/packages/superpowers", "sources": ["obra/superpowers"] }
    ],
    "skills": [
      {
        "name": "add-init",
        "description": "Create a new /init-* command.",
        "revision": 1,
        "content_hash": "e42bc8ef",
        "targets": ["claude", "codex", "cursor", "gemini"],
        "managed": true,
        "path": "/Users/me/.scribe/skills/add-init"
      }
    ]
  },
  "meta": {
    "duration_ms": 478,
    "bootstrap_ms": 6,
    "command": "scribe list",
    "scribe_version": "dev"
  }
}
```

`scribe sync --json` with one failed install:

```json
{
  "status": "partial_success",
  "format_version": "1",
  "data": {
    "reconcile": { "installed": 2, "relinked": 0, "removed": 2, "conflicts_count": 0 },
    "summary":   { "failed": 1, "installed": 0, "skipped": 73, "updated": 0 }
  },
  "meta": {
    "duration_ms": 8999,
    "bootstrap_ms": 4,
    "command": "scribe sync",
    "scribe_version": "dev"
  }
}
```

`scribe status --json`:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": {
    "version": "dev",
    "registries": ["Artistfy/hq", "anthropics/skills", "Naoray/skills"],
    "installed_count": 129,
    "last_sync": "2026-04-30T10:10:47Z"
  },
  "meta": { "duration_ms": 2, "bootstrap_ms": 3, "command": "scribe status", "scribe_version": "dev" }
}
```

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | success |
| 1 | general operational failure |
| 2 | usage or invalid flags |
| 3 | not found (command, registry, skill, schema) |
| 4 | permission or authentication failure |
| 5 | conflict requiring user action |
| 6 | network or remote service failure |
| 7 | temporarily unavailable local dependency |
| 8 | validation failure |
| 9 | user canceled |
| 10 | partial success — inspect `data.summary.failed` or item-level errors |

## Field projection (`--fields`)

Read-only tabular commands accept `--fields name,version,...` to project specific columns. Fields not listed in the schema are silently ignored. Use `scribe schema <command> --json | jq '.data.output_schema'` to enumerate available fields before calling.

```bash
scribe list --json --fields name,managed,targets
```

## Schema introspection

Scribe ships JSON Schema (Draft 2020-12) for every migrated command's input flags and output payload.

```bash
scribe schema list --json
scribe schema --all --json | jq 'keys'        # list migrated commands
scribe schema sync --json | jq '.data.output_schema'
```

Use the schema before composing calls so flag typos and shape drift fail loudly.

## Pre-envelope commands

Some wave-3 commands still reject `--json` with the error `JSON_NOT_SUPPORTED` and exit code `2`. Their pre-envelope shape is unchanged. The remediation field points at the relevant migration todo so you can track when they land.

```json
{
  "status": "error",
  "format_version": "1",
  "error": {
    "code": "JSON_NOT_SUPPORTED",
    "message": "scribe registry list does not support --json yet",
    "retryable": false,
    "remediation": "scribe schema --all --json | jq 'keys' lists JSON-capable commands",
    "exit_code": 2
  },
  "meta": {}
}
```

## Migration notes

Previously top-level keys are now nested under `data`:

- `jq '.skills'` → `jq '.data.skills'`
- `jq '.summary'` → `jq '.data.summary'`
- `jq '.installed_count'` → `jq '.data.installed_count'`

`status` becomes `"partial_success"` when `data.summary.failed > 0`. Pair the status check with the exit code (`10`) for double safety in shell pipelines.

## Spec deviations

- `--fields name,version` is a separate flag rather than overloaded `--json name,version`. Keeps boolean `--json=true` compatibility intact.
- `meta.duration_ms` is leaf execution only; `meta.bootstrap_ms` is everything before that. Sum them for wall time.
