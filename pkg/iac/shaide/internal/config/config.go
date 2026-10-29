package appconfig

import (
	"fmt"
	"sort"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// NodeAffinityFor builds a soft (preferred) nodeAffinity for a component.
// override takes precedence over the global NodeSelector.
// Returns nil when both are empty (no scheduling preference) — same short-circuit as the
// nodeSelector this replaces. Soft, not hard: the component should prefer a node carrying
// NodeSelectorKey=val, but must still be able to run elsewhere if none is available.
// Returns just the NodeAffinity sub-block (not the full AffinityArgs wrapper) so callers can
// compose it alongside PodAntiAffinityFor in a single Affinity value.
//
// A Placement, when set, replaces all of that: every component prefers the
// nodes it admits, one preferred term per placement term.
func (c Values) NodeAffinityFor(override string) *corev1.NodeAffinityArgs {
	if len(c.Placement) > 0 {
		return c.placementAffinity()
	}

	val := override
	if val == "" {
		val = c.NodeSelector
	}
	if val == "" {
		return nil
	}
	return &corev1.NodeAffinityArgs{
		PreferredDuringSchedulingIgnoredDuringExecution: corev1.PreferredSchedulingTermArray{
			&corev1.PreferredSchedulingTermArgs{
				Weight: pulumi.Int(100),
				Preference: &corev1.NodeSelectorTermArgs{
					MatchExpressions: corev1.NodeSelectorRequirementArray{
						&corev1.NodeSelectorRequirementArgs{
							Key:      pulumi.String(c.NodeSelectorKey),
							Operator: pulumi.String("In"),
							Values:   pulumi.StringArray{pulumi.String(val)},
						},
					},
				},
			},
		},
	}
}

func (c Values) placementAffinity() *corev1.NodeAffinityArgs {
	terms := make(corev1.PreferredSchedulingTermArray, 0, len(c.Placement))
	for _, term := range c.Placement {
		values := append([]string(nil), term.Values...)
		sort.Strings(values)

		terms = append(terms, &corev1.PreferredSchedulingTermArgs{
			Weight: pulumi.Int(100),
			Preference: &corev1.NodeSelectorTermArgs{
				MatchExpressions: corev1.NodeSelectorRequirementArray{
					&corev1.NodeSelectorRequirementArgs{
						Key:      pulumi.String(term.Key),
						Operator: pulumi.String("In"),
						Values:   pulumi.ToStringArray(values),
					},
				},
			},
		})
	}

	return &corev1.NodeAffinityArgs{PreferredDuringSchedulingIgnoredDuringExecution: terms}
}

// PlacementTerm admits the nodes whose Key label is one of Values, e.g. a node
// pool label and the pools' names.
type PlacementTerm struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

type ServiceNames struct {
	ControlPanel string
	WebApp       string
	Rustfs       string
	Qdrant       string
}

type Images struct {
	ShaideServer    string
	ControlPanel    string
	WebApp          string
	Rustfs          string
	Qdrant          string
	Busybox         string // init container image used by rustfs for permission fixup
	KnowledgeCenter string // only set when the Knowledge Center is installed
}

// Resources are a container's requests and limits.
type Resources struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}

// KnowledgeCenterValues configure the KC workload. Its credentials are in
// AppSecrets.
type KnowledgeCenterValues struct {
	// EmbeddingUsername is the ordinary shaide-server account KC requests
	// embeddings as. It must exist before KC indexes anything.
	EmbeddingUsername string
	Resources         Resources
}

type Routing struct {
	InfraStackRef    string
	GatewayHostname  string // direct hostname for on-prem; takes effect when InfraStackRef is empty
	GatewayName      string
	GatewayNamespace string
}

