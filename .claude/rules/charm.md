---
paths: "**/*.go"
---

# Charm (Bubble Tea) TUI Rules — v2

### v2 Migration — Module Paths
All Charm libraries moved to `charm.land` in v2:
```
github.com/charmbracelet/bubbletea  → charm.land/bubbletea/v2
github.com/charmbracelet/lipgloss   → charm.land/lipgloss/v2
github.com/charmbracelet/bubbles    → charm.land/bubbles/v2
github.com/charmbracelet/huh        → charm.land/huh/v2
```

### Elm Architecture
```go
type Model interface {
    Init() Cmd                    // Initial command (or nil)
    Update(Msg) (Model, Cmd)      // Handle messages, return new state
    View() View                   // Returns tea.View (declarative), NOT string
}
```
**Never modify state outside Update()**. View must be pure function of model.

`View()` returns a `tea.View` struct. Use `tea.NewView(content)` constructor and set fields for alt screen, cursor, mouse, etc:
```go
func (m model) View() tea.View {
    v := tea.NewView(m.renderContent())          // content via constructor
    v.AltScreen = true                           // replaces tea.WithAltScreen()
    v.MouseMode = tea.MouseModeCellMotion        // enum: MouseModeNone, MouseModeCellMotion, MouseModeAllMotion
    v.ReportFocus = true                         // replaces tea.WithReportFocus()
    v.WindowTitle = "My App"                     // declarative window title
    if m.showCursor {
        v.Cursor = &tea.Cursor{                  // nil = hidden; set struct to show
            Position: tea.Position{X: m.cursorX, Y: m.cursorY},
            Shape:    tea.CursorBar,
            Blink:    true,
        }
    }
    return v
}
```

**`MouseMode` is an enum, not a boolean.** `MouseModeNone` (default), `MouseModeCellMotion` (clicks/wheel/drag), `MouseModeAllMotion` (all events including passive motion).

### Message Handling
```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:              // NOT tea.KeyMsg (now an interface)
        switch msg.String() {
        case "q", "ctrl+c": return m, tea.Quit
        case "space":                  // NOT " " — space bar is now "space"
            m.toggle()
        }
        // Modifier checking (beyond String()):
        if msg.Mod == tea.ModCtrl && msg.Code == 's' {
            return m, m.save()
        }
    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
    case tea.BackgroundColorMsg:       // detect light/dark terminal
        m.isDark = msg.IsDark()
    case tea.InterruptMsg:             // SIGINT handling
        return m, tea.Quit
    case tea.PasteMsg:                 // paste events (was KeyMsg.Paste)
        m.input += msg.Content           // use .Content, not string(msg)
    case errMsg:
        m.err = msg.err
    }
    return m, nil
}
```

### Mouse Messages
Mouse uses an interface-based approach. `MouseMsg` is an interface with a `Mouse()` method; four concrete types handle specific events:
```go
case tea.MouseClickMsg:
    if msg.Button == tea.MouseLeft {
        m.handleClick(msg.Mouse().X, msg.Mouse().Y)  // position via .Mouse() method
    }
case tea.MouseReleaseMsg:
    // click ended — same .Mouse() accessor
case tea.MouseWheelMsg:
    if msg.Button == tea.MouseWheelUp {               // direction via .Button, not .Delta
        m.scrollUp()
    }
case tea.MouseMotionMsg:
    m.hover(msg.Mouse().X, msg.Mouse().Y)
```

