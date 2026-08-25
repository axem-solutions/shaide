package stacks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStackConfigFile(t *testing.T) {
	got := stackConfigFile("/bundle/deployments/app-shaide", "shaide")
	want := "/bundle/deployments/app-shaide/Pulumi.shaide.yaml"
	if got != want {
		t.Errorf("stackConfigFile() = %q, want %q", got, want)
	}
}

func TestStackConfigString(t *testing.T) {
	dir := t.TempDir()

	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	withHarbor := write("Pulumi.harbor.yaml", `config:
  app-shaide:namespace: app-shaide
  app-shaide:harborHostname: harbor.harbor.svc.cluster.local
  app-shaide:lbAnnotations:
    some/annotation: value
`)

	withoutHarbor := write("Pulumi.ghcr.yaml", `config:
  app-shaide:namespace: app-shaide
  app-shaide:ghcrUser: axem-bot
`)

	noConfig := write("Pulumi.empty.yaml", "name: app-shaide\n")

	tests := []struct {
		name string
		path string
		key  string
		want string
	}{
		{"scalar value", withHarbor, "app-shaide:harborHostname", "harbor.harbor.svc.cluster.local"},
		{"key absent", withoutHarbor, "app-shaide:harborHostname", ""},
		{"no config block", noConfig, "app-shaide:harborHostname", ""},
		{"file absent", filepath.Join(dir, "nope.yaml"), "app-shaide:harborHostname", ""},
		// lbAnnotations is a mapping, not a scalar: callers asking for a string
		// must get "" rather than a serialised map.
		{"non-scalar value", withHarbor, "app-shaide:lbAnnotations", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stackConfigString(tt.path, tt.key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("stackConfigString(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}

	t.Run("malformed yaml is an error", func(t *testing.T) {
		bad := write("Pulumi.bad.yaml", "config:\n  key: [unclosed\n")
		if _, err := stackConfigString(bad, "key"); err == nil {
			t.Error("expected an error for malformed yaml, got nil")
		}
	})
}
