# Opt-In Public Registry Telemetry Reporter ADR

Date: 2026-05-17
Status: Proposed
Scope: v1 public registry usage telemetry reporter design only. No service implementation in this repo.

## Context

Scribe can install skills from public registries, but maintainers currently have no privacy-preserving signal for whether a public registry entry is useful, broken, abandoned, or worth improving. Todo #184 sets the direction for public registry usage tracking; Todo #195 locks the maintainer decisions for privacy, hosting, allow-listing, and rollout; Todo #186 asks for the Phase 2 ADR before service work begins.

The v1 reporter must answer only narrow public-registry product questions:

- Which public registry skills are installed, updated, previewed, removed, or fail often enough to need maintainer attention?
- Which public registry repos are active enough to prioritize in the bootstrap experience?
- Are there client-version or platform compatibility issues visible at the aggregate level?

The reporter must not become a general analytics system. It is off by default, explicitly enabled by the user, bounded to public registry activity, and designed so private/local paths never leave the machine.

## Decision Summary

1. Telemetry is **opt-in and off by default** in v1.
2. The CLI owns local reporter UX under a new `scribe telemetry` command group.
3. The ingest service is hosted outside this repo in `Naoray/scribe-aggregator`, deployed on Cloudflare Workers with R2 storage, behind `https://api.usescribe.dev`.
4. Events are limited to public registry operations for an allow-listed registry set in v1.
5. Events carry a random install ID plus a daily-rotating actor ID derived from a local secret salt; the raw salt never leaves the machine.
6. v1 events do **not** include AI tool name, private registry names, local paths, usernames, hostnames, email addresses, git remotes, command arguments, project names, or skill contents.
7. Raw event retention is 30 days. Aggregates may be retained indefinitely.
8. `DO_NOT_TRACK`, `DISABLE_TELEMETRY`, `SCRIBE_NO_TELEMETRY`, and CI environments force telemetry off.
9. Users can preview/export the pending payloads before enabling or sending.
10. Users can request deletion by install ID from the CLI; deletion removes raw events and suppresses future aggregate contribution where feasible from raw data.
11. A repo privacy notice at `docs/PRIVACY.md` and equivalent `usescribe.dev/privacy` page must be live for at least 30 days before any default-flip discussion.

## Goals

1. Give maintainers aggregate public-registry usage signal while preserving a conservative privacy posture.
2. Make telemetry explicit, inspectable, reversible, and easy to disable.
3. Keep the service boundary clean: this repository defines reporter behavior and docs, while `Naoray/scribe-aggregator` implements ingest, storage, aggregation, deletion, and public reporting.
4. Ensure private registries, local skills, local paths, and project-specific usage never enter v1 telemetry.
5. Establish rollout gates before any implementation or future default changes.

## Non-Goals

- No default-on telemetry in v1.
- No AI tool name in v1 events. A later version may add it only behind a separate explicit flag.
- No telemetry service implementation in this repo.
- No private registry reporting.
- No local-only skill reporting.
- No command-line argument capture beyond the normalized event type and public registry object fields defined below.
- No per-project analytics.
- No content upload, stack traces, prompt text, skill files, environment dumps, package manifests, or shell history.
- No use of the reporter as a license check, entitlement system, or required network dependency.

## Command Surface

Add a `scribe telemetry` command group. All commands must support `--json` using the existing CLI envelope style where practical.

### `scribe telemetry status`

Shows the effective telemetry state:

- `enabled`: true only when local config opts in and no hard-disable condition applies.
- `configured`: whether the user explicitly enabled telemetry.
- `blocked_by`: ordered reasons such as `do_not_track`, `disable_telemetry`, `scribe_no_telemetry`, `ci`, or `offline_mode`.
- `endpoint`: `https://api.usescribe.dev/v1/events`.
- `install_id`: redacted by default in text output; present in JSON only when `--show-identifiers` is passed.
- `actor_rotation`: daily.
- `pending_events`: local queue count, if a queue exists.
- `last_upload_at`: timestamp or empty.

