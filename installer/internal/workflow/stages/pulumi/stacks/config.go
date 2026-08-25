package stacks

import (
	"encoding/json"
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/iac/decoder"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func resolveConfigKey(reporter core.Reporter, key decoder.ConfigKeySpec) (auto.ConfigValue, error) {
	switch {
	case len(key.Options) > 0:
		values, err := reporter.MultiSelect(key.Description, key.Options)
		if err != nil {
			return auto.ConfigValue{}, err
		}

		encoded, err := json.Marshal(values)
		if err != nil {
			return auto.ConfigValue{}, fmt.Errorf("marshal selected config value: %w", err)
		}
		return auto.ConfigValue{Value: string(encoded)}, nil

	case key.Secret:
		value, err := reporter.Input(key.Description, "", "")
		if err != nil {
			return auto.ConfigValue{}, err
		}
		return auto.ConfigValue{Value: value, Secret: true}, nil

	case key.Prompt:
		value, err := reporter.Input(key.Description, key.Default, key.Default)
		if err != nil {
			return auto.ConfigValue{}, err
		}
		return auto.ConfigValue{Value: value}, nil

	default:
		return auto.ConfigValue{Value: key.Default}, nil
	}
}
