package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func TestApplyDefaults(t *testing.T) {
	configuration := New("/projects/app-serving", stack.Options{}, Sources{})
	values := Values{ModelStorageClass: "managed-csi"}
	values.Models.Generative = []Model{
		{Enabled: true, ModelSource: &ModelSource{}},
	}

	configuration.applyDefaults(&values)

	if values.ModelServerImage != DefaultModelServerImage || values.RouterImage != DefaultRouterImage ||
		values.SimulatorImage != DefaultSimulatorImage {
		t.Errorf("images = %q, %q, %q, want the upstream defaults", values.ModelServerImage, values.RouterImage, values.SimulatorImage)
	}
	if got := values.Models.Generative[0].ModelSource.StorageClass; got != "managed-csi" {
		t.Errorf("model storage class = %q, want %q", got, "managed-csi")
	}
}

func TestResolveResolvesPathsAndModelDefaults(t *testing.T) {
	projectDir := t.TempDir()
	writeDefinition(t, projectDir, CategoryGenerative, "ExampleModel", "example")

	configuration := New(projectDir, stack.Options{}, Sources{})
	values := Values{
		Platform: cluster.GCP,
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
	if model.Definition.Model.Name != "org/example" {
		t.Errorf("definition = %+v, want the packaged model.yaml", model.Definition)
	}
	if want := filepath.Join(projectDir, DefaultRouterChartPath); values.RouterChartPath != want {
		t.Errorf("router chart path = %q, want %q", values.RouterChartPath, want)
	}
}

func TestValidate(t *testing.T) {
	projectDir := t.TempDir()
	definitionPath := writeDefinition(t, projectDir, CategoryGenerative, "example", "example")

	configuration := New("", stack.Options{}, Sources{})
	valid := Values{
		Platform:         cluster.GCP,
		RouterChartPath:  DefaultRouterChartPath,
		ModelServerImage: DefaultModelServerImage,
		RouterImage:      DefaultRouterImage,
		SimulatorImage:   DefaultSimulatorImage,
	}
	valid.Models.Generative = []Model{{
		Name:        "example",
		Enabled:     true,
		Namespace:   "llm-d-example",
		ReleaseName: "infra-example",
		ModelPaths: ModelPaths{
			Slug:           "example",
			DefinitionPath: definitionPath,
		},
	}}

	tests := []struct {
		name    string
		mutate  func(*Values)
		wantErr string
	}{
		{name: "valid cloud config"},
		{
			name: "invalid provider",
			mutate: func(values *Values) {
				values.Platform = "unsupported"
			},
			wantErr: "invalid provider",
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
				values.RouterChartPath = ""
			},
			wantErr: "router chart path cannot be empty",
		},
		{
			name: "missing definition",
			mutate: func(values *Values) {
				values.Models.Generative[0].DefinitionPath = filepath.Join(projectDir, "missing.yaml")
			},
			wantErr: "model definition not found",
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
				values.Platform = cluster.OnPrem
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
		Platform:   cluster.Azure,
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
		"app-serving:modelServerImage":  DefaultModelServerImage,
		"app-serving:routerImage":       DefaultRouterImage,
		"app-serving:simulatorImage":    DefaultSimulatorImage,
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
	want := filepath.Join(projectDir, "charts", "llm-d-router-gateway")
	if got := resolveProjectPath(projectDir, "charts/llm-d-router-gateway"); got != want {
		t.Errorf("resolveProjectPath() = %q, want %q", got, want)
	}

	absolute := "/opt/charts/llm-d-router-gateway"
	if got := resolveProjectPath(projectDir, absolute); got != absolute {
		t.Errorf("absolute path = %q, want %q", got, absolute)
	}
}

// writeDefinition packages a minimal model.yaml for name under category and
// returns its path.
func writeDefinition(t *testing.T, projectDir, category, name, slug string) string {
	t.Helper()

	dir := filepath.Join(projectDir, deploymentFolder, modelFolder, category, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create test model directory: %v", err)
	}

	path := filepath.Join(dir, "model.yaml")
	content := "slug: " + slug + "\nmodel:\n  name: org/" + slug + "\n  size: 1Gi\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test model definition: %v", err)
	}
	return path
}

// TestDefaultRouterChartPathStaysInsideProject guards against a default that
// escapes the project directory. The chart ships inside the project's charts/
// folder, so resolveProjectPath must be able to anchor it there; a "../" or
// absolute default resolves to a path the installer image never populates.
func TestDefaultRouterChartPathStaysInsideProject(t *testing.T) {
	if filepath.IsAbs(DefaultRouterChartPath) {
		t.Fatalf("DefaultRouterChartPath = %q, want a relative path", DefaultRouterChartPath)
	}

	projectDir := "/projects/app-serving"
	resolved := resolveProjectPath(projectDir, DefaultRouterChartPath)
	if !strings.HasPrefix(resolved, projectDir+string(filepath.Separator)) {
		t.Errorf("resolveProjectPath(%q, %q) = %q, want a path inside the project directory",
			projectDir, DefaultRouterChartPath, resolved)
	}
}

// The vendored chart is what every platform deploys; the default must point
// at it.
func TestDefaultRouterChartIsVendored(t *testing.T) {
	chart := filepath.Join("..", "..", "..", "..", "..", "app_serving", DefaultRouterChartPath, "Chart.yaml")
	if _, err := os.Stat(chart); err != nil {
		t.Fatalf("vendored router chart missing: %v", err)
	}
}