type AppEnv struct {
	ShaideServerUiFQDN string
	ShaideServerUiPort string
	DatabaseURL        string
	S3User             string // RustFS access key; storage-owned, never given to shaide-server
	S3Port             string // RustFS port as KC addresses it
	S3FQDN             string // RustFS host as KC addresses it
	RustFSWebhookARN   string // read by KC when it configures bucket notifications
	VectorDBUrl        string
	MCPNamespace       string // where shaide-server discovers MCP servers; default app-mcp
	RustLibBacktrace   string
	RustSpantrace      string
	Trial              string // "TRUE" for trial deployments, "FALSE" otherwise
}

type AppSecrets struct {
	AdminAuthKey  pulumi.StringOutput
	S3Password    pulumi.StringOutput
	JWTSecret     pulumi.StringOutput
	SessionSecret pulumi.StringOutput

	// Knowledge Center credentials, set only when it is installed.
	KCInternalToken     pulumi.StringOutput
	KCWebhookToken      pulumi.StringOutput
	KCGatewayToken      pulumi.StringOutput
	KCScopedToken       pulumi.StringOutput
	KCContextSigningKey pulumi.StringOutput
	KCContextPublicKey  pulumi.StringOutput
	KCEmbeddingPassword pulumi.StringOutput
}

type RustFSEnv struct {
	ConsoleEnabled        bool
	WebhookEnableShaide   string
	WebhookEndpointShaide string
	WebhookQueueDirShaide string
}

// Config is the typed view of Pulumi stack config used by this stack.
type Values struct {
	Namespace                   string
	Placement                   []PlacementTerm // preferred nodes; replaces the node selector keys below when set
	NodeSelectorKey             string          // label key used for node selection (default: nodegroup)
	NodeSelector                string          // global fallback — applies to all components when no per-component key is set
	NodeSelectorShaide          string          // optional — overrides NodeSelector for shaide-server
	NodeSelectorControlPanel    string          // optional — overrides NodeSelector for control-panel
	NodeSelectorWebApp          string          // optional — overrides NodeSelector for webapp
	NodeSelectorRustfs          string          // optional — overrides NodeSelector for rustfs
	NodeSelectorQdrant          string          // optional — overrides NodeSelector for qdrant
	NodeSelectorKnowledgeCenter string          // optional — overrides NodeSelector for the Knowledge Center
	CloudProvider               string          // informational only — identifies the target platform (e.g. "gcp", "aws", "on-prem")
	StorageClassName            string          // optional — if empty, PVCs use the cluster default StorageClass
	PVNodeHostname              string          // optional — node hostname for hostPath PV nodeAffinity (on-prem only)
	Kubeconfig                  string          // optional — path to kubeconfig file; empty = use KUBECONFIG env / ~/.kube/config
	Context                     string          // optional — kubeconfig context to deploy to; empty = the kubeconfig's current-context
	ShaidePVSize                string          // optional — shaide-server PV/PVC size (default: 5Gi)
	RustfsPVSize                string          // optional — rustfs PV/PVC size (default: 5Gi)
	QdrantPVSize                string          // optional — qdrant PV/PVC size (default: 5Gi)
	KnowledgeCenterEnabled      bool            // whether KC is deployed; also injected into control-panel as KNOWLEDGE_CENTER_ENABLED (configured presence, not health); default: false
	WebAppEnabled               bool            // whether the webapp is deployed; injected into control-panel as APP_ENABLED; default: false
	RustfsClientNamespaces      []string        // namespaces admitted to RustFS once KC restricts its ingress; default: monitoring
	KnowledgeCenter             KnowledgeCenterValues

	ServiceAccountAnnotations map[string]string
	ServiceAccountName        string
	Services                  ServiceNames
	Images                    Images
	Routing                   Routing
	ShaideEnv                 AppEnv
	RustEnv                   RustFSEnv
	Secrets                   AppSecrets
}

// WebAppURL is where shaide-server reaches the webapp it proxies under /app.
func (v Values) WebAppURL() string {
	return fmt.Sprintf("http://%s:%d", v.Services.WebApp, WebAppPort)
}

func getWithDefault(cfg *pulumiconfig.Config, key, fallback string) string {
	v := cfg.Get(key)
	if v == "" {
		return fallback
	}
	return v
}

