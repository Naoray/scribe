## Unreleased

### BREAKING: --json output now uses versioned envelope (format_version=1)

`scribe list --json`, `scribe status --json`, `scribe doctor --json`, `scribe explain --json`, `scribe guide --json` now wrap their previous payload as:

```json
{
  "status": "ok",
  "format_version": "1",
  "data": { /* previously top-level keys are now here */ },
  "meta": { "duration_ms": 12, "command": "scribe list", "scribe_version": "..." }
}
```

Migration: `jq '.data.foo'` works; `jq '.foo'` requires update.

Mutator commands (`sync`, `add`, `adopt`) keep their pre-envelope output for now; PR #<C> ships their migration.

### Added

- `--fields f1,f2` projection on read-only commands with tabular output (gh-style; opt-in per command via `output.AttachFieldsFlag`).
- `scribe schema list`, `scribe schema status`, `scribe schema doctor`, `scribe schema explain`, `scribe schema guide` now return JSON Schema 2020-12 for both inputs and outputs.

### Spec deviations

- PR #89 spec §43, §118 propose field selection via overloaded `--json name,version`. We diverged: `--json` stays Bool; field selection uses companion `--fields name,version` flag to avoid breaking `--json=true` shell scripts. See `scribe schema <cmd>` for valid field names.
