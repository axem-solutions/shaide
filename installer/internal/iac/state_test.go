package iac

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func noProgram(*pulumi.Context) error { return nil }

func localDeployer(t *testing.T, stateDir, workDir string, config auto.ConfigMap) *Deployer {
	t.Helper()
	deployer, err := NewDeployer(DeployerOptions{
		ProjectName: "state-test",
		StackName:   "test",
		WorkDir:     workDir,
		StateDir:    stateDir,
		Passphrase:  "test-passphrase",
		Config:      config,
		Logger:      io.Discard,
	})
	if err != nil {
		t.Fatalf("NewDeployer() error = %v", err)
	}
	return deployer
}

// The installer reuses secrets an earlier run stored, and must find out that
// a stack was never created without creating it.
func TestExistsAndStoredConfigUseTheLocalBackend(t *testing.T) {
	if _, err := exec.LookPath("pulumi"); err != nil {
		t.Skip("the pulumi CLI is not installed")
	}

	ctx := context.Background()
	stateDir := filepath.Join(t.TempDir(), "state")
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "Pulumi.yaml"), []byte("name: state-test\nruntime: go\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reader := localDeployer(t, stateDir, workDir, nil)
	if exists, err := reader.Exists(ctx, noProgram); err != nil || exists {
		t.Fatalf("Exists() = %v, %v before any run; want false", exists, err)
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Error("Exists() created the state directory")
	}

	// An earlier run wrote its config, a secret included.
	writer := localDeployer(t, stateDir, workDir, auto.ConfigMap{
		"state-test:token": {Value: "kept-secret", Secret: true},
		"state-test:plain": {Value: "kept"},
	})
	if _, err := writer.StateResources(ctx, noProgram); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if exists, err := reader.Exists(ctx, noProgram); err != nil || !exists {
		t.Fatalf("Exists() = %v, %v after a run; want true", exists, err)
	}

	stored, err := reader.StoredConfig(ctx, noProgram)
	if err != nil {
		t.Fatalf("StoredConfig() error = %v", err)
	}
	if got := stored["state-test:token"]; got.Value != "kept-secret" || !got.Secret {
		t.Errorf("stored secret = %+v, want it decrypted and marked secret", got)
	}
	if got := stored["state-test:plain"].Value; got != "kept" {
		t.Errorf("stored plain value = %q, want kept", got)
	}
}
