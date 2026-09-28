package model

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

// choiceOutcome is what a key press did to the choice prompt as a whole.
type choiceOutcome int

const (
	choicePending choiceOutcome = iota
	choiceSubmitted
	choiceCancelled
)

type choiceKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	HalfDown key.Binding
	HalfUp   key.Binding
	Prev     key.Binding
	Next     key.Binding
	Open     key.Binding
	Reset    key.Binding
	Help     key.Binding
	Cancel   key.Binding
}

func newChoiceKeyMap() choiceKeyMap {
	return choiceKeyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "half page down")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "half page up")),
		Prev:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous option")),
		Next:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next option")),
		Open:     key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter", "open / continue")),
		Reset:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo row")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
		Cancel:   key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel")),
	}
}

func (k choiceKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.Up, k.Next, k.Open, k.Help}
}

func (k choiceKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Down, k.Up, k.Top, k.Bottom, k.HalfDown, k.HalfUp},
		{k.Prev, k.Next, k.Open, k.Reset, k.Cancel, k.Help},
	}
}

// choiceModel is a table whose rows each carry a dropdown of options, for
// example an action per model. The row after the last one is a Continue
// button, which submits the chosen option of every row.
type choiceModel struct {
	columns []string
	rows    []messages.ChoiceRow

	// chosen and initial are option indexes per row. initial is what the row
	// started at, so changed rows can be marked and reset.
	chosen  []int
	initial []int

	// cursor is a row index; len(rows) is the Continue button.
	cursor int
	// offset is the first row shown when the rows do not fit.
	offset int

	dropdownOpen   bool
	dropdownCursor int

	keys     choiceKeyMap
	help     help.Model
	fullHelp bool

	isDark bool
}

func newChoiceModel(msg messages.PromptChoiceMessage, isDark bool) choiceModel {
	c := choiceModel{
		columns: msg.Columns,
		rows:    msg.Rows,
		chosen:  make([]int, len(msg.Rows)),
		initial: make([]int, len(msg.Rows)),
		keys:    newChoiceKeyMap(),
		help:    help.New(),
	}

	for i, row := range msg.Rows {
		for j, option := range row.Options {
			if option == row.Current {
				c.chosen[i] = j
				c.initial[i] = j
				break
			}
		}
	}

	c.setDark(isDark)

	return c
}

func (c *choiceModel) setDark(isDark bool) {
	c.isDark = isDark
	c.help.Styles = help.DefaultStyles(isDark)
}

// values is the reply: the chosen option of every row, in row order.
func (c choiceModel) values() []string {
	values := make([]string, len(c.rows))
	for i, row := range c.rows {
		values[i] = row.Options[c.chosen[i]]
	}

	return values
}

func (c choiceModel) changes() int {
	n := 0
	for i := range c.rows {
		if c.chosen[i] != c.initial[i] {
			n++
		}
	}

	return n
}

func (c choiceModel) onContinue() bool {
	return c.cursor == len(c.rows)
}

// update handles a key press. height is how many lines the view may use, so
// paging and scrolling match what is on screen.
func (c choiceModel) update(msg tea.KeyPressMsg, height int) (choiceModel, choiceOutcome) {
	if c.dropdownOpen {
		return c.updateDropdown(msg), choicePending
	}

	switch {
	case key.Matches(msg, c.keys.Cancel):
		return c, choiceCancelled
	case key.Matches(msg, c.keys.Help):
		c.fullHelp = !c.fullHelp
	case key.Matches(msg, c.keys.Up):
		c.moveCursor(-1)
	case key.Matches(msg, c.keys.Down):
		c.moveCursor(1)
	case key.Matches(msg, c.keys.Top):
		c.cursor = 0
	case key.Matches(msg, c.keys.Bottom):
		c.cursor = len(c.rows)
	case key.Matches(msg, c.keys.HalfDown):
		c.moveCursor(maxInt(c.rowsHeight(height)/2, 1))
	case key.Matches(msg, c.keys.HalfUp):
		c.moveCursor(-maxInt(c.rowsHeight(height)/2, 1))
	case key.Matches(msg, c.keys.Prev):
		c.cycleOption(-1)
	case key.Matches(msg, c.keys.Next):
		c.cycleOption(1)
	case key.Matches(msg, c.keys.Reset):
		if !c.onContinue() {
			c.chosen[c.cursor] = c.initial[c.cursor]
		}
	case key.Matches(msg, c.keys.Open):
		if c.onContinue() {
			return c, choiceSubmitted
		}
		c.dropdownOpen = true
		c.dropdownCursor = c.chosen[c.cursor]
	}

	c.scrollToCursor(height)

	return c, choicePending
}

