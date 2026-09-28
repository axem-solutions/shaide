package model

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
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
	Group    key.Binding
	Visual   key.Binding
	Clear    key.Binding
	Reset    key.Binding
	Help     key.Binding
	Cancel   key.Binding
}

func newChoiceKeyMap(grouped, clearable bool) choiceKeyMap {
	k := choiceKeyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "half page down")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "half page up")),
		Prev:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous option")),
		Next:     key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next option")),
		Open:     key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter", "choose")),
		Group:    key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1-9", "move")),
		Visual:   key.NewBinding(key.WithKeys("V"), key.WithHelp("V", "select rows")),
		Clear:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "clear")),
		Reset:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo row")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
		Cancel:   key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel")),
	}

	k.Group.SetEnabled(grouped)
	k.Clear.SetEnabled(clearable)

	return k
}

func (k choiceKeyMap) ShortHelp() []key.Binding {
	if k.Group.Enabled() {
		return []key.Binding{k.Down, k.Up, k.Group, k.Open, k.Help}
	}

	return []key.Binding{k.Down, k.Up, k.Next, k.Open, k.Help}
}

func (k choiceKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Down, k.Up, k.Top, k.Bottom, k.HalfDown, k.HalfUp},
		{k.Prev, k.Next, k.Open, k.Group, k.Visual},
		{k.Clear, k.Reset, k.Cancel, k.Help},
	}
}

// choiceModel is a list of rows that each carry a dropdown of options, for
// example an action per model or a pool per node. In grouped mode the rows
// are shown under a header per option, so choosing an option moves the row to
// that group. The line after the rows is a Continue button, which submits the
// chosen option of every row.
type choiceModel struct {
	prompt core.ChoicePrompt

	// chosen and initial are option indexes per row. initial is what the row
	// started at, so changed rows can be marked and reset.
	chosen  []int
	initial []int

	// cursor is a row index; len(rows) is the Continue button.
	cursor int
	// offset is the first display line shown when the lines do not fit.
	offset int

	// visual selects the rows between visualAnchor and the cursor, in
	// display order, so one choice applies to all of them.
	visual       bool
	visualAnchor int

	dropdownOpen    bool
	dropdownCursor  int
	dropdownOptions []string

	// notice replaces the detail footer until the next key press, for
	// example to say why Continue did not submit.
	notice string

	keys     choiceKeyMap
	help     help.Model
	fullHelp bool
}

func newChoiceModel(prompt core.ChoicePrompt, isDark bool) choiceModel {
	c := choiceModel{
		prompt:  prompt,
		chosen:  make([]int, len(prompt.Rows)),
		initial: make([]int, len(prompt.Rows)),
		keys:    newChoiceKeyMap(len(prompt.Groups) > 0, prompt.ClearOption != ""),
		help:    help.New(),
	}

	for i, row := range prompt.Rows {
		if j := optionIndex(row, row.Current); j >= 0 {
			c.chosen[i] = j
			c.initial[i] = j
		}
	}

	if order := c.order(); len(order) > 0 {
		c.cursor = order[0]
	}

	c.setDark(isDark)

	return c
}

func (c *choiceModel) setDark(isDark bool) {
	c.help.Styles = help.DefaultStyles(isDark)
}

func optionIndex(row core.ChoiceRow, option string) int {
	for i, candidate := range row.Options {
		if candidate == option {
			return i
		}
	}

	return -1
}

func (c choiceModel) grouped() bool {
	return len(c.prompt.Groups) > 0
}

func (c choiceModel) rows() []core.ChoiceRow {
	return c.prompt.Rows
}

func (c choiceModel) option(row int) string {
	return c.rows()[row].Options[c.chosen[row]]
}

// values is the reply: the chosen option of every row, in row order.
func (c choiceModel) values() []string {
	values := make([]string, len(c.rows()))
	for i := range c.rows() {
		values[i] = c.option(i)
	}

	return values
}

func (c choiceModel) check() core.ChoiceCheck {
	if c.prompt.Check == nil {
		return core.ChoiceCheck{}
	}

	return c.prompt.Check(c.values())
}

