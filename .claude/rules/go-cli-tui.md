---
paths: "**/*.go"
---

# Go CLI + TUI Integration Rules

Best practices for Go CLI tools that combine Cobra commands with Bubble Tea v2 (Charm) interactive UIs and event-driven architecture. Complements `go-cli.md` (Cobra) and `charm.md` (Bubble Tea).

## Architecture: Three-Tier Separation

```
cmd/                    # Cobra commands — wire flags, detect TTY, bridge to core
internal/
  <domain>/             # UI-agnostic core — emits events, returns errors, never prints
  ui/                   # Bubble Tea models — pure presentation consuming events
  state/                # Persistent state — atomic file ops
  targets/              # Output writers — file/symlink strategies
```

### Rules
- Core packages (`sync/`, `state/`, `github/`, etc.) MUST NOT import `fmt.Print*`, `os.Stdout`, `bubbletea`, `huh`, or `lipgloss`
- All output decisions live in `cmd/` or `internal/ui/`
- No arrows point upward — core never imports cmd or ui
- The `cmd/` layer is the bridge: it prepares inputs, wires callbacks, and converts errors to exit codes

## Cobra + Bubble Tea v2 Bridge

### RunE to tea.NewProgram Bridge

```go
func runSync(cmd *cobra.Command, args []string) error {
    cfg := configFromFlags()  // flags → config struct
    m := newModel(cfg)        // config → model constructor
    p := tea.NewProgram(m, tea.WithContext(cmd.Context()))  // propagate Cobra context

    finalModel, err := p.Run()
    switch {
    case errors.Is(err, tea.ErrInterrupted):
        os.Exit(130)  // SIGINT convention
    case err != nil:
        return fmt.Errorf("TUI error: %w", err)
    }
    // Type-assert final model for business results
    if fm, ok := finalModel.(model); ok && fm.err != nil {
        return fm.err
    }
    return nil
}
```

### Rules
- State sharing: flags → config struct → model constructor → type-assert final model for results. Never use globals.
- `p.Run()` returns framework errors only (`ErrInterrupted`, `ErrProgramKilled`, `ErrProgramPanic`). Business logic errors must flow through the model.
- Pass `cmd.Context()` from Cobra for context cancellation when needed.

### Signal Handling in Raw Mode
- Bubble Tea puts the terminal in raw mode — ctrl+c produces `tea.InterruptMsg`, NOT SIGINT
- Must handle `tea.InterruptMsg` explicitly in `Update` or users cannot exit
- `tea.Suspend` for ctrl+z job control (v2 only)
- External SIGTERM triggers clean terminal restoration automatically
- Use `tea.WithoutSignalHandler` only if you need custom signal handling

## Cobra + Huh v2: Flags-Then-Prompt Pattern

The production-ready pattern is a three-tier priority chain — **flags override config, config overrides interactive prompts**:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    name, _ := cmd.Flags().GetString("name")
    isTTY := isatty.IsTerminal(os.Stdin.Fd())

    // Prompt only for missing values, only if interactive
    if name == "" && isTTY {
        if err := huh.NewInput().Title("Name").Value(&name).Run(); err != nil {
            return err
        }
    }
    if name == "" {
        return fmt.Errorf("--name is required in non-interactive mode")
    }
    return execute(name)
}
```

### Standalone fields vs forms
- **Standalone field** (single prompt): `huh.NewInput().Title(...).Value(&v).Run()` — no `NewForm` wrapper needed
- **Full form** (multi-field with context): `huh.NewForm(huh.NewGroup(...)).RunWithContext(cmd.Context())`
- Use `RunWithContext(cmd.Context())` for forms so Cobra's signal handling flows through
- Standalone `.Run()` is fine for quick single-value prompts

### Rules
- Always check TTY with `isatty.IsTerminal(os.Stdin.Fd())` before running any Huh prompt
- Non-TTY must fail with an actionable error ("--flag is required in non-interactive mode")
- Flags take precedence over prompts — only prompt for values not provided via flags

## Event-Driven Core Pattern

Use a callback-based event emission pattern to keep the core UI-agnostic while remaining compatible with Bubble Tea's `tea.Msg` protocol.

### Emit Callback

```go
// In core package — no bubbletea import needed
type Syncer struct {
    Emit func(any)  // caller provides; nil-safe
    // ...
}

// Nil-safe guard method
func (s *Syncer) emit(msg any) {
    if s.Emit != nil {
        s.Emit(msg)
    }
}
```

### Event Type Naming

```go
// Name events with *Msg suffix — drop-in compatible with tea.Msg
type SkillResolvedMsg struct { ... }
type SkillInstalledMsg struct { ... }
type SkillErrorMsg     struct { Name string; Err error }
type SyncCompleteMsg   struct { Installed, Updated, Skipped, Failed int }
```

### Rules
- Event types use `*Msg` suffix following Bubble Tea convention, but live in the core package (no `bubbletea` import)
- The `Emit` field is `func(any)` — avoids coupling to `tea.Msg` interface
- `tea.Msg` is still `interface{}` / `any` in v2 — the `func(any)` callback pattern works unchanged
- Always provide a nil-safe `emit()` guard method so the core works without a callback
- Emit all "resolved" events before starting work (populate-then-act) so TUI can render the full list immediately
- Per-item errors emit `*ErrorMsg` and continue — never abort the loop for a single failure
- Emit a final `*CompleteMsg` with aggregate counts

### Bridging to Bubble Tea

```go
// In cmd/ — plain text mode
syncer.Emit = func(msg any) {
    switch m := msg.(type) {
    case sync.SkillInstalledMsg:
        fmt.Printf("  ✓ %s\n", m.Name)
    case sync.SkillErrorMsg:
        fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", m.Name, m.Err)
    // ...
    }
}