func (c choiceModel) updateDropdown(msg tea.KeyPressMsg) choiceModel {
	options := c.rows[c.cursor].Options

	switch {
	case key.Matches(msg, c.keys.Cancel):
		c.dropdownOpen = false
	case key.Matches(msg, c.keys.Up):
		c.dropdownCursor = (c.dropdownCursor - 1 + len(options)) % len(options)
	case key.Matches(msg, c.keys.Down):
		c.dropdownCursor = (c.dropdownCursor + 1) % len(options)
	case key.Matches(msg, c.keys.Top):
		c.dropdownCursor = 0
	case key.Matches(msg, c.keys.Bottom):
		c.dropdownCursor = len(options) - 1
	case key.Matches(msg, c.keys.Open):
		c.chosen[c.cursor] = c.dropdownCursor
		c.dropdownOpen = false
	}

	return c
}

func (c *choiceModel) moveCursor(delta int) {
	c.cursor += delta
	if c.cursor < 0 {
		c.cursor = 0
	}
	if c.cursor > len(c.rows) {
		c.cursor = len(c.rows)
	}
}

func (c *choiceModel) cycleOption(delta int) {
	if c.onContinue() {
		return
	}

	n := len(c.rows[c.cursor].Options)
	c.chosen[c.cursor] = (c.chosen[c.cursor] + delta + n) % n
}

// scrollToCursor keeps the focused row, and its open dropdown, on screen.
func (c *choiceModel) scrollToCursor(height int) {
	visible := c.rowsHeight(height)
	if c.dropdownOpen {
		visible -= c.dropdownHeight()
	}
	visible = maxInt(visible, 1)

	// The Continue button is a line of its own below the rows.
	last := minInt(c.cursor, len(c.rows)-1)
	if c.cursor < c.offset {
		c.offset = c.cursor
	}
	if last >= c.offset+visible {
		c.offset = last - visible + 1
	}
	if c.offset < 0 {
		c.offset = 0
	}
}

// Fixed lines around the rows: the header, the Continue button with a blank
// line above it, and the detail footer's separator.
const choiceChromeLines = 4

// rowsHeight is how many table rows fit in height, after the header, the
// Continue button, the detail footer and the help.
func (c choiceModel) rowsHeight(height int) int {
	return height - choiceChromeLines - c.detailHeight() - c.helpHeight()
}

func (c choiceModel) detailHeight() int {
	// Room for the tallest detail, so the table does not jump while moving.
	lines := 1
	for _, row := range c.rows {
		lines = maxInt(lines, strings.Count(row.Detail, "\n")+1)
	}

	return lines
}

func (c choiceModel) helpHeight() int {
	return lipgloss.Height(c.helpView())
}

func (c choiceModel) dropdownHeight() int {
	if c.onContinue() {
		return 0
	}

	// One line per option inside a bordered box.
	return len(c.rows[c.cursor].Options) + 2
}

func (c choiceModel) helpView() string {
	if c.fullHelp {
		return c.help.FullHelpView(c.keys.FullHelp())
	}

	return c.help.ShortHelpView(c.keys.ShortHelp())
}

// layout is the width of every cell column and of the option column, fitted
// into width by shrinking the first column.
type choiceLayout struct {
	cells  []int
	option int
}

const (
	choiceCursorWidth = 2 // "▸ "
	choiceMarkWidth   = 2 // " ●"
	choiceMinFirstCol = 8
)

func (c choiceModel) layout(width int) choiceLayout {
	l := choiceLayout{cells: make([]int, len(c.columns))}

	for i, column := range c.columns {
		l.cells[i] = lipgloss.Width(column)
	}
	for _, row := range c.rows {
		for i := range l.cells {
			if i < len(row.Cells) {
				l.cells[i] = maxInt(l.cells[i], lipgloss.Width(row.Cells[i]))
			}
		}
		for _, option := range row.Options {
			l.option = maxInt(l.option, lipgloss.Width(option))
		}
	}

	total := choiceCursorWidth + lipgloss.Width(optionCell("", l.option)) + choiceMarkWidth
	for _, w := range l.cells {
		total += w + 1
	}

	if over := total - width; over > 0 && len(l.cells) > 0 {
		l.cells[0] = maxInt(l.cells[0]-over, choiceMinFirstCol)
	}

	return l
}

// optionCell renders a chosen option as a closed dropdown.
func optionCell(option string, width int) string {
	return "[" + padRight(option, width) + " ▾]"
}