func (c choiceModel) changes() int {
	n := 0
	for i := range c.rows() {
		if c.chosen[i] != c.initial[i] {
			n++
		}
	}

	return n
}

func (c choiceModel) onContinue() bool {
	return c.cursor == len(c.rows())
}

// choiceLine is one display line: a row, a group header, or the placeholder
// of an empty group.
type choiceLine struct {
	row   int
	group int
}

func (c choiceModel) lines() []choiceLine {
	if !c.grouped() {
		lines := make([]choiceLine, 0, len(c.rows()))
		for i := range c.rows() {
			lines = append(lines, choiceLine{row: i, group: -1})
		}
		return lines
	}

	var lines []choiceLine
	for g, group := range c.prompt.Groups {
		lines = append(lines, choiceLine{row: -1, group: g})

		members := 0
		for i := range c.rows() {
			if c.option(i) == group.Option {
				lines = append(lines, choiceLine{row: i, group: -1})
				members++
			}
		}
		if members == 0 {
			lines = append(lines, choiceLine{row: -1, group: -1})
		}
	}

	return lines
}

// order is the rows in display order, which is the order the cursor moves in.
func (c choiceModel) order() []int {
	var order []int
	for _, line := range c.lines() {
		if line.row >= 0 {
			order = append(order, line.row)
		}
	}

	return order
}

func position(order []int, row int) int {
	for i, candidate := range order {
		if candidate == row {
			return i
		}
	}

	return len(order)
}

// update handles a key press. height is how many lines the view may use, so
// paging and scrolling match what is on screen.
func (c choiceModel) update(msg tea.KeyPressMsg, height int) (choiceModel, choiceOutcome) {
	c.notice = ""

	if c.dropdownOpen {
		c = c.updateDropdown(msg)
		c.scrollToCursor(height)
		return c, choicePending
	}

	switch {
	case key.Matches(msg, c.keys.Cancel):
		if c.visual {
			c.visual = false
			break
		}
		return c, choiceCancelled
	case key.Matches(msg, c.keys.Help):
		c.fullHelp = !c.fullHelp
	case key.Matches(msg, c.keys.Up):
		c.moveCursor(-1)
	case key.Matches(msg, c.keys.Down):
		c.moveCursor(1)
	case key.Matches(msg, c.keys.Top):
		c.moveCursor(-len(c.rows()) - 1)
	case key.Matches(msg, c.keys.Bottom):
		c.cursor = len(c.rows())
	case key.Matches(msg, c.keys.HalfDown):
		c.moveCursor(maxInt(c.rowsHeight(height)/2, 1))
	case key.Matches(msg, c.keys.HalfUp):
		c.moveCursor(-maxInt(c.rowsHeight(height)/2, 1))
	case key.Matches(msg, c.keys.Prev):
		c.cycleOption(-1)
	case key.Matches(msg, c.keys.Next):
		c.cycleOption(1)
	case key.Matches(msg, c.keys.Group):
		c.moveToGroup(int(msg.String()[0] - '1'))
	case key.Matches(msg, c.keys.Visual):
		c.toggleVisual()
	case key.Matches(msg, c.keys.Clear):
		c.apply(c.prompt.ClearOption)
	case key.Matches(msg, c.keys.Reset):
		for _, row := range c.targets() {
			c.chosen[row] = c.initial[row]
		}
		c.visual = false
	case key.Matches(msg, c.keys.Open):
		if c.onContinue() {
			if blocking := c.check().Blocking; blocking != "" {
				c.notice = blocking
				break
			}
			return c, choiceSubmitted
		}
		c.openDropdown()
	}

	c.scrollToCursor(height)

	return c, choicePending
}

func (c choiceModel) updateDropdown(msg tea.KeyPressMsg) choiceModel {
	n := len(c.dropdownOptions)

	switch {
	case key.Matches(msg, c.keys.Cancel):
		c.dropdownOpen = false
	case key.Matches(msg, c.keys.Up):
		c.dropdownCursor = (c.dropdownCursor - 1 + n) % n
	case key.Matches(msg, c.keys.Down):
		c.dropdownCursor = (c.dropdownCursor + 1) % n
	case key.Matches(msg, c.keys.Top):
		c.dropdownCursor = 0
	case key.Matches(msg, c.keys.Bottom):
		c.dropdownCursor = n - 1
	case key.Matches(msg, c.keys.Open):
		c.dropdownOpen = false
		c.apply(c.dropdownOptions[c.dropdownCursor])
	}

	return c
}

