package mcp

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/internal/pulumitest"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	typeDeployment    = "kubernetes:apps/v1:Deployment"
	typeService       = "kubernetes:core/v1:Service"
	typeSecret        = "kubernetes:core/v1:Secret"
	typeNamespace     = "kubernetes:core/v1:Namespace"
	typeNetworkPolicy = "kubernetes:networking.k8s.io/v1:NetworkPolicy"
	typeRole          = "kubernetes:rbac.authorization.k8s.io/v1:Role"
	typeRoleBinding   = "kubernetes:rbac.authorization.k8s.io/v1:RoleBinding"

	kcMCPHarborImage = "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide_knowledge_center_mcp:v0.2.0"
)

type noPrompts struct{}

func (noPrompts) Input(title, _, _ string) (string, error) {
	return "", fmt.Errorf("unexpected prompt %q", title)
}
func (noPrompts) Select(title, _ string, _ []string) (string, error) {
	return "", fmt.Errorf("unexpected prompt %q", title)
}
func (noPrompts) MultiSelect(title string, _ []string) ([]string, error) {
	return nil, fmt.Errorf("unexpected prompt %q", title)
}

func generic() appconfig.Datasource {
	return appconfig.Datasource{Name: "jira", Image: "harbor/mcp-jira:v1", Port: 9000}
}

// kcOptions is what the installer passes for the single KC answer, plus any
// generic datasources.
func kcOptions(enabled bool, datasources ...appconfig.Datasource) Options {
	options := Options{
		KnowledgeCenterEnabled: &enabled,
		Datasources:            datasources,
		ShaideNamespace:        "app-shaide",
		ShaideSAName:           "shaide-server",
		Images:                 map[string]string{},
	}
	if enabled {
		options.Images[KnowledgeCenterMCPImage] = kcMCPHarborImage
		options.KnowledgeCenter = knowledgecentertest.Credentials()
	}
	return options
}

func deploy(t *testing.T, options Options, override map[string]string) (pulumitest.Result, error) {
	t.Helper()
	stack := NewStack("/projects/app-mcp", stackpkg.Options{Kubeconfig: "/.kube/config", Context: "test"}, options)
	resolved, err := stack.Config().Resolve(noPrompts{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	for key, value := range override {
		resolved[key] = auto.ConfigValue{Value: value}
	}
	return pulumitest.Run(t, "app-mcp", resolved, func(ctx *pulumi.Context) error {
		return stack.Deploy(ctx)
	})
}

func mustDeploy(t *testing.T, options Options) pulumitest.Result {
	t.Helper()
	result, err := deploy(t, options, nil)
	if err != nil {
		t.Fatalf("deploy error = %v", err)
	}
	return result
}

func stringMap(value any) map[string]string {
	out := map[string]string{}
	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			out[k], _ = v.(string)
		}
	}
	return out
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func kcContainer(t *testing.T, result pulumitest.Result) map[string]any {
	t.Helper()
	d := result.Must(t, typeDeployment, "kc-mcp")
	c := pulumitest.Named(pulumitest.List(d.Inputs, "spec.template.spec.containers"), "kc-mcp")
	if c == nil {
		t.Fatal("kc-mcp has no kc-mcp container")
	}
	return c
}

// A KC-only MCP stack: KC-MCP in app-mcp, from Harbor, with no generic
// datasource.
func TestKnowledgeCenterOnlyStack(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))

	ns := result.Must(t, typeNamespace, "app-mcp")
	if pulumitest.String(ns.Inputs, "metadata.name") != "app-mcp" {
		t.Errorf("namespace = %v", ns.Inputs)
	}
	d := result.Must(t, typeDeployment, "kc-mcp")
	if pulumitest.String(d.Inputs, "metadata.namespace") != "app-mcp" {
		t.Errorf("kc-mcp namespace = %q", pulumitest.String(d.Inputs, "metadata.namespace"))
	}
	if image := kcContainer(t, result)["image"]; image != kcMCPHarborImage {
		t.Errorf("kc-mcp image = %v", image)
	}
	for _, res := range result.OfType(typeDeployment) {
		if res.Name != "kc-mcp" {
			t.Errorf("unexpected Deployment %s in a KC-only stack", res.Name)
		}
	}

	svc := result.Must(t, typeService, "kc-mcp-svc")
	if pulumitest.String(svc.Inputs, "metadata.name") != "kc-mcp" || pulumitest.String(svc.Inputs, "spec.type") != "ClusterIP" {
		t.Errorf("kc-mcp Service = %v", svc.Inputs)
	}
	if port := pulumitest.Get(svc.Inputs, "spec.ports.0.port"); port != float64(8080) {
		t.Errorf("kc-mcp Service port = %v", port)
	}

	// shaide-server may watch pods here.
	binding := result.Must(t, typeRoleBinding, "shaide-mcp-pod-reader")
	if pulumitest.String(binding.Inputs, "metadata.namespace") != "app-mcp" ||
		pulumitest.String(binding.Inputs, "subjects.0.namespace") != "app-shaide" ||
		pulumitest.String(binding.Inputs, "subjects.0.name") != "shaide-server" {
		t.Errorf("pod-reader binding = %v", binding.Inputs)
	}
}

