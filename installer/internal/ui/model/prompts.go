package model

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

func (m Model) startInputPrompt(msg messages.PromptInputMessage) Model {
	m = m.clearProgress()
	m.Mode = ModeInput
	m.ReplyCh = msg.ReplyCh
	m.PromptTitle = msg.Title
	m.PromptPlaceholder = msg.Placeholder

	m.Input.SetValue(msg.DefaultValue)
	m.Input.Placeholder = msg.Placeholder
	m.Input.CursorEnd()
	m.Input.Focus()
	m.resizeComponents()

	return m
}

func (m Model) startMultiSelectPrompt(msg messages.PromptMultiSelectMessage) Model {
	m = m.clearProgress()

	items := make([]list.Item, 0, len(msg.Options))
	for _, option := range msg.Options {
		items = append(items, multiSelectItem{Value: option})
	}

	delegate := newPromptListDelegate(m.HasDarkBackground)
	l := list.New(items, delegate, m.selectListWidth(), m.selectListHeight())
	configurePromptList(&l, msg.Title, m.HasDarkBackground)
	l.AdditionalShortHelpKeys = multiSelectShortHelpKeys
	l.AdditionalFullHelpKeys = multiSelectFullHelpKeys

	m.Mode = ModeMultiSelect
	m.ReplyCh = msg.ReplyCh
	m.PromptTitle = msg.Title
	m.SelectWidth = multiSelectPromptWidth(msg.Options)
	m.Input.Blur()
	l.SetSize(m.selectListWidth(), m.selectListHeight())
	m.List = l
	m.resizeComponents()

	return m
}

func (m Model) startSelectPrompt(msg messages.PromptSelectMessage) Model {
	m = m.clearProgress()

	items := make([]list.Item, 0, len(msg.Options))
	for _, option := range msg.Options {
		items = append(items, selectItem(option))
	}

	delegate := newPromptListDelegate(m.HasDarkBackground)
	l := list.New(items, delegate, m.selectListWidth(), m.selectListHeight())
	configurePromptList(&l, msg.Title, m.HasDarkBackground)

	for i, option := range msg.Options {
		if option == msg.Current {
			l.Select(i)
			break
		}
	}

	m.Mode = ModeSelect
	m.ReplyCh = msg.ReplyCh
	m.PromptTitle = msg.Title
	m.SelectWidth = selectPromptWidth(msg.Options)
	m.Input.Blur()
	l.SetSize(m.selectListWidth(), m.selectListHeight())
	m.List = l
	m.resizeComponents()

	return m
}

func (m Model) applyTerminalBackground(isDark bool) Model {
	m.HasDarkBackground = isDark
	m.Input.SetStyles(textinput.DefaultStyles(isDark))

	if m.Mode == ModeSelect || m.Mode == ModeMultiSelect {
		applyPromptListStyles(&m.List, isDark)
	}

	return m
}

func newPromptListDelegate(isDark bool) list.DefaultDelegate {
	lightDark := lipgloss.LightDark(isDark)
	textColor := lightDark(lipgloss.Color("#000000"), lipgloss.Color("#FFFFFF"))
	dimmedTextColor := lightDark(lipgloss.Color("#4B5563"), lipgloss.Color("#D1D5DB"))

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(textColor).
		Padding(0, 1, 0, 1)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Bold(true).
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#F7C948")).
		BorderForeground(lipgloss.Color("#000000")).
		Padding(0, 1, 0, 1)
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.
		Foreground(dimmedTextColor).
		Padding(0, 1, 0, 1)
	delegate.Styles.FilterMatch = delegate.Styles.FilterMatch.
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#FDE68A"))

	return delegate
}

func configurePromptList(l *list.Model, title string, isDark bool) {
	applyPromptListStyles(l, isDark)
	l.Title = title
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
}

func applyPromptListStyles(l *list.Model, isDark bool) {
	styles := list.DefaultStyles(isDark)
	l.Styles = styles
	l.FilterInput.SetStyles(styles.Filter)
	l.Help.Styles = help.DefaultStyles(isDark)
	l.SetDelegate(newPromptListDelegate(isDark))
}

func (m Model) selectListWidth() int {
	w := m.leftContentWidth() - 2
	if w < 10 {
		return 10
	}

	return w
}

func selectPromptWidth(options []string) int {
	width := 0
	for _, option := range options {
		if optionWidth := lipgloss.Width(option); optionWidth > width {
			width = optionWidth
		}
	}

	// Account for list item padding and the selected row cursor/border.
	return width + 4
}

func multiSelectPromptWidth(options []string) int {
	return selectPromptWidth(options) + 4
}

func multiSelectShortHelpKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle"),
		),
		key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
	}
}

func multiSelectFullHelpKeys() []key.Binding {
	return multiSelectShortHelpKeys()
}

func (m *Model) sendPromptReply(reply messages.PromptReply) {
	if m.ReplyCh == nil {
		return
	}

	m.ReplyCh <- reply
}

func (m Model) clearPrompt() Model {
	m.Mode = ModeIdle
	m.ReplyCh = nil
	m.PromptTitle = ""
	m.PromptPlaceholder = ""
	m.SelectWidth = 0
	m.Input.SetValue("")
	m.Input.Blur()
	m.resizeComponents()

	return m
}

func (m Model) clearProgress() Model {
	m.ProgressActive = false
	m.ProgressID = ""
	m.ProgressPercent = 0
	m.ProgressBytes = 0
	m.ProgressTotal = 0

	return m
}

type selectItem string

func (i selectItem) FilterValue() string {
	return string(i)
}

func (i selectItem) Title() string {
	return string(i)
}

func (i selectItem) Description() string {
	return ""
}

type multiSelectItem struct {
	Value    string
	Selected bool
}

func (i multiSelectItem) FilterValue() string {
	return i.Value
}

func (i multiSelectItem) Title() string {
	if i.Selected {
		return "[x] " + i.Value
	}

	return "[ ] " + i.Value
}

func (i multiSelectItem) Description() string {
	return ""
}
