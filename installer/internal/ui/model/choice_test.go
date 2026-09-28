package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

func testChoice(rows int) choiceModel {
	prompt := core.ChoicePrompt{Columns: []string{"Model", "Status"}}
	for i := 0; i < rows; i++ {
		prompt.Rows = append(prompt.Rows, core.ChoiceRow{
			Cells:   []string{"Model-" + string(rune('A'+i)), "Available"},
			Detail:  "detail " + string(rune('A'+i)),
			Options: []string{"-", "install"},
			Current: "-",
		})
	}

	return newChoiceModel(prompt, true)
}

// testGroups is three nodes, two with a GPU, grouped into a model pool, CPU
// and unassigned. The model pool must not be empty.
func testGroups() choiceModel {
	gpu := []string{"model:a", "cpu", "none"}
	cpu := []string{"cpu", "none"}

	prompt := core.ChoicePrompt{
		Columns: []string{"Node"},
		Rows: []core.ChoiceRow{
			{Cells: []string{"gpu-1"}, Options: gpu, Current: "none"},
			{Cells: []string{"cpu-1"}, Options: cpu, Current: "cpu"},
			{Cells: []string{"gpu-2"}, Options: gpu, Current: "none"},
		},
		Groups: []core.ChoiceGroup{
			{Option: "model:a", Title: "Model A"},
			{Option: "cpu", Title: "CPU only"},
			{Option: "none", Title: "Unassigned"},
		},
		ClearOption: "none",
		Check: func(values []string) core.ChoiceCheck {
			for _, value := range values {
				if value == "model:a" {
					return core.ChoiceCheck{}
				}
			}
			return core.ChoiceCheck{Blocking: "Assign at least one node to Model A."}
		},
	}

	return newChoiceModel(prompt, true)
}

func TestGroupedStartsOnFirstRowInDisplayOrder(t *testing.T) {
	c := testGroups()

	// Model A is empty, so the first row shown is cpu-1 under CPU only.
	if c.cursor != 1 {
		t.Fatalf("cursor = %d, want cpu-1", c.cursor)
	}
	if got := c.order(); len(got) != 3 || got[0] != 1 {
		t.Errorf("order = %v, want cpu-1 first", got)
	}
}

func TestGroupedDigitMovesTheRowAndTheCursorFollows(t *testing.T) {
	c := testGroups()

	c, _ = press(t, c, 30, "j", "1")
	if c.values()[0] != "model:a" {
		t.Fatalf("values = %v, want gpu-1 in Model A", c.values())
	}
	if c.cursor != 0 {
		t.Errorf("cursor = %d, want it to follow gpu-1", c.cursor)
	}
	if c.order()[0] != 0 {
		t.Errorf("order = %v, want gpu-1 shown first, under Model A", c.order())
	}
}

func TestGroupedDigitSkipsRowsThatDoNotOfferTheGroup(t *testing.T) {
	c := testGroups()

	// cpu-1 has no GPU, so it cannot join the model pool.
	c, _ = press(t, c, 30, "1")
	if c.values()[1] != "cpu" {
		t.Errorf("values = %v, want cpu-1 left in CPU only", c.values())
	}
}

func TestVisualModeMovesSeveralRows(t *testing.T) {
	c := testGroups()

	// Display order is cpu-1, gpu-1, gpu-2: select the two GPU nodes.
	c, _ = press(t, c, 30, "j", "V", "j", "1")
	if got := strings.Join(c.values(), ","); got != "model:a,cpu,model:a" {
		t.Errorf("values = %s, want both GPU nodes in Model A", got)
	}
	if c.visual {
		t.Error("visual mode still on after moving the selection")
	}
}

func TestVisualDropdownOffersOnlySharedOptions(t *testing.T) {
	c := testGroups()

	// cpu-1 and gpu-1: model:a is not offered by cpu-1.
	c, _ = press(t, c, 30, "V", "j", "enter")
	if !c.dropdownOpen {
		t.Fatal("enter did not open the dropdown")
	}
	if got := strings.Join(c.dropdownOptions, ","); got != "cpu,none" {
		t.Errorf("dropdown = %s, want the options both rows offer", got)
	}
}

func TestClearSetsTheClearOption(t *testing.T) {
	c := testGroups()

	c, _ = press(t, c, 30, "x")
	if c.values()[1] != "none" {
		t.Errorf("values = %v, want cpu-1 cleared", c.values())
	}
}

func TestContinueIsBlockedUntilTheCheckPasses(t *testing.T) {
	c := testGroups()

	c, outcome := press(t, c, 30, "G", "enter")
	if outcome != choicePending {
		t.Fatalf("outcome = %v, want the blocked Continue to stay", outcome)
	}
	if !strings.Contains(c.view(60, 30), "Assign at least one node to Model A.") {
		t.Error("the blocking reason is not shown")
	}

	c, _ = press(t, c, 30, "g", "j", "1")
	if _, outcome = press(t, c, 30, "G", "enter"); outcome != choiceSubmitted {
		t.Errorf("outcome = %v, want submitted once the pool has a node", outcome)
	}
}

func TestGroupedViewShowsHeadersAndEmptyGroups(t *testing.T) {
	view := testGroups().view(60, 30)

	for _, want := range []string{"1 Model A", "2 CPU only", "3 Unassigned", "(none)"} {
		if !strings.Contains(view, want) {
			t.Errorf("view is missing %q:\n%s", want, view)
		}
	}
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
	c.prompt.Rows[0].Cells[0] = "Devstral-Small-2-24B-Instruct-2512"

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
