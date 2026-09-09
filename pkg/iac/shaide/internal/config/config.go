package appconfig

import (
	"fmt"

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
func (c Values) NodeAffinityFor(override string) *corev1.NodeAffinityArgs {
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

type ServiceNames struct {
	ControlPanel string
	WebApp       string
	Rustfs       string
	Qdrant       string
}

type Images struct {
	ShaideServer string
	ControlPanel string
	WebApp       string
	Rustfs       string
	Qdrant       string
	Busybox      string // init container image used by rustfs for permission fixup
}

type Routing struct {
	InfraStackRef    string
	GatewayHostname  string // direct hostname for on-prem; takes effect when InfraStackRef is empty
	GatewayName      string
	GatewayNamespace string
}

type Registry struct {
	GHCRUser  string
	GHCRToken pulumi.StringOutput
}

type AppEnv struct {
	ShaideServerUiFQDN       string
	ShaideServerUiPort       string
	DatabaseURL              string
	S3User                   string
	S3Port                   string
	S3FQDN                   string
	S3UploadProxyRoutePrefix string
	RustFSWebhookARN         string
	VectorDBUrl              string
	MCPNamespace             string // optional — MCP deployment is skipped entirely when unset
	RustLibBacktrace         string
	RustSpantrace            string
	Trial                    string // "TRUE" for trial deployments, "FALSE" otherwise
}

type AppSecrets struct {
	AdminAuthKey  pulumi.StringOutput
	S3Password    pulumi.StringOutput
	JWTSecret     pulumi.StringOutput
	SessionSecret pulumi.StringOutput
}

type RustFSEnv struct {
	ConsoleEnabled        bool
	WebhookEnableShaide   string
	WebhookEndpointShaide string
	WebhookQueueDirShaide string
}

// Config is the typed view of Pulumi stack config used by this stack.
type Values struct {
	Namespace                string
	NodeSelectorKey          string // label key used for node selection (default: nodegroup)
	NodeSelector             string // global fallback — applies to all components when no per-component key is set
	NodeSelectorShaide       string // optional — overrides NodeSelector for shaide-server
	NodeSelectorControlPanel string // optional — overrides NodeSelector for control-panel
	NodeSelectorWebApp       string // optional — overrides NodeSelector for webapp
	NodeSelectorRustfs       string // optional — overrides NodeSelector for rustfs
	NodeSelectorQdrant       string // optional — overrides NodeSelector for qdrant
	CloudProvider            string // informational only — identifies the target platform (e.g. "gcp", "aws", "on-prem")
	StorageClassName         string // optional — if empty, PVCs use the cluster default StorageClass
	PVNodeHostname           string // optional — node hostname for hostPath PV nodeAffinity (on-prem only)
	HarborHostname           string // optional — internal Harbor registry hostname (on-prem only, e.g. harbor.internal.lan)
	Kubeconfig               string // optional — path to kubeconfig file; empty = use KUBECONFIG env / ~/.kube/config
	ShaidePVSize             string // optional — shaide-server PV/PVC size (default: 5Gi)
	RustfsPVSize             string // optional — rustfs PV/PVC size (default: 5Gi)
	QdrantPVSize             string // optional — qdrant PV/PVC size (default: 5Gi)
	KnowledgeCenterEnabled   bool   // optional — presence of the Knowledge Center feature; injected into control-panel as KNOWLEDGE_CENTER_ENABLED; default: false

	LBAnnotations             map[string]string
	ServiceAccountAnnotations map[string]string
	ServiceAccountName        string
	Registry                  Registry
	Services                  ServiceNames
	Images                    Images
	Routing                   Routing
	ShaideEnv                 AppEnv
	RustEnv                   RustFSEnv
	Secrets                   AppSecrets
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
	values, err := c.definition.Load(ctx)
	if err != nil {
		return Values{}, fmt.Errorf("load stack config: %w", err)
	}

	applyDefaults(&values)

	return values, nil
}

func applyDefaults(cfg *Values) {
	if cfg.NodeSelectorKey == "" {
		cfg.NodeSelectorKey = DefaultNodeSelectorKey
	}

	if cfg.Images.Busybox == "" {
		cfg.Images.Busybox = DefaultBusyboxImage
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
