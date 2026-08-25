package model

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/axem-solutions/ai_platform/installer/internal/ui/events"

	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+left":
		m.LogViewport.ScrollLeft(12)
		return m, nil
	case "ctrl+right":
		m.LogViewport.ScrollRight(12)
		return m, nil
	case "ctrl+y":
		return m.saveLogsToStorage()
	}

	switch m.Mode {
	case ModeInput:
		return m.handleInputKey(msg)
	case ModeSelect:
		return m.handleSelectKey(msg)
	case ModeMultiSelect:
		return m.handleMultiSelectKey(msg)
	case ModeDone:
		return m.handleDoneKey(msg)
	default:
		return m.handleIdleKey(msg)
	}
}

func (m Model) handleIdleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	}

	return m.updateComponents(msg)
}

func (m Model) handleDoneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "ctrl+c", "esc":
		return m, tea.Quit
	}

	return m.updateComponents(msg)
}

func (m Model) handleInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.sendPromptReply(messages.PromptReply{
			Err: events.ErrPromptCancelled,
		})

		m = m.clearPrompt()
		return m, nil

	case "enter":
		m.sendPromptReply(messages.PromptReply{
			Values: []string{m.Input.Value()},
		})

		m = m.clearPrompt()
		return m, nil
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

func (m Model) handleSelectKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.sendPromptReply(messages.PromptReply{
			Err: events.ErrPromptCancelled,
		})

		m = m.clearPrompt()
		return m, nil

	case "enter":
		selected := m.List.SelectedItem()

		item, ok := selected.(selectItem)
		if !ok {
			m.sendPromptReply(messages.PromptReply{
				Err: events.ErrPromptCancelled,
			})
		} else {
			m.sendPromptReply(messages.PromptReply{
				Values: []string{string(item)},
			})
		}

		m = m.clearPrompt()
		return m, nil
	}

	var cmd tea.Cmd
	m.List, cmd = m.List.Update(msg)
	return m, cmd
}

func (m Model) handleMultiSelectKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.sendPromptReply(messages.PromptReply{
			Err: events.ErrPromptCancelled,
		})

		m = m.clearPrompt()
		return m, nil

	case "enter":
		if !m.List.SettingFilter() {
			m.sendPromptReply(messages.PromptReply{
				Values: selectedMultiSelectValues(m.List.Items()),
			})

			m = m.clearPrompt()
			return m, nil
		}

	case " ", "space":
		if !m.List.SettingFilter() {
			return m.toggleMultiSelectItem()
		}
	}

	var cmd tea.Cmd
	m.List, cmd = m.List.Update(msg)
	return m, cmd
}

func (m Model) toggleMultiSelectItem() (tea.Model, tea.Cmd) {
	selected := m.List.SelectedItem()

	item, ok := selected.(multiSelectItem)
	if !ok {
		return m, nil
	}

	item.Selected = !item.Selected
	cmd := m.List.SetItem(m.List.GlobalIndex(), item)

	return m, cmd
}

func selectedMultiSelectValues(items []list.Item) []string {
	values := make([]string, 0, len(items))
	for _, listItem := range items {
		item, ok := listItem.(multiSelectItem)
		if !ok || !item.Selected {
			continue
		}

		values = append(values, item.Value)
	}

	return values
}
