package shaide

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/internal/pulumitest"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	typeDeployment    = "kubernetes:apps/v1:Deployment"
	typeStatefulSet   = "kubernetes:apps/v1:StatefulSet"
	typeService       = "kubernetes:core/v1:Service"
	typeSecret        = "kubernetes:core/v1:Secret"
	typeConfigMap     = "kubernetes:core/v1:ConfigMap"
	typeNetworkPolicy = "kubernetes:networking.k8s.io/v1:NetworkPolicy"

	kcHarborImage = "harbor.harbor.svc.cluster.local/shaide/axem-solutions/shaide_knowledge_center:v0.2.0"
)

// kcOptions are the installer's options for one answer to the Knowledge
// Center question.
func kcOptions(enabled bool) Options {
	images := testImages()
	options := Options{KnowledgeCenterEnabled: &enabled, Images: images}
	if enabled {
		images[KnowledgeCenterImage] = kcHarborImage
		options.KnowledgeCenter = knowledgecentertest.Credentials()
	}
	return options
}

// deploy resolves the installer options exactly as the installer would, then
// runs the program against mocks with the resolved config.
func deploy(t *testing.T, options Options, override map[string]string) (pulumitest.Result, error) {
	t.Helper()
	stack := newTestStack(options)
	resolved, err := stack.Config().Resolve(answeringPrompter{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	// The generic provider adds no cloud resources.
	resolved["app-shaide:cloudProvider"] = auto.ConfigValue{Value: "generic"}
	for key, value := range override {
		resolved[key] = auto.ConfigValue{Value: value}
	}
	return pulumitest.Run(t, "app-shaide", resolved, func(ctx *pulumi.Context) error {
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

func container(t *testing.T, res pulumitest.Resource, name string) map[string]any {
	t.Helper()
	c := pulumitest.Named(pulumitest.List(res.Inputs, "spec.template.spec.containers"), name)
	if c == nil {
		t.Fatalf("%s has no container %q", res.Name, name)
	}
	return c
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

// --- Disabled: core platform only --------------------------------------

func TestDisabledDeploysNoKnowledgeCenter(t *testing.T) {
	result := mustDeploy(t, kcOptions(false))

	if _, ok := result.Find(typeDeployment, "knowledge-center"); ok {
		t.Error("KC Deployment created while declined")
	}
	if _, ok := result.Find(typeService, "knowledge-center-svc"); ok {
		t.Error("KC Service created while declined")
	}
	for _, name := range []string{"knowledge-center", "knowledge-center-context", "kc-context-signer"} {
		if _, ok := result.Find(typeSecret, name); ok {
			t.Errorf("Secret %s created while declined", name)
		}
	}
	if policies := result.OfType(typeNetworkPolicy); len(policies) != 0 {
		t.Errorf("NetworkPolicies created while declined: %d", len(policies))
	}

	cp := result.Must(t, typeDeployment, "control-panel")
	if env := pulumitest.Env(container(t, cp, "control-panel")); env["KNOWLEDGE_CENTER_ENABLED"] != "false" {
		t.Errorf("KNOWLEDGE_CENTER_ENABLED = %q, want false", env["KNOWLEDGE_CENTER_ENABLED"])
	}

	server := result.Must(t, typeStatefulSet, "shaide-server")
	if volumes := pulumitest.List(server.Inputs, "spec.template.spec.volumes"); len(volumes) != 0 {
		t.Errorf("shaide-server mounts %v while KC is declined", volumes)
	}

	data := stringMap(result.Must(t, typeConfigMap, "shaide-config").Inputs["data"])
	for _, key := range []string{"KNOWLEDGE_CENTER_URL", "MCP_COLLECTION_CONTEXT_TARGETS", "MCP_SERVERS"} {
		if _, ok := data[key]; ok {
			t.Errorf("shaide-config carries %s while KC is declined", key)
		}
	}
	// Generic MCP discovery is unaffected.
	if data["MCP_NAMESPACE"] != knowledgecenter.MCPNamespace {
		t.Errorf("MCP_NAMESPACE = %q", data["MCP_NAMESPACE"])
	}
}

// Without the Knowledge Center, the KC image and credentials are not needed
// at all, from installer options to resources.
func TestDisabledNeedsNoKnowledgeCenterInputs(t *testing.T) {
	disabled := false
	images := testImages() // no KC image
	result := mustDeploy(t, Options{KnowledgeCenterEnabled: &disabled, Images: images})
	if _, ok := result.Find(typeDeployment, "knowledge-center"); ok {
		t.Error("KC deployed without inputs")
	}
}

// --- Storage credentials ------------------------------------------------

// The public server no longer receives storage credentials or S3 settings.
// RustFS keeps the same credential values, now from a storage-owned Secret
// that shaide-server does not reference.
func TestStorageCredentialsLeaveTheServer(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		result := mustDeploy(t, kcOptions(enabled))

		secretData := stringMap(result.Must(t, typeSecret, "shaide-secrets").Inputs["stringData"])
		if _, ok := secretData["S3_PASSWORD"]; ok {
			t.Errorf("enabled=%t: shaide-secrets carries S3_PASSWORD", enabled)
		}
		configData := stringMap(result.Must(t, typeConfigMap, "shaide-config").Inputs["data"])
		for _, key := range []string{"S3_USER", "SHAIDE_SERVER_S3_FQDN", "SHAIDE_SERVER_S3_PORT", "S3_UPLOAD_PROXY_ROUTE_PREFIX", "RUSTFS_WEBHOOK_ARN"} {
			if _, ok := configData[key]; ok {
				t.Errorf("enabled=%t: shaide-config carries %s", enabled, key)
			}
		}

		storage := result.Must(t, typeSecret, "rustfs-credentials")
		storageData := stringMap(storage.Inputs["stringData"])
		if storageData["access-key"] != "rustfsuser" {
			t.Errorf("enabled=%t: access-key = %q, want the existing s3User", enabled, storageData["access-key"])
		}
		// The prompted s3Password is carried over unchanged, not regenerated.
		if storageData["secret-key"] != "answer" {
			t.Errorf("enabled=%t: secret-key is not the configured s3Password", enabled)
		}

		rustfs := result.Must(t, typeStatefulSet, "rustfs")
		env := pulumitest.Env(container(t, rustfs, "rustfs"))
		for name, want := range map[string]string{
			"RUSTFS_ACCESS_KEY":    "secret:rustfs-credentials/access-key",
			"RUSTFS_ROOT_USER":     "secret:rustfs-credentials/access-key",
			"RUSTFS_SECRET_KEY":    "secret:rustfs-credentials/secret-key",
			"RUSTFS_ROOT_PASSWORD": "secret:rustfs-credentials/secret-key",
		} {
			if env[name] != want {
				t.Errorf("enabled=%t: rustfs %s = %q, want %q", enabled, name, env[name], want)
			}
		}

		server := result.Must(t, typeStatefulSet, "shaide-server")
		serverContainer := container(t, server, "shaide-server")
		for _, ref := range pulumitest.List(serverContainer, "envFrom") {
			if pulumitest.String(ref, "secretRef.name") == "rustfs-credentials" {
				t.Errorf("enabled=%t: shaide-server reads the storage Secret", enabled)
			}
		}
		for _, value := range pulumitest.Env(serverContainer) {
			if strings.Contains(value, "rustfs-credentials") {
				t.Errorf("enabled=%t: shaide-server references the storage Secret", enabled)
			}
		}
	}
}

// --- Enabled ------------------------------------------------------------

func TestEnabledDeploysKnowledgeCenterFromHarbor(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))

	kc := result.Must(t, typeDeployment, "knowledge-center")
	if ns := pulumitest.String(kc.Inputs, "metadata.namespace"); ns != "app-shaide" {
		t.Errorf("KC namespace = %q, want app-shaide", ns)
	}
	c := container(t, kc, "knowledge-center")
	if c["image"] != kcHarborImage {
		t.Errorf("KC image = %v, want the Harbor reference", c["image"])
	}

	svc := result.Must(t, typeService, "knowledge-center-svc")
	if pulumitest.String(svc.Inputs, "spec.type") != "ClusterIP" {
		t.Errorf("KC Service type = %q, want ClusterIP", pulumitest.String(svc.Inputs, "spec.type"))
	}
	if port := pulumitest.Get(svc.Inputs, "spec.ports.0.port"); port != float64(8080) {
		t.Errorf("KC Service port = %v, want 8080", port)
	}

	cp := result.Must(t, typeDeployment, "control-panel")
	if env := pulumitest.Env(container(t, cp, "control-panel")); env["KNOWLEDGE_CENTER_ENABLED"] != "true" {
		t.Errorf("KNOWLEDGE_CENTER_ENABLED = %q, want true", env["KNOWLEDGE_CENTER_ENABLED"])
	}
}

func TestKnowledgeCenterRuntimeContract(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))
	kc := result.Must(t, typeDeployment, "knowledge-center")
	c := container(t, kc, "knowledge-center")
	env := pulumitest.Env(c)

	want := map[string]string{
		"HOST":                            "0.0.0.0",
		"PORT":                            "8080",
		"S3_FQDN":                         "rustfs",
		"S3_PORT":                         "9000",
		"RUSTFS_WEBHOOK_ARN":              "arn:rustfs:sqs:eu-central-1:shaide:webhook",
		"VECTOR_DB_URL":                   "http://qdrant:6334",
		"SHAIDE_SERVER_URL":               "http://shaide-server",
		"KC_SHAIDE_USERNAME":              "kc-embedding",
		"KC_CONTEXT_ISSUER":               knowledgecenter.Issuer,
		"KC_CONTEXT_TARGET":               "knowledge-center",
		"KC_CONTEXT_AUDIENCE":             "kc:knowledge-center",
		"KC_CONTEXT_MCP_AUDIENCE":         "kc-mcp:knowledge-center",
		"KC_CONTEXT_KID":                  "kc-context-v1",
		"KC_CONTEXT_PUBLIC_KEY_FILE":      "/run/secrets/kc-context/public.pem",
		"KC_MCP_SCOPED_TOKEN_FILE":        "/run/secrets/kc-context/scoped_token",
		"TMPDIR":                          "/tmp",
		"XBERG_CACHE_DIR":                 "/tmp/xberg",
		"KNOWLEDGE_CENTER_INTERNAL_TOKEN": "secret:knowledge-center/KNOWLEDGE_CENTER_INTERNAL_TOKEN",
		"KC_SHAIDE_PASSWORD":              "secret:knowledge-center/KC_SHAIDE_PASSWORD",
		"S3_USER":                         "secret:rustfs-credentials/access-key",
		"S3_PASSWORD":                     "secret:rustfs-credentials/secret-key",
	}
	if !reflect.DeepEqual(env, want) {
		t.Errorf("KC env keys = %v\nwant %v", keys(env), keys(want))
		for k, v := range want {
			if env[k] != v {
				t.Errorf("KC %s = %q, want %q", k, env[k], v)
			}
		}
	}

	// UID/GID 10001 is the image's user; fsGroup makes 0440 secret files
	// readable by it.
	spec := pulumitest.Map(kc.Inputs, "spec.template.spec")
	for path, want := range map[string]any{
		"securityContext.runAsUser":    float64(10001),
		"securityContext.runAsGroup":   float64(10001),
		"securityContext.fsGroup":      float64(10001),
		"securityContext.runAsNonRoot": true,
		"automountServiceAccountToken": false,
	} {
		if got := pulumitest.Get(spec, path); got != want {
			t.Errorf("KC %s = %v, want %v", path, got, want)
		}
	}
	if pulumitest.Get(c, "securityContext.readOnlyRootFilesystem") != true || pulumitest.Get(c, "securityContext.allowPrivilegeEscalation") != false {
		t.Error("KC container is not read-only and non-escalating")
	}

	context := pulumitest.Named(pulumitest.List(spec, "volumes"), "kc-context")
	if pulumitest.String(context, "secret.secretName") != "knowledge-center-context" || pulumitest.Get(context, "secret.defaultMode") != float64(0o440) {
		t.Errorf("kc-context volume = %v, want knowledge-center-context with mode 0440", context)
	}
	mount := pulumitest.Named(pulumitest.List(c, "volumeMounts"), "kc-context")
	if pulumitest.String(mount, "mountPath") != "/run/secrets/kc-context" || mount["readOnly"] != true {
		t.Errorf("kc-context mount = %v", mount)
	}
	if pulumitest.Named(pulumitest.List(c, "volumeMounts"), "tmp") == nil {
		t.Error("KC has no writable /tmp on its read-only root")
	}

	// KC's files: verification key and scoped token, never the signing key.
	files := stringMap(result.Must(t, typeSecret, "knowledge-center-context").Inputs["stringData"])
	if !reflect.DeepEqual(keys(files), []string{"public.pem", "scoped_token"}) {
		t.Errorf("knowledge-center-context keys = %v", keys(files))
	}
	envSecret := stringMap(result.Must(t, typeSecret, "knowledge-center").Inputs["stringData"])
	if !reflect.DeepEqual(keys(envSecret), []string{"KC_SHAIDE_PASSWORD", "KNOWLEDGE_CENTER_INTERNAL_TOKEN"}) {
		t.Errorf("knowledge-center keys = %v", keys(envSecret))
	}

	// HTTP liveness only: /v1/health does not check storage or embeddings.
	for _, probe := range []string{"startupProbe", "readinessProbe", "livenessProbe"} {
		if pulumitest.String(c, probe+".httpGet.path") != "/v1/health" || pulumitest.Get(c, probe+".httpGet.port") != float64(8080) {
			t.Errorf("KC %s = %v, want GET /v1/health on 8080", probe, c[probe])
		}
	}

	limits := stringMap(pulumitest.Get(c, "resources.limits"))
	requests := stringMap(pulumitest.Get(c, "resources.requests"))
	if limits["cpu"] == "" || limits["memory"] == "" || requests["cpu"] == "" || requests["memory"] == "" {
		t.Errorf("KC resources requests=%v limits=%v, want both set", requests, limits)
	}
}

// KC prefers the installer's CPU pools like the other app-shaide components.
func TestKnowledgeCenterUsesCPUPlacement(t *testing.T) {
	options := kcOptions(true)
	options.Placement = []PlacementTerm{{Key: "kubernetes.azure.com/agentpool", Values: []string{"cpu"}}}
	result := mustDeploy(t, options)

	kc := result.Must(t, typeDeployment, "knowledge-center")
	term := pulumitest.Get(kc.Inputs, "spec.template.spec.affinity.nodeAffinity.preferredDuringSchedulingIgnoredDuringExecution.0.preference.matchExpressions.0")
	if pulumitest.String(term, "key") != "kubernetes.azure.com/agentpool" || !reflect.DeepEqual(pulumitest.List(term, "values"), []any{"cpu"}) {
		t.Errorf("KC node affinity = %v, want the CPU pool", term)
	}
}

func TestKnowledgeCenterResourcesAreConfigurable(t *testing.T) {
	result, err := deploy(t, kcOptions(true), map[string]string{
		"app-shaide:knowledgeCenterMemoryLimit": "6Gi",
		"app-shaide:knowledgeCenterCpuLimit":    "4",
	})
	if err != nil {
		t.Fatalf("deploy error = %v", err)
	}
	c := container(t, result.Must(t, typeDeployment, "knowledge-center"), "knowledge-center")
	limits := stringMap(pulumitest.Get(c, "resources.limits"))
	if limits["memory"] != "6Gi" || limits["cpu"] != "4" {
		t.Errorf("KC limits = %v, want the configured values", limits)
	}
}

// The server forwards to KC and signs KC-MCP requests. Discovery stays in
// Kubernetes (no MCP_SERVERS), in app-mcp.
func TestEnabledServerWiring(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))

	data := stringMap(result.Must(t, typeConfigMap, "shaide-config").Inputs["data"])
	for key, want := range map[string]string{
		"KNOWLEDGE_CENTER_URL":                    "http://knowledge-center:8080",
		"KNOWLEDGE_CENTER_ALLOW_LOCAL_HTTP":       "true",
		"MCP_COLLECTION_CONTEXT_TARGETS":          knowledgecenter.ContextTargetsJSON(),
		"MCP_COLLECTION_CONTEXT_ALLOW_LOCAL_HTTP": "true",
		"MCP_NAMESPACE":                           "app-mcp",
	} {
		if data[key] != want {
			t.Errorf("shaide-config %s = %q, want %q", key, data[key], want)
		}
	}
	if _, ok := data["MCP_SERVERS"]; ok {
		t.Error("MCP_SERVERS set; Kubernetes discovery would be disabled")
	}

	creds := knowledgecentertest.Credentials()
	secret := result.Must(t, typeSecret, "shaide-secrets")
	secretData := stringMap(secret.Inputs["stringData"])
	if secretData["KNOWLEDGE_CENTER_INTERNAL_TOKEN"] != creds.InternalToken || secretData["S3_WEBHOOK_TOKEN"] != creds.WebhookToken {
		t.Error("shaide-secrets does not carry the installer's KC tokens")
	}
	if len(secret.Secrets) == 0 {
		t.Error("shaide-secrets inputs are not marked secret")
	}

	signer := stringMap(result.Must(t, typeSecret, "kc-context-signer").Inputs["stringData"])
	if !reflect.DeepEqual(keys(signer), []string{"gateway_token", "private.pem"}) {
		t.Errorf("kc-context-signer keys = %v", keys(signer))
	}
	if signer["private.pem"] != creds.ContextSigningKey || signer["gateway_token"] != creds.GatewayToken {
		t.Error("kc-context-signer does not carry the installer's values")
	}

	server := result.Must(t, typeStatefulSet, "shaide-server")
	volume := pulumitest.Named(pulumitest.List(server.Inputs, "spec.template.spec.volumes"), "kc-context")
	if pulumitest.String(volume, "secret.secretName") != "kc-context-signer" {
		t.Errorf("shaide-server kc-context volume = %v", volume)
	}
	mount := pulumitest.Named(pulumitest.List(container(t, server, "shaide-server"), "volumeMounts"), "kc-context")
	if pulumitest.String(mount, "mountPath") != "/run/secrets/kc-context" || mount["readOnly"] != true {
		t.Errorf("shaide-server kc-context mount = %v", mount)
	}
}

