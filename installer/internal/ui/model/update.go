package model

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

var ErrPromptAlreadyActive = errors.New("another prompt is already active")
var ErrNoSelectOptions = errors.New("select prompt has no options")

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m = m.applyTerminalBackground(msg.IsDark())
		return m, nil

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.resizeComponents()
		return m, nil

	case messages.LogMessage:
		if !msg.OK {
			return m, nil
		}

		for _, entry := range msg.Entries {
			m.appendLog(entry)
		}
		return m, messages.WaitForLog(m.LogCh)

	case logsSavedMsg:
		m = m.handleLogsSaved(msg)
		return m, nil

	case messages.PromptInputMessage:
		if m.ReplyCh != nil {
			msg.ReplyCh <- messages.PromptReply{
				Err: ErrPromptAlreadyActive,
			}
			return m, nil
		}

		m = m.startInputPrompt(msg)
		return m, nil

	case messages.PromptSelectMessage:
		if m.ReplyCh != nil {
			msg.ReplyCh <- messages.PromptReply{Err: ErrPromptAlreadyActive}
			return m, nil
		}

		if len(msg.Options) == 0 {
			msg.ReplyCh <- messages.PromptReply{Err: ErrNoSelectOptions}
			return m, nil
		}

		m = m.startSelectPrompt(msg)
		return m, nil

	case messages.PromptMultiSelectMessage:
		if m.ReplyCh != nil {
			msg.ReplyCh <- messages.PromptReply{Err: ErrPromptAlreadyActive}
			return m, nil
		}

		if len(msg.Options) == 0 {
			msg.ReplyCh <- messages.PromptReply{Err: ErrNoSelectOptions}
			return m, nil
		}

		m = m.startMultiSelectPrompt(msg)
		return m, nil

	case messages.WorkflowDoneMessage:
		m.Done = true
		m.Err = msg.Err
		m.Mode = ModeDone
		m = m.clearProgress()

		if msg.Err != nil {
			m.appendLog(messages.LogEntry{
				Line: workflowErrorLogLine(msg.Err),
			})
		}

		return m, nil

	case messages.ModelProgressMessage:
		if msg.Done {
			m = m.clearProgress()
			return m, nil
		}

		if m.ReplyCh != nil {
			return m, nil
		}

		m.ProgressActive = true
		m.ProgressID = msg.ID
		m.ProgressPercent = msg.Percent
		m.ProgressBytes = msg.Bytes
		m.ProgressTotal = msg.TotalBytes

		m.Progress.SetWidth(m.leftContentWidth())

		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m.updateComponents(msg)
}