Text example:

```text
Telemetry: disabled
Reason: not enabled

Scribe only reports public registry usage after you run:
  scribe telemetry enable
```

If an env var or CI blocks telemetry, status must say the effective state is disabled even if config says enabled.

### `scribe telemetry enable`

Explicitly enables telemetry in the user-level Scribe config. The command must:

1. Show the same summary as the first-run prompt.
2. Link to `https://usescribe.dev/privacy`.
3. State that v1 only reports public registry activity from the bootstrap allow-list.
4. State that raw events are retained for 30 days and aggregates may be retained indefinitely.
5. Generate the local telemetry identity material if missing.
6. Refuse to enable when a hard-disable env var is active unless `--write-config-only` is passed.

`--yes` may skip the confirmation prompt in scripts, but hard-disable env vars and CI still force runtime off.

### `scribe telemetry disable`

Disables telemetry in local config. It must not delete the install ID by default because retaining the ID allows later deletion requests. It should print the deletion command:

```text
Telemetry disabled.
To request deletion of previously uploaded raw telemetry, run:
  scribe telemetry delete-request
```

### `scribe telemetry preview`

Prints a representative payload for the next eligible event without sending it. This is the main transparency tool. It must show:

- effective enabled/disabled state,
- all fields that would be sent,
- fields that are intentionally omitted,
- whether the selected example would be allowed by the public registry boundary.

If no event context is supplied, preview should use a harmless example:

```text
event_type: registry_skill_install
registry_repo: Naoray/scribe-skills-essentials
skill_name_hash: sha256:...
```

Options:

- `--event <type>` chooses an event type.
- `--registry <owner/repo>` and `--skill <name>` allow previewing a specific public registry event.
- `--json` emits the exact JSON object except for request signing metadata if the service later adds it.

### `scribe telemetry export`

Exports local telemetry state and any unsent queued events to stdout or a file:

- install ID,
- current config,
- hard-disable reasons,
- queue entries not yet sent,
- last upload metadata.

It must not read server-side data. It is a local transparency and support command.

### `scribe telemetry purge`

Deletes local telemetry queue state and rotates local identity material:

- clears unsent events,
- generates a new install ID,
- generates a new actor secret,
- keeps telemetry enabled/disabled config unchanged unless `--disable` is passed.

This is local-only and does not contact the deletion endpoint. The command must point users to `delete-request` for server-side deletion.

### `scribe telemetry delete-request`

Starts a server-side deletion request for the current install ID.

Default flow:

1. Show the install ID prefix and the endpoint.
2. Ask for confirmation unless `--yes`.
3. Send a deletion request to `https://api.usescribe.dev/v1/deletion-requests`.
4. Print the returned deletion request ID and status URL.
5. Offer to disable telemetry locally after the request; `--disable` performs that in one step.

Options:

- `--install-id <id>` lets a user request deletion for a copied ID after reinstalling.
- `--reason <text>` is local UX only unless the service explicitly supports it.
- `--json` returns service response fields.

If the service-side deletion endpoint is unavailable or a user cannot access the current install ID, v1 privacy docs may direct users to open a GitHub Issue in `Naoray/scribe` for deletion/privacy help. Maintainers have approved GitHub Issues as the v1 contact path. A dedicated privacy email or form can be added later, but it is not a blocker for this ADR or v1 docs.

## First-Run Prompt UX

The first-run prompt may be shown only when all of these are true:

- Scribe is running interactively.
- No telemetry choice is already recorded.
- No hard-disable env var is active.
- CI is not detected.
- The privacy notice has shipped in the CLI build.

Prompt copy:

```text
Help improve public Scribe registries?

Scribe can send minimal, opt-in telemetry about public registry activity:
installs, updates, previews, removals, and failures for allow-listed public
registries. It never sends private registry names, local paths, project names,
skill contents, prompts, hostnames, usernames, or AI tool names.

Raw events are kept for 30 days. Aggregates may be kept indefinitely.
Privacy details: https://usescribe.dev/privacy

Enable public registry telemetry? [y/N]
```

The default answer is no. A skipped prompt records no enablement and telemetry remains off. Non-interactive runs must never prompt.

## Local Identity Model

### Install ID

On first explicit enable, generate a random UUIDv4 install ID and store it in the user-level Scribe state directory. Requirements:

- Generated locally with cryptographic randomness.
- Stable across Scribe runs until `scribe telemetry purge` rotates it.
- Not derived from machine hardware, username, hostname, git config, or OS account data.
- Used for deletion requests and service-side abuse/rate controls.
- Included in raw events but excluded from public aggregates.

### Daily Actor Salt

Generate a separate random local actor secret on first enable. For each UTC date, derive:

```text
actor_id = base64url(HMAC-SHA256(actor_secret, "scribe-telemetry-v1:" + yyyy-mm-dd))
```

The event includes the derived `actor_id` and `actor_day`, not the actor secret. This lets the service count approximate daily active installs without making raw events linkable across days by actor ID alone. The install ID is still present in raw events for deletion and abuse handling, so raw storage access remains sensitive and must stay limited.

`scribe telemetry purge` rotates both install ID and actor secret after clearing local queued events.

## Event Eligibility

An event is eligible only if all checks pass:

1. Local config says telemetry is enabled.
2. No hard-disable env var is active.
3. CI is not detected.
4. The operation concerns a public registry entry.
5. The registry is in the v1 bootstrap allow-list.
6. The event type is in schema v1.
7. No private/local path data is required to describe the event.

If any check fails, the reporter must drop the event before queueing. Dropped telemetry must not be logged with sensitive context.

## Hard-Disable Rules

The reporter must honor these environment variables before reading user config:

- `DO_NOT_TRACK`: any non-empty value except `0`, `false`, or `no` disables telemetry.
- `DISABLE_TELEMETRY`: any non-empty value except `0`, `false`, or `no` disables telemetry.
- `SCRIBE_NO_TELEMETRY`: any non-empty value except `0`, `false`, or `no` disables telemetry.

CI auto-off:

- `CI`: any non-empty value except `0`, `false`, or `no` disables telemetry.
- Known provider markers such as `GITHUB_ACTIONS`, `GITLAB_CI`, `BUILDKITE`, `CIRCLECI`, `TF_BUILD`, `TEAMCITY_VERSION`, and `JENKINS_URL` also disable telemetry.

These are hard runtime blocks. `scribe telemetry status` must surface them. `scribe telemetry enable` may write config only with `--write-config-only`, but actual reporting remains disabled while the block exists.

## Public Registry Boundary

v1 reports only public registry activity. The reporter must classify an event source before constructing a payload.

Allowed:

- Public GitHub `owner/repo` registry identifiers in the bootstrap allow-list.
- In-tree builtins, represented as `registry_repo: "builtin"` with `source_kind: "builtin"`.

Denied:

- Local skills from `~/.scribe/skills`, project-local `.ai/skills`, or any absolute/relative filesystem path.
- Private GitHub repositories.
- Team registries not in the bootstrap allow-list.
- Git URL registries, SSH remotes, self-hosted Git providers, file URLs, and any registry source that is not known public.
- Any event where privacy classification is unknown.

The reporter should prefer false negatives over false positives. If public/private status cannot be proven from existing registry metadata, drop the event.

## Bootstrap Allow-List v1

The v1 allow-list is fixed by maintainer decision:

- `Naoray/scribe`
- `Naoray/scribe-skills-essentials`
- `anthropics/skills`
- `mattpocock/skills`
- in-tree builtins

Behavior:

- Events from allowed sources may be reported when all other checks pass.
- Events from any other registry are dropped in v1, even if the registry is public.
- The allow-list must be visible in `scribe telemetry preview` and privacy docs.
- Expanding the allow-list requires maintainer review and privacy doc update.

## Schema v1

All event payloads are JSON sent over HTTPS. Unknown fields are rejected by the service in v1 so schema drift is visible during development.

```json
{
  "schema_version": 1,
  "event_id": "018f6f4e-9f0d-7b50-8f8f-95c4b8d7dd98",
  "event_type": "registry_skill_install",
  "occurred_at": "2026-05-17T09:30:00Z",
  "sent_at": "2026-05-17T09:30:03Z",
  "install_id": "5a1e3c8b-6b8d-4e3d-8f3f-7c43c70a7d24",
  "actor_id": "v1.wQmW9u9f1E5g1XlYhFlkJw",
  "actor_day": "2026-05-17",
  "client": {
    "name": "scribe",
    "version": "1.5.0",
    "commit": "79d07a9",
    "os": "darwin",
    "arch": "arm64"
  },
  "source_kind": "github_public_registry",
  "registry_repo": "Naoray/scribe-skills-essentials",
  "registry_visibility": "public",
  "object_kind": "skill",
  "object_name_hash": "sha256:44c901...",
  "object_slug": "tdd",
  "object_version": "8f67a2b4c9d0",
  "operation": "install",
  "result": "success",
  "error_code": "",
  "duration_ms": 842,
  "queue_age_ms": 3000,
  "network": {
    "attempt": 1,
    "offline": false
  }
}
```

### Field Rationale

| Field | Required | Privacy rationale |
|---|---:|---|
| `schema_version` | yes | Enables strict versioning without inferring from client version. |
| `event_id` | yes | Random UUID for dedupe; not derived from user or machine data. |
| `event_type` | yes | Normalized product event; no raw command arguments. |
| `occurred_at` | yes | Needed for daily aggregates and 30-day retention. |
| `sent_at` | yes | Distinguishes queued/offline sends from actual event time. |
| `install_id` | yes | Random local ID for deletion and abuse controls; not public. |
| `actor_id` | yes | Daily HMAC pseudonym for daily active counts with limited cross-day linkage. |
| `actor_day` | yes | Makes rotation explicit and auditable. |
| `client.name` | yes | Fixed to `scribe`; avoids tool-name confusion. |
| `client.version` | yes | Helps find version-specific regressions. |
| `client.commit` | no | Useful for prerelease/debug builds; may be empty in release builds. |
| `client.os` | yes | Coarse compatibility signal. |
| `client.arch` | yes | Coarse compatibility signal. |
| `source_kind` | yes | Explicitly separates public GitHub registry vs builtin. |
| `registry_repo` | yes | Public allow-listed owner/repo only, or `builtin`; never private. |
| `registry_visibility` | yes | Must be `public` or `builtin`; defensive service validation. |
| `object_kind` | yes | `skill`, `kit`, or `package`; v1 starts with public registry skills but reserves known object categories. |
| `object_name_hash` | yes | Hash supports stable aggregates without relying on free-form names. |
| `object_slug` | conditional | Plain slug is allowed only for allow-listed public registries and builtins; omit if future policy narrows. |
| `object_version` | no | Commit SHA, release tag, or manifest version when already public. |
| `operation` | yes | `preview`, `install`, `update`, `remove`, `sync`, or `delete_request`. |
| `result` | yes | `success`, `failure`, `skipped`, or `cancelled`. |
| `error_code` | no | Scribe-defined coarse error code; no raw error strings. |
| `duration_ms` | no | Performance aggregate; no local path or command data. |
| `queue_age_ms` | no | Operational health for offline queueing. |
| `network.attempt` | no | Retry/debug aggregate without request logs. |
| `network.offline` | no | Distinguishes intentional local queueing from server failure. |

