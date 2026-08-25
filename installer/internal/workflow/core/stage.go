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
	ProgressModel(progress ModelProgress)
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