// RustFS notifications drive KC ingestion; the server accepts them only with
// the webhook token.
func TestEnabledRustfsNotifications(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))
	env := pulumitest.Env(container(t, result.Must(t, typeStatefulSet, "rustfs"), "rustfs"))
	for name, want := range map[string]string{
		"RUSTFS_NOTIFY_ENABLE":                    "true",
		"RUSTFS_NOTIFY_WEBHOOK_ENABLE_SHAIDE":     "on",
		"RUSTFS_NOTIFY_WEBHOOK_ENDPOINT_SHAIDE":   "http://shaide-server/v1/object-storage/event",
		"RUSTFS_NOTIFY_WEBHOOK_AUTH_TOKEN_SHAIDE": "secret:rustfs-credentials/notify-webhook-token",
		"RUSTFS_OUTBOUND_ALLOW_ORIGINS":           "http://shaide-server",
	} {
		if env[name] != want {
			t.Errorf("rustfs %s = %q, want %q", name, env[name], want)
		}
	}
	storage := stringMap(result.Must(t, typeSecret, "rustfs-credentials").Inputs["stringData"])
	if storage["notify-webhook-token"] != knowledgecentertest.Credentials().WebhookToken {
		t.Error("rustfs-credentials does not carry the webhook token")
	}

	disabled := mustDeploy(t, kcOptions(false))
	disabledEnv := pulumitest.Env(container(t, disabled.Must(t, typeStatefulSet, "rustfs"), "rustfs"))
	for _, name := range []string{"RUSTFS_NOTIFY_ENABLE", "RUSTFS_NOTIFY_WEBHOOK_AUTH_TOKEN_SHAIDE", "RUSTFS_OUTBOUND_ALLOW_ORIGINS"} {
		if _, ok := disabledEnv[name]; ok {
			t.Errorf("rustfs %s set while KC is declined", name)
		}
	}
}

