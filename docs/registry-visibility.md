# Registry Visibility

Scribe tracks registry visibility so future discovery features can make a clear privacy decision before reading or reporting registry metadata.

Visibility has three values:

- `public`: GitHub reported the registry repository as public, or a legacy `community` registry was migrated as the best available historical signal.
- `private`: GitHub reported the repository as private, or a legacy `team` registry was migrated as private.
- `unknown`: Scribe could not verify visibility, including legacy `github` rows and GitHub API errors.

Only `public` registries are eligible for future public discovery features. `private` and `unknown` registries are treated as private. That fail-closed rule means an unavailable GitHub API, missing credentials, renamed repo, or permission error prevents public treatment.

`scribe registry connect` checks GitHub repository metadata and stores the result in `~/.scribe/config.yaml`. Existing configs are migrated on load with conservative defaults:

- `community` -> `public`
- `team` -> `private`
- `github` -> `unknown`

Scribe does not phone home about registries in this phase. Visibility is local plumbing only.

## Local Public Registry Index

Scribe keeps a local-only cache of public registries at `~/.scribe/index/registries.json`. `scribe registry connect` and successful `scribe sync` runs update this file for registries whose stored visibility is `public`.

The index stores public repo identity and manifest metadata:
- repo and source repo
- visibility
- default branch and current head SHA
- manifest presence, kind, and team name
- skill and kit counts
- last fetched timestamp

`private` and `unknown` registries are skipped. If the file is missing, Scribe treats it as an empty index. If the file is corrupt, commands that read or update it report the parse error and tell the user to remove the file or reconnect public registries to rebuild.
