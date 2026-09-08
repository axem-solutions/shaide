package config

import (
	"fmt"

	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type Setter[T any] func(*T, *pulumiconfig.Config)

type Entry[T any] struct {
	Key    Key
	Source Source
	Prompt *Prompt
	Policy Policy
	Setter Setter[T]
}

func (e Entry[T]) validate() error {
	if e.Key == "" {
		return fmt.Errorf("stack config entry name cannot be empty")
	}

	if e.Prompt == nil {
		return nil
	}

	if e.Prompt.Title == "" {
		return fmt.Errorf("stack config entry %q has an empty prompt title", e.Key)
	}

	switch e.Prompt.Kind {
	case PromptInput:
	case PromptSelect, PromptMultiSelect:
		if len(e.Prompt.Options) == 0 {
			return fmt.Errorf("stack config entry %q requires prompt options", e.Key)
		}
	default:
		return fmt.Errorf("stack config entry %q has invalid prompt kind %d", e.Key, e.Prompt.Kind)
	}

	return nil
}

type Key string

func (k Key) String() string {
	return string(k)
}

type Source struct {
	Value   any
	Default any
}

func (e Entry[T]) resolve(p Prompter) (any, bool, error) {
	if e.Source.Value != nil {
		return e.Source.Value, true, nil
	}

	if e.Prompt != nil {
		if p == nil {
			return nil, false, fmt.Errorf("prompt is required but no prompter was provided")
		}

		value, err := e.resolvePrompt(p)
		if err != nil {
			return nil, false, err
		}

		return value, true, nil
	}

	if e.Source.Default != nil {
		return e.Source.Default, true, nil
	}

	return nil, false, nil
}

func (e Entry[T]) resolvePrompt(p Prompter) (any, error) {
	defaultValue := ""
	if e.Source.Default != nil {
		encoded, err := encode(e.Source.Default)
		if err != nil {
			return nil, fmt.Errorf("encode default value: %w", err)
		}
		defaultValue = encoded
	}

	switch e.Prompt.Kind {
	case PromptInput:
		if e.Policy.Secret {
			if secretPrompter, ok := p.(SecretPrompter); ok {
				return secretPrompter.SecretInput(e.Prompt.Title, e.Prompt.Placeholder, defaultValue)
			}
		}

		return p.Input(e.Prompt.Title, e.Prompt.Placeholder, defaultValue)
	case PromptSelect:
		return p.Select(e.Prompt.Title, defaultValue, e.Prompt.Options)
	case PromptMultiSelect:
		return p.MultiSelect(e.Prompt.Title, e.Prompt.Options)
	default:
		return nil, fmt.Errorf("invalid prompt kind %d", e.Prompt.Kind)
	}
}
