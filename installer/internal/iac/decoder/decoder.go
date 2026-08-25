package decoder

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

type ConfigKeySpec struct {
	Description string   `yaml:"description"`
	Default     string   `yaml:"default"`
	Options     []string `yaml:"options"`
	Secret      bool     `yaml:"secret"`
	Prompt      bool     `yaml:"prompt"`
}

type ConfigKey struct {
	Name string
	Key  ConfigKeySpec
}

type Template struct {
	Name        string `yaml:"name"`
	Runtime     string `yaml:"runtime"`
	Main        string `yaml:"main"`
	Description string `yaml:"description"`
	Template    struct {
		Description string    `yaml:"description"`
		Config      yaml.Node `yaml:"config"`
	} `yaml:"template"`
}

func LoadTemplateFile(path string) (*Template, []ConfigKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading template file: %w", err)
	}

	var temp Template
	if err := yaml.Unmarshal(data, &temp); err != nil {
		return nil, nil, fmt.Errorf("parsing template yaml: %w", err)
	}

	node := temp.Template.Config
	if node.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("template.config is not a mapping")
	}

	keys := make([]ConfigKey, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		nameNode, valNode := node.Content[i], node.Content[i+1]

		var ck ConfigKeySpec
		if err := valNode.Decode(&ck); err != nil {
			return nil, nil, fmt.Errorf("decoding config key %q: %w", nameNode.Value, err)
		}
		keys = append(keys, ConfigKey{Name: nameNode.Value, Key: ck})
	}

	return &temp, keys, nil
}