// shaide-server discovers KC-MCP by these pod annotations; the URL must be
// the origin the signed target names, or the server refuses to forward.
func TestKCMCPDiscovery(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))
	d := result.Must(t, typeDeployment, "kc-mcp")

	annotations := stringMap(pulumitest.Get(d.Inputs, "spec.template.metadata.annotations"))
	if annotations["mcp.shaide/datasource"] != "knowledge-center" {
		t.Errorf("mcp.shaide/datasource = %q, want the signed target's server_id", annotations["mcp.shaide/datasource"])
	}
	if annotations["mcp.shaide/url"] != "http://kc-mcp.app-mcp.svc.cluster.local:8080" {
		t.Errorf("mcp.shaide/url = %q", annotations["mcp.shaide/url"])
	}
	if !strings.Contains(knowledgecenter.ContextTargetsJSON(), `"url":"`+annotations["mcp.shaide/url"]+`"`) {
		t.Error("discovery URL differs from the signed target origin")
	}

	labels := stringMap(pulumitest.Get(d.Inputs, "spec.template.metadata.labels"))
	if labels["app.kubernetes.io/component"] != "mcp-server" || labels["app.kubernetes.io/name"] != "kc-mcp" {
		t.Errorf("kc-mcp pod labels = %v", labels)
	}
}

