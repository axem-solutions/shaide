package model

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	minLeftWidth = 44
	minLogWidth  = 24
	panelGap     = 1
)

func (m *Model) resizeComponents() {
	m.Input.SetWidth(m.inputWidth())
	m.Progress.SetWidth(m.leftContentWidth())

	xOffset := m.LogViewport.XOffset()
	m.LogViewport = newLogViewport(m.logViewportWidth(), m.logViewportHeight())
	m.LogViewport.SetContent(strings.Join(m.Logs, "\n"))
	m.LogViewport.SetXOffset(xOffset)
	m.LogViewport.GotoBottom()

	if m.Mode == ModeSelect || m.Mode == ModeMultiSelect {
		m.List.SetSize(m.selectListWidth(), m.selectListHeight())
	}
}

// leftPanelWidth gives the prompt panel a third of the terminal and the logs
// the rest. Below minLeftWidth the prompts stop being usable, so the logs
// shrink first.
func (m Model) leftPanelWidth() int {
	if m.Width <= 0 {
		return minLeftWidth
	}

	maxLeft := m.Width - minLogWidth - panelGap
	if maxLeft < minLeftWidth {
		maxLeft = m.Width - panelGap
	}
	if maxLeft < minLeftWidth {
		return minLeftWidth
	}

	return minInt(maxInt(m.Width/3, minLeftWidth), maxLeft)
}

func (m Model) rightPanelWidth() int {
	w := m.Width - m.leftPanelWidth() - panelGap
	if w < minLogWidth {
		return minLogWidth
	}

	return w
}

func (m Model) panelHeight() int {
	if m.Height <= 0 {
		return 0
	}

	return m.Height - 1
}

func (m Model) leftContentWidth() int {
	w := m.leftPanelWidth() - leftPanelStyle.GetHorizontalFrameSize()
	if w < 10 {
		return 10
	}

	return w
}

func (m Model) leftContentHeight() int {
	h := m.panelHeight() - leftPanelStyle.GetVerticalFrameSize()
	if h < 5 {
		return 5
	}

	return h
}

func (m Model) inputWidth() int {
	return m.leftContentWidth()
}

func (m Model) promptTitleHeight() int {
	if m.PromptTitle == "" {
		return 0
	}

	return lipgloss.Height(m.renderPromptTitle())
}

func (m Model) selectListHeight() int {
	h := m.leftContentHeight()
	if m.PromptTitle != "" {
		h -= m.promptTitleHeight() + 1
	}
	if promptHelpHeight := m.promptHelpHeight(); promptHelpHeight > 0 {
		h -= promptHelpHeight + 1
	}
	if h < 3 {
		return 3
	}

	return h
}

func (m Model) promptHelpHeight() int {
	if m.promptHelpText() == "" {
		return 0
	}

	return lipgloss.Height(m.renderPromptHelp())
}

func (m Model) logViewportWidth() int {
	w := m.rightPanelWidth() - rightPanelStyle.GetHorizontalFrameSize()
	if w < 10 {
		return 10
	}

	return w
}

func (m Model) logViewportHeight() int {
	h := m.panelHeight() - rightPanelStyle.GetVerticalFrameSize() - 2
	if h < 3 {
		return 3
	}

	return h
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
