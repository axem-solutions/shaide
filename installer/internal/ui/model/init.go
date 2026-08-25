package model

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		func() tea.Msg {
			return textinput.Blink()
		},
		messages.WaitForLog(m.LogCh),
	)
}
