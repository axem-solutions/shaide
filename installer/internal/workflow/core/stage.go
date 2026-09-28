package core

type Step struct {
	Name    string
	When    func(*Runtime) bool
	Run     func(*Runtime) error
	Recover func(*Runtime, error) (RecoveryAction, error)
}

type Stage struct {
	Name     string
	NewState func() any
	Steps    []Step
	Cleanup  func(*Runtime) error
}

type Reporter interface {
	Select(title string, current string, options []string) (string, error)
	MultiSelect(title string, options []string) ([]string, error)
	Input(title string, placeholder string, defaultValue string) (string, error)
	Choose(prompt ChoicePrompt) ([]string, error)
	ProgressModel(progress ModelProgress)
}

// ChoicePrompt asks for one option per row of a table, for example an action
// per model. The reply holds the chosen option of every row, in row order.
type ChoicePrompt struct {
	Title   string
	Columns []string
	Rows    []ChoiceRow

	// Groups, when set, shows the rows under a header per option instead of
	// in one table, in this order, so moving a row to another option moves it
	// to that group. Every row option must be one of them.
	Groups []ChoiceGroup

	// ClearOption is the option a row is set to when it is cleared, for
	// example "unassigned". Empty disables clearing.
	ClearOption string

	// Check reports, for the current choices, the status shown in each group
	// header and whether the choices can be submitted. Nil accepts any
	// choice.
	Check func(values []string) ChoiceCheck
}

type ChoiceGroup struct {
	Option string
	Title  string
}

type ChoiceCheck struct {
	// Groups maps an option to the status shown in its group header.
	Groups map[string]GroupStatus
	// Blocking, when set, explains why the choices cannot be submitted yet.
	Blocking string
}

type GroupStatus struct {
	OK   bool
	Text string
}

type ChoiceRow struct {
	// Cells are shown under Columns; the chosen option is shown after them.
	Cells []string
	// Detail is shown below the table while the row is focused.
	Detail string
	// Options are what the row can be set to. Current is preselected and
	// must be one of them.
	Options []string
	Current string
}

type ModelProgress struct {
	ID         string
	Bytes      int64
	TotalBytes int64
	Files      int
	TotalFiles int
	Percent    int
	Done       bool
}

type Installation int

const (
	Update Installation = iota
	Install
)

func (i Installation) String() string {
	switch i {
	case Update:
		return "Update"
	case Install:
		return "Install"
	}
	return "unknown"
}

type RecoveryAction int

const (
	RecoveryFail RecoveryAction = iota
	RecoveryRetryStep
	RecoveryRestartStage
	RecoveryContinue
)
