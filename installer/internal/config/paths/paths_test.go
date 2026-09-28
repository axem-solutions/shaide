package paths

import "testing"

func TestDescribe(t *testing.T) {
	p := NewPaths("/var/shaide-installer")

	tests := map[string]string{
		// Under the mount: relative, as the operator finds it on the host.
		p.Logs + "/installer-logs-20260927-165610.log": "logs/installer-logs-20260927-165610.log in the storage directory",
		p.ModelManifestPath:                            "manifests/models.yaml in the storage directory",
		// Outside the mount, e.g. MODEL_MANIFEST_PATH: unchanged.
		"/manifests/models.yaml": "/manifests/models.yaml",
		// The root itself, and a sibling sharing its prefix, are not under it.
		"/var/shaide-installer":       "/var/shaide-installer",
		"/var/shaide-installer-other": "/var/shaide-installer-other",
	}

	for path, want := range tests {
		if got := p.Describe(path); got != want {
			t.Errorf("Describe(%q) = %q, want %q", path, got, want)
		}
	}
}
