package model

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	minLeftWidth       = 32
	preferredLeftWidth = 42
	minRightWidth      = 40
	minLogWidth        = 24
	panelGap           = 1
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

func (m Model) leftPanelWidth() int {
	preferred := preferredLeftWidth

	if titleWidth := m.promptTitlePanelWidth(); titleWidth > preferred {
		preferred = titleWidth
	}

	if (m.Mode == ModeSelect || m.Mode == ModeMultiSelect) && m.SelectWidth > 0 {
		preferred = maxInt(preferred, m.SelectWidth+6)
	}

	if progressWidth := m.progressPanelWidth(); progressWidth > preferred {
		preferred = progressWidth
	}

	if m.Width <= 0 {
		return preferred
	}

	maxLeft := m.Width - minLogWidth - panelGap
	if maxLeft < minLeftWidth {
		maxLeft = m.Width - panelGap
	}
	if maxLeft < minLeftWidth {
		return minLeftWidth
	}

	if preferred > maxLeft {
		return maxLeft
	}
	if (m.Mode == ModeSelect || m.Mode == ModeMultiSelect) && m.SelectWidth > 0 {
		return preferred
	}

	if m.Width < preferredLeftWidth+minRightWidth+panelGap {
		w := m.Width / 2
		if w < minLeftWidth {
			return minLeftWidth
		}
		return w
	}

	return preferred
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

func (m Model) promptTitlePanelWidth() int {
	if m.PromptTitle == "" {
		return 0
	}

	return lipgloss.Width(m.PromptTitle) + leftPanelStyle.GetHorizontalFrameSize()
}

func (m Model) progressPanelWidth() int {
	if !m.ProgressActive {
		return 0
	}

	status, detail := progressLabelParts(m.ProgressID)
	headerWidth := lipgloss.Width(m.progressStatusText(status)) + 1 + lipgloss.Width(progressPercentText(m.ProgressPercent))
	detailWidth := lipgloss.Width(detail)
	contentWidth := maxInt(headerWidth, detailWidth)
	if contentWidth == 0 {
		return 0
	}

	return contentWidth + leftPanelStyle.GetHorizontalFrameSize()
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
