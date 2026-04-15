# scribe sync — Full Sync Flow

```
                                scribe sync ──── Full Sync Flow
                                ═══════════

                                      │
                                      ▼
                           ┌─────────────────────┐
                           │   Parse CLI Flags    │
                           │ --json  --registry   │
                           └──────────┬──────────┘
                                      │
                      ┌───────────────┼───────────────┐
                      ▼               ▼               ▼
              ┌──────────────┐ ┌─────────────┐ ┌────────────────┐
              │ config.Load()│ │ state.Load()│ │ TTY detection  │
              │              │ │             │ │                │
              │ config.yaml  │ │ state.json  │ │ --json or pipe │
              │ → TeamRepos  │ │ → Installed │ │ → auto-JSON    │
              │ → Token      │ │ → LastSync  │ │                │
              └──────┬───────┘ └──────┬──────┘ └───────┬────────┘
                     │                │                │
                     └────────────────┼────────────────┘
                                      │
                                      ▼
                           ┌─────────────────────┐
                           │ len(TeamRepos) == 0? │
                           └──────┬───────────┬───┘
                                 YES          NO
                                  │           │
                                  ▼           ▼
                           ┌───────────┐  ┌───────────────────┐
                           │ ✗ Error:  │  │ filterRegistries()│
                           │ "not      │  │                   │
                           │ connected"│  │ --registry flag?  │
                           └───────────┘  │ match → [repo]    │
                                          │ empty → all repos │
                                          └────────┬──────────┘
                                                   │
                                                   ▼
                                       ┌──────────────────────┐
                                       │ Create Syncer        │
                                       │                      │
                                       │ Client: gh.NewClient │
                                       │  (gh CLI → env →     │
                                       │   config → unauth)   │
                                       │                      │
                                       │ Targets: [Claude,    │
                                       │           Cursor]    │
                                       │                      │
                                       │ Emit: callback wired │
                                       └──────────┬───────────┘
                                                   │
                              ╔════════════════════╧═══════════════════╗
                              ║     FOR EACH teamRepo in repos        ║
                              ╚════════════════════╤═══════════════════╝
                                                   │
                        ┌──────────────────────────┘
                        │
          ══════════════╪══════════════════════════════════
           PHASE 1:     │  DIFF (compare local vs remote)
          ══════════════╪══════════════════════════════════
                        │
                        ▼
             ┌─────────────────────┐
             │ FetchFile()         │
             │                     │
             │ GitHub Contents API │
             │ → scribe.yaml      │
             └──────────┬──────────┘
                        │
                        ▼
             ┌─────────────────────┐
             │ manifest.Parse()    │
             │                     │
             │ YAML unmarshal →    │
             │ Manifest {          │
             │   Team,             │
             │   Skills map,       │
             │   Targets           │
             │ }                   │
             └──────────┬──────────┘
                        │
       ╔════════════════╧════════════════════╗
       ║  FOR EACH skill in manifest.Skills  ║
       ╚════════════════╤════════════════════╝
                        │
                        ▼
             ┌─────────────────────┐
             │ ParseSource()       │
             │ "github:owner/      │
             │  repo@ref"          │
             │ → { Owner, Repo,    │
             │     Ref }           │
             └──────────┬──────────┘
                        │
                        ▼
             ┌─────────────────────┐     ┌────────────────────┐
             │ IsBranch()?         │────>│ LatestCommitSHA()  │
             │ (ref has no dots,   │ YES │                    │
             │  doesn't start 'v') │     │ Git Refs API →     │
             └──────────┬──────────┘     │ latest SHA         │
                        │                └─────────┬──────────┘
                        │◄─────────────────────────┘
                        ▼
             ┌─────────────────────────────────────┐
             │ compareSkill(loadout, installed)     │
             │                                     │
             │ not installed          → Missing     │
             │ branch: SHA match      → Current     │
             │ branch: SHA mismatch   → Outdated    │
             │ semver: local >= loadout → Current   │
             │ semver: local <  loadout → Outdated  │
             │ tag: exact match       → Current     │
             │ tag: mismatch          → Outdated    │
             └──────────────────┬──────────────────┘
                                │
                                ▼
                ┌──────────────────────────┐
                │ → SkillStatus {          │
                │     Name, Status,        │
                │     Installed,           │
                │     LoadoutRef,          │
                │     Maintainer }         │
                └──────────────────────────┘
                        │
       ╚════════════════╧═══╝  (end skill loop)
                        │
                        ▼
             ┌──────────────────────┐
             │ Extra skills?        │
             │ (installed but NOT   │
             │  in loadout)         │
             │ → StatusExtra        │
             └──────────┬───────────┘
                        │
                        ▼
            EMIT SkillResolvedMsg × N
            (UI renders full list before downloads)
                        │
          ══════════════╪══════════════════════════════════
           PHASE 2:     │  SYNC (download + install)
          ══════════════╪══════════════════════════════════
                        │
       ╔════════════════╧════════════════════════╗
       ║  FOR EACH SkillStatus                   ║
       ╚════════════════╤════════════════════════╝
                        │
               ┌────────┴────────┐
               ▼                 ▼
        ┌─────────────┐   ┌──────────────┐
        │ Current /   │   │ Missing /    │
        │ Extra       │   │ Outdated     │
        │             │   │              │
        │ EMIT        │   │ (install)    │
        │ SkillSkipped│   │              │
        │ Msg         │   └──────┬───────┘
        └─────────────┘          │
                                 ▼
                      ┌─────────────────────┐
                      │ EMIT                │
                      │ SkillDownloadingMsg │
                      └──────────┬──────────┘
                                 │
                                 ▼
                      ┌─────────────────────┐
                      │ FetchDirectory()    │
                      │                     │
                      │ Git Trees API       │
                      │ (recursive=true)    │
                      │ → filter by path    │
                      │ → FetchFile × N     │
                      │ → []SkillFile       │
                      └──────────┬──────────┘
                                 │
                                 ▼
                      ┌─────────────────────┐
                      │ WriteToStore()      │
                      │                     │
                      │ ~/.scribe/skills/   │
                      │   <name>/           │
                      │     SKILL.md        │
                      │     scripts/...     │
                      │     .cursor.mdc ◄── │
                      │     (generated)     │
                      └──────────┬──────────┘
                                 │
                                 ▼
                  ┌──────────────┴──────────────┐
                  ▼                             ▼
        ┌──────────────────┐         ┌──────────────────┐
        │  ClaudeTarget    │         │  CursorTarget    │
        │  .Install()      │         │  .Install()      │
        │                  │         │                  │
        │  ~/.claude/      │         │  .cursor/rules/  │
        │   skills/<name>  │         │   <name>.mdc     │
        │       │          │         │       │          │
        │       ▼          │         │       ▼          │
        │   symlink to     │         │   symlink to     │
        │  ~/.scribe/      │         │  ~/.scribe/      │
        │  skills/<name>/  │         │  skills/<name>/  │
        │                  │         │  .cursor.mdc     │
        └────────┬─────────┘         └────────┬─────────┘
                 │                            │
                 └──────────┬─────────────────┘
                            │
                            ▼
                 ┌─────────────────────┐
                 │ st.RecordInstall()  │
                 │                     │
                 │ InstalledSkill {    │
                 │   Version,          │
                 │   CommitSHA,        │
                 │   Source,           │
                 │   Targets,          │
                 │   Paths,            │
                 │   InstalledAt: now  │
                 │ }                   │
                 └──────────┬──────────┘
                            │
                            ▼
                 ┌─────────────────────┐
                 │ st.Save()           │
                 │ (incremental —      │
                 │  after EACH skill)  │
                 │                     │
                 │ state.json.tmp →    │
                 │ atomic rename       │
                 └──────────┬──────────┘
                            │
                            ▼
                 EMIT SkillInstalledMsg
                 { Name, Version, Updated }
                            │
       ╚════════════════════╧═══╝  (end status loop)
                            │
          ══════════════════╪══════════════════════════════
           PHASE 3:         │  FINALIZE
          ══════════════════╪══════════════════════════════
                            │
                            ▼
                 ┌─────────────────────┐
                 │ st.RecordSync()     │
                 │ LastSync = now      │
                 │                     │
                 │ st.Save() (final)   │
                 └──────────┬──────────┘
                            │
                            ▼
                 EMIT SyncCompleteMsg
                 { Installed, Updated,
                   Skipped, Failed }
                            │
              ╚═════════════╧═══╝  (end repo loop)
                            │
                            ▼
               ┌────────────┴────────────┐
               ▼                         ▼
        ┌─────────────┐          ┌───────────────┐
        │  Text Mode  │          │  JSON Mode    │
        │             │          │               │
        │ "done: 2    │          │ {             │
        │  installed, │          │  "registries":│
        │  1 updated, │          │   [...],      │
        │  3 current" │          │  "summary":   │
        │             │          │   { counts }  │
        └─────────────┘          │ }             │
                                 └───────────────┘


           Disk After Sync
           ════════════════

 ~/.scribe/                ~/.claude/             .cursor/
 ┌──────────────────┐      ┌──────────────┐      ┌──────────────────┐
 │ config.yaml      │      │ skills/      │      │ rules/           │
 │ state.json       │      │  gstack/ ──────────>│  gstack.mdc ────>│
 │ skills/          │ ◄────│  deploy/ ──────────>│  deploy.mdc ────>│
 │   gstack/        │      │              │      │                  │
 │     SKILL.md     │      │ (dir         │      │ (file            │
 │     scripts/     │      │  symlinks)   │      │  symlinks to     │
 │     .cursor.mdc  │      └──────────────┘      │  .cursor.mdc)    │
 │   deploy/        │                            └──────────────────┘
 │     SKILL.md     │
 │     .cursor.mdc  │
 └──────────────────┘
   canonical store
```

---
*Generated with /ascii*
