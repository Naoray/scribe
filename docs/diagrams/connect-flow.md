# scribe connect — Registry Connection Flow

```
                        scribe connect [owner/repo]
                        ═══════════════════════════

                                    │
                                    ▼
                         ┌─────────────────────┐
                         │   resolveRepo()     │
                         └──────────┬──────────┘
                                    │
                       ┌────────────┴────────────┐
                       ▼                         ▼
                ┌─────────────┐          ┌──────────────┐
                │ arg given?  │          │ no arg       │
                │             │          └──────┬───────┘
                │ use args[0] │                 │
                └──────┬──────┘        ┌────────┴────────┐
                       │               ▼                 ▼
                       │        ┌─────────────┐   ┌────────────┐
                       │        │ stdin TTY?  │   │ not TTY    │
                       │        │             │   │            │
                       │        │ huh.Input() │   │ ✗ Error:   │
                       │        │ "Team skills│   │ "no repo   │
                       │        │  repo"      │   │  specified"│
                       │        └──────┬──────┘   └────────────┘
                       │               │
                       └───────┬───────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │  config.Load()      │
                    │  ~/.scribe/         │
                    │    config.toml      │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ connectToRepo()     │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ parseOwnerRepo()    │
                    │ "owner/repo"        │
                    │ → owner, repo       │
                    └──────────┬──────────┘
                               │
                               ▼
                    ┌─────────────────────┐
                    │ Already connected?  │
                    │ (case-insensitive   │
                    │  dedup check)       │
                    └───────┬─────────┬───┘
                           YES        NO
                            │         │
                            ▼         │
                    ┌──────────────┐   │
                    │ "Already     │   │
                    │  connected"  │   │
                    │ (return nil) │   │
                    └──────────────┘   │
                                      ▼
                           ┌─────────────────────┐
                           │ FetchFile()         │
                           │                     │
                           │ GitHub Contents API │
                           │ → scribe.toml       │
                           └──────────┬──────────┘
                                      │
                                      ▼
                           ┌─────────────────────┐
                           │ manifest.Parse()    │
                           │                     │
                           │ TOML → Manifest     │
                           └──────────┬──────────┘
                                      │
                                      ▼
                           ┌─────────────────────┐
                           │ IsLoadout()?        │
                           │ (has [team] section)│
                           └───────┬─────────┬───┘
                                  NO        YES
                                   │         │
                                   ▼         │
                           ┌──────────────┐  │
                           │ ✗ Error:     │  │
                           │ "no [team]   │  │
                           │  section"    │  │
                           └──────────────┘  │
                                             ▼
                           ┌─────────────────────────┐
                           │ cfg.TeamRepos +=  repo  │
                           │ cfg.Save()              │
                           │                         │
                           │ "Connected to <repo>"   │
                           └──────────┬──────────────┘
                                      │
                                      ▼
                    ┌──────────────────────────────────┐
                    │         AUTO-SYNC                 │
                    │                                  │
                    │  state.Load()                    │
                    │  Syncer { Client, Targets }      │
                    │  Emit: plain text callback       │
                    │                                  │
                    │  "syncing skills..."             │
                    │                                  │
                    │  syncer.Run(ctx, repo, st)       │
                    └──────────┬────────────┬──────────┘
                               │            │
                          success        failure
                               │            │
                               ▼            ▼
                        ┌───────────┐ ┌──────────────────┐
                        │  Events:  │ │ stderr: "warning │
                        │           │ │  sync failed"    │
                        │  ✓ name   │ │ stderr: "run     │
                        │  installed│ │  scribe sync"    │
                        │  to v1.0  │ │                  │
                        │           │ │ ┌──────┬───────┐ │
                        │  "done:   │ │ │ TTY? │!TTY?  │ │
                        │  N inst,  │ │ │      │       │ │
                        │  N upd,   │ │ │swallow│return │ │
                        │  N curr"  │ │ │(nil) │error  │ │
                        └───────────┘ │ └──────┴───────┘ │
                                      └──────────────────┘
```

---
*Generated with /ascii*
