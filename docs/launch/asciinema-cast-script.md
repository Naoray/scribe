# Asciinema cast — scribe v1.0 demo  **AWAITING-VOICE-REVIEW**

> *Storyboard order and which commands to feature need user review. Recording itself is human work.*

Target length: **60-90 seconds**.

The cast tells one story: **from a fresh machine to a synced project, with a lockfile to prove it's reproducible.**

## Recording setup

```bash
# Clean shell, minimal prompt, fixed window size for portrait readability on web embeds.
PS1='% ' bash --noprofile --norc

# Record with a generous idle cap so pauses look natural without bloating the cast.
asciinema rec scribe-v1-demo.cast \
  --idle-time-limit 1.5 \
  --cols 80 --rows 24 \
  --title "scribe v1.0 — connect, sync, lock" \
  --command "bash --noprofile --norc"
```

Inside the recording shell, set up a clean home so the demo isn't polluted by the recorder's real config:

```bash
export HOME=$(mktemp -d)
export PS1='% '
mkdir my-project && cd my-project
clear
```

## Storyboard (run in this order, type at conversational speed)

```bash
# 0. Sanity — show this is a fresh machine. Beat: 1s pause after.
scribe --version

# 1. Connect to the curated essentials registry.
#    Beat: pause briefly while the catalog fetches so the registry name is readable.
scribe registry connect Naoray/scribe-skills-essentials

# 2. Sync — writes scribe.lock, projects skills into ~/.claude/skills/, ~/.codex/skills/, etc.
#    Use --json | jq for a clean structured summary instead of the full TUI flow.
scribe sync --all --json | jq '.data.summary'

# 3. Show the lockfile that just got written. This is the reproducibility receipt.
cat scribe.lock | head -20

# 4. List what we got. The TUI flashes by; this is fine — it shows the count + names.
scribe list --json --fields name,managed,targets | jq '.data.skills[:5]'

# 5. Show source attribution on a single skill.
scribe explain tdd --json | jq '.data.source'

# 6. Re-sync against the lockfile — proves reproducibility.
#    Should report 0 changes.
scribe sync --json | jq '.data.summary'

# 7. (Closing beat) doctor for the agent contract. Quick health check.
scribe doctor --json | jq '.status, .data'
```

## Beat sheet (~75s budget)

| Beat | Command | Pause after | Why it earns its seconds |
|---:|---|---:|---|
| 1 | `scribe --version` | 1s | Establishes "fresh install" baseline. |
| 2 | `scribe registry connect ...` | 2s | Names the canonical 1-step setup. |
| 3 | `scribe sync --all --json \| jq` | 3s | The headline result — reproducible install with a structured receipt. |
| 4 | `cat scribe.lock` | 3s | Shows `commit_sha` + `content_hash`. The lockfile is the v1.0 hero. |
| 5 | `scribe list --json --fields ...` | 2s | Demonstrates `--fields` projection — agent ergonomics. |
| 6 | `scribe explain tdd --json` | 2s | Source attribution = "where did this skill come from?" |
| 7 | `scribe sync --json` (re-run) | 2s | "0 changes" proves the lockfile works. |
| 8 | `scribe doctor --json` | 2s | Closing health check, JSON envelope on screen. |

Total: ~75s with prompt + typing. Trim beat 7 or 8 if the recording overruns 90s.

## Notes for recording

- **Window**: 80×24 — narrow enough to read on a phone-width blog embed, wide enough that `scribe.lock` rows don't wrap.
- **Prompt**: Set `PS1='% '`. Default zsh/bash prompts eat half the line.
- **Typing speed**: Slightly slower than natural. The viewer needs to read the command before output flashes by.
- **Pauses**: Let `--idle-time-limit 1.5` do the trimming. Don't try to be perfectly cadenced live.
- **`jq` availability**: Confirm `jq` is on PATH in the demo shell. It's the one external dep this storyboard relies on.
- **Sanitize**: Run once, then watch the playback at full speed before uploading. Look for stray paths under `/var/folders/...` (the `mktemp` home) — if they show up, redact in post or set `HOME` to a friendlier path like `/tmp/scribe-demo`.
- **Exit code on beat 3**: `scribe sync --all --json` may emit `partial_success` exit code `10` on a fresh install if any skill fails to project (rare, but possible on macOS without `gh` auth). If that happens during recording, either re-run after fixing or trim the trailing failure to keep the story clean. Don't fake it.

## Upload

```bash
asciinema upload scribe-v1-demo.cast
# returns a URL like https://asciinema.org/a/<id>
```

Embed it in:

- `README.md` — replace the `<!-- TODO: hero terminal screenshot or asciinema GIF of scribe list TUI -->` placeholder near the top.
- `docs/launch/blog-post.md` — link in the **Walkthrough** section ("The asciinema cast linked at the top walks through the same flow in 90 seconds.").
- The HN submission's first reply if the post does well — works as a low-friction "show, don't tell" follow-up.

## Optional: GIF fallback for the README

Asciinema embeds need JS. For pure-static markdown surfaces (release notes, mirrors), produce a GIF as well:

```bash
# Requires agg (https://github.com/asciinema/agg)
agg scribe-v1-demo.cast scribe-v1-demo.gif --speed 1.2 --cols 80 --rows 24
```

Keep the GIF under 4 MB; trim with `gifsicle -O3` if needed.

---

*AWAITING-VOICE-REVIEW: storyboard order, which subset of commands to feature, and copy choices (e.g. `tdd` as the example skill in beat 6) need user review. Re-record after any CLI behavior changes between draft and v1.0.0 tag.*