func (c choiceModel) view(width, height int) string {
	c.help.SetWidth(width)
	l := c.layout(width)

	lines := []string{c.renderHeader(l)}

	visible := c.rowsHeight(height)
	if c.dropdownOpen {
		visible -= c.dropdownHeight()
	}
	visible = maxInt(visible, 1)

	end := minInt(c.offset+visible, len(c.rows))
	for i := c.offset; i < end; i++ {
		lines = append(lines, c.renderRow(i, l))
		if c.dropdownOpen && i == c.cursor {
			lines = append(lines, c.renderDropdown(l))
		}
	}
	if more := len(c.rows) - end; more > 0 {
		lines = append(lines, helpStyle.Render(fmt.Sprintf("  … %d more", more)))
	}

	lines = append(lines, "", c.renderContinue())
	lines = append(lines, helpStyle.Render(strings.Repeat("─", width)))
	lines = append(lines, c.renderDetail(width))

	body := lipgloss.JoinVertical(lipgloss.Left, lines...)

	// Pin the help to the bottom of the panel.
	gap := height - lipgloss.Height(body) - c.helpHeight()
	if gap > 0 {
		body += strings.Repeat("\n", gap)
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(body + "\n" + c.helpView())
}

func (c choiceModel) renderHeader(l choiceLayout) string {
	cells := make([]string, 0, len(c.columns)+1)
	for i, column := range c.columns {
		cells = append(cells, padRight(truncate(column, l.cells[i]), l.cells[i]))
	}

	header := strings.Repeat(" ", choiceCursorWidth) + strings.Join(cells, " ")
	if n := c.changes(); n > 0 {
		header += " " + choiceChangedStyle.Render(fmt.Sprintf("%d %s", n, plural(n, "change", "changes")))
	}

	return helpStyle.Render(header)
}

func (c choiceModel) renderRow(i int, l choiceLayout) string {
	row := c.rows[i]
	focused := i == c.cursor

	cells := make([]string, 0, len(l.cells))
	for j, w := range l.cells {
		cell := ""
		if j < len(row.Cells) {
			cell = row.Cells[j]
		}
		cells = append(cells, padRight(truncate(cell, w), w))
	}

	text := strings.Join(cells, " ")
	option := optionCell(row.Options[c.chosen[i]], l.option)

	cursor := "  "
	if focused {
		cursor = choiceCursorStyle.Render("▸ ")
		text = choiceFocusedStyle.Render(text)
		option = choiceFocusedOptionStyle.Render(option)
	}

	mark := "  "
	if c.chosen[i] != c.initial[i] {
		mark = " " + choiceChangedStyle.Render("●")
	}

	return cursor + text + " " + option + mark
}

// renderDropdown draws the open dropdown below the focused row, lined up
// under its option cell.
func (c choiceModel) renderDropdown(l choiceLayout) string {
	options := c.rows[c.cursor].Options

	lines := make([]string, 0, len(options))
	for i, option := range options {
		text := padRight(option, l.option)
		if i == c.dropdownCursor {
			lines = append(lines, choiceFocusedOptionStyle.Render("▸"+text+" "))
			continue
		}
		lines = append(lines, " "+text+" ")
	}

	indent := choiceCursorWidth
	for _, w := range l.cells {
		indent += w + 1
	}

	box := choiceDropdownStyle.Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().MarginLeft(indent).Render(box)
}

func (c choiceModel) renderContinue() string {
	if c.onContinue() {
		return choiceCursorStyle.Render("▸ ") + choiceFocusedOptionStyle.Render("[ Continue ]")
	}

	return "  [ Continue ]"
}

func (c choiceModel) renderDetail(width int) string {
	detail := ""
	switch {
	case c.onContinue() && c.changes() == 0:
		detail = "No changes. Continue to proceed."
	case c.onContinue():
		n := c.changes()
		detail = fmt.Sprintf("%d %s. Continue to apply.", n, plural(n, "change", "changes"))
	default:
		detail = c.rows[c.cursor].Detail
	}

	lines := strings.Split(detail, "\n")
	for i, line := range lines {
		lines[i] = truncate(line, width)
	}

	return lipgloss.NewStyle().
		Height(c.detailHeight()).
		Render(strings.Join(lines, "\n"))
}

var (
	choiceCursorStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#F7C948"))

	choiceFocusedStyle = lipgloss.NewStyle().
				Bold(true)

	choiceFocusedOptionStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("#000000")).
					Background(lipgloss.Color("#F7C948"))

	choiceChangedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F7C948"))

	choiceDropdownStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("99"))
)

// truncate shortens text to width, marking the cut with an ellipsis.
func truncate(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	if width <= 1 {
		return strings.Repeat("…", maxInt(width, 0))
	}

	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}

	return string(runes) + "…"
}

func padRight(text string, width int) string {
	if pad := width - lipgloss.Width(text); pad > 0 {
		return text + strings.Repeat(" ", pad)
	}

	return text
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
