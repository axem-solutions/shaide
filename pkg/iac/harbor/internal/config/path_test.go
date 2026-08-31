package config

import (
	"path/filepath"
	"testing"
)

func TestResolveChartPath(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		chartPath string
		want      string
	}{
		{
			// The installer case: inline program, so the relative path from
			// Pulumi.harbor.yaml has to be anchored to the project directory.
			name:      "relative path is anchored to dir",
			dir:       "/var/shaide-installer/bundle/deployments/cloud-harbor",
			chartPath: "./charts/harbor-1.18.2.tgz",
			want:      "/var/shaide-installer/bundle/deployments/cloud-harbor/charts/harbor-1.18.2.tgz",
		},
		{
			// The standalone `pulumi up` case: dir is ".", so the result stays
			// relative to the working directory, as before.
			name:      "dot dir keeps the path relative",
			dir:       ".",
			chartPath: "./charts/harbor-1.18.2.tgz",
			want:      "charts/harbor-1.18.2.tgz",
		},
		{
			name:      "absolute chartPath is left alone",
			dir:       "/var/shaide-installer/bundle/deployments/cloud-harbor",
			chartPath: "/opt/charts/harbor-1.18.2.tgz",
			want:      "/opt/charts/harbor-1.18.2.tgz",
		},
		{
			// A chart name rather than a path (remote repo) must not be turned
			// into a filesystem path.
			name:      "empty dir leaves the value untouched",
			dir:       "",
			chartPath: "./charts/harbor-1.18.2.tgz",
			want:      "./charts/harbor-1.18.2.tgz",
		},
		{
			name:      "empty chartPath stays empty",
			dir:       "/somewhere",
			chartPath: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveProjectPath(tt.dir, tt.chartPath)
			if got != filepath.Clean(tt.want) && got != tt.want {
				t.Errorf("resolveChartPath(%q, %q) = %q, want %q",
					tt.dir, tt.chartPath, got, tt.want)
			}
		})
	}
}

func TestDefaultChartPathIsRelative(t *testing.T) {
	// The default has to stay relative so it goes through the same anchoring
	// as a configured value; an absolute default would bypass resolveChartPath.
	if filepath.IsAbs(DefaultChartPath) {
		t.Errorf("defaultChartPath = %q, want a relative path", DefaultChartPath)
	}
}

func TestDefaultChartPathIsAnchoredAfterDefaults(t *testing.T) {
	cfg := Config{}
	cfg.Storage.Mode = StorageModeHostPath

	if err := applyDefaults(&cfg); err != nil {
		t.Fatalf("applyDefaults() error = %v", err)
	}

	projectDir := "/var/shaide-installer/projects/harbor"
	cfg.Harbor.ChartPath = resolveProjectPath(projectDir, cfg.Harbor.ChartPath)

	want := filepath.Join(projectDir, "charts/harbor-1.18.2.tgz")
	if cfg.Harbor.ChartPath != want {
		t.Errorf("default chart path = %q, want %q", cfg.Harbor.ChartPath, want)
	}
}