func TestKCMCPRuntimeContract(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))
	c := kcContainer(t, result)

	want := map[string]string{
		"HOST":                        "0.0.0.0",
		"PORT":                        "8080",
		"KNOWLEDGE_CENTER_URL":        "http://knowledge-center.app-shaide.svc.cluster.local:8080",
		"KC_CONTEXT_ISSUER":           knowledgecenter.Issuer,
		"KC_CONTEXT_TARGET":           "knowledge-center",
		"KC_CONTEXT_AUDIENCE":         "kc-mcp:knowledge-center",
		"KC_CONTEXT_KC_AUDIENCE":      "kc:knowledge-center",
		"KC_CONTEXT_KID":              "kc-context-v1",
		"KC_CONTEXT_PUBLIC_KEY_FILE":  "/run/secrets/kc-context/public.pem",
		"KC_MCP_GATEWAY_TOKEN_FILE":   "/run/secrets/kc-context/gateway_token",
		"KC_MCP_SCOPED_TOKEN_FILE":    "/run/secrets/kc-context/scoped_token",
		"KC_CONTEXT_ALLOW_LOCAL_HTTP": "true",
		"KC_MCP_ALLOWED_HOSTS":        "kc-mcp.app-mcp.svc.cluster.local",
	}
	if env := pulumitest.Env(c); !reflect.DeepEqual(env, want) {
		t.Errorf("kc-mcp env keys = %v\nwant %v", keys(env), keys(want))
		for k, v := range want {
			if env[k] != v {
				t.Errorf("kc-mcp %s = %q, want %q", k, env[k], v)
			}
		}
	}

	d := result.Must(t, typeDeployment, "kc-mcp")
	spec := pulumitest.Map(d.Inputs, "spec.template.spec")
	for path, want := range map[string]any{
		"securityContext.runAsUser":    float64(10001),
		"securityContext.runAsGroup":   float64(10001),
		"securityContext.fsGroup":      float64(10001),
		"securityContext.runAsNonRoot": true,
		"automountServiceAccountToken": false,
	} {
		if got := pulumitest.Get(spec, path); got != want {
			t.Errorf("kc-mcp %s = %v, want %v", path, got, want)
		}
	}
	if pulumitest.Get(c, "securityContext.readOnlyRootFilesystem") != true {
		t.Error("kc-mcp root filesystem is writable")
	}

	volume := pulumitest.Named(pulumitest.List(spec, "volumes"), "kc-context")
	if pulumitest.String(volume, "secret.secretName") != "kc-mcp-context" || pulumitest.Get(volume, "secret.defaultMode") != float64(0o440) {
		t.Errorf("kc-context volume = %v", volume)
	}
	mount := pulumitest.Named(pulumitest.List(c, "volumeMounts"), "kc-context")
	if pulumitest.String(mount, "mountPath") != "/run/secrets/kc-context" || mount["readOnly"] != true {
		t.Errorf("kc-context mount = %v", mount)
	}

	// Public key and hop tokens only: no signing key, no storage, no
	// internal token.
	secret := result.Must(t, typeSecret, "kc-mcp-context")
	files := stringMap(secret.Inputs["stringData"])
	if !reflect.DeepEqual(keys(files), []string{"gateway_token", "public.pem", "scoped_token"}) {
		t.Errorf("kc-mcp-context keys = %v", keys(files))
	}
	creds := knowledgecentertest.Credentials()
	if files["public.pem"] != creds.ContextPublicKey || files["gateway_token"] != creds.GatewayToken || files["scoped_token"] != creds.ScopedToken {
		t.Error("kc-mcp-context differs from the installer values")
	}
	if len(secret.Secrets) == 0 {
		t.Error("kc-mcp-context inputs are not marked secret")
	}
	for _, res := range result.Resources {
		for _, value := range stringMap(res.Inputs["stringData"]) {
			for _, forbidden := range []string{creds.ContextSigningKey, creds.InternalToken, creds.WebhookToken, creds.EmbeddingPassword} {
				if value == forbidden {
					t.Errorf("%s %s carries a credential KC-MCP must not hold", res.Type, res.Name)
				}
			}
		}
	}

	// No HTTP health route: TCP probes only.
	for _, probe := range []string{"startupProbe", "readinessProbe", "livenessProbe"} {
		if pulumitest.Get(c, probe+".tcpSocket.port") != float64(8080) || pulumitest.Get(c, probe+".httpGet") != nil {
			t.Errorf("kc-mcp %s = %v, want a TCP probe on 8080", probe, c[probe])
		}
	}
	limits := stringMap(pulumitest.Get(c, "resources.limits"))
	if limits["cpu"] == "" || limits["memory"] == "" {
		t.Errorf("kc-mcp limits = %v", limits)
	}
}

func TestKCMCPUsesCPUPlacement(t *testing.T) {
	options := kcOptions(true)
	options.Placement = []PlacementTerm{{Key: "kubernetes.azure.com/agentpool", Values: []string{"cpu"}}}
	result := mustDeploy(t, options)
	d := result.Must(t, typeDeployment, "kc-mcp")
	term := pulumitest.Get(d.Inputs, "spec.template.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.preference.matchExpressions.0")
	if pulumitest.String(term, "key") != "kubernetes.azure.com/agentpool" || !reflect.DeepEqual(pulumitest.List(term, "values"), []any{"cpu"}) {
		t.Errorf("kc-mcp node affinity = %v", term)
	}
}

func peerNames(rule any, field string) []string {
	var out []string
	for _, peer := range pulumitest.List(rule, field) {
		ns := stringMap(pulumitest.Get(peer, "namespaceSelector.matchLabels"))
		pods := stringMap(pulumitest.Get(peer, "podSelector.matchLabels"))
		out = append(out, ns["kubernetes.io/metadata.name"]+"/"+pods["app.kubernetes.io/name"])
	}
	sort.Strings(out)
	return out
}

