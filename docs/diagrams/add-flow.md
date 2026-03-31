# scribe add — Add Skill to Loadout (Stub)

```
                         scribe add <source>
                         ═══════════════════
                         --path  --yes

                                │
                                ▼
                     ┌─────────────────────┐
                     │ cobra.ExactArgs(1)  │
                     │ validates 1 arg     │
                     └──────────┬──────────┘
                                │
                                ▼
                     ┌─────────────────────┐
                     │                     │
                     │  "TODO: add <src>"  │
                     │                     │
                     │  (not yet           │
                     │   implemented)      │
                     │                     │
                     └─────────────────────┘


   Planned Architecture (from design spec)
   ════════════════════════════════════════

   Three modes:

   ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
   │ Mode 1: Named    │  │ Mode 2: Browse   │  │ Mode 3: Error    │
   │                  │  │                  │  │                  │
   │ scribe add       │  │ scribe add       │  │ scribe add       │
   │   <skill-name>   │  │ (no args, TTY)   │  │ (no args, !TTY)  │
   │                  │  │                  │  │                  │
   │ Resolve skill    │  │ Bubble Tea list  │  │ "specify skill   │
   │ from local +     │  │ with search,     │  │  name" error     │
   │ remote discovery │  │ multi-select,    │  │                  │
   │                  │  │ confirmation     │  │                  │
   └────────┬─────────┘  └────────┬─────────┘  └──────────────────┘
            │                     │
            └──────────┬──────────┘
                       │
                       ▼
            ┌─────────────────────┐
            │ Adder.Add()         │
            │                     │
            │ DiscoverLocal()     │
            │ → scan ~/.claude/   │
            │   skills/, etc.     │
            │                     │
            │ DiscoverRemote()    │
            │ → search connected  │
            │   registries        │
            │                     │
            │ Deduplicate         │
            │ → merge candidates  │
            └──────────┬──────────┘
                       │
                       ▼
            ┌─────────────────────┐
            │ Strategy decision   │
            │                     │
            │ ┌─────────────────┐ │
            │ │ Reference:      │ │
            │ │ point to source │ │
            │ │ repo in toml    │ │
            │ └─────────────────┘ │
            │ ┌─────────────────┐ │
            │ │ Upload:         │ │
            │ │ push local file │ │
            │ │ to registry     │ │
            │ └─────────────────┘ │
            └──────────┬──────────┘
                       │
                       ▼
            ┌─────────────────────┐
            │ Update scribe.toml  │
            │ in registry repo    │
            │                     │
            │ Auto-sync to        │
            │ install locally     │
            └─────────────────────┘
```

---
*Generated with /ascii*
