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
