# ASCII Logo & Interactive Root Command

## Overview

Add an ANSI Shadow ASCII art logo with a teal→cyan per-line gradient to Scribe's CLI, displayed when the user runs `scribe` with no arguments. The bare `scribe` command becomes an interactive hub showing status and quick actions.

## Logo

### Style

ANSI Shadow filled-block characters (`█`, `╔`, `╗`, `║`, `═`, `╚`, `╝`) — the 2025–2026 standard for CLI branding (Laravel, Claude Code, Gemini CLI).

### Two Sizes

**Full** (~48 chars wide × 6 lines, shown when terminal ≥60 cols):
```
███████╗ ██████╗██████╗ ██╗██████╗ ███████╗
██╔════╝██╔════╝██╔══██╗██║██╔══██╗██╔════╝
███████╗██║     ██████╔╝██║██████╔╝█████╗
╚════██║██║     ██╔══██╗██║██╔══██╗██╔══╝
███████║╚██████╗██║  ██║██║██████╔╝███████╗
╚══════╝ ╚═════╝╚═╝  ╚═╝╚═╝╚═════╝ ╚══════╝
```

**Compact** (~28 chars wide × 4 lines, shown when terminal ≥40 cols):
```
 ___  ___ ___ ___ ___ ___
/ __|/ __| _ \_ _| _ ) __|
\__ \ (__|   /| || _ \ _|
|___/\___|_|_\___|___/___|
```

**Plain text** (terminal < 40 cols): `Scribe vX.Y.Z`

### Color Gradient

Teal → Cyan (Ocean palette) via `lipgloss.Blend1D`. Bold styling applied. Per-line application — each of the 6 logo lines gets a color from the gradient.

**Dark terminals:**
- Start: `#00B4D8`
- End: `#60E890`

**Light terminals** (detected via `lipgloss.HasDarkBackground()`):
- Start: `#0077B6`
- End: `#2D6A4F`

The deeper palette ensures legibility on white backgrounds. Detection is blocking but acceptable for a one-shot render on startup.

### Version Display

Version string printed on the line below the logo, styled subtly (dimmed).

### No Tagline

Logo + version only. The help text explains the tool.

### Future: ASCII Mascot

The package structure anticipates a companion ASCII figure (a scribe character — quill/scroll/ink pot motif) that can sit beside the text logo via `lipgloss.JoinHorizontal`. Not in scope for this implementation, but `Render` should accept the full terminal width so composition is possible later. The mascot may eventually animate.

## Interactive Root Command

When the user runs `scribe` with no arguments in a TTY, the root command becomes an interactive hub.

### TTY Mode

1. **Logo** — rendered via `logo.Render`
2. **Status summary** — Lip Gloss styled:
   - Connected registries (count + names)
   - Installed skills count
   - Last sync time (relative, e.g. "2 hours ago")
   - Pending updates if known from last sync (no network calls — local state only)
3. **Stdin TTY check** — verify `isatty.IsTerminal(os.Stdin.Fd())` before showing the menu. If stdin is not a TTY (piped input), show logo + status only, then print a hint: `Run 'scribe --help' to see available commands`.
4. **Action menu** — `huh.NewSelect` with options: Sync, List, Connect, Guide, Help. Ctrl+C exits cleanly with code 130.
5. **On selection** — execute the selected command via `cmd.ExecuteContext(cmd.Context())` to run the full Cobra lifecycle (PreRunE, flag defaults, context propagation). Do not call `RunE` directly.

### Non-TTY / `--json` Mode

Skip logo and interactive menu. Output status as JSON:

```json
{
  "version": "1.0.0",
  "registries": ["ArtistfyHQ/team-skills"],
  "installed_count": 12,
  "last_sync": "2026-04-06T10:00:00Z",
  "pending_updates": 2,
  "stale_status": true
}
```

### CI Detection

When `CI` env var is set, behave the same as `--json`.

## Environment Detection & Degradation

Cascade (checked in order — monotonically degrading):

1. `--json` flag → JSON output, no logo, no menu
2. `!isatty(stdout)` → JSON output, no logo, no menu
3. `CI` env var set → same as `--json`
4. `TERM=dumb` → plain text version string + static status text, no menu, no block characters
5. `SCRIBE_NO_BANNER` env var set → skip logo, still show status + menu
6. `NO_COLOR` env var set → logo renders without ANSI colors (plain block characters), menu still works
7. Terminal width < 40 → plain text logo
8. Terminal width 40–59 → compact logo
9. Terminal width ≥ 60 → full logo

Logo suppression logic lives in `internal/logo/`. Menu/status decisions live in `cmd/root_hub.go`. The `logo.Render` function handles its own suppression internally.

## File Map

| Area | File | Action | Responsibility |
|------|------|--------|----------------|
| Logo | `internal/logo/logo.go` | Create | Logo constants, `Render(w, version)`, gradient, width detection, suppression |
| Logo | `internal/logo/logo_test.go` | Create | TTY/NO_COLOR/width tests, output assertions |
| Root hub | `cmd/root_hub.go` | Create | Status gathering, Lip Gloss styled output, Huh action menu, JSON mode |
| Root hub | `cmd/root_hub_test.go` | Create | Status formatting, JSON output tests |
| Root | `cmd/root.go` | Modify | Add `RunE` to `rootCmd`, add `--json` local flag, set `Args: cobra.NoArgs`, set `SilenceUsage: true` |

## Dependencies

- `charm.land/lipgloss/v2` — already in `go.mod`
- `github.com/mattn/go-isatty` — already in `go.mod`
- `golang.org/x/term` — **new**, for `term.GetSize(fd)` terminal width detection
- `charm.land/huh/v2` — already in `go.mod` (for action menu)

## Architecture Notes

- `internal/logo/` has **zero dependencies** on other Scribe internals — only Lip Gloss, go-isatty, x/term
- `cmd/root_hub.go` imports `config`, `state`, and `logo`
- Core packages remain UI-agnostic — the logo package is a presentation utility, not core logic
