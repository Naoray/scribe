# Scribe Privacy Notice

Draft for maintainer review. This notice covers the proposed opt-in public
registry telemetry for Scribe. It should be mirrored to
`https://usescribe.dev/privacy` before telemetry collection starts. This
repository does not contain the website source, so the website copy should be
published from the website repository.

## Short Version

Scribe telemetry is off by default. If you enable it, Scribe sends a small set of
public registry metadata to `https://api.usescribe.dev` so maintainers can learn
which public skill registries exist, keep public discovery reliable, and debug
registry ecosystem health.

Scribe does not collect private registry URLs, local filesystem paths, skill
contents, prompts, secrets, or the name of the AI tool you use in v1.

Raw telemetry events are kept for 30 days. Aggregate public registry metrics may
be kept indefinitely.

## Purpose

Scribe's public registry telemetry has two purposes:

- public registry ecosystem discovery, such as identifying public registries,
  skill counts, kit counts, and manifest metadata worth indexing for discovery;
- reliability, such as understanding whether public registry sync and discovery
  paths are failing because of missing manifests, invalid metadata, GitHub API
  errors, or Scribe version compatibility.

Telemetry is not used to inspect private work, read skill contents, profile local
projects, or identify which AI coding tool a user runs.

## Opt-In State

Telemetry is opt-in and off by default. Scribe must not send public registry
telemetry until the user explicitly enables it.

Planned CLI controls:

- `scribe telemetry enable` enables telemetry for future Scribe runs.
- `scribe telemetry disable` disables telemetry for future Scribe runs.
- `scribe telemetry status` prints whether telemetry is enabled, disabled by
  configuration, or disabled by environment.

The exact storage location for the telemetry preference may change before
implementation, but the default remains disabled.

## Disable and Opt-Out Methods

Any of these settings disables telemetry, even if telemetry was previously
enabled:

- `DO_NOT_TRACK=1`
- `DISABLE_TELEMETRY=1`
- `SCRIBE_NO_TELEMETRY=1`
- CI environments. If Scribe detects `CI=true`, telemetry is treated as off.
- `scribe telemetry disable`

If multiple settings conflict, Scribe should choose the privacy-preserving
result and not send telemetry.

`scribe telemetry purge` is a planned command for requesting deletion from the
CLI. Until it exists, use the deletion process below.

## V1 Data Fields

The v1 event schema is expected to include only public registry telemetry fields.
Final field names may be refined by the implementation ADR, but every shipped
field should be documented here before telemetry is enabled.

Envelope fields:

- `schema_version`: telemetry schema version.
- `event_id`: random event identifier used for de-duplication and debugging.
- `event_name`: event type, such as `public_registry_connect`,
  `public_registry_sync`, or `public_registry_index_update`.
- `event_time`: time the event was created.
- `scribe_version`: Scribe CLI version.
- `os`: operating system family, such as `darwin` or `linux`.
- `arch`: CPU architecture, such as `arm64` or `amd64`.
- `install_id`: random Scribe installation identifier. It is not derived from
  machine hardware, usernames, paths, Git remotes, or AI tool configuration.
- `request_id`: server-side request identifier for support and deletion lookup.

Public registry fields:

- `registry_repo`: public registry repository in `owner/repo` form.
- `registry_url`: canonical public GitHub URL for the registry.
- `registry_visibility`: expected to be `public` for sent events. Private and
  unknown registries must not be sent.
- `source_repo`: public source repository for a skill or package if it differs
  from `registry_repo`.
- `default_branch`: public registry default branch.
- `head_sha`: public commit SHA observed for the registry manifest or index.
- `manifest_path`: manifest path, normally `scribe.yaml`.
- `manifest_kind`: manifest type or version when available.
- `team_name`: public team or registry display name from the manifest, if
  present.
- `skill_count`: number of skills declared by the public registry.
- `kit_count`: number of kits declared by the public registry.
- `package_count`: number of package entries declared by the public registry, if
  applicable.

Operation and reliability fields:

- `command`: coarse Scribe command surface, such as `registry connect`,
  `registry resync`, `sync`, or `browse`.
- `operation`: coarse operation, such as `connect`, `sync`, `browse`, or
  `index_update`.
- `result`: `success`, `partial_success`, or `error`.
- `error_code`: Scribe error code or category when an operation fails.
- `http_status`: upstream or API status code when relevant.
- `duration_ms`: operation duration in milliseconds.
- `retry_count`: number of retries attempted by Scribe.

## Data Scribe Does Not Collect

Scribe public registry telemetry must not collect:

- private registry URLs or private repository names;
- registries whose visibility is `private` or `unknown`;
- local filesystem paths;
- skill file contents, package contents, snippets, kits, prompts, or agent
  instructions;
- secrets, tokens, environment variable values, SSH remotes, or Git credentials;
- usernames, hostnames, project names, or working directory names;
- the AI tool name in v1, including whether the user runs Claude Code, Codex,
  Cursor, Gemini, or a custom registered tool;
- full command-line arguments that may contain local paths, registry names the
  user did not opt to report, or other private data.

## Retention

Raw telemetry events are retained for 30 days.

Aggregate public registry metrics may be retained indefinitely. Aggregates are
intended to answer questions such as how many public registries are active, which
public registry manifests are valid, and whether public discovery is reliable
across Scribe versions.

## Deletion Requests

To request deletion of telemetry linked to your Scribe installation, open a
GitHub issue at <https://github.com/Naoray/scribe/issues> and include the
`install_id` shown by `scribe telemetry status` once that command is available.
If you have a `request_id` from an error report or maintainer support thread,
include that too.

Maintainers should delete matching raw events that are still inside the 30-day
retention window. Aggregated public registry metrics may remain if they no
longer identify an installation or request.

Before publishing this notice publicly, maintainers should replace or supplement
the GitHub issue process with a dedicated privacy contact if one exists.

## GDPR and CCPA

Scribe telemetry is designed to avoid collecting personal data where practical:
it is opt-in, limited to public registry metadata, and does not include private
registry URLs, local paths, prompts, secrets, skill contents, or AI tool name in
v1.

Some telemetry fields, especially `install_id`, may still be treated as personal
data or personal information under privacy laws when they can be linked to a
person. Users can disable telemetry, request deletion of retained raw events, and
ask maintainers what telemetry is associated with their `install_id`.

This notice is practical project documentation, not legal advice or a claim that
Scribe is fully compliant with every privacy regulation in every jurisdiction.
Maintainers should get light legal review before publishing if the final
telemetry design adds user identifiers, geolocation, cross-site tracking,
third-party processors beyond the Scribe API host, or any default-on behavior.

## Publication Gate

This notice, or materially equivalent wording, must be live at
`https://usescribe.dev/privacy` for at least 30 days before maintainers discuss
changing telemetry from opt-in to any default-enabled mode.

Any default-flip discussion must re-check the shipped telemetry fields, disable
paths, deletion process, and retention policy against the public notice before a
release is planned.