### Commands (Async)
**Use `tea.Cmd` for ALL I/O** — never raw goroutines:
```go
func fetchData(url string) tea.Cmd {
    return func() tea.Msg {
        resp, err := http.Get(url)
        if err != nil { return errMsg{err} }
        return dataMsg{resp}
    }
}
```
- `tea.Batch(cmd1, cmd2)` — concurrent (no order guarantee)
- `tea.Sequence(cmd1, cmd2)` — ordered execution
- `tea.Quit`, `tea.ClearScreen`, `tea.Suspend` (ctrl+z job control)
- `tea.Tick(d, fn)` — one-shot timer; `tea.Every(d, fn)` — recurring timer
- `tea.Exec(cmd, callback)` — execute external process (e.g. $EDITOR)
- `tea.SetClipboard("text")` / receive `tea.ClipboardMsg`
- `tea.Println(args...)` / `tea.Printf(fmt, args...)` — print above inline program
- `tea.RequestBackgroundColor` — query terminal for dark/light; response arrives as `BackgroundColorMsg`
- `tea.RequestWindowSize` — query terminal dimensions
- Removed: `tea.EnterAltScreen`, `tea.ExitAltScreen`, `tea.EnableMouseCellMotion` — use `tea.View` fields instead

### Component Composition
Embed Bubbles in model, forward messages, collect commands:
```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd
    m.list, cmd = m.list.Update(msg); cmds = append(cmds, cmd)
    m.input, cmd = m.input.Update(msg); cmds = append(cmds, cmd)
    return m, tea.Batch(cmds...)
}
```
**Track focus state** to route keyboard input to correct component.

### Program Options
```go
p := tea.NewProgram(model{},
    tea.WithContext(ctx),          // propagate cancellation from Cobra
    tea.WithFPS(60),              // custom frame rate
)
// Send external messages: go func() { p.Send(myMsg{}) }()
```
Alt screen, mouse, etc. are now set via `tea.View` fields in `View()`, not program options.

Surviving program options: `WithContext(ctx)`, `WithFPS(fps)`, `WithFilter(fn)`, `WithColorProfile(p)`, `WithInput(r)`, `WithOutput(w)`, `WithEnvironment(env)`, `WithWindowSize(w, h)`, `WithoutCatchPanics()`, `WithoutSignalHandler()`.

Sentinel errors for program exit:
- `tea.ErrInterrupted` — SIGINT received
- `tea.ErrProgramKilled` — program killed
- `tea.ErrProgramPanic` — panic recovered

### Lip Gloss v2 Styling
```go
import "charm.land/lipgloss/v2"

style := lipgloss.NewStyle().
    Bold(true).Foreground(lipgloss.Color("#FAFAFA")).  // Color() is now a function returning color.Color
    Padding(1, 2).Border(lipgloss.RoundedBorder()).Width(40)
output := style.Render("Hello")

// Light/dark adaptive colors — AdaptiveColor is removed
fg := lipgloss.LightDark(m.isDark)("#000", "#FFF")   // use isDark from tea.BackgroundColorMsg

// Color utilities
darker := lipgloss.Darken(lipgloss.Color("#FF0000"), 0.2)
lighter := lipgloss.Lighten(lipgloss.Color("#FF0000"), 0.2)
comp := lipgloss.Complementary(lipgloss.Color("#FF0000"))
semi := lipgloss.Alpha(lipgloss.Color("#FF0000"), 0.5)

// Layout
lipgloss.JoinHorizontal(lipgloss.Top, a, b)
lipgloss.JoinVertical(lipgloss.Left, header, content)
lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
w, h := lipgloss.Size(block)

// Compositing/layering (with mouse hit testing)
sidebar := lipgloss.NewLayer(box.Render("Sidebar")).X(0).Y(0).Z(1).ID("sidebar")
main := lipgloss.NewLayer(box.Render("Main")).X(22).Y(0).Z(0).ID("main")
comp := lipgloss.NewCompositor(sidebar, main)
output := comp.Render()
hit := comp.Hit(mouseX, mouseY)  // returns layer ID at coordinates

// Standalone output (outside Bubble Tea)
lipgloss.Println("styled output")   // handles color downsampling
lipgloss.Printf("hello %s", name)
```
Inside Bubble Tea, the Cursed Renderer handles color downsampling automatically.

### Bubbles v2 Components