func (c *choiceModel) moveCursor(delta int) {
	order := c.order()

	pos := position(order, c.cursor) + delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(order) {
		c.cursor = len(c.rows())
		return
	}

	c.cursor = order[pos]
}

// targets is the rows a choice applies to: the visual selection, or the
// focused row.
func (c choiceModel) targets() []int {
	if c.onContinue() && !c.visual {
		return nil
	}
	if !c.visual {
		return []int{c.cursor}
	}

	order := c.order()
	from, to := position(order, c.visualAnchor), position(order, c.cursor)
	if from > to {
		from, to = to, from
	}
	if to >= len(order) {
		to = len(order) - 1
	}

	return append([]int(nil), order[from:to+1]...)
}

// apply sets every target row that offers option to it, and ends the visual
// selection.
func (c *choiceModel) apply(option string) {
	if option == "" {
		return
	}

	for _, row := range c.targets() {
		if j := optionIndex(c.rows()[row], option); j >= 0 {
			c.chosen[row] = j
		}
	}

	c.visual = false
}

func (c *choiceModel) cycleOption(delta int) {
	if c.onContinue() || c.visual {
		return
	}

	n := len(c.rows()[c.cursor].Options)
	c.chosen[c.cursor] = (c.chosen[c.cursor] + delta + n) % n
}

func (c *choiceModel) moveToGroup(group int) {
	if group < 0 || group >= len(c.prompt.Groups) {
		return
	}

	c.apply(c.prompt.Groups[group].Option)
}

func (c *choiceModel) toggleVisual() {
	if c.visual {
		c.visual = false
		return
	}
	if c.onContinue() {
		return
	}

	c.visual = true
	c.visualAnchor = c.cursor
}

// openDropdown lists the options every target row offers, in group order
// when grouped.
func (c *choiceModel) openDropdown() {
	targets := c.targets()
	if len(targets) == 0 {
		return
	}

	candidates := c.rows()[targets[0]].Options
	if c.grouped() {
		candidates = nil
		for _, group := range c.prompt.Groups {
			candidates = append(candidates, group.Option)
		}
	}

	var options []string
	for _, option := range candidates {
		offered := true
		for _, row := range targets {
			if optionIndex(c.rows()[row], option) < 0 {
				offered = false
				break
			}
		}
		if offered {
			options = append(options, option)
		}
	}
	if len(options) == 0 {
		c.notice = "The selected rows have no option in common."
		return
	}

	c.dropdownOptions = options
	c.dropdownCursor = 0
	for i, option := range options {
		if option == c.option(c.cursor) {
			c.dropdownCursor = i
		}
	}
	c.dropdownOpen = true
}

// Fixed lines around the rows: the header, the blank line and Continue button
// below them, and the detail footer's separator.
const choiceChromeLines = 4

// rowsHeight is how many display lines fit in height, after the header, the
// Continue button, the detail footer and the help.
func (c choiceModel) rowsHeight(height int) int {
	return height - choiceChromeLines - c.detailHeight() - c.helpHeight()
}

// visibleLines is how many display lines are shown, leaving room for an open
// dropdown and for the "more" marker when the lines do not all fit.
func (c choiceModel) visibleLines(height int) int {
	visible := c.rowsHeight(height)
	if c.dropdownOpen {
		visible -= c.dropdownHeight()
	}
	if len(c.lines()) > visible {
		visible--
	}

	return maxInt(visible, 1)
}

func (c choiceModel) detailHeight() int {
	// Room for the tallest detail, so the list does not jump while moving.
	lines := 1
	for _, row := range c.rows() {
		lines = maxInt(lines, strings.Count(row.Detail, "\n")+1)
	}

	return lines
}

func (c choiceModel) helpHeight() int {
	return lipgloss.Height(c.helpView())
}

func (c choiceModel) dropdownHeight() int {
	// One line per option inside a bordered box.
	return len(c.dropdownOptions) + 2
}

