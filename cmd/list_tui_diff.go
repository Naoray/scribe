package cmd

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func newDiffViewport(width, height int) viewport.Model {
	v := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	v.SoftWrap = false
	v.FillHeight = false
	return v
}

func redistributeViewportHeights(m *listModel, paneHeight int) {
	if m.height < 20 {
		m.viewYours.SetHeight(1)
		m.viewIncoming.SetHeight(1)
		return
	}
	if m.height < 28 {
		h := paneHeight - 8
		if h < 3 {
			h = 3
		}
		if h > 16 {
			h = 16
		}
		m.viewYours.SetHeight(h)
		m.viewIncoming.SetHeight(h)
		return
	}
	h := paneHeight / 2
	if h > 12 {
		h = 12
	}
	if h < 3 {
		h = 3
	}
	m.viewYours.SetHeight(h)
	m.viewIncoming.SetHeight(h)
}

func forwardScrollKey(m *listModel, msg tea.KeyPressMsg) tea.Cmd {
	target := &m.viewYours
	if m.activeViewport == viewportIncoming {
		target = &m.viewIncoming
	}
	switch msg.String() {
	case "j", "down":
		target.ScrollDown(1)
	case "k", "up":
		target.ScrollUp(1)
	case "pgdown", "pagedown", "d":
		target.ScrollDown(target.Height())
	case "pgup", "pageup", "u":
		target.ScrollUp(target.Height())
	}
	return nil
}

func (m listModel) renderUpdateChoiceDetail(width int) string {
	var b strings.Builder
	b.WriteString(ltHeaderStyle.Render("Update diff") + "\n")
	if m.updatePreview.loading {
		b.WriteString(ltDimStyle.Render(spinnerFrames[m.spinnerFrame]+" Fetching upstream…") + "\n")
	}
	if m.updatePreview.err != nil {
		b.WriteString(ltErrorStyle.Render("Could not reach registry — diff unavailable. [l] keep local, [esc] cancel.") + "\n")
	}
	paneHeight := m.contentHeight()
	contentWidth := width - 2
	if contentWidth < 20 {
		contentWidth = 20
	}
	m.viewYours.SetWidth(contentWidth)
	m.viewIncoming.SetWidth(contentWidth)
	redistributeViewportHeights(&m, paneHeight)
	switch {
	case m.height >= 28:
		b.WriteString(m.renderDiffViewport("Your edits", m.viewYours, m.updatePreview.diffYours) + "\n")
		b.WriteString(m.renderDiffViewport("Incoming", m.viewIncoming, m.updatePreview.diffIncoming) + "\n")
	case m.height >= 20:
		combined := combineDiffs(m.updatePreview.diffYours, m.updatePreview.diffIncoming)
		v := newDiffViewport(contentWidth, m.viewYours.Height())
		v.SetContent(combined)
		b.WriteString(m.renderDiffViewport("Diff", v, combined) + "\n")
	case m.height >= 12:
		yAdd, yDel := diffStats(m.updatePreview.diffYours)
		iAdd, iDel := diffStats(m.updatePreview.diffIncoming)
		b.WriteString(ltDimStyle.Render(fmt.Sprintf("+%d −%d local · +%d −%d incoming", yAdd, yDel, iAdd, iDel)) + "\n")
	default:
		b.WriteString(ltDimStyle.Render("(diff hidden — terminal too small)") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.renderUpdateChoiceBlock())
	return b.String()
}

func (m listModel) renderDiffViewport(title string, v viewport.Model, content string) string {
	var b strings.Builder
	b.WriteString(ltDimStyle.Render(title) + "\n")
	if strings.TrimSpace(content) == "" {
		b.WriteString(ltDimStyle.Render("(no changes)") + "\n")
		return b.String()
	}
	b.WriteString(v.View())
	return b.String()
}

func combineDiffs(yours, incoming string) string {
	parts := []string{"Your edits", strings.TrimSpace(yours), "", "Incoming", strings.TrimSpace(incoming)}
	return strings.Join(parts, "\n")
}

func diffStats(diff string) (additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
}

func (m listModel) renderUpdateChoiceBlock() string {
	if !m.updateHasMods {
		return ltDimStyle.Render("[u] update now") + "\n" + ltDimStyle.Render("[esc] cancel") + "\n"
	}
	return strings.Join([]string{
		ltDimStyle.Render("[m] merge with upstream"),
		ltDimStyle.Render("[r] replace with registry version"),
		ltDimStyle.Render("[l] keep local version"),
		ltDimStyle.Render("[esc] cancel"),
	}, "\n") + "\n"
}
