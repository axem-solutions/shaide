package stack

import (
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Options struct {
	Platform   platform.Platform
	Kubeconfig string
	Context    string
}

type Config struct {
	projectName string
	stackName   string
	projectDir  string
	definition  stackconfig.Definition
}

func NewConfig(projectName, stackName, projectDir string, definition stackconfig.Definition) Config {
	return Config{
		projectName: projectName,
		stackName:   stackName,
		projectDir:  projectDir,
		definition:  definition,
	}
}

func (c Config) ProjectName() string {
	return c.projectName
}

func (c Config) StackName() string {
	return c.stackName
}

func (c Config) ProjectDir() string {
	return c.projectDir
}

func (c Config) Definition() stackconfig.Definition {
	return c.definition
}

func (c Config) Resolve(prompter stackconfig.Prompter) (auto.ConfigMap, error) {
	return c.definition.Resolve(prompter)
}

type Stack interface {
	Deploy(*pulumi.Context) error
	Config() Config
}