| Component | Usage |
|-----------|-------|
| `textinput` | `.Focus()`, `.Placeholder`, `.EchoMode`, styles via `.Styles().Focused`/`.Styles().Blurred` (getter method) |
| `textarea` | Multi-line, styles via `.Styles().Focused`/`.Styles().Blurred` (getter method) |
| `list` | Items implement `Title()`, `Description()`, `FilterValue()` |
| `table` | `WithColumns()`, `WithRows()`, `WithFocused(true)` |
| `viewport` | Functional options constructor, `.SoftWrap`, `.LeftGutterFunc`, `.SetHighlights()`, `.ScrollRight()` |
| `spinner` | `.Spinner = spinner.Dot`, use `s.Tick()` method (not package-level `Tick()`) |
| `progress` | `.SetPercent()` returns Cmd, `WithColors(stops...)`, `WithDefaultBlend()`, `WithColorFunc(fn)` |
| `filepicker` | `.CurrentDirectory`, `.AllowedTypes` |
| `help` | KeyMap with `ShortHelp()`, `FullHelp()` |

Key Bubbles v2 changes:
- Width/Height fields replaced by getter/setter methods
- `DefaultKeyMap` variable replaced by `DefaultKeyMap()` function
- `DefaultStyles()` now takes `isDark bool` parameter
- Viewport constructor: `viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))`
- No `HighPerformanceRendering` — single renderer in Bubble Tea v2
- `runeutil` and `memoization` moved to internal (not importable)

### Key Bindings
```go
import "charm.land/bubbles/v2/key"

keys := struct{ Up, Quit key.Binding }{
    Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
    Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
// In Update: if key.Matches(msg, keys.Up) { ... }
```

### Huh v2 Forms

**Full form (multiple fields grouped):**
```go
import "charm.land/huh/v2"

var name string
form := huh.NewForm(huh.NewGroup(
    huh.NewInput().Title("Name").Value(&name).Validate(func(s string) error {
        if len(s) < 3 { return errors.New("too short") }
        return nil
    }),
)).WithTheme(huh.ThemeCharm(isDark))

err := form.RunWithContext(cmd.Context())  // propagate Cobra context for cancellation
```

**Standalone field (single prompt — no NewForm wrapper needed):**
```go
var repo string
err := huh.NewInput().
    Title("Team skills repo").
    Placeholder("owner/repo").
    Validate(validateRepo).
    Value(&repo).
    Run()  // standalone fields support .Run() directly
```

**Standalone confirm:**
```go
var yes bool
err := huh.NewConfirm().Title("Continue?").Value(&yes).Run()
```

Fields: `NewInput()`, `NewText()`, `NewSelect[T]()`, `NewMultiSelect[T]()`, `NewConfirm()`, `NewFilePicker()`, `NewNote()`

**Dynamic options (re-evaluate when watched value changes):**
```go
huh.NewSelect[string]().Title("State").
    OptionsFunc(func() []huh.Option[string] {
        return huh.NewOptions(statesFor(country)...)
    }, &country).  // re-evaluates when &country changes
    Value(&state)
```

**Form-level options:** `WithTimeout(d)`, `WithLayout(huh.LayoutColumns(n))`, `WithWidth(w)`, `WithHeight(h)`, `WithKeyMap(km)`, `WithAccessible(bool)`, `WithShowHelp(bool)`, `WithProgramOptions(opts...)`

**Form data access by key:** Set `.Key("name")` on fields, then `form.GetString("name")`, `form.GetInt("level")`, `form.GetBool("confirmed")` after submission.

Huh v2 specifics:
- Themes take `isDark bool`: `huh.ThemeCharm(isDark)`, `huh.ThemeBase16(isDark)`, `huh.ThemeDracula(isDark)`
- Always use `form.RunWithContext(cmd.Context())` for forms in Cobra commands
- Standalone fields use `.Run()` directly (no context variant — keep prompts short)
- Spinner: `ActionWithErr(func(ctx context.Context) error)` for cancellable operations
- Use `WithViewHook` for Bubble Tea integration
- Does NOT auto-detect non-TTY — caller must check with `isatty` and fall back to flag-only or accessible mode

