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
