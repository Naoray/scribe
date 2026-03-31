# scribe list — Skill Inventory Flow

```
                              scribe list ──── Skill Inventory
                              ═══════════

                                    │
                                    ▼
                         ┌─────────────────────┐
                         │   Parse CLI Flags    │
                         │ --local  --json      │
                         │ --registry           │
                         └──────────┬──────────┘
                                    │
                         ┌──────────┴──────────┐
                         │ --local + --registry?│
                         └──────────┬──────────┘
                                    │
                        ┌───────────┴───────────┐
                        ▼                       ▼
                  ┌───────────┐          ┌─────────────┐
                  │   YES     │          │     NO      │
                  │ ✗ Error:  │          │  continue   │
                  │ mutually  │          └──────┬──────┘
                  │ exclusive │                 │
                  └───────────┘                 │
                                                ▼
                              ┌──────────────────────────────┐
                              │        Decision Gate         │
                              │                              │
                              │  --local set?    ───► LOCAL  │
                              │  no registries?  ───► LOCAL  │
                              │  otherwise       ───► REMOTE │
                              └───────────┬──────────────────┘
                                          │
                         ┌────────────────┴────────────────┐
                         ▼                                 ▼
                  ┌─────────────┐                  ┌──────────────┐
                  │ LOCAL PATH  │                  │ REMOTE PATH  │
                  └──────┬──────┘                  └──────┬───────┘
                         │                                │
                         │                                ▼
                         │                  ┌──────────────────────┐
                         │                  │ filterRegistries()   │
                         │                  │ → resolve --registry │
                         │                  │   or return all      │
                         │                  └──────────┬───────────┘
                         │                             │
                         │                ╔════════════╧═══════════╗
                         │                ║ FOR EACH teamRepo     ║
                         │                ╚════════════╤═══════════╝
                         │                             │
                         │                             ▼
                         │                  ┌──────────────────────┐
                         │                  │ syncer.Diff()        │
                         │                  │                      │
                         │                  │ GitHub API →         │
                         │                  │ fetch scribe.toml →  │
                         │                  │ compare each skill   │
                         │                  │ → []SkillStatus      │
                         │                  └──────────┬───────────┘
                         │                             │
                         │                    ┌────────┴────────┐
                         │                    ▼                 ▼
                         │              ┌───────────┐    ┌───────────┐
                         │              │ Table     │    │ JSON      │
                         │              │           │    │           │
                         │              │ SKILL     │    │ {         │
                         │              │ VERSION   │    │  "regis-  │
                         │              │ STATUS    │    │   tries": │
                         │              │ AGENTS    │    │   [...]   │
                         │              │           │    │ }         │
                         │              │ ── repo ──│    │           │
                         │              │ 2 current │    └───────────┘
                         │              │ 1 outdated│
                         │              │ Last sync │
                         │              └───────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │    state.Load()     │
              │                     │
              │ ~/.scribe/state.json│
              │ ┌─────────────────┐ │
              │ │ Installed: {    │ │
              │ │   "gstack": {   │ │
              │ │     Version,    │ │
              │ │     Source,     │ │
              │ │     Targets,    │ │
              │ │     Registries  │ │
              │ │   }             │ │
              │ │ }               │ │
              │ └─────────────────┘ │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │ sortedSkillNames()  │
              │ alphabetical sort   │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │ len(installed) == 0?│
              └───────┬─────────┬───┘
                     YES        NO
                      │         │
                      ▼         ▼
              ┌────────────┐   ┌──────────────────┐
              │ Empty State│   │  Output Format?  │
              │            │   └────────┬─────────┘
              │ "No skills │            │
              │  installed"│     ┌──────┴──────┐
              │ + connect  │     ▼             ▼
              │   hint     │  ┌────────┐  ┌──────────┐
              └────────────┘  │ JSON   │  │  TABLE   │
                              │        │  │          │
                              │--json  │  │ TTY      │
                              │or pipe │  │ detected │
                              └───┬────┘  └────┬─────┘
                                  │            │
                                  ▼            ▼
                           ┌───────────┐ ┌────────────────┐
                           │printLocal │ │ printLocal     │
                           │JSON()     │ │ Table()        │
                           │           │ │                │
                           │ [{        │ │ SKILL  VERSION │
                           │  "name",  │ │ TARGETS SOURCE │
                           │  "ver..", │ │ ────────────── │
                           │  "src..", │ │ gstack v0.12.. │
                           │  ...      │ │ deploy main@.. │
                           │ }]        │ │                │
                           └───────────┘ └───────┬────────┘
                                                 │
                                                 ▼
                                      ┌────────────────────┐
                                      │ Fallback hint?     │
                                      │                    │
                                      │ !--local AND       │
                                      │ no registries AND  │
                                      │ skills exist       │
                                      └─────────┬──────────┘
                                                │
                                           ┌────┴────┐
                                          YES        NO
                                           │         │
                                           ▼         ▼
                                   ┌──────────────┐ (done)
                                   │"Tip: connect │
                                   │ a registry   │
                                   │ with scribe  │
                                   │ connect"     │
                                   └──────────────┘
```

---
*Generated with /ascii*
