package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"github.com/Naoray/scribe/internal/sync"
)

// SkillState tracks the display state of a single skill row.
type SkillState int

const (
	SkillPending     SkillState = iota
	SkillDownloading
	SkillInstalled
	SkillSkipped
	SkillFailed
)

// SkillRow is a single row in the sync progress display.
type SkillRow struct {
	Name    string
	State   SkillState
	Version string
	Targets string
	Error   string
}

// SyncProgress is a Bubble Tea model that displays real-time sync progress.
type SyncProgress struct {
	Registry string
	Skills   []SkillRow
	Summary  sync.SyncCompleteMsg
	Done     bool
	spinner  spinner.Model
}

// NewSyncProgress creates a new sync progress model for the given registry.
func NewSyncProgress(registry string) SyncProgress {
	s := spinner.New(spinner.WithSpinner(spinner.Dot))
	return SyncProgress{
		Registry: registry,
		spinner:  s,
	}
}

func (m SyncProgress) Init() tea.Cmd {
	return m.spinner.Tick
}

// findSkill returns a pointer to the SkillRow with the given name, or nil.
func (m *SyncProgress) findSkill(name string) *SkillRow {
	for i := range m.Skills {
		if m.Skills[i].Name == name {
			return &m.Skills[i]
		}
	}
	return nil
}

func (m SyncProgress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case tea.InterruptMsg:
		return m, tea.Quit

	case sync.SkillResolvedMsg:
		m.Skills = append(m.Skills, SkillRow{
			Name:  msg.Name,
			State: SkillPending,
		})

	case sync.SkillDownloadingMsg:
		if sk := m.findSkill(msg.Name); sk != nil {
			sk.State = SkillDownloading
		}

	case sync.SkillInstalledMsg:
		if sk := m.findSkill(msg.Name); sk != nil {
			sk.State = SkillInstalled
			sk.Version = msg.Version
		}

	case sync.SkillSkippedMsg:
		if sk := m.findSkill(msg.Name); sk != nil {
			sk.State = SkillSkipped
		}

	case sync.SkillErrorMsg:
		if sk := m.findSkill(msg.Name); sk != nil {
			sk.State = SkillFailed
			sk.Error = msg.Err.Error()
		} else {
			m.Skills = append(m.Skills, SkillRow{
				Name:  msg.Name,
				State: SkillFailed,
				Error: msg.Err.Error(),
			})
		}

	case sync.SyncCompleteMsg:
		m.Summary = msg
		m.Done = true
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m SyncProgress) View() tea.View {
	var b strings.Builder

	b.WriteString(Title.Render(fmt.Sprintf("Syncing %s", m.Registry)))
	b.WriteString("\n\n")

	installed := 0
	total := len(m.Skills)

	for _, sk := range m.Skills {
		var icon, detail string
		switch sk.State {
		case SkillPending:
			icon = CheckPending.Render("○")
			detail = Subtle.Render("pending")
		case SkillDownloading:
			icon = m.spinner.View()
			detail = Subtle.Render("downloading...")
		case SkillInstalled:
			icon = CheckOK.Render("✓")
			detail = sk.Version
			installed++
		case SkillSkipped:
			icon = Subtle.Render("–")
			detail = Subtle.Render("current")
			installed++
		case SkillFailed:
			icon = CheckFail.Render("✗")
			detail = CheckFail.Render(sk.Error)
		}

		b.WriteString(fmt.Sprintf("  %s %-20s %s\n", icon, sk.Name, detail))
	}

	if total > 0 {
		b.WriteString(fmt.Sprintf("\n  %d/%d skills processed\n", installed, total))
	}

	return tea.NewView(b.String())
}