`object_name_hash` is `sha256(lowercase(registry_repo) + "\n" + object_kind + "\n" + object_slug)`. This is not a secret, because allow-listed slugs may be enumerable, but it gives aggregate systems a stable key if display slugs are later removed.

### Explicitly Omitted

The schema must not include:

- AI tool name (`claude`, `codex`, `cursor`, etc.).
- Local username, hostname, home directory, absolute path, relative project path, current working directory, or git remote.
- Project name, repository name for the user's working project, branch name, or commit SHA.
- Raw CLI arguments.
- Skill content, README content, frontmatter, prompts, generated files, logs, or stack traces.
- IP address in the event body. The service may receive IP metadata at the HTTP layer but must not copy it into event JSON or aggregates.

## Event Types

Supported `event_type` values in v1:

- `registry_skill_preview`
- `registry_skill_install`
- `registry_skill_update`
- `registry_skill_remove`
- `registry_skill_sync`
- `registry_skill_failure`
- `telemetry_delete_request`

`registry_skill_failure` uses `operation` and `error_code` to describe the failed operation. Error code examples: `registry_not_found`, `manifest_invalid`, `network_unavailable`, `hash_mismatch`, `permission_denied`, `install_command_failed`, `user_cancelled`.

## Ingest Endpoint Contract

Base URL: `https://api.usescribe.dev`

### `POST /v1/events`

Request:

- HTTPS only.
- `Content-Type: application/json`.
- Body is either a single schema v1 event or a batch envelope:

```json
{
  "schema_version": 1,
  "events": []
}
```

Response:

- `202 Accepted` with `{ "accepted": <n>, "rejected": <n>, "request_id": "..." }` when at least one event is accepted.
- `400 Bad Request` for schema errors.
- `401 Unauthorized` only if a future signed-client contract is added; v1 should avoid secrets in the CLI.
- `413 Payload Too Large` when batch limits are exceeded.
- `429 Too Many Requests` with `Retry-After`.
- `5xx` for server failures.

Client behavior:

- Queue eligible events locally if offline or on `429`/`5xx`.
- Drop malformed events locally rather than retrying forever.
- Use bounded queue storage with oldest-first eviction.
- Never block the primary user command on telemetry upload success.
- Apply short network timeouts.
- Do not send telemetry during `scribe telemetry preview`, `status`, `export`, or local `purge`.

### Validation

The service must reject events when:

- `schema_version` is not `1`.
- `registry_repo` is not in the v1 allow-list or `builtin`.
- `registry_visibility` is not `public` or `builtin`.
- unknown top-level fields appear.
- required fields are missing.
- `client.name` is not `scribe`.
- `actor_day` does not match `occurred_at` UTC day.

## Deletion Endpoint Flow

### `POST /v1/deletion-requests`

Request:

```json
{
  "schema_version": 1,
  "install_id": "5a1e3c8b-6b8d-4e3d-8f3f-7c43c70a7d24",
  "requested_at": "2026-05-17T09:40:00Z",
  "client": {
    "name": "scribe",
    "version": "1.5.0"
  }
}
```

Response:

```json
{
  "request_id": "del_01J...",
  "status": "queued",
  "status_url": "https://api.usescribe.dev/v1/deletion-requests/del_01J..."
}
```

Required service behavior:

- Delete raw events with the matching `install_id`.
- Delete queued raw deletion-request metadata after the operational retention window needed to prove completion.
- Prevent matching raw events still in the current 30-day raw window from contributing to future aggregate rebuilds.
- Document that already-published aggregate counts may not be perfectly subtractable if they are non-identifying and no longer contain install IDs.

### `GET /v1/deletion-requests/{request_id}`

Returns `queued`, `processing`, `completed`, `not_found`, or `failed`. The CLI may show this in a future `scribe telemetry delete-status` command, but v1 can print the URL returned by `delete-request`.

## Storage, Retention, and Aggregation

The service in `Naoray/scribe-aggregator` stores:

- Raw JSON events in R2 for 30 days.
- Derived aggregate tables indefinitely.
- Deletion request operational records only as long as needed for processing and audit.

Aggregation rules:

- Aggregate by day, event type, registry repo, object kind, object hash/slug, client version, OS, arch, operation, result, and coarse error code.
- Do not include install ID or actor ID in public aggregate outputs.
- Do not publish cells below a minimum threshold if they could identify a single reporter's behavior.
- Do not store raw IP address in event records or aggregate keys.

Raw retention is enforced by lifecycle policy and verified by a service-side scheduled job. If the lifecycle policy and job disagree, the service should fail closed by pausing public aggregate publication until retention is fixed.

## Privacy Notice Requirements

Before implementation ships, the repository needs `docs/PRIVACY.md` and the website needs equivalent `usescribe.dev/privacy` content covering:

- Telemetry is off by default in v1.
- How to enable, disable, preview, export, purge, and request deletion.
- Every schema v1 field and why it is collected.
- Public-only registry boundary and v1 allow-list.
- Explicit exclusions: private registries, local paths, project names, skill contents, AI tool name.
- Raw 30-day retention and indefinite aggregate retention.
- Environment variable and CI opt-out behavior.
- Deletion process and limitations for already-derived non-identifying aggregates.
- GDPR/CCPA stance and contact path. GitHub Issues in `Naoray/scribe` are approved for the v1 privacy/deletion contact path; a dedicated privacy email or form can be added later.
- Hosting boundary: CLI in this repo, service in `Naoray/scribe-aggregator`, API at `api.usescribe.dev`.

The privacy page must be live for at least 30 days before maintainers discuss any default-flip. This ADR does not approve a default flip.

## Rollout Gates

1. This ADR lands before service spike Todo #187 begins.
2. Maintainer review is required before CLI implementation.
3. Maintainer review is required before `Naoray/scribe-aggregator` accepts production traffic.
4. `docs/PRIVACY.md` and `usescribe.dev/privacy` must exist before any release that exposes `scribe telemetry enable`.
5. Privacy page must be live for at least 30 days before any default-flip discussion.
6. Any allow-list expansion requires maintainer review and privacy doc update.
7. Any addition of AI tool name requires a separate explicit flag, schema revision, and privacy review.

## Failure Modes

| Failure | Required behavior |
|---|---|
| Env var disables telemetry after user enabled it | Runtime off; `status` reports hard-disable reason. |
| CI detected | Runtime off; no prompt, no queueing, no upload. |
| Registry visibility unknown | Drop event before payload construction. |
| Private registry detected | Drop event before payload construction. |
| Endpoint unavailable | Queue bounded event or drop if queue full; never fail the user command. |
| Queue full | Evict oldest telemetry event and record only local count metadata. |
| Schema rejected by service | Drop event and increment local rejected count; do not retry forever. |
| User purges local telemetry | Clear queue, rotate install ID and actor secret. |
| User requests deletion | Send current install ID to deletion endpoint and show request ID/status URL. |

## Implementation Notes for Future Work

This ADR intentionally stops short of implementation, but future CLI work should keep these boundaries:

- Reporter code should live behind a small internal interface so workflows can emit typed public-registry events without importing HTTP details.
- Event construction should accept already-classified public registry metadata, not raw paths.
- Tests should cover hard-disable env vars, CI auto-off, allow-list filtering, private/local source exclusion, preview output, identity rotation, queue drop behavior, and deletion-request payloads.
- The default reporter in tests should be a no-op unless a test explicitly installs a fake reporter.
- Network upload should be best effort and should not make command tests flaky.

## Open Questions

- Whether `object_slug` should remain in raw v1 payloads or be removed before launch. This ADR allows it only for allow-listed public sources because the source catalogs are already public.
- Whether deletion status needs a first-class CLI command in v1 or the returned status URL is enough.
- Exact minimum threshold for public aggregate cells belongs to `Naoray/scribe-aggregator` implementation review.
