package model

import "charm.land/lipgloss/v2"

var (
	leftPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 1)

	rightPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	progressDetailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0"))

	progressPercentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46"))
)
