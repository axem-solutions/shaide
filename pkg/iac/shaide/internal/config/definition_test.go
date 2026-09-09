package appconfig

import (
	"sort"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
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
		Platform:   platform.Azure,
		Kubeconfig: "/.kube/config",
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
		HarborHostname:    "harbor.harbor.svc.cluster.local",
		RegistryUser:      "robot$k8s-harbor-sa",
		RegistryToken:     "robot-password",
		GatewayHostname:   "shaide.example.com",
		ShaideServerImage: "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide_server:v0.11.0",
		ControlPanelImage: "harbor.harbor.svc.cluster.local/shaide/axem-solutions/control_panel:v0.4.0",
		WebappImage:       "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide-webapp:v0.1.1",
		RustfsImage:       "harbor.harbor.svc.cluster.local/shaide/rustfs/rustfs:1.0.0-alpha.92",
		QdrantImage:       "harbor.harbor.svc.cluster.local/shaide/qdrant/qdrant:v1.17",
		BusyboxImage:      "harbor.harbor.svc.cluster.local/shaide/busybox:1.37",
	}
}

func TestDefinitionValidates(t *testing.T) {
	cfg := New("/projects/app-shaide", stack.Options{Platform: platform.Azure}, completeSources())
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
		"app-shaide:ghcrToken",
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
		"app-shaide:shaideServerImage": sources.ShaideServerImage,
		"app-shaide:controlPanelImage": sources.ControlPanelImage,
		"app-shaide:webappImage":       sources.WebappImage,
		"app-shaide:rustfsImage":       sources.RustfsImage,
		"app-shaide:qdrantImage":       sources.QdrantImage,
		"app-shaide:busyboxImage":      sources.BusyboxImage,
	} {
		if values[key] != want {
			t.Errorf("%s = %q, want %q", key, values[key], want)
		}
	}

	if got := values["app-shaide:harborHostname"]; got != sources.HarborHostname {
		t.Errorf("harborHostname = %q, want %q", got, sources.HarborHostname)
	}
	if got := values["app-shaide:ghcrUser"]; got != sources.RegistryUser {
		t.Errorf("ghcrUser = %q, want the Harbor robot %q", got, sources.RegistryUser)
	}
}

// A required value the installer could not supply must fail at resolve time,
// not deep inside the Pulumi program with a bare "missing configuration".
func TestMissingImageIsRejected(t *testing.T) {
	sources := completeSources()
	sources.ShaideServerImage = ""

	cfg := New("/projects/app-shaide", stack.Options{Platform: platform.Azure}, sources)
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
