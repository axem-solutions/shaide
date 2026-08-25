package stacks

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// stackConfigFile returns the path of a stack's config file inside a project
// directory, e.g. <workdir>/app-shaide/Pulumi.shaide.yaml.
func stackConfigFile(projectDir, stackName string) string {
	return filepath.Join(projectDir, fmt.Sprintf("Pulumi.%s.yaml", stackName))
}

// stackConfigString reads a scalar value from a stack config file's top-level
// config map. key is the fully qualified Pulumi key, e.g.
// "app-shaide:harborHostname".
//
// A missing file, a missing key, or a non-scalar value all yield an empty
// string, so callers can treat "not configured" uniformly. Only a malformed
// file is reported as an error.
func stackConfigString(path, key string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read stack config %s: %w", path, err)
	}

	var file struct {
		Config map[string]yaml.Node `yaml:"config"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return "", fmt.Errorf("parse stack config %s: %w", path, err)
	}

	node, ok := file.Config[key]
	if !ok || node.Kind != yaml.ScalarNode {
		return "", nil
	}

	return node.Value, nil
}
