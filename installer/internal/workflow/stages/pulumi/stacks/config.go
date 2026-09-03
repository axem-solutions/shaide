package stacks

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/iac/decoder"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func resolveConfigKey(reporter core.Reporter, key decoder.ConfigKeySpec) (auto.ConfigValue, error) {
	switch {
	case len(key.Options) > 0 && key.Prompt:
		value, err := reporter.Select(key.Description, key.Default, key.Options)
		if err != nil {
			return auto.ConfigValue{}, err
		}
		value = strings.TrimSpace(value)
		if key.Required && value == "" {
			return auto.ConfigValue{}, fmt.Errorf("config value is required")
		}
		if value != "" && !slices.Contains(key.Options, value) {
			return auto.ConfigValue{}, fmt.Errorf("config value %q is not one of the allowed options", value)
		}
		return auto.ConfigValue{Value: value}, nil

	case len(key.Options) > 0:
		values, err := reporter.MultiSelect(key.Description, key.Options)
		if err != nil {
			return auto.ConfigValue{}, err
		}
		if key.Required && len(values) == 0 {
			return auto.ConfigValue{}, fmt.Errorf("at least one config value is required")
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
		if key.Required && strings.TrimSpace(value) == "" {
			return auto.ConfigValue{}, fmt.Errorf("config value is required")
		}
		return auto.ConfigValue{Value: value, Secret: true}, nil

	case key.Prompt || key.Required:
		value, err := reporter.Input(key.Description, key.Default, key.Default)
		if err != nil {
			return auto.ConfigValue{}, err
		}
		value = strings.TrimSpace(value)
		if key.Required && value == "" {
			return auto.ConfigValue{}, fmt.Errorf("config value is required")
		}
		return auto.ConfigValue{Value: value}, nil

	default:
		if key.Required && strings.TrimSpace(key.Default) == "" {
			return auto.ConfigValue{}, fmt.Errorf("config value is required")
		}
		return auto.ConfigValue{Value: key.Default}, nil
	}
}
