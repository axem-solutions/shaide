package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

func testChoice(rows int) choiceModel {
	msg := messages.PromptChoiceMessage{Columns: []string{"Model", "Status"}}
	for i := 0; i < rows; i++ {
		msg.Rows = append(msg.Rows, messages.ChoiceRow{
			Cells:   []string{"Model-" + string(rune('A'+i)), "Available"},
			Detail:  "detail " + string(rune('A'+i)),
			Options: []string{"-", "install"},
			Current: "-",
		})
	}

	return newChoiceModel(msg, true)
}

func press(t *testing.T, c choiceModel, height int, keys ...string) (choiceModel, choiceOutcome) {
	t.Helper()

	outcome := choicePending
	for _, k := range keys {
		c, outcome = c.update(keyPress(k), height)
	}

	return c, outcome
}

func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+d":
		return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
	}

	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func TestChoiceStartsAtCurrentAndSubmitsFromContinue(t *testing.T) {
	c := testChoice(2)

	c, outcome := press(t, c, 30, "G", "enter")
	if outcome != choiceSubmitted {
		t.Fatalf("outcome = %v, want submitted", outcome)
	}
	if got := strings.Join(c.values(), ","); got != "-,-" {
		t.Errorf("values = %s, want the untouched defaults", got)
	}
}

func TestChoiceCyclesWithVimKeys(t *testing.T) {
	c := testChoice(3)

	c, _ = press(t, c, 30, "j", "l", "j", "l", "h", "k")
	if got := strings.Join(c.values(), ","); got != "-,install,-" {
		t.Errorf("values = %s, want only the second row changed", got)
	}
	if c.cursor != 1 {
		t.Errorf("cursor = %d, want 1 after j j k", c.cursor)
	}
	if c.changes() != 1 {
		t.Errorf("changes = %d, want 1", c.changes())
	}

	c, _ = press(t, c, 30, "u")
	if c.changes() != 0 {
		t.Errorf("u did not reset the row: %v", c.values())
	}
}

func TestChoiceDropdownPicksAnOption(t *testing.T) {
	c := testChoice(2)

	c, _ = press(t, c, 30, "enter")
	if !c.dropdownOpen {
		t.Fatal("enter on a row did not open its dropdown")
	}

	c, outcome := press(t, c, 30, "j", "enter")
	if outcome != choicePending || c.dropdownOpen {
		t.Fatalf("picking closed = %v, outcome = %v", !c.dropdownOpen, outcome)
	}
	if c.values()[0] != "install" {
		t.Errorf("values = %v, want the picked option", c.values())
	}
}

func TestChoiceEscClosesDropdownBeforeCancelling(t *testing.T) {
	c := testChoice(1)

	c, outcome := press(t, c, 30, "space", "j", "esc")
	if outcome != choicePending || c.dropdownOpen {
		t.Fatalf("esc in the dropdown: open = %v, outcome = %v", c.dropdownOpen, outcome)
	}
	if c.values()[0] != "-" {
		t.Errorf("closing the dropdown changed the row: %v", c.values())
	}

	if _, outcome = press(t, c, 30, "esc"); outcome != choiceCancelled {
		t.Errorf("esc outside the dropdown: outcome = %v, want cancelled", outcome)
	}
}

func TestChoiceScrollsToKeepTheCursorVisible(t *testing.T) {
	c := testChoice(40)
	height := 20

	c, _ = press(t, c, height, "G")
	view := c.view(60, height)

	if !strings.Contains(view, "Model-"+string(rune('A'+39))) {
		t.Errorf("last row not visible after G:\n%s", view)
	}
	if lipgloss.Height(view) > height {
		t.Errorf("view is %d lines, want at most %d", lipgloss.Height(view), height)
	}

	c, _ = press(t, c, height, "g")
	if c.offset != 0 {
		t.Errorf("offset = %d after g, want 0", c.offset)
	}

	c, _ = press(t, c, height, "ctrl+d")
	if c.cursor == 0 {
		t.Error("ctrl+d did not move the cursor")
	}
}

func TestChoiceViewFitsNarrowPanels(t *testing.T) {
	c := testChoice(3)
	c.rows[0].Cells[0] = "Devstral-Small-2-24B-Instruct-2512"

	for _, width := range []int{40, 60} {
		view := c.view(width, 20)
		for _, line := range strings.Split(view, "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("width %d: line is %d wide: %q", width, w, line)
			}
		}
	}
}

func TestLeftPanelIsAThirdOfTheTerminal(t *testing.T) {
	tests := []struct {
		width int
		want  int
	}{
		{width: 240, want: 80},
		{width: 120, want: minLeftWidth},
		{width: 60, want: minLeftWidth},
	}

	for _, test := range tests {
		m := Model{Width: test.width}
		if got := m.leftPanelWidth(); got != test.want {
			t.Errorf("width %d: left = %d, want %d", test.width, got, test.want)
		}
	}
}
