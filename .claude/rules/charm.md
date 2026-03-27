---
paths: "**/*.go"
---

# Charm (Bubble Tea) TUI Rules

### Elm Architecture
```go
type Model interface {
    Init() Cmd                    // Initial command (or nil)
    Update(Msg) (Model, Cmd)      // Handle messages, return new state
    View() string                 // Pure render function
}
```
**Never modify state outside Update()**. View must be pure function of model.

### Message Handling
```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c": return m, tea.Quit
        }
    case tea.WindowSizeMsg:  // Sent at startup + resize
        m.width, m.height = msg.Width, msg.Height
    case errMsg:
        m.err = msg.err
    }
    return m, nil
}
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
- `tea.Quit`, `tea.ClearScreen`, `tea.EnterAltScreen`, `tea.ExitAltScreen`

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

### Lip Gloss Styling
```go
style := lipgloss.NewStyle().
    Bold(true).Foreground(lipgloss.Color("#FAFAFA")).
    Padding(1, 2).Border(lipgloss.RoundedBorder()).Width(40)
output := style.Render("Hello")

// Adaptive colors (light/dark terminals)
color := lipgloss.AdaptiveColor{Light: "#000", Dark: "#FFF"}

// Layout
lipgloss.JoinHorizontal(lipgloss.Top, a, b)
lipgloss.JoinVertical(lipgloss.Left, header, content)
lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, content)
w, h := lipgloss.Size(block)
```

### Bubbles Components

| Component | Usage |
|-----------|-------|
| `textinput` | Single-line input, `.Focus()`, `.Placeholder`, `.EchoMode` |
| `textarea` | Multi-line input |
| `list` | Items implement `Title()`, `Description()`, `FilterValue()` |
| `table` | `WithColumns()`, `WithRows()`, `WithFocused(true)` |
| `viewport` | Scrollable content, `.SetContent()`, `.MouseWheelEnabled` |
| `spinner` | `.Spinner = spinner.Dot`, return `s.Tick` from Init() |
| `progress` | `.SetPercent()` returns Cmd, or `.ViewAs()` for static |
| `filepicker` | `.CurrentDirectory`, `.AllowedTypes` |
| `help` | KeyMap with `ShortHelp()`, `FullHelp()` |

### Key Bindings
```go
import "github.com/charmbracelet/bubbles/key"

keys := struct{ Up, Quit key.Binding }{
    Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
    Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
// In Update: if key.Matches(msg, keys.Up) { ... }
```

### Huh Forms
```go
var name string
form := huh.NewForm(huh.NewGroup(
    huh.NewInput().Title("Name").Value(&name).Validate(func(s string) error {
        if len(s) < 3 { return errors.New("too short") }
        return nil
    }),
))
err := form.Run()
```
Fields: `NewInput()`, `NewText()`, `NewSelect[T]()`, `NewMultiSelect[T]()`, `NewConfirm()`

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

### Program Options
```go
p := tea.NewProgram(model{},
    tea.WithAltScreen(),        // Full-screen
    tea.WithMouseCellMotion(),  // Mouse support
)
// Send external messages: go func() { p.Send(myMsg{}) }()
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
// In View: if m.err != nil { return errorStyle.Render(m.err.Error()) }
```

### Testing
```go
import "github.com/charmbracelet/x/exp/teatest"

tm := teatest.NewTestModel(t, initialModel())
tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
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

### Mouse Support
```go
case tea.MouseMsg:
    if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease {
        // Click at msg.X, msg.Y
    }
```
Use `bubblezone` for named click regions.

### Other Charm Tools
- **Wish**: SSH apps serving Bubble Tea
- **Harmonica**: Spring animations
- **Gum**: Shell script TUI (`gum input`, `gum choose`, `gum confirm`)
- **Glamour**: Markdown rendering

### Common Mistakes
- Never use raw goroutines — use `tea.Cmd`
- Never mutate model outside Update — return new model
- Never log to stdout — use `tea.LogToFile()`
- Always handle WindowSizeMsg for responsive layout
- Always return child Init() commands from your Init()
- Always track focus to route input to correct component