// Load reads the stack configuration through the entry setters declared in the
// definition, then applies the defaults that the definition deliberately does
// not write, so a direct pulumi up behaves like an installer-driven one.
func (c Config) Load(ctx *pulumi.Context) (Values, error) {
	if err := validateLoadedKnowledgeCenter(ctx); err != nil {
		return Values{}, err
	}

	values, err := c.definition.Load(ctx)
	if err != nil {
		return Values{}, fmt.Errorf("load stack config: %w", err)
	}

	applyDefaults(&values)

	if err := values.Validate(); err != nil {
		return Values{}, err
	}

	return values, nil
}

// Validate rejects combinations the deployment cannot serve. It runs on every
// load, so a direct pulumi up is checked as well as an installer run.
func (v Values) Validate() error {
	if !v.KnowledgeCenterEnabled {
		return nil
	}

	if v.Namespace != knowledgecenter.Namespace {
		return fmt.Errorf("the Knowledge Center runs in namespace %q, but this stack deploys to %q", knowledgecenter.Namespace, v.Namespace)
	}

	// shaide-server discovers MCP servers in one namespace, and KC-MCP runs
	// in app-mcp. Moving discovery off an existing namespace would hide the
	// MCP servers there, so it is left to the operator.
	if v.ShaideEnv.MCPNamespace != knowledgecenter.MCPNamespace {
		return fmt.Errorf(
			"the Knowledge Center needs MCP discovery in namespace %q, but %s is %q; "+
				"move the MCP servers there and set %s:%s=%s deliberately",
			knowledgecenter.MCPNamespace, KeyMCPNamespace, v.ShaideEnv.MCPNamespace,
			Namespace, KeyMCPNamespace, knowledgecenter.MCPNamespace,
		)
	}

	if v.Images.KnowledgeCenter == "" {
		return fmt.Errorf("%s is required when the Knowledge Center is installed", KeyKnowledgeCenterImage)
	}

	return nil
}

// Knowledge Center resource defaults. Extraction and OCR of a 16 MiB upload
// are CPU- and memory-bound; the limits match the runtime the KC compose
// reference was tested with, with headroom for the request.
const (
	DefaultKnowledgeCenterCPURequest    = "250m"
	DefaultKnowledgeCenterMemoryRequest = "512Mi"
	DefaultKnowledgeCenterCPULimit      = "2"
	DefaultKnowledgeCenterMemoryLimit   = "2Gi"
)

func applyDefaults(cfg *Values) {
	if cfg.NodeSelectorKey == "" {
		cfg.NodeSelectorKey = DefaultNodeSelectorKey
	}

	if cfg.Images.Busybox == "" {
		cfg.Images.Busybox = DefaultBusyboxImage
	}

	if cfg.ShaideEnv.MCPNamespace == "" {
		cfg.ShaideEnv.MCPNamespace = DefaultMCPNamespace
	}

	if len(cfg.RustfsClientNamespaces) == 0 {
		cfg.RustfsClientNamespaces = []string{DefaultRustfsClientNamespace}
	}

	for _, resource := range []struct {
		field *string
		value string
	}{
		{&cfg.KnowledgeCenter.Resources.CPURequest, DefaultKnowledgeCenterCPURequest},
		{&cfg.KnowledgeCenter.Resources.MemoryRequest, DefaultKnowledgeCenterMemoryRequest},
		{&cfg.KnowledgeCenter.Resources.CPULimit, DefaultKnowledgeCenterCPULimit},
		{&cfg.KnowledgeCenter.Resources.MemoryLimit, DefaultKnowledgeCenterMemoryLimit},
	} {
		if *resource.field == "" {
			*resource.field = resource.value
		}
	}

	for _, size := range []*string{
		&cfg.ShaidePVSize,
		&cfg.RustfsPVSize,
		&cfg.QdrantPVSize,
	} {
		if *size == "" {
			*size = DefaultPVSize
		}
	}
}
