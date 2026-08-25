package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) updateComponents(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	var logCmd tea.Cmd
	m.LogViewport, logCmd = m.LogViewport.Update(msg)
	cmds = append(cmds, logCmd)

	switch m.Mode {
	case ModeInput:
		var inputCmd tea.Cmd
		m.Input, inputCmd = m.Input.Update(msg)
		cmds = append(cmds, inputCmd)

	case ModeSelect, ModeMultiSelect:
		var listCmd tea.Cmd
		m.List, listCmd = m.List.Update(msg)
		cmds = append(cmds, listCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) renderLeftPanel() string {
	var body string

	switch m.Mode {
	case ModeInput:
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderPromptTitle(),
			"",
			m.Input.View(),
			"",
			m.renderPromptHelp(),
		)

	case ModeSelect, ModeMultiSelect:
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderPromptTitle(),
			"",
			m.List.View(),
			"",
			m.renderPromptHelp(),
		)

	case ModeDone:
		text := "Workflow complete."
		if m.Err != nil {
			text = "Workflow failed."
		}

		body = lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render(text),
			"",
			helpStyle.Render("enter to exit"),
		)

	default:
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("Installer"),
			"",
			helpStyle.Render("waiting for workflow input..."),
			"",
			helpStyle.Render("esc/ctrl+c to quit"),
		)
	}

	if progressLine := m.renderProgressLine(); progressLine != "" {
		progressTop := m.leftContentHeight() - lipgloss.Height(progressLine)
		if progressTop < 0 {
			progressTop = 0
		}
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().
				Height(progressTop).
				Render(body),
			progressLine,
		)
	}

	return leftPanelStyle.
		Width(m.leftPanelWidth()).
		Height(m.panelHeight()).
		Render(body)
}

func (m Model) renderRightPanel() string {
	logView := lipgloss.NewStyle().
		MaxWidth(m.logViewportWidth()).
		Render(m.LogViewport.View())

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderLogHeader(),
		"",
		logView,
	)

	return rightPanelStyle.
		Width(m.rightPanelWidth()).
		Height(m.panelHeight()).
		Render(body)
}

func (m Model) renderLogHeader() string {
	title := titleStyle.Render("Logs")

	action := "ctrl+y save logs"
	if m.LogSaveStatus != "" {
		action = m.LogSaveStatus
	}

	action = helpStyle.Render(action)
	spacerWidth := m.logViewportWidth() - lipgloss.Width(title) - lipgloss.Width(action)
	if spacerWidth < 1 {
		return title
	}

	return title + strings.Repeat(" ", spacerWidth) + action
}

func (m Model) renderProgressLine() string {
	if !m.ProgressActive {
		return ""
	}

	percent := float64(m.ProgressPercent) / 100
	width := m.leftContentWidth()
	status, detail := progressLabelParts(m.ProgressID)
	status = m.progressStatusText(status)

	bar := m.Progress
	bar.ShowPercentage = false
	bar.SetWidth(width)

	lines := []string{
		progressHeader(status, progressPercentText(m.ProgressPercent), width),
	}
	if detail != "" {
		lines = append(lines, progressDetailStyle.Render(detail))
	}
	lines = append(lines, bar.ViewAs(percent))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		lines...,
	)
}

func (m Model) progressStatusText(status string) string {
	if byteLine := m.renderProgressBytes(); byteLine != "" {
		return strings.TrimSpace(status + " " + byteLine)
	}

	return status
}

func progressPercentText(percent int) string {
	return fmt.Sprintf("%3d%%", percent)
}

func (m Model) renderProgressBytes() string {
	if m.ProgressTotal <= 0 || strings.HasPrefix(m.ProgressID, "Building artifact") {
		return ""
	}

	return fmt.Sprintf(
		"%.2f GB / %.2f GB",
		bytesToGB(m.ProgressBytes),
		bytesToGB(m.ProgressTotal),
	)
}

func bytesToGB(bytes int64) float64 {
	const gb = 1000 * 1000 * 1000
	return float64(bytes) / gb
}

func progressLabelParts(label string) (string, string) {
	lines := strings.Split(label, "\n")
	status := strings.TrimSpace(lines[0])
	if len(lines) == 1 {
		return status, ""
	}

	detail := strings.TrimSpace(strings.Join(lines[1:], " "))
	return status, detail
}

func progressHeader(status string, percent string, width int) string {
	percentWidth := lipgloss.Width(percent)
	statusWidth := width - percentWidth
	if statusWidth <= 0 {
		return percent
	}

	status = titleStyle.Render(wrapText(status, statusWidth))
	statusLines := strings.Split(status, "\n")
	lastLine := statusLines[len(statusLines)-1]
	spacerWidth := width - lipgloss.Width(lastLine) - percentWidth
	if spacerWidth < 0 {
		spacerWidth = 0
	}
	statusLines[len(statusLines)-1] = lastLine + strings.Repeat(" ", spacerWidth) + progressPercentStyle.Render(percent)

	return strings.Join(statusLines, "\n")
}

func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(text) <= maxWidth {
		return text
	}

	var lines []string
	var line strings.Builder
	width := 0

	for _, r := range text {
		runeWidth := lipgloss.Width(string(r))
		if width > 0 && width+runeWidth > maxWidth {
			lines = append(lines, line.String())
			line.Reset()
			width = 0
		}

		line.WriteRune(r)
		width += runeWidth
	}

	if line.Len() > 0 {
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderPromptTitle() string {
	return titleStyle.
		Width(m.leftContentWidth()).
		Render(m.PromptTitle)
}

func (m Model) renderPromptHelp() string {
	return helpStyle.
		Width(m.leftContentWidth()).
		Render(m.promptHelpText())
}

func (m Model) promptHelpText() string {
	switch m.Mode {
	case ModeMultiSelect:
		return "space to select, enter to continue, esc to cancel"
	case ModeInput, ModeSelect:
		return "enter to continue, esc to cancel"
	default:
		return ""
	}
}