func (c choiceModel) helpView() string {
	if c.fullHelp {
		return c.help.FullHelpView(c.keys.FullHelp())
	}

	return c.help.ShortHelpView(c.keys.ShortHelp())
}

// scrollToCursor keeps the focused line on screen.
func (c *choiceModel) scrollToCursor(height int) {
	lines := c.lines()
	visible := c.visibleLines(height)

	focus := len(lines) - 1
	if !c.onContinue() {
		for i, line := range lines {
			if line.row == c.cursor {
				focus = i
				break
			}
		}
	}

	// Keep a group's header in view with its first row.
	top := focus
	if top > 0 && lines[top-1].group >= 0 {
		top--
	}

	if top < c.offset {
		c.offset = top
	}
	if focus >= c.offset+visible {
		c.offset = focus - visible + 1
	}
	if c.offset < 0 {
		c.offset = 0
	}
}

// choiceLayout is the width of every cell column and of the option column,
// fitted into width by shrinking the first column.
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
	l := choiceLayout{cells: make([]int, len(c.prompt.Columns))}

	for i, column := range c.prompt.Columns {
		l.cells[i] = lipgloss.Width(column)
	}
	for _, row := range c.rows() {
		for i := range l.cells {
			if i < len(row.Cells) {
				l.cells[i] = maxInt(l.cells[i], lipgloss.Width(row.Cells[i]))
			}
		}
		if !c.grouped() {
			for _, option := range row.Options {
				l.option = maxInt(l.option, lipgloss.Width(option))
			}
		}
	}

	total := choiceCursorWidth + choiceMarkWidth
	if !c.grouped() {
		total += lipgloss.Width(optionCell("", l.option)) + 1
	}
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
	check := c.check()
	selected := map[int]bool{}
	if c.visual {
		for _, row := range c.targets() {
			selected[row] = true
		}
	}

	out := []string{c.renderHeader(l)}

	lines := c.lines()
	end := minInt(c.offset+c.visibleLines(height), len(lines))
	for _, line := range lines[c.offset:end] {
		switch {
		case line.row >= 0:
			out = append(out, c.renderRow(line.row, l, selected[line.row]))
			if c.dropdownOpen && line.row == c.cursor {
				out = append(out, c.renderDropdown(l))
			}
		case line.group >= 0:
			out = append(out, c.renderGroupHeader(line.group, check, width))
		default:
			out = append(out, helpStyle.Render("    (none)"))
		}
	}
	if more := len(lines) - end; more > 0 {
		out = append(out, helpStyle.Render(fmt.Sprintf("  … %d more", more)))
	}

	out = append(out, "", c.renderContinue(check))
	out = append(out, helpStyle.Render(strings.Repeat("─", width)))
	out = append(out, c.renderDetail(width, check))

	body := lipgloss.JoinVertical(lipgloss.Left, out...)

	// Pin the help to the bottom of the panel.
	if gap := height - lipgloss.Height(body) - c.helpHeight(); gap > 0 {
		body += strings.Repeat("\n", gap)
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(body + "\n" + c.helpView())
}

func (c choiceModel) renderHeader(l choiceLayout) string {
	cells := make([]string, 0, len(c.prompt.Columns))
	for i, column := range c.prompt.Columns {
		cells = append(cells, padRight(truncate(column, l.cells[i]), l.cells[i]))
	}

	header := strings.Repeat(" ", choiceCursorWidth) + strings.Join(cells, " ")
	if n := c.changes(); n > 0 {
		header += " " + choiceChangedStyle.Render(fmt.Sprintf("%d %s", n, plural(n, "change", "changes")))
	}

	return helpStyle.Render(header)
}

func (c choiceModel) renderGroupHeader(g int, check core.ChoiceCheck, width int) string {
	group := c.prompt.Groups[g]

	title := group.Title
	if g < 9 {
		title = fmt.Sprintf("%d %s", g+1, title)
	}

	status := check.Groups[group.Option]
	statusText := ""
	if status.Text != "" {
		mark := "✓ "
		style := choiceOKStyle
		if !status.OK {
			mark = "✗ "
			style = errorStyle
		}
		statusText = style.Render(mark + status.Text)
	}

	// "── <title> ─── <status>", with the title shortened to make room.
	room := width - lipgloss.Width(statusText) - 5
	title = truncate(title, maxInt(room, 4))
	fill := maxInt(width-lipgloss.Width(title)-lipgloss.Width(statusText)-5, 1)

	line := "── " + choiceGroupStyle.Render(title) + " " + strings.Repeat("─", fill)
	if statusText != "" {
		line += " " + statusText
	}

	return helpStyle.Render(line)
}

