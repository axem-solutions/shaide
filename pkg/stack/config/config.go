package config

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

type Definition interface {
	Key(Key) string
	Validate() error
	Resolve(Prompter) (auto.ConfigMap, error)
}

type Config[T any] struct {
	Namespace string
	Entries   []Entry[T]
}

func (c Config[T]) Key(key Key) string {
	return c.Namespace + ":" + key.String()
}

func (c Config[T]) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("stack config namespace cannot be empty")
	}

	seen := make(map[Key]struct{}, len(c.Entries))

	for _, entry := range c.Entries {
		if err := entry.validate(); err != nil {
			return err
		}

		if _, exists := seen[entry.Key]; exists {
			return fmt.Errorf("duplicate stack config entry %q", entry.Key)
		}

		if entry.Policy.When != nil {
			for _, dependency := range entry.Policy.When.Dependencies() {
				if _, exists := seen[dependency]; !exists {
					return fmt.Errorf("stack config entry %q depends on %q, which must be declared earlier", entry.Key, dependency)
				}
			}
		}

		seen[entry.Key] = struct{}{}
	}

	return nil
}