// In cmd/ — Bubble Tea v2 mode
syncer.Emit = func(msg any) {
    p.Send(msg)  // unchanged — *Msg types are still tea.Msg compatible
}
```

## Non-TTY Detection and Fallback

### Format Resolution

```go
func resolveFormat(requested string) string {
    if requested != "auto" { return requested }
    if term.IsTerminal(int(os.Stdout.Fd())) { return "tui" }
    return "text"
}
```

### Detection Points

```go
// Output mode — check STDOUT (golang.org/x/term or go-isatty)
useJSON := flagJSON || !term.IsTerminal(int(os.Stdout.Fd()))

// Interactive input — check STDIN
if !term.IsTerminal(int(os.Stdin.Fd())) {
    return fmt.Errorf("interactive mode requires a terminal — pass arguments directly")
}
```

### Rules
- Check `os.Stdout.Fd()` for output format decisions (JSON vs pretty)
- Check `os.Stdin.Fd()` for interactive prompt decisions (huh forms, TUI)
- Auto-enable JSON when stdout is piped — don't require explicit `--json`
- `--json` flag provides explicit override for TTY environments that want machine output
- Non-TTY errors should be actionable: tell the user what flag to pass instead
- Use `tea.WithoutRenderer` for non-interactive modes that still benefit from Elm architecture
- Huh v2 does NOT auto-detect non-TTY — always gate with isatty check before running forms
- Use `form.WithInput(r).WithOutput(w)` for CI/testing scenarios

### Three Output Modes

| Context | Detection | Output |
|---|---|---|
| Interactive terminal | `isatty(stdout) && !--json` | Bubble Tea TUI or styled text |
| Piped / redirected | `!isatty(stdout)` | Auto-JSON |
| Explicit JSON | `--json` flag | JSON regardless of TTY |

### Background Detection for Styles

```go
// In Bubble Tea Update handler
case tea.BackgroundColorMsg:
    m.isDark = msg.IsDark()
    m.styles = newStyles(m.isDark)  // rebuild styles for dark/light
```

## Cobra Command Structure

### One command per file

```go
// cmd/sync.go
var syncJSON bool

var syncCmd = &cobra.Command{
    Use:   "sync",
    Short: "Sync skills from team registries",
    RunE:  runSync,
}

func init() {
    syncCmd.Flags().BoolVar(&syncJSON, "json", false, "output as JSON")
}

func runSync(cmd *cobra.Command, args []string) error {
    // Implementation here — this is where TTY detection and Emit wiring happen
}
```

### Rules
- One file per command in `cmd/` — `sync.go`, `list.go`, `connect.go`, etc.
- All commands use `RunE` (returns `error`), never `Run`
- No `os.Exit()` in command handlers — return errors, let Cobra handle exit codes. Exception: `os.Exit(130)` after `tea.ErrInterrupted` for SIGINT convention.
- Package-level `var` for flags (`var syncJSON bool`), read directly in `runX` functions
- Register subcommands in `root.go:init()` via `rootCmd.AddCommand(...)`
- Version injection via `var Version = "dev"` + ldflags override

## Atomic State Persistence

### Pattern: Load-gracefully, mutate-in-memory, save-atomically

```go
// Load — missing file returns empty state, not error
func Load(path string) (*State, error) {
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return &State{Installed: make(map[string]InstalledSkill)}, nil
    }
    // ...
    if s.Installed == nil {
        s.Installed = make(map[string]InstalledSkill)
    }
    return &s, nil
}

