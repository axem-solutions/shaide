package stacks

import (
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/iac/decoder"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

type templateConfigReporter struct {
	selectValue string
	multiValues []string
	inputValue  string
}

func (r *templateConfigReporter) Select(string, string, []string) (string, error) {
	return r.selectValue, nil
}

func (r *templateConfigReporter) MultiSelect(string, []string) ([]string, error) {
	return r.multiValues, nil
}

func (r *templateConfigReporter) Input(string, string, string) (string, error) {
	return r.inputValue, nil
}

func (*templateConfigReporter) ProgressModel(core.ModelProgress) {}

func TestResolveConfigKeyUsesSingleSelectForPromptedOptions(t *testing.T) {
	reporter := &templateConfigReporter{selectValue: "azure"}

	got, err := resolveConfigKey(reporter, decoder.ConfigKeySpec{
		Description: "Cloud platform",
		Default:     "on-prem",
		Options:     []string{"gcp", "aws", "azure", "on-prem"},
		Prompt:      true,
		Required:    true,
	})
	if err != nil {
		t.Fatalf("resolveConfigKey() error = %v", err)
	}
	if got.Value != "azure" {
		t.Fatalf("resolveConfigKey() value = %q, want azure", got.Value)
	}
}

func TestResolveConfigKeyKeepsUnpromptedOptionsAsMultiSelect(t *testing.T) {
	reporter := &templateConfigReporter{multiValues: []string{"loki", "grafana"}}

	got, err := resolveConfigKey(reporter, decoder.ConfigKeySpec{
		Description: "Components",
		Options:     []string{"loki", "grafana"},
	})
	if err != nil {
		t.Fatalf("resolveConfigKey() error = %v", err)
	}
	if got.Value != `["loki","grafana"]` {
		t.Fatalf("resolveConfigKey() value = %q", got.Value)
	}
}

func TestResolveConfigKeyRejectsMissingRequiredValue(t *testing.T) {
	reporter := &templateConfigReporter{}

	_, err := resolveConfigKey(reporter, decoder.ConfigKeySpec{
		Description: "Gateway hostname",
		Prompt:      true,
		Required:    true,
	})
	if err == nil {
		t.Fatal("resolveConfigKey() error = nil, want required-value error")
	}
}
