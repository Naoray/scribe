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

`View()` returns a `tea.View` struct. Set fields on it for alt screen, cursor, mouse, etc:
```go
func (m model) View() tea.View {
    var v tea.View
    v.Body = m.renderContent()       // string content
    v.AltScreen = true               // replaces tea.WithAltScreen()
    v.MouseCellMotion = true         // replaces tea.WithMouseCellMotion()
    v.Cursor = tea.NewCursor(m.x, m.y) // declarative cursor positioning
    return v
}
```

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
    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
    case tea.BackgroundColorMsg:       // detect light/dark terminal
        m.isDark = msg.IsDark()
    case tea.InterruptMsg:             // SIGINT handling
        return m, tea.Quit
    case tea.PasteMsg:                 // paste events (was KeyMsg.Paste)
        m.input += string(msg)
    case errMsg:
        m.err = msg.err
    }
    return m, nil
}
```

### Mouse Messages
Mouse is now split into specific message types (no generic `tea.MouseMsg` struct):
```go
case tea.MouseClickMsg:
    // msg.X, msg.Y, msg.Button
case tea.MouseReleaseMsg:
    // click ended
case tea.MouseWheelMsg:
    // msg.Delta
case tea.MouseMotionMsg:
    // mouse moved
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
- `tea.SetClipboard("text")` / receive `tea.ClipboardMsg`
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
p := tea.NewProgram(model{})
// Send external messages: go func() { p.Send(myMsg{}) }()
```
Alt screen, mouse, etc. are now set via `tea.View` fields in `View()`, not program options.

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

// Compositing/layering
layer := lipgloss.NewLayer()
canvas := lipgloss.NewCanvas()

// Standalone output (outside Bubble Tea)
lipgloss.Println("styled output")   // handles color downsampling
lipgloss.Printf("hello %s", name)
```
Inside Bubble Tea, the Cursed Renderer handles color downsampling automatically.

### Bubbles v2 Components

| Component | Usage |
|-----------|-------|
| `textinput` | `.Focus()`, `.Placeholder`, `.EchoMode`, styles via `.Styles.Focused`/`.Styles.Blurred` |
| `textarea` | Multi-line, styles via `.Styles.Focused`/`.Styles.Blurred` |
| `list` | Items implement `Title()`, `Description()`, `FilterValue()` |
| `table` | `WithColumns()`, `WithRows()`, `WithFocused(true)` |
| `viewport` | Functional options constructor, soft wrapping, search, horizontal scroll |
| `spinner` | `.Spinner = spinner.Dot`, use `s.Tick()` method (not package-level `Tick()`) |
| `progress` | `.SetPercent()` returns Cmd, colors via `lipgloss.Color()` not raw strings |
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
```go
import "charm.land/huh/v2"

var name string
form := huh.NewForm(huh.NewGroup(
    huh.NewInput().Title("Name").Value(&name).Validate(func(s string) error {
        if len(s) < 3 { return errors.New("too short") }
        return nil
    }),
))
form.WithAccessible(true)  // accessible mode is form-level only (per-field removed)
err := form.Run()
```
Fields: `NewInput()`, `NewText()`, `NewSelect[T]()`, `NewMultiSelect[T]()`, `NewConfirm()`

Huh v2 specifics:
- Themes take `isDark bool`: `huh.ThemeCharm(false)`
- Spinner action: `ActionWithErr` takes `func(ctx context.Context) error`
- Use `WithViewHook` for Bubble Tea integration
- Does NOT auto-detect non-TTY — caller must check and set accessible mode

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
// In View: if m.err != nil { v.Body = errorStyle.Render(m.err.Error()); return v }
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

### Common Mistakes
- Never use raw goroutines — use `tea.Cmd`
- Never mutate model outside Update — return new model
- Never log to stdout — use `tea.LogToFile()`
- Never use `tea.KeyMsg` in type switch — use `tea.KeyPressMsg` (KeyMsg is an interface in v2)
- Never use `" "` for space bar — use `"space"` in v2
- Never use `tea.WithAltScreen()` or `tea.WithMouseCellMotion()` — set `v.AltScreen`/`v.MouseCellMotion` in View()
- Never use `lipgloss.AdaptiveColor` — use `lipgloss.LightDark(isDark)` with `tea.BackgroundColorMsg`
- Never use `lipgloss.Color("string")` as a type — `lipgloss.Color()` is now a function returning `color.Color`
- Never use `HighPerformanceRendering` — removed in v2
- Never use package-level `spinner.Tick()` — use `model.Tick()` method
- Always handle `tea.WindowSizeMsg` for responsive layout
- Always handle `tea.BackgroundColorMsg` to support light/dark themes
- Always return child Init() commands from your Init()
- Always track focus to route input to correct component
- Always check TTY before running Huh forms — it does not auto-detect non-TTY
