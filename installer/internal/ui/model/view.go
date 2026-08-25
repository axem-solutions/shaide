package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		v := tea.NewView("initialising...")
		v.AltScreen = true
		return v
	}

	left := m.renderLeftPanel()
	right := m.renderRightPanel()

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		left,
		strings.Repeat(" ", panelGap),
		right,
	)

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	return v
}