// KC reuses the shared Qdrant and RustFS: no second workload or volume.
func TestEnabledAddsNoStorageWorkload(t *testing.T) {
	enabled := mustDeploy(t, kcOptions(true))
	disabled := mustDeploy(t, kcOptions(false))

	names := func(r pulumitest.Result) []string {
		var out []string
		for _, s := range r.OfType(typeStatefulSet) {
			out = append(out, s.Name)
		}
		sort.Strings(out)
		return out
	}
	if !reflect.DeepEqual(names(enabled), names(disabled)) {
		t.Errorf("StatefulSets with KC = %v, without = %v", names(enabled), names(disabled))
	}
	if pvcs := enabled.OfType("kubernetes:core/v1:PersistentVolumeClaim"); len(pvcs) != 0 {
		t.Errorf("KC added PersistentVolumeClaims: %d", len(pvcs))
	}
	if len(pulumitest.List(enabled.Must(t, typeDeployment, "knowledge-center").Inputs, "spec.template.spec.volumes")) != 2 {
		t.Error("KC has volumes beyond its secret files and /tmp")
	}
}

func peersOf(rule map[string]any, field string) []map[string]any {
	var out []map[string]any
	for _, peer := range pulumitest.List(rule, field) {
		if m, ok := peer.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func peerNames(peers []map[string]any) []string {
	var out []string
	for _, peer := range peers {
		nsLabels := stringMap(pulumitest.Get(peer, "namespaceSelector.matchLabels"))
		podLabels := stringMap(pulumitest.Get(peer, "podSelector.matchLabels"))
		out = append(out, nsLabels["kubernetes.io/metadata.name"]+"/"+podLabels["app.kubernetes.io/name"])
	}
	sort.Strings(out)
	return out
}

// ports renders a rule's ports as "TCP/8080".
func ports(rule map[string]any) []string {
	var out []string
	for _, p := range pulumitest.List(rule, "ports") {
		out = append(out, fmt.Sprintf("%s/%v", pulumitest.String(p, "protocol"), pulumitest.Get(p, "port")))
	}
	sort.Strings(out)
	return out
}

// Network policies: KC is reachable only from shaide-server and KC-MCP; the
// shared Qdrant keeps workspace-code search (shaide-server) and admits KC;
// RustFS admits KC and the monitoring namespace (Loki).
func TestEnabledNetworkPolicies(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))

	kc := result.Must(t, typeNetworkPolicy, "knowledge-center-netpol")
	if got := stringMap(pulumitest.Get(kc.Inputs, "spec.podSelector.matchLabels")); got["app.kubernetes.io/name"] != "knowledge-center" {
		t.Errorf("KC policy selects %v", got)
	}
	ingress := pulumitest.List(kc.Inputs, "spec.ingress")
	if len(ingress) != 1 {
		t.Fatalf("KC ingress rules = %d, want 1", len(ingress))
	}
	rule := ingress[0].(map[string]any)
	if got, want := peerNames(peersOf(rule, "from")), []string{"/shaide-server", "app-mcp/kc-mcp"}; !reflect.DeepEqual(got, want) {
		t.Errorf("KC ingress from %v, want %v", got, want)
	}
	if got := ports(rule); !reflect.DeepEqual(got, []string{"TCP/8080"}) {
		t.Errorf("KC ingress ports %v", got)
	}

	var egress []string
	hasDNS := false
	for _, item := range pulumitest.List(kc.Inputs, "spec.egress") {
		rule := item.(map[string]any)
		if len(pulumitest.List(rule, "to")) == 0 {
			hasDNS = reflect.DeepEqual(ports(rule), []string{"TCP/53", "UDP/53"})
			continue
		}
		egress = append(egress, strings.Join(peerNames(peersOf(rule, "to")), ",")+"@"+strings.Join(ports(rule), ","))
	}
	sort.Strings(egress)
	if want := []string{"/qdrant@TCP/6334", "/rustfs@TCP/9000", "/shaide-server@TCP/8080"}; !reflect.DeepEqual(egress, want) {
		t.Errorf("KC egress %v, want %v", egress, want)
	}
	if !hasDNS {
		t.Error("KC egress has no DNS rule")
	}

	qdrant := result.Must(t, typeNetworkPolicy, "qdrant-clients-netpol")
	qrule := pulumitest.List(qdrant.Inputs, "spec.ingress")[0].(map[string]any)
	if got, want := peerNames(peersOf(qrule, "from")), []string{"/knowledge-center", "/shaide-server"}; !reflect.DeepEqual(got, want) {
		t.Errorf("qdrant clients %v, want %v", got, want)
	}
	if got := ports(qrule); !reflect.DeepEqual(got, []string{"TCP/6333", "TCP/6334"}) {
		t.Errorf("qdrant ports %v", got)
	}

	rustfs := result.Must(t, typeNetworkPolicy, "rustfs-clients-netpol")
	rrule := pulumitest.List(rustfs.Inputs, "spec.ingress")[0].(map[string]any)
	if got, want := peerNames(peersOf(rrule, "from")), []string{"/knowledge-center", "monitoring/"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rustfs clients %v, want %v", got, want)
	}
}

