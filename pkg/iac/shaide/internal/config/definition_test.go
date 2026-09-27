package appconfig

import (
	"sort"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/stack"
)

type stubPrompter struct {
	asked []string
}

func (p *stubPrompter) Input(title, _, defaultValue string) (string, error) {
	p.asked = append(p.asked, title)
	return "answered-" + title, nil
}

func (p *stubPrompter) Select(title, current string, _ []string) (string, error) {
	p.asked = append(p.asked, title)
	return current, nil
}

func (p *stubPrompter) MultiSelect(title string, _ []string) ([]string, error) {
	p.asked = append(p.asked, title)
	return nil, nil
}

func resolve(t *testing.T, sources Sources) (map[string]string, map[string]bool, []string) {
	t.Helper()

	cfg := New("/projects/app-shaide", stack.Options{
		Platform:   cluster.Azure,
		Kubeconfig: "/.kube/config",
		Context:    "selected-cluster",
	}, sources)

	prompter := &stubPrompter{}
	resolved, err := cfg.Resolve(prompter)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	values := map[string]string{}
	secret := map[string]bool{}
	for key, value := range resolved {
		values[key] = value.Value
		secret[key] = value.Secret
	}

	sort.Strings(prompter.asked)

	return values, secret, prompter.asked
}

func completeSources() Sources {
	return Sources{
		GatewayHostname: "shaide.example.com",
		Images: map[string]string{
			ShaideServerImageName: "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide_server:v0.11.0",
			ControlPanelImageName: "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide_control_panel:v0.4.0",
			WebappImageName:       "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide-webapp:v0.1.1",
			RustfsImageName:       "harbor.harbor.svc.cluster.local/shaide/rustfs/rustfs:1.0.0-alpha.92",
			QdrantImageName:       "harbor.harbor.svc.cluster.local/shaide/qdrant/qdrant:v1.17",
			BusyboxImageName:      "harbor.harbor.svc.cluster.local/shaide/busybox:1.37",

			// Mirrored for another stack; app-shaide must ignore it.
			"istio/pilot": "harbor.harbor.svc.cluster.local/services/istio/pilot:1.28.1",
		},
	}
}

func TestDefinitionValidates(t *testing.T) {
	cfg := New("/projects/app-shaide", stack.Options{Platform: cluster.Azure}, completeSources())
	if err := cfg.Definition().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

// Only the secrets are asked for. Everything else is supplied by the installer
// or ships as a default describing how the components address each other.
func TestOnlySecretsArePrompted(t *testing.T) {
	_, _, asked := resolve(t, completeSources())

	want := []string{
		"Control panel session secret",
		"JWT signing secret",
		"Object storage (rustfs) password",
		"Shaide admin password",
	}

	if len(asked) != len(want) {
		t.Fatalf("asked %v, want %v", asked, want)
	}
	for i := range want {
		if asked[i] != want[i] {
			t.Fatalf("asked %v, want %v", asked, want)
		}
	}
}

func TestSecretsAreMarkedSecret(t *testing.T) {
	_, secret, _ := resolve(t, completeSources())

	for _, key := range []string{
		"app-shaide:adminAuthKey",
		"app-shaide:s3Password",
		"app-shaide:jwtSecret",
		"app-shaide:sessionSecret",
	} {
		if !secret[key] {
			t.Errorf("%s was written without the secret flag", key)
		}
	}
}

// The images the installer mirrored must reach the stack verbatim; a hand-typed
// tag is exactly what this removes.
func TestImagesComeFromTheInstaller(t *testing.T) {
	sources := completeSources()
	values, _, _ := resolve(t, sources)

	for key, want := range map[string]string{
		"app-shaide:shaideServerImage": sources.Images[ShaideServerImageName],
		"app-shaide:controlPanelImage": sources.Images[ControlPanelImageName],
		"app-shaide:webappImage":       sources.Images[WebappImageName],
		"app-shaide:rustfsImage":       sources.Images[RustfsImageName],
		"app-shaide:qdrantImage":       sources.Images[QdrantImageName],
		"app-shaide:busyboxImage":      sources.Images[BusyboxImageName],
	} {
		if values[key] != want {
			t.Errorf("%s = %q, want %q", key, values[key], want)
		}
	}
}

// Every registry the images come from allows anonymous pulls, so the stack
// carries no pull credentials.
func TestNoRegistryCredentialsAreWritten(t *testing.T) {
	values, _, _ := resolve(t, completeSources())

	for _, key := range []string{
		"app-shaide:harborHostname",
		"app-shaide:ghcrUser",
		"app-shaide:ghcrToken",
	} {
		if _, ok := values[key]; ok {
			t.Errorf("%s was written; app-shaide pulls its images anonymously", key)
		}
	}
}

// The provider must target the context the operator selected. Without it the
// kubeconfig's current-context wins, and the stack deploys to another cluster.
func TestSelectedContextIsWritten(t *testing.T) {
	values, _, _ := resolve(t, completeSources())

	if got := values["app-shaide:context"]; got != "selected-cluster" {
		t.Errorf("context = %q, want %q", got, "selected-cluster")
	}
}

// A required value the installer could not supply must fail at resolve time,
// not deep inside the Pulumi program with a bare "missing configuration".
func TestMissingImageIsRejected(t *testing.T) {
	sources := completeSources()
	delete(sources.Images, ShaideServerImageName)

	cfg := New("/projects/app-shaide", stack.Options{Platform: cluster.Azure}, sources)
	if _, err := cfg.Resolve(&stubPrompter{}); err == nil {
		t.Fatal("Resolve() succeeded without a shaide-server image")
	}
}

// Values with neither a source nor a prompt are left unwritten so a stack value
// set by hand survives the next run.
func TestOptionalEntriesAreNotWritten(t *testing.T) {
	values, _, _ := resolve(t, completeSources())

	for _, key := range []string{
		"app-shaide:infraStackRef",
		"app-shaide:nodeSelector",
		"app-shaide:storageClassName",
		"app-shaide:pvNodeHostname",
		"app-shaide:lbAnnotations",
		"app-shaide:serviceAccountAnnotations",
	} {
		if _, ok := values[key]; ok {
			t.Errorf("%s was written; it should be left to the program default or a hand-set value", key)
		}
	}
}

// The service names describe how components address each other in-cluster and
// are identical in every deployment, so they ship rather than being asked for.
func TestServiceDefaultsAreWritten(t *testing.T) {
	values, _, _ := resolve(t, completeSources())

	for key, want := range map[string]string{
		"app-shaide:namespace":           DefaultNamespace,
		"app-shaide:controlPanelService": DefaultControlPanelService,
		"app-shaide:qdrantService":       DefaultQdrantService,
		"app-shaide:vectorDBUrl":         DefaultVectorDBURL,
		"app-shaide:mcpNamespace":        DefaultMCPNamespace,
		"app-shaide:shaideServerUiPort":  DefaultShaideServerUiPort,
	} {
		if values[key] != want {
			t.Errorf("%s = %q, want %q", key, values[key], want)
		}
	}
}
