# HN submission — scribe 1.0  **AWAITING-VOICE-REVIEW**

> *Title and pitch need user review. Aim for HN tone — concrete, no marketing fluff.*

## Title (under 80 chars)

**Primary:**
> Show HN: Scribe 1.0 – lockfile-driven skill manager for AI coding agents

(72 chars)

**Alternates to consider:**
- `Show HN: Scribe – one manifest for Claude Code, Codex, and Cursor skills` (72 chars)
- `Show HN: Scribe – package manager for AI coding-agent skills` (60 chars)
- `Show HN: Scribe 1.0 – scribe.lock for reproducible AI agent setups` (66 chars)

## URL

```
https://github.com/Naoray/scribe
```

(Switch to the v1.0.0 release URL once tagged: `https://github.com/Naoray/scribe/releases/tag/v1.0.0`.)

## First comment / Show HN body

> Hi HN — I built scribe because every AI coding agent on my laptop wanted its own copy of the same skills, and there was no good way to keep them in sync. Claude Code reads `~/.claude/skills/`, Codex reads `~/.codex/skills/`, Cursor reads `.cursor/rules/`, and so on. Sharing skills across a team meant Slack links and copy-paste. Skills went stale silently and nobody could answer "what's installed and why?"
>
> Scribe is a CLI that treats `SKILL.md` files like packages. You connect a GitHub registry once (`scribe registry connect <owner>/<repo>`), run `scribe sync`, and a canonical store under `~/.scribe/skills/` projects to every detected agent at the right paths. `scribe.lock` pins each skill by `commit_sha` and `content_hash`, so a teammate running `scribe sync` gets the exact same skill set you have. `scribe check` shows pending updates; `scribe update` advances the lock after review. `scribe adopt` claims hand-rolled skills already on the machine via symlink — nothing moves, nothing breaks.
>
> The other thing I cared about: agent ergonomics. Every migrated command emits a versioned JSON envelope (`{status, format_version, data, meta}`), uses semantic exit codes (10 for partial success, etc.), and exposes its JSON Schema via `scribe schema <command> --json`, so an agent can compose calls without guessing flags. Project-level `.scribe.yaml` scopes which skills load per-project, which keeps Codex inside its 5440-byte session-description budget without manual pruning.
>
> Skill format is the open [agentskills.io](https://agentskills.io) `SKILL.md`, so anything that works with skills.sh or Paks works with scribe. The [comparison doc](https://github.com/Naoray/scribe/blob/main/docs/comparison.md) is honest about when other tools (Superpowers, Cursor MDC, Cline/Roo, MCP servers) are a better fit — scribe is not trying to be an IDE agent or an MCP server, it's the layer that ships and pins the skills those tools consume.
>
> Brew: `brew install Naoray/tap/scribe`. Or paste the README's "install via your agent" block into Claude Code / Cursor / Codex with shell access and it bootstraps itself. Curious what's missing — please tire-kick.

---

## Notes for the user before posting

- HN is allergic to marketing. Drop any sentence that sounds like a release post if rewriting. The current draft leans concrete on purpose; trim further if it still reads "launchy."
- Mention what scribe is *not* (not an IDE agent, not an MCP server) early to pre-empt "isn't this just X?" comments.
- Have answers ready for: "why not just commit a folder of skills to the team repo?" (no cross-tool projection, no lockfile, no adoption, no per-project scoping); "how is this different from skills.sh?" (skills.sh is the format spec; scribe is the manager); "why Go?" (single binary, no runtime to install on a teammate's laptop).
- Pick the title last. The "lockfile-driven" framing tends to land well with the package-manager-comfortable HN crowd.
- Submit Tuesday-Thursday morning Pacific for best window. Avoid Friday/Sunday.
- If posting requires a Show HN, the body above already opens with "Hi HN — I built scribe…" which fits the form. If posting as a regular submission, drop the "Hi HN" line.

---

*AWAITING-VOICE-REVIEW: title, body voice, and timing all need user review. Cross-check claims against the v1.0.0 release before submitting.*