// An installation that pinned the legacy discovery namespace is not moved:
// enabling KC there fails until the operator migrates it.
func TestEnabledRejectsLegacyMCPNamespace(t *testing.T) {
	_, err := deploy(t, kcOptions(true), map[string]string{"app-shaide:mcpNamespace": "mcp-gateway"})
	if err == nil || !strings.Contains(err.Error(), "mcp-gateway") {
		t.Fatalf("deploy error = %v, want the legacy namespace reported", err)
	}

	// Without KC the pinned namespace keeps working as before.
	result, err := deploy(t, kcOptions(false), map[string]string{"app-shaide:mcpNamespace": "mcp-gateway"})
	if err != nil {
		t.Fatalf("deploy error = %v for a KC-less legacy installation", err)
	}
	data := stringMap(result.Must(t, typeConfigMap, "shaide-config").Inputs["data"])
	if data["MCP_NAMESPACE"] != "mcp-gateway" {
		t.Errorf("MCP_NAMESPACE = %q, want the pinned legacy value kept", data["MCP_NAMESPACE"])
	}
}

// No secret is invented by the program: every KC credential in the
// resources is the installer's value.
func TestKnowledgeCenterSecretsAreTheInstallerValues(t *testing.T) {
	result := mustDeploy(t, kcOptions(true))
	creds := knowledgecentertest.Credentials()

	env := stringMap(result.Must(t, typeSecret, "knowledge-center").Inputs["stringData"])
	if env["KNOWLEDGE_CENTER_INTERNAL_TOKEN"] != creds.InternalToken || env["KC_SHAIDE_PASSWORD"] != creds.EmbeddingPassword {
		t.Error("knowledge-center Secret differs from the installer values")
	}
	files := stringMap(result.Must(t, typeSecret, "knowledge-center-context").Inputs["stringData"])
	if files["public.pem"] != creds.ContextPublicKey || files["scoped_token"] != creds.ScopedToken {
		t.Error("knowledge-center-context Secret differs from the installer values")
	}
}
