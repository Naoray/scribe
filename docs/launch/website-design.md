# Scribe website — design exploration

**Status:** RESEARCH ONLY. No build work commissioned. This document is a single-pass exploration intended to give the user enough material to make decisions about a dedicated landing site for scribe (i.e. a `scribe.<tld>` web presence beyond the GitHub README).

**Out of scope of this doc:** writing code, picking a final stack, paying for a domain, designing logos. The doc surfaces options and tradeoffs and ends with an explicit list of decisions that only the user can make.

**Source material consulted:**

- `docs/launch/blog-post.md` (PR #143, branch `agent-launch-drafts`) — voice, hero claims, walkthrough script
- `docs/launch/hn-pitch.md` — title options, concrete rebuttals to "isn't this just X?"
- `docs/launch/asciinema-cast-script.md` — 60–90 s storyboard, ready-to-record beat sheet
- `README.md` — install snippets, "install via your agent" block, ASCII logo
- `docs/comparison.md` — when-to-pick framing vs alternatives
- `internal/logo/logo.go` — current brand surface (ASCII only) + the gradient palette already in use

> Voice + technical claims in those docs are still flagged **AWAITING-VOICE-REVIEW**. The website must not introduce new claims; it should pull from approved blog/README copy.

---

## 1. Goals and non-goals

### Goals

1. **One-page pitch** that lets a developer decide in <60 seconds whether scribe is for them.
2. **Show, don't tell** the install-via-agent and lockfile-driven flows. The cast script in `docs/launch/asciinema-cast-script.md` is the canonical demo — the website is a vehicle for it.
3. **Funnel to GitHub.** The README is the source of truth for docs. The site links into it for everything beyond the pitch.
4. **Agent-first signal.** The site itself should feel built for the same audience scribe targets: developers who paste prompts into Claude Code / Codex / Cursor and expect things to work.
5. **Low maintenance.** Site copy must not duplicate fast-moving content (command tables, exit codes, schemas). Link to the README/`docs/` markdown for those.

### Non-goals

- **No SaaS surface.** No login, no dashboard, no pricing, no analytics consent banner beyond the bare minimum.
- **No CMS.** Blog posts (if any) live as markdown in the repo, not in a backend.
- **No multi-page docs portal.** Docs stay in `docs/` on GitHub. Possible future: surface the rendered docs via Starlight, but that is a v2 decision, not v1.
- **No theming engine, no i18n.** Single locale (en), single theme (one dark, one light if effort is cheap, otherwise dark only).
- **No newsletter signup.** A "watch on GitHub" CTA is enough until there is an actual reason to email people.

---

## 2. Audience

Two reader profiles, in priority order:

1. **The developer who already runs `brew install` for new tools.** Has at least one of: Claude Code, Codex, Cursor, Gemini. Has at some point pasted a `SKILL.md` into a folder and forgotten about it. Wants the site to answer: what is this, why should I care, how do I install it, where is the code.
2. **The team lead deciding whether to standardise.** Wants the comparison framing, lockfile pitch, and an "install via your agent" snippet they can paste into a team chat.

A third audience — the agent itself, when a user pastes "set this up for me" — does not directly visit the site, but the site should be readable with `curl` and `lynx`, since agents may fetch it. That means: text-first HTML, no JS-only rendering for the headline, copy that survives without the asciinema demo.

---

## 3. Information architecture

### 3.1 Page list

Single page initially. One additional page only if pressure justifies it:

| Path | Purpose | Ship in v1? |
|---|---|---|
| `/` | Landing page (hero → install → demo → why → comparison → docs) | yes |
| `/docs/` | Optional: rendered `docs/*.md` via Starlight | **no** — defer |
| `/launch/` | Optional: blog-post.md rendered as a permalink for the v1.0 launch post | maybe — see §3.3 |
| `/og.png` etc. | Social card assets | yes |

The site is intentionally single-page. If the user wants `/docs/` rendered as Starlight later, that is an additive decision; the v1 doesn't need it because GitHub renders the markdown beautifully and that is where contributors already live.

### 3.2 Section order on `/`

Order matters more than copy here. Each section answers exactly one question.

```
1. Hero          — what is scribe + one CTA
2. Install       — how do I get it (3 paths: brew / curl / agent)
3. Demo          — what does a session feel like
4. Why scribe    — the four bullets from the README
5. How it works  — a tiny diagram of canonical store + projection
6. Comparison    — when to pick scribe (lifted from docs/comparison.md)
7. Docs / GitHub — links out
8. Footer        — license, repo, contributing, version
```

Rationale per section:

**1. Hero.** Headline + sub + two CTAs. No "trusted by" logo wall — there is none. No animated background. Black or near-black, gradient accent matching the existing logo palette (cyan #00B4D8 → green #60E890 in dark mode).

**2. Install.** Three tabs or three stacked code blocks: `brew install Naoray/tap/scribe`, `curl ... | tar xz`, and the "install via your agent" prose block from `README.md` lines 56–68. Each block has a copy-to-clipboard button. The "install via your agent" block is the differentiator and should be the prominent default tab on desktop.

**3. Demo.** The asciinema cast plays here. Below it: a one-line caption. Above it: a one-line setup line ("From a fresh machine to a synced project in 75 seconds."). See §5 for player choice.

**4. Why scribe.** Four cards from the README "Why scribe?" section. Each card: heading + 2–3 lines + maybe a tiny code snippet. Avoid feature lists longer than four; the comparison page handles thoroughness.

**5. How it works.** One ASCII or SVG diagram showing `~/.scribe/skills/` projecting to `~/.claude/skills/`, `~/.codex/skills/`, `.cursor/rules/`. The same layout is in `.claude/rules/go-cli-tui.md` ("canonical store + symlinks per target") — lift it. ASCII is fine; matches the brand.

**6. Comparison.** Either embed the table from `docs/comparison.md` directly (rebuild it as HTML) or link to GitHub for the full table and show only the "When to pick scribe" / "When to pick alternative" prose blocks. Recommendation: link to the full table; the prose blocks are the part that converts.

**7. Docs / GitHub.** Six link cards mapping to the six documentation links currently in `README.md` lines 129–136. No clever copy. Just labels + one-line teasers + outbound links.

**8. Footer.** MIT, repo link, latest release version (read from a JSON build step or hardcoded at deploy time), `agentskills.io` credit, link back to the comparison page.

### 3.3 Content hierarchy and length budgets

To prevent the page from drifting into marketing length:

- Hero: ≤ 25 words above the fold.
- Each "Why scribe" card: ≤ 35 words.
- Total page word count target: 350–550 words. The asciinema cast carries the demo weight; copy should not duplicate it.
- Code blocks count separately and should be exact paste-ables, not pseudo-syntax.

### 3.4 Optional `/launch/` permalink

If the v1.0 blog post in `docs/launch/blog-post.md` ships externally (HN, dev.to, mailing lists), it needs a stable URL. Two cheap options:

1. Render it at `/launch/` on the site itself. Adds a second page but lets the GitHub README link to a pretty URL.
2. Skip the website rendering and use the GitHub permalink (`https://github.com/Naoray/scribe/blob/main/docs/launch/blog-post.md`) as the canonical URL.

Recommendation: option 2 for v1; revisit if the blog post ends up being a recurring format. Spending build complexity on a one-off is wasteful.

---

## 4. Hero treatment

### 4.1 Recommended copy

```
HEADLINE
One manifest. Every agent. Skills always in sync.

SUB
Scribe is the package manager for AI coding-agent skills.
Connect a registry once, sync across Claude Code, Codex, Cursor, and Gemini.

PRIMARY CTA
brew install Naoray/tap/scribe                    [copy]

SECONDARY CTA
View on GitHub  →                                 [link]
```

All three lines come straight from approved copy:

- Headline merges README hero (`one manifest, one command`) with blog-post lead.
- Sub is the README tagline minus the punctuation salad.
- Primary CTA is the README install line, not a marketing "Get started" button. Devs prefer to see the actual command than to land on `/install`.

### 4.2 Alternates

If the user wants a tighter / louder headline:

- `scribe.lock for AI coding agents.` — leans on the package-manager analogy. Best if the site is HN-launch-aligned (HN crowd already reads "lockfile" as a positive signal).
- `Skills, sync, and ship — across every agent.` — more verb-y, more marketing.
- `Stop copy-pasting SKILL.md.` — problem-led, punchier, riskier (assumes audience already knows what `SKILL.md` is).

Recommendation: lead with the recommended copy. It is honest and matches the README. Save the punchier line for HN title.

### 4.3 Visual treatment

- Background: solid near-black (#0B0F14 or similar) on a dark theme. The gradient already used in `internal/logo/logo.go` (`#00B4D8` → `#60E890` for dark, `#0077B6` → `#2D6A4F` for light) sets the brand. Use it as a subtle gradient on the headline word "scribe" and on link/button accents.
- Hero asset: render the `logoFull` ANSI Shadow art as actual styled HTML/SVG above or beside the headline. There is no other logo. See §10 for whether to invest in a real logo.
- No hero illustration, no abstract gradients, no "AI sparkles." The brand is terminal-native; the site should feel like a terminal-native tool's docs page (closer to charm.sh than Vercel.com).
- Single fixed-width font for code (JetBrains Mono / IBM Plex Mono) and a clean sans for prose (Inter). Both are free to self-host.

---

## 5. Interactive demo options

The cast file from `docs/launch/asciinema-cast-script.md` is the *content*; the question is which *player* renders it.

### 5.1 Options considered

#### a. Embed `asciinema.org` iframe

- Pros: zero infra, three-line embed, works everywhere asciinema.org is reachable.
- Cons: third-party iframe; requires uploading the cast publicly to asciinema.org; styling control is limited (inherit asciinema.org's chrome). If asciinema.org goes down or rate-limits, demo breaks.
- Verdict: works as fallback. Not the primary embed.

#### b. `asciinema-player` v3.x via CDN (self-hosted cast file)

- Pros: open source (MIT), themeable via CSS variables, plays a `.cast` file you ship from your own origin, no third-party dependency, supports speed control + click-to-pause, accessible. Available as ESM/UMD on jsDelivr and unpkg.
- Cons: ~80 KB JS; one extra `.cast` asset to keep in sync with CLI behaviour; needs a small `<script>` and `<link>` integration. Re-record needed when CLI behaviour changes (already a known cost from the cast script).
- Verdict: **recommended primary.** Matches the "this looks like a tool I'd trust" vibe of charm.sh and asciinema.org's own site. The cast script is already targeted at this player ("Embed it in… README" — same player powers the README embed). Concrete reference: `https://github.com/asciinema/asciinema-player`, `https://docs.asciinema.org/manual/player/embedding/`.

#### c. Custom xterm.js terminal in the browser, replaying recorded keystrokes

- Pros: full control over chrome, can interleave keystroke replay with idle "press space to step" UX, dark/light theming trivial.
- Cons: writing a cast → xterm.js converter is non-trivial; xterm.js is ~250 KB; building a thing for the sake of building it. Re-records would need conversion every time. asciinema-player already does the same job at higher fidelity.
- Verdict: **reject.** Does not earn its complexity. The cast file ships ready to use.

#### d. Live wasm playground

- Pros: would let visitors run `scribe --version`, `scribe schema`, etc.
- Cons: scribe is a Go CLI that touches the filesystem (`~/.scribe/`, `~/.claude/`), spawns `gh`, writes lockfiles, hits the GitHub API. None of that maps cleanly to a wasm sandbox without a parallel "demo mode" implementation. Even if it were buildable, the install-via-agent angle is what differentiates scribe — running scribe in isolation from any agent is the *least* compelling demo. Also: maintenance cost is high (must rebuild the wasm bundle on every release; subtle behaviour drift between native and wasm is a real risk).
- Verdict: **reject for v1.** Could be revisited when scribe ships a `scribe playground` subcommand or when there is a clean read-only surface to expose.

#### e. Loom / MP4 video with chapter markers

- Pros: trivial to record and embed, captions easy, viewers tolerate auto-pause/scrub, chapters do well in YouTube embeds.
- Cons: large bytes (a 75 s 1080p clip easily breaches 8 MB), no copy-paste from the demo (asciinema lets you select text), ages worse (every CLI change requires re-rendering a video). Loom adds tracking; YouTube adds Google. MP4 self-host is large and imperfectly responsive.
- Verdict: **second-tier fallback** only if the asciinema embed misbehaves on a target platform.

#### f. Animated GIF

- Pros: pure-static, works everywhere including GitHub README, RSS readers, and `lynx` (with image preview).
- Cons: huge file size for any reasonable resolution, pixellated text, no pause/scrub, accessibility-hostile (no captions). The cast script already calls out `agg` to produce a GIF as a README fallback.
- Verdict: **third-tier fallback.** Useful as the README hero, NOT as the website primary demo.

### 5.2 Recommended combination

- **Primary:** asciinema-player v3.x loaded from jsDelivr CDN, playing a self-hosted `.cast` file shipped at `/demo/scribe-v1-demo.cast`. Theme via CSS variables to match the site palette.
  - Player JS: `https://cdn.jsdelivr.net/npm/asciinema-player@3/dist/bundle/asciinema-player.min.js`
  - Player CSS: `https://cdn.jsdelivr.net/npm/asciinema-player@3/dist/bundle/asciinema-player.css`
  - Config: `cols=80 rows=24 idleTimeLimit=1.5 speed=1.2 autoPlay=false poster=npt:00:01`
  - Re-record trigger: any time the CLI surfaces in the cast change (already documented in `asciinema-cast-script.md`).
- **Fallback (no JS):** the `agg`-generated GIF from the cast script, rendered with a `<noscript>` block. Same content, lower fidelity.
- **Tertiary fallback (catastrophic JS failure):** a static `<pre>` block showing the eight-line transcript of the demo so the page still tells the story to `curl` / `lynx` consumers.

### 5.3 What the demo must show

Lift the beat sheet directly from `asciinema-cast-script.md`:

1. `scribe --version` (fresh install)
2. `scribe registry connect Naoray/scribe-skills-essentials`
3. `scribe sync --all --json | jq '.data.summary'`
4. `cat scribe.lock | head -20`
5. `scribe list --json --fields name,managed,targets | jq '.data.skills[:5]'`
6. `scribe explain tdd --json | jq '.data.source'`
7. `scribe sync --json` (re-run, 0 changes — proves reproducibility)
8. `scribe doctor --json`

The site copy should NOT duplicate this list — the cast IS the list.

### 5.4 Surfacing the install-via-agent angle

The "install via your agent" angle is the strongest differentiator and the asciinema cast does NOT show it (the cast assumes scribe is already installed). Two options:

1. Record a second, shorter cast (≈30 s) showing only the "paste this prompt → agent installs scribe" flow. Embed it as a tabbed alternate to the main cast. *Cost:* recording time + a second cast file.
2. Surface the install-via-agent block as the visually loudest tab in §3.2's Install section, so the agent angle is unmissable even though the demo doesn't film it. *Cost:* none.

Recommendation: option 2 first; option 1 only if user feedback says the agent angle is being missed. The blog post and HN pitch both lead with that angle in copy, which is enough.

---

## 6. Reference sites

Five concrete dev-tool sites with shapes scribe should consider. For each: URL, what they do well, what to borrow, what to skip.

### 6.1 [bun.sh](https://bun.sh)

**What they do well:**
- Single-page above-the-fold pitch with one giant install command.
- Animated terminal preview shows the value (speed) within seconds.
- Mascot gives the brand personality without a rendered hero illustration.
- Clear "Why Bun?" feature grid with code snippets.

**Borrow:** the install-command-as-hero pattern. The "Why X?" section directly under install. The fact that the page is fast and not crowded with logos.

**Skip:** the mascot route — scribe doesn't need one and shouldn't invent one. The benchmarking emphasis — scribe is not pitching speed.

### 6.2 [charm.sh](https://charm.sh)

**What they do well:**
- Terminal-native aesthetic without being kitschy. Pinks and purples carry consistently across `bubbletea`, `wish`, `glow`, etc.
- Animated SVG / asciinema-style demos for each tool. Each demo is a real recorded session.
- Heavy use of fixed-width type for headers, sans for body. Reads as "this is a tool for terminal people."
- `charm.land/v2/...` for the Go module path is broken into the navigation.

**Borrow:** the entire visual register — terminal-native, fixed-width-friendly, a single distinct palette. Charm.sh is the closest reference for what scribe should look like. The "demo for every tool" pattern.

**Skip:** charm has many products; scribe has one. Don't replicate their multi-product nav. Their dark-theme-first colour choice is a good cue but scribe's existing palette (cyan-green) shouldn't ape pink-purple.

### 6.3 [biomejs.dev](https://biomejs.dev)

**What they do well:**
- Built on Astro Starlight; the docs and the landing live in the same project with minimal infra.
- Comparison-heavy framing ("Biome vs Prettier") right on the homepage. Honest about what they replace.
- Clean install / quickstart / why blocks.
- "Powered by" section near the footer that converts ("X uses Biome").

**Borrow:** Starlight as a future-proofing option (see §7). The comparison-as-conversion pattern — `docs/comparison.md` should make it onto the site, even if condensed.

**Skip:** the "powered by" logo wall — scribe has no public users to point to. Wait until there are.

### 6.4 [astral.sh/ruff](https://astral.sh/ruff) and [docs.astral.sh/ruff](https://docs.astral.sh/ruff/)

**What they do well:**
- Mercilessly concise hero. "An extremely fast Python linter" is the whole pitch.
- Performance-as-feature framing — bar charts, real benchmarks, with sources.
- Docs site cleanly separate from the marketing site. Marketing is Hugo (or similar static); docs is mkdocs-material.

**Borrow:** the discipline of saying one thing in the hero. The clean separation of marketing site from docs (apply later — for v1, GitHub renders the docs).

**Skip:** the benchmark obsession — scribe is not faster than alternatives, it has different goals. Don't fake metrics that don't exist.

### 6.5 [htmx.org](https://htmx.org)

**What they do well:**
- Aggressively minimal. One page. Low byte count. No tracking. Demonstrates the philosophy by being the philosophy.
- Code samples are the centerpiece — they are the marketing.
- The `htmx.org/docs/` is plain HTML, no fancy framework.

**Borrow:** the minimalism. The "the code samples ARE the marketing" rule. The willingness to ship a one-page site and stop.

**Skip:** the retro Web 1.0 styling — scribe's audience expects modern visual polish even if the surface is one page. The point is the byte budget and minimalism, not the aesthetic.

### 6.6 Honourable mentions (not deep-dived)

- [vite.dev](https://vite.dev) — feature grid + comparison patterns.
- [oven.sh / bun.sh](https://oven.sh) — same as bun.
- [zed.dev](https://zed.dev) — heavy on demo videos; instructive but heavier than scribe should be.
- [pkl-lang.org](https://pkl-lang.org) — Apple-style doc site; very polished, more effort than v1 should pay for.

---

## 7. Tech stack proposal

Three serious candidates plus two "if you really want to over-engineer" options. Recommendation at the end.

### 7.1 Astro (with optional Starlight)

- **What it is:** static-first framework, ships zero JS by default, lets you island-hydrate components when needed (asciinema-player in our case).
- **Why it fits:** scribe site is mostly static content + one interactive embed. Astro renders as static HTML, ships the player as a single island, gives you content collections later if `/docs/` ever moves on-site. Adopting Starlight in the future is low-friction.
- **Cost:** Node toolchain in the build (acceptable; CI already has Node). One `astro.config.mjs`. Build command: `npm run build`. Output: pure static HTML/CSS/JS in `dist/`.
- **References used in the wild:** biomejs.dev (Starlight), bun.sh (next-gen but Astro is the closer analogue for scribe), countless dev-tool sites in 2025–26.
- **Verdict:** **primary recommendation.** Cheapest path that scales if scribe outgrows a single page.

### 7.2 Plain HTML + Tailwind (CDN or build-time)

- **What it is:** a single `index.html` with a `<style>` block (or Tailwind built once), plus a small `<script>` for the asciinema player.
- **Why it fits:** zero framework overhead, fastest possible build, easiest contributor onboarding ("edit index.html and push").
- **Cost:** no component reuse, no MDX, hand-rolled if anything grows. Tailwind via CDN is fine for static sites; a one-time JIT build via `tailwindcss` CLI is also viable.
- **Verdict:** **fine fallback** if Astro feels heavy. Realistic ceiling: ~3 pages before this gets tedious. Starts to hurt if a `/docs/` ever ships.

### 7.3 11ty (Eleventy)

- **What it is:** Node-based static site generator. Markdown-first.
- **Why it fits:** simpler than Astro for "content site," excellent at rendering the markdown that already exists in `docs/`.
- **Cost:** less popular than Astro in this corner of the ecosystem; fewer ready-made components for asciinema-player; the Astro ecosystem is where the new dev-tool sites live.
- **Verdict:** **viable but not recommended.** No advantage over Astro for the same content set.

### 7.4 Hugo

- **What it is:** Go-based SSG.
- **Why it fits:** spiritually aligned with a Go CLI tool. Fast builds.
- **Cost:** templating language is its own learning curve; ecosystem of "react component for X" is non-existent. asciinema-player needs a manual `<script>` shim.
- **Verdict:** **viable**, mainly attractive if the user has a Hugo bias from prior projects. No strong recommendation.

### 7.5 Next.js (static export)

- **What it is:** Next.js with `output: 'export'`.
- **Why it fits:** familiar to many.
- **Cost:** drags in React hydration even when nothing is interactive. Pays for features (App Router, server actions) that the site does not need. Higher build complexity.
- **Verdict:** **reject.** Wrong tool for the job.

### 7.6 Recommendation

**Astro, no Starlight in v1.** Reasons:

- Static output by default. No surprise client JS.
- Asciinema-player slots in as one component (ESM import), no plumbing.
- If the site ever needs `/docs/`, switch to Astro Starlight by adding the integration — no migration off Astro.
- Build is trivial (`npm run build` → `dist/`); deployments are static-hosting friendly.
- Familiar to almost every dev-tool maintainer in 2025–26; lowering the bar for outside contributions.

If Astro feels like overkill for what is genuinely a one-page site, the **fallback is a hand-written `index.html` + Tailwind + a tiny `<script>` for the asciinema embed.** That works and is two days to ship. Don't reach for 11ty/Hugo/Next as a compromise — pick the high or the low.

---

## 8. Hosting and domain

### 8.1 Hosting tradeoffs

| Host | Pros | Cons | Verdict |
|---|---|---|---|
| **Cloudflare Pages** | Free generous tier (500 builds/month, unlimited bandwidth), fast global CDN, easy preview deploys per PR, Astro template ready, strong DDoS protection out of the box. | Account-bound (Cloudflare team needed for collaboration); analytics are Cloudflare-Web-Analytics-only without extra config. | **Primary recommendation.** |
| **Vercel** | Astro is Vercel-blessed; preview deploys per PR; great DX. | Bandwidth caps on free tier (100 GB/month) — fine for a launch but can bite during HN day; "Powered by Vercel" in metadata if free tier. | **Strong fallback.** |
| **Netlify** | Same shape as Vercel. Build minutes more generous historically. | Slightly slower CDN than CF; UI noisier. | **Equal alternative.** |
| **GitHub Pages** | Lives where the code already lives. Free. | No preview deploys per PR (Actions can simulate but it's clunky); CDN slower than CF/Vercel; no built-in form/edge if needed later. Default subdomain (`naoray.github.io/scribe`) leaks ownership in URL. | **Acceptable for a "no separate domain yet" launch.** |
| **Self-host on a VPS** | Total control. | Maintenance overhead unjustified for a static site. | **Reject.** |

**Recommendation:** Cloudflare Pages. Free, fast, good DX for static. Vercel as second choice. GitHub Pages only if the user wants zero new accounts and is fine with the `.github.io` URL.

### 8.2 Domain

`scribe.sh` is **registered via Cloudflare** (verified via WHOIS during this exploration; expiry 2027-05-29). Not currently scribe-the-CLI's. Possible to inquire whether the owner will sell, but realistically: pick a different domain.

#### Bulk availability check (May 2026, from this exploration)

| Domain | Status |
|---|---|
| scribe.sh | ❌ taken (Cloudflare) |
| scribe.dev | ❌ taken |
| scribe.io | ❌ taken |
| scribe.so | ❌ taken |
| scribe.run | ❌ taken |
| scribe.tools | ❌ taken |
| scribe.team | ❌ taken |
| scribe.codes | ❌ taken |
| scribecli.dev | ❌ taken |
| scribecli.com | ✅ available |
| scribe-cli.dev | ❌ taken |
| scribe-cli.com | ✅ available |
| getscribe.dev | ❌ taken |
| tryscribe.dev | ❌ taken |
| usescribe.com | ❌ taken |
| skillmanager.dev | ❌ taken |
| skillmanager.com | ❌ taken |
| agentskills.dev | ❌ taken |
| skillkit.dev | ❌ taken |
| runscribe.dev | ❌ taken |
| scribehq.com | ❌ taken |
| scribepkg.dev | ❌ taken |
| scribepkg.com | ✅ available |

(Treat this as a snapshot; re-check before purchasing.)

#### Recommended domains (pick one)

1. **`scribecli.com`** — clearest fit. "scribe" alone is too overloaded a word; `scribecli` removes ambiguity, signals "this is a CLI", reads on social cards. Downside: longer.
2. **`scribepkg.com`** — emphasises the package-manager framing. "scribepkg" is novel and Google-able; matches the lockfile / registry pitch. Downside: less obvious to a casual reader.
3. **GitHub Pages on `naoray.github.io/scribe`** — zero-cost, zero-decision. Fine for v1; the URL leaks repo ownership but devs already know how to read that.

Anti-recommendations:

- `naorayscribe.com` (available) — branded around the maintainer, not the tool. Bad if maintainership ever changes.
- `scribe-cli.com` (available) — hyphenated `.com` reads as squatter-bait; nine times out of ten people misremember the URL.

### 8.3 Subdomain on existing infra

A pragmatic v1: stand the site up at `scribe.naoray.dev` (if `naoray.dev` is owned) or `scribe.<existing-domain>`. Costs zero new domain dollars, ships fast, can rebrand to a dedicated domain later. The site URL can be migrated; what costs is the *rebuild*, not the URL.

---

## 9. Asset checklist

Audit of existing brand surface:

| Asset | Exists? | Where | Notes |
|---|---|---|---|
| ASCII logo (full) | ✅ | `internal/logo/logo.go` `logoFull` | "ANSI Shadow" style, 6 lines, used in TUI banner |
| ASCII logo (compact) | ✅ | `internal/logo/logo.go` `logoCompact` | FIGlet-style, 4 lines, narrow terminals |
| Brand palette | ✅ | `internal/logo/logo.go` `gradient()` | Dark: `#00B4D8` → `#60E890`. Light: `#0077B6` → `#2D6A4F`. Cyan → green, blue → green |
| Typography | partial | (none) | No font choice committed; site should pick sans + mono pair |
| Pixel logo (PNG/SVG) | ❌ | nowhere | None. Choice: render the ASCII logo as styled HTML/SVG, or commission a wordmark |
| Favicon | ❌ | nowhere | Needed |
| OG image | ❌ | nowhere | Needed (1200×630 PNG, branded) |
| Twitter card | ❌ | nowhere | Same dimensions as OG; reuse |
| Apple touch icon | ❌ | nowhere | Needed (180×180) |
| README hero asset | ❌ | `<!-- TODO: hero terminal screenshot or asciinema GIF of `scribe list` TUI -->` | Cast script's GIF fallback fills this slot |
| `assets/` folder in repo | ❌ | nowhere | Repo has `internal/logo/` only |
| `.github/` social previews | ❌ | only `.github/workflows/` exists | GitHub repo social card not set |

### 9.1 Minimum viable asset set for v1

Hard-required for launch:

1. **Favicon set** — 16×16, 32×32, 48×48, plus 180×180 apple-touch and a 32×32 `favicon.ico` fallback. Source from a 256×256 master.
2. **OG image** — one PNG, 1200×630. Black background, gradient on the wordmark, the eight-line ASCII compact logo. ~2 hours work in any vector tool.
3. **Wordmark** — render the existing ASCII art as a stylised SVG, OR use a dev-styled wordmark in JetBrains Mono / IBM Plex Mono with the gradient applied. Both are cheap.

Nice-to-have but not required for v1:

- A real logo mark (something that compresses to 16×16 and still reads). The ASCII art doesn't shrink that small. Could be a simple "S" with a gradient. Could be deferred.
- Animated GIF of TUI for the README. Already documented in cast script (`agg` step).

### 9.2 Risks of "no real logo"

The current state — ASCII art only — is fine for terminal output but reads as "this is a placeholder" in a website hero. Two paths:

1. Stylise the ASCII art with the existing gradient as the hero element. Treat the ASCII as the brand. Charm.sh / htmx.org can carry this kind of look.
2. Commission a real wordmark / mark. ~1 designer-day from someone who knows how to make a clean wordmark. Worth it if the user expects to build broader brand presence.

Recommendation: ship v1 with stylised ASCII; add a real wordmark in v1.x once there is signal that the site needs to scale beyond the GitHub-adjacent audience.

---

## 10. Out of scope (explicit)

These do **not** belong on the v1 site, no matter how easy they would be:

- **Auth, sign-up, login, accounts** — scribe is OSS CLI; the site has no concept of a user.
- **Pricing page** — there is no commercial offering. Implying one with "free / pro / enterprise" tiers would be dishonest and confuse the audience.
- **Dashboard / web app surface** — scribe does not have one. The "TUI" referenced in copy is a *terminal* TUI, not a web one.
- **Blog CMS** — markdown in the repo is enough. The launch post lives in `docs/launch/blog-post.md`. Future posts go beside it.
- **Comments, discussions** — link to GitHub Discussions if the user wants engagement. No on-site forum.
- **Newsletter sign-up** — premature. Add only when there is a recurring publishing cadence to support.
- **Telemetry collection beyond bare hosting analytics** — scribe is positioned as a respectful local tool; the site should match.
- **Cookie consent banner** — not needed if there are no third-party cookies. Use Cloudflare/Plausible analytics that don't set cookies.
- **A "team" / "about us" page** — not yet. Maintainer link in footer is enough.
- **An interactive command builder** — fun to imagine, easy to under-deliver on. The cast covers the same ground.
- **Live `scribe schema --json` browser** — the CLI exposes this; let agents call it. Surfacing it on the website without buy-in from agent vendors is decoration.
- **Per-target install pickers** (OS detection on the page) — adds complexity for a one-time install action. The README's three install paths are clear enough on a single page.
- **Multi-language docs** — out of scope until there is signal from non-English users.

---

## 11. Open questions for the user

These decisions cannot be made without input. The site cannot ship until they are answered.

1. **Domain.** `scribecli.com`, `scribepkg.com`, GitHub Pages subdomain, or a subdomain on an existing domain you already own (e.g. `scribe.naoray.dev`)? Buy decision aside, the URL choice influences the OG image (the URL appears on the social card) and the README's "Get started" link.

2. **Hosting.** Cloudflare Pages (recommended), Vercel, Netlify, or GitHub Pages? If you have an existing CF/Vercel account that already hosts something else, lean toward that.

3. **Tech stack.** Astro (recommended) or one-page hand-rolled HTML+Tailwind? Don't pick mid-tier (11ty/Hugo) without a strong reason — pick the high or the low.

4. **Logo investment.** Stylise the existing ASCII as the brand for v1, or commission a real wordmark / mark before launch? Cost difference is approximately one designer-day.

5. **Demo duplication.** Record a second 30-second cast specifically for the install-via-agent flow, or rely on copy alone to surface that angle? If "yes," it should happen alongside the v1.0 cast recording, not as a follow-up.

6. **Launch post hosting (optional).** Render `docs/launch/blog-post.md` at `/launch/` on the site, or use the GitHub permalink? Recommendation is to skip site rendering for v1, but if HN traffic is the goal, a clean URL helps.

7. **Analytics.** Cloudflare Web Analytics (cookieless, free), Plausible (paid, cookieless, well-regarded), or none at all? Defaulting to none is a defensible position for an OSS tool.

---

## 12. Out-the-door checklist (when work starts)

For when the user greenlights the build. Not part of this exploration, but useful as a starting point so this doc doesn't get re-derived in two months:

- [ ] Decide §11 questions
- [ ] Provision domain + hosting account
- [ ] Scaffold repo (sibling repo `Naoray/scribe-website` or `apps/website` inside `Naoray/scribe`)
- [ ] Stand up Astro project (or chosen alternative)
- [ ] Lift hero copy verbatim from `README.md`
- [ ] Lift "Why scribe" copy verbatim from `README.md` lines 122–127
- [ ] Lift install snippets verbatim from `README.md` lines 24–68
- [ ] Render `internal/logo/logo.go` ASCII as SVG/CSS for the hero
- [ ] Generate favicon set + OG image
- [ ] Drop in asciinema-player CDN, ship `.cast` file from the launch PR's recording
- [ ] Wire `noscript` GIF fallback
- [ ] Render comparison "When to pick scribe / alternatives" prose blocks; link out for the table
- [ ] Add footer with version pulled from latest GitHub release at deploy time
- [ ] Set up preview deploys per PR
- [ ] Add analytics (or explicitly skip)
- [ ] Verify on iPhone Safari, Android Chrome, desktop Firefox at 320 / 768 / 1280 px
- [ ] Verify `curl https://<domain>/` shows the headline + install snippet without JS
- [ ] Add `<link rel="canonical">`, `<meta>` OG tags, Twitter card, JSON-LD `SoftwareApplication`
- [ ] Run Lighthouse; target ≥95 on all four scores
- [ ] Update `README.md` to link to the site
- [ ] Update `docs/launch/blog-post.md` to use the site URL where appropriate

---

## 13. Appendix — copy snippets ready to lift

For convenience when implementation starts. All sourced from approved copy in this repo.

### 13.1 Hero (from `README.md` + `docs/launch/blog-post.md`)

> One manifest. Every agent. Skills always in sync.
>
> Scribe is the package manager for AI coding-agent skills.
> Connect a registry once, sync across Claude Code, Codex, Cursor, and Gemini.

### 13.2 Install via your agent (from `README.md` lines 60–68)

```
I want to use Scribe to manage my AI coding-agent skills on this machine.
Repo: https://github.com/Naoray/scribe (setup steps: /blob/main/SKILL.md)

Please set it up for me:
  1. If `scribe --version` fails, install it (prefer brew, fall back to release binary, last resort `go install`).
  2. Register Scribe's own agent-facing skill: `scribe add Naoray/scribe:scribe --yes --json`
  3. Show me `scribe list --json` to confirm.
```

### 13.3 Why scribe (from `README.md` lines 122–127)

- **Agents-first**: versioned JSON envelope, JSON Schema for every migrated command, distinct exit codes, machine-readable error remediation.
- **Project-local projection**: scopes skill availability to the project you're in, instead of dumping every installed skill into every session.
- **Adoption, not migration**: claims hand-rolled skills already in `~/.claude/skills/` etc. via symlink — nothing moves, nothing breaks.
- **One manifest, every tool**: `scribe.yaml` works across every supported agent — built-ins (Claude Code, Codex, Cursor, Gemini) plus any custom tool you register.

### 13.4 Comparison prose (from `docs/comparison.md`)

When-to-pick-scribe block (lines 22–27) and when-to-pick-alternative block (lines 30–37) are the two prose paragraphs to lift. The table itself should link to GitHub.

### 13.5 Footer

- Repo: `https://github.com/Naoray/scribe`
- License: MIT
- Skill format: [agentskills.io](https://agentskills.io)
- Latest release: read at deploy time from `https://api.github.com/repos/Naoray/scribe/releases/latest`

---

## 14. References

- Asciinema player: `https://github.com/asciinema/asciinema-player`
- Asciinema player embedding docs: `https://docs.asciinema.org/manual/player/embedding/`
- Astro: `https://astro.build`
- Astro Starlight: `https://starlight.astro.build`
- Tailwind: `https://tailwindcss.com`
- Cloudflare Pages: `https://pages.cloudflare.com`
- Vercel: `https://vercel.com`
- bun.sh: `https://bun.sh`
- charm.sh: `https://charm.sh`
- biomejs.dev: `https://biomejs.dev`
- astral.sh/ruff: `https://astral.sh/ruff`
- htmx.org: `https://htmx.org`

---

*Author: agent (exploration). Do not merge to main; this is a draft for the user to review and answer §11 before any build work begins.*