func ports(rule any) []string {
	var out []string
	for _, p := range pulumitest.List(rule, "ports") {
		out = append(out, fmt.Sprintf("%s/%v", pulumitest.String(p, "protocol"), pulumitest.Get(p, "port")))
	}
	sort.Strings(out)
	return out
}

// KC-MCP accepts shaide-server and reaches only KC (and DNS). The namespace
// ingress policy stays, so generic MCP servers keep their access.
func TestKCMCPNetworkPolicy(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))
	result.Must(t, typeNetworkPolicy, "allow-shaide-ingress")

	policy := result.Must(t, typeNetworkPolicy, "kc-mcp-netpol")
	if got := stringMap(pulumitest.Get(policy.Inputs, "spec.podSelector.matchLabels")); got["app.kubernetes.io/name"] != "kc-mcp" {
		t.Errorf("kc-mcp policy selects %v", got)
	}
	ingress := pulumitest.List(policy.Inputs, "spec.ingress")
	if len(ingress) != 1 || !reflect.DeepEqual(peerNames(ingress[0], "from"), []string{"app-shaide/shaide-server"}) || !reflect.DeepEqual(ports(ingress[0]), []string{"TCP/8080"}) {
		t.Errorf("kc-mcp ingress = %v", ingress)
	}
	var egress []string
	dns := false
	for _, rule := range pulumitest.List(policy.Inputs, "spec.egress") {
		if len(pulumitest.List(rule, "to")) == 0 {
			dns = reflect.DeepEqual(ports(rule), []string{"TCP/53", "UDP/53"})
			continue
		}
		egress = append(egress, strings.Join(peerNames(rule, "to"), ",")+"@"+strings.Join(ports(rule), ","))
	}
	if !reflect.DeepEqual(egress, []string{"app-shaide/knowledge-center@TCP/8080"}) || !dns {
		t.Errorf("kc-mcp egress = %v dns=%t", egress, dns)
	}
}

// Generic datasources deploy exactly as before alongside KC-MCP.
func TestKnowledgeCenterPreservesGenericMCP(t *testing.T) {
	with := mustDeploy(t, kcOptions(true, generic()))
	without := mustDeploy(t, kcOptions(false, generic()))

	a := with.Must(t, typeDeployment, "mcp-jira")
	b := without.Must(t, typeDeployment, "mcp-jira")
	if !reflect.DeepEqual(a.Inputs, b.Inputs) {
		t.Error("mcp-jira Deployment changed when KC was enabled")
	}
	if _, ok := without.Find(typeDeployment, "kc-mcp"); ok {
		t.Error("kc-mcp deployed while KC is declined")
	}
	if _, ok := without.Find(typeSecret, "kc-mcp-context"); ok {
		t.Error("kc-mcp-context created while KC is declined")
	}
	if _, ok := without.Find(typeNetworkPolicy, "kc-mcp-netpol"); ok {
		t.Error("kc-mcp policy created while KC is declined")
	}
}

// A stack pinned to the legacy namespace is not moved: generic MCP keeps
// working there, and enabling KC fails until a deliberate migration.
func TestLegacyNamespaceIsNotMoved(t *testing.T) {
	result, err := deploy(t, kcOptions(false, generic()), map[string]string{"app-mcp:namespace": "mcp-gateway"})
	if err != nil {
		t.Fatalf("deploy error = %v", err)
	}
	if _, ok := result.Find(typeNamespace, "mcp-gateway"); !ok {
		t.Error("the pinned legacy namespace was not kept")
	}
	if _, ok := result.Find(typeNamespace, "app-mcp"); ok {
		t.Error("app-mcp created for a pinned legacy stack")
	}

	if _, err := deploy(t, kcOptions(true), map[string]string{"app-mcp:namespace": "mcp-gateway"}); err == nil || !strings.Contains(err.Error(), "mcp-gateway") {
		t.Fatalf("deploy error = %v, want the legacy namespace reported", err)
	}
}

// The role binding still lets shaide-server watch the namespace it
// discovers in.
func TestPodReaderRoleIsKept(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))
	role := result.Must(t, typeRole, "mcp-pod-reader")
	if pulumitest.String(role.Inputs, "metadata.namespace") != "app-mcp" {
		t.Errorf("role namespace = %q", pulumitest.String(role.Inputs, "metadata.namespace"))
	}
}
