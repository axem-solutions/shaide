package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

func TestApplyDefaults(t *testing.T) {
	configuration := New("/var/lib/shaide/monitoring", stack.Options{}, Sources{})
	values := Values{Platform: cluster.OnPrem}

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
	configuration := New(".", stack.Options{}, Sources{})
	values := Values{Platform: cluster.OnPrem}
	values.Loki.Version = "99.1.2"

	configuration.applyDefaults(&values)
	configuration.resolveChartPaths(&values)

	if values.Loki.ChartPath != "charts/loki-99.1.2.tgz" {
		t.Errorf("Loki chart path = %q, want version-derived path", values.Loki.ChartPath)
	}
}

func TestValidate(t *testing.T) {
	configuration := New("", stack.Options{}, Sources{})
	valid := Values{Platform: cluster.OnPrem}
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
			name: "invalid provider",
			mutate: func(values *Values) {
				values.Platform = "unsupported"
			},
			wantErr: "invalid provider",
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
		Platform:   cluster.Azure,
		Kubeconfig: "/tmp/kubeconfig",
		Context:    "cluster-context",
	}, Sources{})

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

// The storage classes come from the installer, which picks them from the
// cluster or reads them off the existing PVCs; the stack must not ask again.
func TestStorageClassesComeFromTheInstaller(t *testing.T) {
	configuration := New("", stack.Options{Platform: cluster.Azure, Kubeconfig: "/.kube/config"}, Sources{
		LokiStorageClass:       "managed-csi",
		PrometheusStorageClass: "default",
	})

	for _, entry := range configuration.definition.Entries {
		if (entry.Key == KeyLokiStorageClass || entry.Key == KeyPrometheusStorageClass) && entry.Prompt != nil {
			t.Errorf("%s is prompted; the installer resolves it", entry.Key)
		}
	}

	resolved, err := configuration.definition.Resolve(&noPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	for key, want := range map[string]string{
		"monitoring:lokiStorageClass":       "managed-csi",
		"monitoring:prometheusStorageClass": "default",
	} {
		if got := resolved[key].Value; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// An empty class is left unwritten, so the chart uses the cluster default and
// a value set by hand in the stack config survives. This holds on every
// platform: there is no longer a hardcoded on-prem class.
func TestEmptyStorageClassIsNotWritten(t *testing.T) {
	for _, target := range []cluster.Provider{cluster.Azure, cluster.AWS, cluster.GCP, cluster.OnPrem} {
		t.Run(string(target), func(t *testing.T) {
			configuration := New("", stack.Options{Platform: target, Kubeconfig: "/.kube/config"}, Sources{})

			resolved, err := configuration.definition.Resolve(&noPrompter{})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			for _, key := range []string{"monitoring:lokiStorageClass", "monitoring:prometheusStorageClass"} {
				if value, ok := resolved[key]; ok {
					t.Errorf("%s = %q was written; want it left to the cluster default", key, value.Value)
				}
			}
		})
	}
}

// noPrompter answers the prompts the monitoring stack still has, so Resolve can
// run without a terminal.
type noPrompter struct{}

func (*noPrompter) Input(string, string, string) (string, error) {
	return "secret", nil
}

func (*noPrompter) Select(_, current string, _ []string) (string, error) {
	return current, nil
}

func (*noPrompter) MultiSelect(_ string, options []string) ([]string, error) {
	return options, nil
}

func cloneComponents(components map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(components))
	for component, enabled := range components {
		clone[component] = enabled
	}
	return clone
}
