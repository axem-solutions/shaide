package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func TestApplyDefaults(t *testing.T) {
	configuration := New("/var/lib/shaide/monitoring", stack.Options{})
	values := Values{Platform: platform.OnPrem}

	configuration.applyDefaults(&values)
	configuration.resolveChartPaths(&values)

	if values.Namespace != DefaultNamespace {
		t.Errorf("namespace = %q, want %q", values.Namespace, DefaultNamespace)
	}
	if !values.Components[ComponentLoki] || len(values.Components) != 1 {
		t.Errorf("components = %#v, want only Loki enabled", values.Components)
	}
	if values.S3.Endpoint != DefaultS3Endpoint {
		t.Errorf("S3 endpoint = %q, want %q", values.S3.Endpoint, DefaultS3Endpoint)
	}
	if values.Loki.S3Bucket != DefaultS3BucketLoki {
		t.Errorf("Loki bucket = %q, want %q", values.Loki.S3Bucket, DefaultS3BucketLoki)
	}
	if values.Loki.S3ClientImage != DefaultS3ClientImage {
		t.Errorf("S3 client image = %q, want %q", values.Loki.S3ClientImage, DefaultS3ClientImage)
	}

	chartTests := []struct {
		name string
		got  string
		want string
	}{
		{"Loki", values.Loki.ChartPath, "charts/loki-" + DefaultLokiVersion + ".tgz"},
		{"Grafana", values.Grafana.ChartPath, "charts/grafana-" + DefaultGrafanaVersion + ".tgz"},
		{"Alloy", values.Alloy.ChartPath, "charts/alloy-" + DefaultAlloyVersion + ".tgz"},
		{"Prometheus", values.Prometheus.ChartPath, "charts/prometheus-" + DefaultPrometheusVersion + ".tgz"},
	}
	for _, tt := range chartTests {
		t.Run(tt.name, func(t *testing.T) {
			want := filepath.Join(configuration.ProjectDir(), tt.want)
			if tt.got != want {
				t.Errorf("chart path = %q, want %q", tt.got, want)
			}
		})
	}
}

func TestApplyDefaultsUsesConfiguredVersionsForChartPaths(t *testing.T) {
	configuration := New(".", stack.Options{})
	values := Values{Platform: platform.OnPrem}
	values.Loki.Version = "99.1.2"

	configuration.applyDefaults(&values)
	configuration.resolveChartPaths(&values)

	if values.Loki.ChartPath != "charts/loki-99.1.2.tgz" {
		t.Errorf("Loki chart path = %q, want version-derived path", values.Loki.ChartPath)
	}
}

func TestValidate(t *testing.T) {
	configuration := New("", stack.Options{})
	valid := Values{Platform: platform.OnPrem}
	configuration.applyDefaults(&valid)

	tests := []struct {
		name    string
		mutate  func(*Values)
		wantErr string
	}{
		{
			name: "valid defaults",
		},
		{
			name: "invalid platform",
			mutate: func(values *Values) {
				values.Platform = "unsupported"
			},
			wantErr: "invalid platform",
		},
		{
			name: "empty namespace",
			mutate: func(values *Values) {
				values.Namespace = ""
			},
			wantErr: "namespace cannot be empty",
		},
		{
			name: "no enabled components",
			mutate: func(values *Values) {
				values.Components = map[string]bool{ComponentLoki: false}
			},
			wantErr: "at least one monitoring component",
		},
		{
			name: "unknown component",
			mutate: func(values *Values) {
				values.Components = map[string]bool{"unknown": true}
			},
			wantErr: `unsupported monitoring component "unknown"`,
		},
		{
			name: "missing Loki setting",
			mutate: func(values *Values) {
				values.Loki.S3Bucket = ""
			},
			wantErr: "Loki S3 bucket cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := valid
			values.Components = cloneComponents(valid.Components)
			if tt.mutate != nil {
				tt.mutate(&values)
			}

			err := configuration.Validate(values)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestDefinition(t *testing.T) {
	configuration := New("/tmp/monitoring", stack.Options{
		Platform:   platform.Azure,
		Kubeconfig: "/tmp/kubeconfig",
		Context:    "cluster-context",
	})

	if configuration.ProjectName() != Namespace {
		t.Errorf("project name = %q, want %q", configuration.ProjectName(), Namespace)
	}
	if configuration.StackName() != Namespace {
		t.Errorf("stack name = %q, want %q", configuration.StackName(), Namespace)
	}
	if err := configuration.Definition().Validate(); err != nil {
		t.Fatalf("definition validation failed: %v", err)
	}
	if got := configuration.Definition().Key(KeyGrafanaAdminPassword); got != "monitoring:grafanaAdminPassword" {
		t.Errorf("Grafana password key = %q", got)
	}
}

func cloneComponents(components map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(components))
	for component, enabled := range components {
		clone[component] = enabled
	}
	return clone
}
