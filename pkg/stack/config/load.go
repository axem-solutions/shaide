package config

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// Load reads a Pulumi stack configuration into T using the setters declared
// by its entries.
func (c Config[T]) Load(ctx *pulumi.Context) (T, error) {
	var values T

	if err := c.Validate(); err != nil {
		return values, err
	}

	root := pulumiconfig.New(ctx, c.Namespace)
	for _, entry := range c.Entries {
		if entry.Setter == nil {
			continue
		}

		entry.Setter(&values, root)
	}

	return values, nil
}