### View Routing (Multiple Screens)
```go
type appState int
const (stateList appState = iota; stateDetail; stateModal)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch m.state {
    case stateList: return m.updateList(msg)
    case stateDetail: return m.updateDetail(msg)
    }
    return m, nil
}
```

### Logging
**Never write to stdout** — corrupts display:
```go
f, _ := tea.LogToFile("debug.log", "myapp")
defer f.Close()
```

### Error Handling
Store in model, display in View:
```go
case errMsg: m.err = msg.err; return m, nil
// In View: if m.err != nil { return tea.NewView(errorStyle.Render(m.err.Error())) }
```

### Testing
```go
import "github.com/charmbracelet/x/exp/teatest"

tm := teatest.NewTestModel(t, initialModel())
tm.Send(tea.KeyPressMsg{Key: tea.KeyEnter})  // KeyPressMsg not KeyMsg
tm.Type("hello")
teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
    return bytes.Contains(b, []byte("hello"))
})
```

### Performance
- Keep Update()/View() fast — offload to commands
- Return `nil` cmd when no async work needed
- Use virtual scrolling for large datasets: `items[offset:offset+visibleRows]`
- Batch related commands: `tea.Batch(cmd1, cmd2, cmd3)`
- Only one renderer (Cursed Renderer) — no HighPerformanceRendering option

### Other Charm Tools
- **Wish**: SSH apps serving Bubble Tea
- **Harmonica**: Spring animations
- **Gum**: Shell script TUI (`gum input`, `gum choose`, `gum confirm`)
- **Glamour**: Markdown rendering

### Known Issues and Gotchas

**tmux:** `tea.RequestBackgroundColor` silently fails — message never arrives. Workaround: force dark/light mode or use `lipgloss.HasDarkBackground()` (blocking). Key release events and keyboard enhancements also don't work in tmux v3.5.

**Window titles** set via View persist after program exit — not cleaned up on shutdown or panic (known open issue).

**Data race during shutdown** between `cancelreader.Close()` and `cancelreader.wait()`, detectable with `-race` flag.

### Common Mistakes
- Never use raw goroutines — use `tea.Cmd`
- Never mutate model outside Update — return new model
- Never log to stdout — use `tea.LogToFile()`
- Never use `tea.KeyMsg` in type switch — use `tea.KeyPressMsg` (KeyMsg is an interface in v2)
- Never use `" "` for space bar — use `"space"` in v2
- Never use `tea.WithAltScreen()` or `tea.WithMouseCellMotion()` — set `v.AltScreen`/`v.MouseMode` in View()
- Never use `lipgloss.AdaptiveColor` — use `lipgloss.LightDark(isDark)` with `tea.BackgroundColorMsg`
- Never use `lipgloss.Color("string")` as a type — `lipgloss.Color()` is now a function returning `color.Color`
- Never use `HighPerformanceRendering` — removed in v2
- Never use package-level `spinner.Tick()` — use `model.Tick()` method
- Never use `compat.AdaptiveColor` in Wish/SSH contexts — blocking I/O + global state. Use `tea.RequestBackgroundColor` inside Bubble Tea
- Always request background color in `Init()` via `tea.RequestBackgroundColor` to trigger `BackgroundColorMsg`
- Always handle `tea.WindowSizeMsg` for responsive layout — account for frame size: `h, v := style.GetFrameSize()`
- Always handle `tea.BackgroundColorMsg` to set `isDark` and propagate to all component styles
- Always return child Init() commands from your Init()
- Always track focus to route input to correct component
- Always check TTY before running Huh forms — it does not auto-detect non-TTY
- Always use `form.RunWithContext(cmd.Context())` for Huh forms in Cobra commands
