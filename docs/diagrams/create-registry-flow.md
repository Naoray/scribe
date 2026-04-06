# scribe create registry — Registry Scaffolding Flow

```
                  scribe create registry
                  ══════════════════════
                  --team  --owner  --repo  --private

                                │
                                ▼
                     ┌─────────────────────┐
                     │ Detect TTY (stdin)  │
                     └──────────┬──────────┘
                                │
                ┌───────────────┴───────────────┐
                ▼                               ▼
         ┌─────────────┐                ┌─────────────┐
         │   IS TTY    │                │  NOT TTY    │
         │             │                │             │
         │ Prompt for  │                │ Require all │
         │ each missing│                │ flags or    │
         │ value via   │                │ error       │
         │ huh.Input() │                │             │
         └──────┬──────┘                └──────┬──────┘
                │                              │
                └──────────────┬───────────────┘
                               │
                               ▼
          ┌────────────────────────────────────────┐
          │          Collect All Values            │
          │                                        │
          │  team:    --team or prompt              │
          │  owner:   --owner or prompt             │
          │  repo:    --repo or prompt (def: team-  │
          │           registry)                     │
          │  private: --private or prompt (def:true)│
          └────────────────────┬───────────────────┘
                               │
                               ▼
          ┌────────────────────────────────────────┐
          │       validateGitHubName() × 3         │
          │                                        │
          │  regex: ^[a-zA-Z0-9][a-zA-Z0-9._-]*$  │
          │  validates: team, owner, repo          │
          └────────────────────┬───────────────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ config.Load()       │
                    │ gh.NewClient(token) │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ Authenticated?      │
                    └───────┬─────────┬───┘
                           NO        YES
                            │         │
                            ▼         │
                    ┌──────────────┐   │
                    │ ✗ Error:     │   │
                    │ "run gh auth │   │
                    │  login"      │   │
                    └──────────────┘   │
                                      ▼
                    ┌──────────────────────────────┐
                    │ client.CreateRepo()          │
                    │                              │
                    │ owner/repo                   │
                    │ desc: "<Team> dev team       │
                    │        skill stack"          │
                    │ private: true/false          │
                    └──────────┬───────────────────┘
                               │
                  ┌────────────┼────────────┐
                  ▼            │             ▼
           ┌───────────┐      │      ┌──────────────┐
           │ Created!  │      │      │ Already      │
           │           │      │      │ exists       │
           │ canonical │      │      │ (ErrRepo-    │
           │ owner/repo│      │      │  Exists)     │
           │ from resp │      │      └──────┬───────┘
           └─────┬─────┘      │             │
                 │            │    ┌────────┴────────┐
                 │            │    ▼                 ▼
                 │            │  ┌────────┐   ┌───────────┐
                 │            │  │  TTY   │   │ NOT TTY   │
                 │            │  │        │   │           │
                 │            │  │ Confirm│   │ ✗ Error:  │
                 │            │  │ "Use   │   │ "already  │
                 │            │  │  it?"  │   │  exists"  │
                 │            │  └───┬────┘   └───────────┘
                 │            │      │
                 │            │   ┌──┴──┐
                 │            │  YES    NO
                 │            │   │     │
                 │            │   │     ▼
                 │            │   │  ┌────────┐
                 │            │   │  │"aborted│
                 │            │   │  └────────┘
                 │            │   │
                 └────────────┴───┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ scribe.yaml exists? │
                    │ (client.FileExists) │
                    └───────┬─────────┬───┘
                           YES        NO
                            │         │
                            │         ▼
                            │  ┌──────────────────────┐
                            │  │ client.PushFiles()   │
                            │  │                      │
                            │  │ ┌──────────────────┐ │
                            │  │ │ scribe.yaml      │ │
                            │  │ │                  │ │
                            │  │ │ apiVersion:      │ │
                            │  │ │  scribe/v1       │ │
                            │  │ │ kind: Registry   │ │
                            │  │ │ team:            │ │
                            │  │ │  name: "..."     │ │
                            │  │ │ catalog: []      │ │
                            │  │ └──────────────────┘ │
                            │  │ ┌──────────────────┐ │
                            │  │ │ README.md        │ │
                            │  │ │                  │ │
                            │  │ │ Setup + usage    │ │
                            │  │ │ instructions     │ │
                            │  │ └──────────────────┘ │
                            │  │                      │
                            │  │ commit: "Initialize  │
                            │  │  skill registry"     │
                            │  └──────────┬───────────┘
                            │             │
                            └──────┬──────┘
                                   │
                                   ▼
                    ┌─────────────────────────────┐
                    │ "Registry created: o/r"     │
                    └──────────────┬──────────────┘
                                   │
                                   ▼
                    ┌─────────────────────────────┐
                    │ connectToRepo()             │
                    │                             │
                    │ (reuses connect command's   │
                    │  logic — dedup, save config,│
                    │  auto-sync skills)          │
                    │                             │
                    │ cfg.TeamRepos += repo       │
                    │ cfg.Save()                  │
                    │ syncer.Run() ──► install    │
                    └─────────────────────────────┘
```

---
*Generated with /ascii*