// Save — write to .tmp then atomic rename
func (s *State) Save() error {
    data, _ := json.MarshalIndent(s, "", "  ")
    tmp := s.path + ".tmp"
    if err := os.WriteFile(tmp, data, 0644); err != nil {
        return err
    }
    return os.Rename(tmp, s.path)
}
```

### Rules
- `os.IsNotExist` → return empty initialized struct, never error
- Always `make()` map fields after unmarshal — JSON `null` → nil map → panic on write
- Atomic save: write `.tmp` then `os.Rename` (POSIX atomic)
- Save incrementally during long operations (after each item, not just at end)
- Ignored save errors (`_ = st.Save()`) are acceptable mid-operation — document with comment
- Mutation methods modify in-memory state; caller is responsible for `Save()`
- No file locking needed for single-user CLI tools

## GitHub API Integration

### Auth Chain Pattern

```go
func resolveToken(configToken string) string {
    // 1. gh auth token (piggyback on GitHub CLI)
    if tok := ghAuthToken(); tok != "" { return tok }
    // 2. GITHUB_TOKEN env
    if tok := os.Getenv("GITHUB_TOKEN"); tok != "" { return tok }
    // 3. Config file token
    if configToken != "" { return configToken }
    // 4. Unauthenticated (public repos only)
    return ""
}
```

### Rules
- Priority: `gh auth token` → `GITHUB_TOKEN` → config file → unauthenticated
- Wrap GitHub API errors into user-actionable messages ("run `gh auth login`", "set GITHUB_TOKEN")
- No retry logic for CLI tools — surface errors immediately, let user retry
- Use `GetTree(recursive=true)` + filter for directory listing (one call vs many)

## Config Migration Pattern

When evolving config fields, migrate forward transparently:

```go
type rawConfig struct {
    TeamRepo  string   `toml:"team_repo"`   // v1 singular
    TeamRepos []string `toml:"team_repos"`  // v2 plural
}

func Load(path string) (*Config, error) {
    var raw rawConfig
    // ...
    if raw.TeamRepo != "" && len(raw.TeamRepos) == 0 {
        raw.TeamRepos = []string{raw.TeamRepo}
    }
    // ...
}
```

### Rules
- Keep old field in a shadow struct for deserialization
- Promote old → new silently on load
- Never break existing config files

## Install Target Pattern

### Canonical store + symlinks per target

```
~/.scribe/skills/<name>/        <- canonical store (one copy of truth)
  SKILL.md
  scripts/deploy.sh

~/.claude/skills/<name>         <- symlink -> canonical dir
<project>/.cursor/rules/<name>  <- symlink -> canonical file
```

### Rules
- `WriteToStore` always does `os.RemoveAll` before writing — clean slate on update
- Generate target-specific formats (e.g., `.cursor.mdc`) during store write
- `Target` interface is minimal: `Name() string` + `Install(skillName, canonicalDir) ([]string, error)`
- Return installed paths so state can record them for future uninstall
- Use struct-level dependency injection (`WorkDir` field) for testability, not interface injection
- `os.Remove` then `os.Symlink` for link replacement (acceptable non-atomic for CLI)

## Testing Patterns

### Filesystem Isolation

```go
func TestSomething(t *testing.T) {
    t.Setenv("HOME", t.TempDir())  // all os.UserHomeDir() calls redirect here
    // test code — no cleanup needed
}
```

### Rules
- `t.Setenv("HOME", t.TempDir())` for any test touching `~/.scribe/` or `~/.claude/`
- Table-driven tests (`[]struct{ name string; ... }` + `t.Run`) for pure logic
- Black-box tests (`package foo_test`) by default — test exported API
- White-box tests (`package foo`) only for unexported helpers worth testing directly
- No mock infrastructure — no mock GitHub clients, no interface injection for tests
- Test non-TTY behavior for free: test runner stdin is not a TTY
- Test atomic write explicitly: verify `.tmp` files don't linger after `Save()`

### Common Mistakes

- **Importing `bubbletea` in core packages** — breaks the UI-agnostic contract. Use `func(any)` callback instead.
- **Using `os.Exit()` in command handlers** — bypasses deferred cleanup. Return errors from `RunE`. Only exception: `os.Exit(130)` after `tea.ErrInterrupted`.
- **Ignoring `tea.InterruptMsg`** — Bubble Tea uses raw mode, so ctrl+c sends `tea.InterruptMsg`, not SIGINT. Must handle it in `Update` or users cannot exit.
- **Not checking `p.Run()` sentinel errors** — always check `errors.Is(err, tea.ErrInterrupted)` before treating as generic error. Framework errors and business errors are separate channels.
- **Checking `isatty` on wrong fd** — stdout for output decisions, stdin for input decisions. Mixing them causes silent failures.
- **Forgetting to `make()` maps after JSON unmarshal** — `null` JSON field → nil map → panic on first write.
- **Atomic write without rename** — writing directly to the state file risks corruption on interrupt. Always write `.tmp` then `os.Rename`.
- **Aborting loops on per-item errors** — emit error event, continue loop, report aggregate failure count.
- **Hardcoding output format** — always support `--json` and auto-detect non-TTY. CI agents and piped commands need machine-readable output.
- **Blocking huh prompts without TTY check** — Huh v2 does NOT auto-detect non-TTY. `huh.NewForm().Run()` hangs on non-TTY stdin. Always gate with `isatty.IsTerminal(os.Stdin.Fd())`.
- **Using globals for state sharing** — pass flags via config struct to model constructor, type-assert final model for results. Never use package-level variables to shuttle data between Cobra and Bubble Tea.
- **Not propagating Cobra context to Huh forms** — use `form.RunWithContext(cmd.Context())` so Cobra's signal handling flows through.
- **Relying on `tea.BackgroundColorMsg` in tmux** — `tea.RequestBackgroundColor` silently fails in tmux. Provide a fallback (force dark mode or use `lipgloss.HasDarkBackground()`).
