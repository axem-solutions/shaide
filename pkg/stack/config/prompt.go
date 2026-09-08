package config

type PromptKind int

const (
	PromptInput PromptKind = iota
	PromptSelect
	PromptMultiSelect
)

type Prompt struct {
	Kind        PromptKind
	Title       string
	Placeholder string
	Options     []string
}

type Prompter interface {
	Input(title, placeholder, defaultValue string) (string, error)
	Select(title, current string, options []string) (string, error)
	MultiSelect(title string, options []string) ([]string, error)
}

// SecretPrompter is an optional interface implemented by prompters that can
// mask secret input. Resolve falls back to Input when it is unavailable.
type SecretPrompter interface {
	SecretInput(title, placeholder, defaultValue string) (string, error)
}
