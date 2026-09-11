package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func TestApplyDefaults(t *testing.T) {
	configuration := New("/projects/app-serving", stack.Options{}, Sources{})
	values := Values{ModelStorageClass: "managed-csi"}
	values.Models.Generative = []Model{
		{Enabled: true, ModelSource: &ModelSource{}},
	}

	configuration.applyDefaults(&values)

	if values.LLMd.ChartPath != DefaultLLMdChartPath {
		t.Errorf("llm-d chart path = %q, want %q", values.LLMd.ChartPath, DefaultLLMdChartPath)
	}
	if got := values.Models.Generative[0].ModelSource.StorageClass; got != "managed-csi" {
		t.Errorf("model storage class = %q, want %q", got, "managed-csi")
	}
}

func TestResolveResolvesPathsAndModelDefaults(t *testing.T) {
	projectDir := t.TempDir()
	modelDir := filepath.Join(projectDir, deploymentFolder, modelFolder, "generative", "ExampleModel")
	writeValuesFile(t, filepath.Join(modelDir, "gaie-example", "values.yaml"))
	writeValuesFile(t, filepath.Join(modelDir, "ms-example", "values.yaml"))

	configuration := New(projectDir, stack.Options{}, Sources{})
	values := Values{
		Platform: platform.GCP,
	}
	values.Models.Generative = []Model{{
		Name:    "ExampleModel",
		Enabled: true,
	}}
	configuration.applyDefaults(&values)

	if err := configuration.resolve(&values); err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if err := configuration.Validate(values); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if len(values.Models.Generative) != 1 {
		t.Fatalf("generative models = %d, want 1", len(values.Models.Generative))
	}
	model := values.Models.Generative[0]
	if model.Namespace != "llm-d-example" {
		t.Errorf("namespace = %q, want %q", model.Namespace, "llm-d-example")
	}
	if model.ReleaseName != "infra-example" {
		t.Errorf("release name = %q, want %q", model.ReleaseName, "infra-example")
	}
	if model.GaieLocalChartPath != filepath.Join(projectDir, DefaultGaieLocalChartPath) {
		t.Errorf("GAIE chart path = %q", model.GaieLocalChartPath)
	}
	wantLLMdPath := filepath.Clean(filepath.Join(projectDir, DefaultLLMdChartPath))
	if values.LLMd.ChartPath != wantLLMdPath {
		t.Errorf("llm-d chart path = %q, want %q", values.LLMd.ChartPath, wantLLMdPath)
	}
}

func TestValidate(t *testing.T) {
	projectDir := t.TempDir()
	gaieValuesPath := filepath.Join(projectDir, "gaie-values.yaml")
	modelServiceValuesPath := filepath.Join(projectDir, "model-service-values.yaml")
	writeValuesFile(t, gaieValuesPath)
	writeValuesFile(t, modelServiceValuesPath)

	configuration := New("", stack.Options{}, Sources{})
	valid := Values{
		Platform: platform.GCP,
	}
	valid.LLMd.ChartPath = DefaultLLMdChartPath
	valid.Models.Generative = []Model{{
		Name:        "example",
		Enabled:     true,
		Namespace:   "llm-d-example",
		ReleaseName: "infra-example",
		ModelPaths: ModelPaths{
			Slug:               "example",
			GaieValuesPath:     gaieValuesPath,
			MsValuesPath:       modelServiceValuesPath,
			GaieLocalChartPath: "charts/inferencepool",
		},
	}}

	tests := []struct {
		name    string
		mutate  func(*Values)
		wantErr string
	}{
		{name: "valid cloud config"},
		{
			name: "invalid platform",
			mutate: func(values *Values) {
				values.Platform = "unsupported"
			},
			wantErr: "invalid platform",
		},
		{
			name: "no enabled models",
			mutate: func(values *Values) {
				values.Models.Generative = nil
			},
			wantErr: "at least one model must be enabled",
		},
		{
			name: "empty chart path",
			mutate: func(values *Values) {
				values.LLMd.ChartPath = ""
			},
			wantErr: "chart path cannot be empty",
		},
		{
			name: "model source requires Harbor",
			mutate: func(values *Values) {
				values.Models.Generative[0].ModelSource = &ModelSource{}
			},
			wantErr: "harbor hostname is required",
		},
		{
			name: "on-prem requires kubeconfig",
			mutate: func(values *Values) {
				values.Platform = platform.OnPrem
				values.Harbor.Hostname = "harbor.internal.lan"
				values.Harbor.User = "robot$user"
				values.Harbor.TokenSet = true
			},
			wantErr: "kubeconfig is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := valid
			values.Models.Generative = append([]Model(nil), valid.Models.Generative...)
			if test.mutate != nil {
				test.mutate(&values)
			}

			err := configuration.Validate(values)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func TestDefinitionResolvesRuntimeSources(t *testing.T) {
	configuration := New("/projects/app-serving", stack.Options{
		Platform:   platform.Azure,
		Kubeconfig: "/tmp/kubeconfig",
		Context:    "aks-context",
	}, Sources{
		HarborUser:        "robot$user",
		HarborToken:       "secret-token",
		ModelStorageClass: "managed-csi",
	})

	if configuration.ProjectName() != Namespace {
		t.Errorf("project name = %q, want %q", configuration.ProjectName(), Namespace)
	}
	if configuration.StackName() != StackName {
		t.Errorf("stack name = %q, want %q", configuration.StackName(), StackName)
	}
	if err := configuration.Definition().Validate(); err != nil {
		t.Fatalf("definition validation failed: %v", err)
	}

	resolved, err := configuration.Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := map[string]string{
		"app-serving:platform":          "azure",
		"app-serving:kubeconfig":        "/tmp/kubeconfig",
		"app-serving:context":           "aks-context",
		"app-serving:llmdChart":         DefaultLLMdChartPath,
		"app-serving:harborUser":        "robot$user",
		"app-serving:harborToken":       "secret-token",
		"app-serving:modelStorageClass": "managed-csi",
	}
	for key, expected := range want {
		if got := resolved[key].Value; got != expected {
			t.Errorf("%s = %q, want %q", key, got, expected)
		}
	}

	if !resolved["app-serving:harborToken"].Secret {
		t.Error("harborToken was not marked secret")
	}
	if _, ok := resolved["app-serving:models"]; ok {
		t.Error("models should remain managed by the Pulumi stack config")
	}
	if _, ok := resolved["app-serving:harborHostname"]; ok {
		t.Error("empty Harbor hostname should not overwrite an existing stack value")
	}
}

func TestResolveProjectPath(t *testing.T) {
	projectDir := "/var/lib/shaide/app-serving"
	want := filepath.Join(projectDir, "charts", "inferencepool")
	if got := resolveProjectPath(projectDir, "charts/inferencepool"); got != want {
		t.Errorf("resolveProjectPath() = %q, want %q", got, want)
	}

	absolute := "/opt/charts/inferencepool"
	if got := resolveProjectPath(projectDir, absolute); got != absolute {
		t.Errorf("absolute path = %q, want %q", got, absolute)
	}
}

func writeValuesFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test model directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("model: test\n"), 0o600); err != nil {
		t.Fatalf("write test values: %v", err)
	}
}