func (c choiceModel) renderRow(i int, l choiceLayout, selected bool) string {
	row := c.rows()[i]
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
	option := ""
	if !c.grouped() {
		option = " " + optionCell(c.option(i), l.option)
	}

	cursor := "  "
	switch {
	case focused:
		cursor = choiceCursorStyle.Render("▸ ")
		text = choiceFocusedStyle.Render(text)
		if option != "" {
			option = " " + choiceFocusedOptionStyle.Render(strings.TrimPrefix(option, " "))
		}
	case selected:
		cursor = choiceCursorStyle.Render("┃ ")
		text = choiceFocusedStyle.Render(text)
	}

	mark := "  "
	if c.chosen[i] != c.initial[i] {
		mark = " " + choiceChangedStyle.Render("●")
	}

	return cursor + text + option + mark
}

// renderDropdown draws the open dropdown below the focused row, lined up
// under its option cell, or indented under the row when grouped.
func (c choiceModel) renderDropdown(l choiceLayout) string {
	labels := make([]string, len(c.dropdownOptions))
	width := 0
	for i, option := range c.dropdownOptions {
		labels[i] = c.optionLabel(option)
		width = maxInt(width, lipgloss.Width(labels[i]))
	}

	lines := make([]string, 0, len(labels))
	for i, label := range labels {
		text := padRight(label, width)
		if i == c.dropdownCursor {
			lines = append(lines, choiceFocusedOptionStyle.Render("▸"+text+" "))
			continue
		}
		lines = append(lines, " "+text+" ")
	}

	indent := choiceCursorWidth + 2
	if !c.grouped() {
		indent = choiceCursorWidth
		for _, w := range l.cells {
			indent += w + 1
		}
	}

	box := choiceDropdownStyle.Render(strings.Join(lines, "\n"))

	return lipgloss.NewStyle().MarginLeft(indent).Render(box)
}

// optionLabel is how an option reads in the dropdown: its group title when
// grouped, numbered like the group header.
func (c choiceModel) optionLabel(option string) string {
	for g, group := range c.prompt.Groups {
		if group.Option == option {
			if g < 9 {
				return fmt.Sprintf("%d %s", g+1, group.Title)
			}
			return group.Title
		}
	}

	return option
}

func (c choiceModel) renderContinue(check core.ChoiceCheck) string {
	label := "[ Continue ]"
	if check.Blocking != "" {
		label = "[ Continue ✗ ]"
	}

	if c.onContinue() {
		return choiceCursorStyle.Render("▸ ") + choiceFocusedOptionStyle.Render(label)
	}

	return "  " + label
}

func (c choiceModel) renderDetail(width int, check core.ChoiceCheck) string {
	style := lipgloss.NewStyle()

	var detail string
	switch {
	case c.notice != "":
		detail = c.notice
		style = errorStyle
	case c.visual:
		n := len(c.targets())
		detail = fmt.Sprintf("%d %s selected. Choose where to move %s.", n, plural(n, "row", "rows"), plural(n, "it", "them"))
	case c.onContinue() && check.Blocking != "":
		detail = check.Blocking
		style = errorStyle
	case c.onContinue() && c.changes() == 0:
		detail = "No changes. Continue to proceed."
	case c.onContinue():
		n := c.changes()
		detail = fmt.Sprintf("%d %s. Continue to apply.", n, plural(n, "change", "changes"))
	default:
		detail = c.rows()[c.cursor].Detail
	}

	lines := strings.Split(wrapText(detail, width), "\n")
	if limit := c.detailHeight(); len(lines) > limit {
		lines = lines[:limit]
	}

	return style.Height(c.detailHeight()).Render(strings.Join(lines, "\n"))
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

	choiceGroupStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("99"))

	choiceOKStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

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
