package appconfig

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// NodeSelectorFor builds a nodeSelector map for a component.
// override takes precedence over the global NodeSelector.
// Returns nil when both are empty (no scheduling constraint).
func (c Config) NodeSelectorFor(override string) pulumi.StringMap {
	val := override
	if val == "" {
		val = c.NodeSelector
	}
	if val == "" {
		return nil
	}
	return pulumi.StringMap{c.NodeSelectorKey: pulumi.String(val)}
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
type Config struct {
	Namespace                 string
	NodeSelectorKey           string // label key used for node selection (default: nodegroup)
	NodeSelector              string // global fallback — applies to all components when no per-component key is set
	NodeSelectorShaide        string // optional — overrides NodeSelector for shaide-server
	NodeSelectorControlPanel  string // optional — overrides NodeSelector for control-panel
	NodeSelectorWebApp        string // optional — overrides NodeSelector for webapp
	NodeSelectorRustfs        string // optional — overrides NodeSelector for rustfs
	NodeSelectorQdrant        string // optional — overrides NodeSelector for qdrant
	CloudProvider             string // informational only — identifies the target platform (e.g. "gcp", "aws", "on-prem")
	StorageClassName          string // optional — if empty, PVCs use the cluster default StorageClass
	PVNodeHostname            string // optional — node hostname for hostPath PV nodeAffinity (on-prem only)
	HarborHostname            string // optional — internal Harbor registry hostname (on-prem only, e.g. harbor.internal.lan)
	Kubeconfig                string // optional — path to kubeconfig file; empty = use KUBECONFIG env / ~/.kube/config
	ShaidePVSize              string // optional — shaide-server PV/PVC size (default: 5Gi)
	RustfsPVSize              string // optional — rustfs PV/PVC size (default: 5Gi)
	QdrantPVSize              string // optional — qdrant PV/PVC size (default: 5Gi)
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

func Load(ctx *pulumi.Context) Config {
	cfg := pulumiconfig.New(ctx, "")

	var lbAnnotations map[string]string
	cfg.GetObject("lbAnnotations", &lbAnnotations)

	var saAnnotations map[string]string
	cfg.GetObject("serviceAccountAnnotations", &saAnnotations)

	return Config{
		Namespace:                 cfg.Require("namespace"),
		NodeSelectorKey:           getWithDefault(cfg, "nodeSelectorKey", "nodegroup"),
		NodeSelector:              cfg.Get("nodeSelector"),
		NodeSelectorShaide:        cfg.Get("nodeSelectorShaide"),
		NodeSelectorControlPanel:  cfg.Get("nodeSelectorControlPanel"),
		NodeSelectorWebApp:        cfg.Get("nodeSelectorWebapp"),
		NodeSelectorRustfs:        cfg.Get("nodeSelectorRustfs"),
		NodeSelectorQdrant:        cfg.Get("nodeSelectorQdrant"),
		CloudProvider:             cfg.Require("cloudProvider"),
		StorageClassName:          cfg.Get("storageClassName"),
		PVNodeHostname:            cfg.Get("pvNodeHostname"),
		HarborHostname:            cfg.Get("harborHostname"),
		Kubeconfig:                cfg.Get("kubeconfig"),
		ShaidePVSize:              getWithDefault(cfg, "shaidePVSize", "5Gi"),
		RustfsPVSize:              getWithDefault(cfg, "rustfsPVSize", "5Gi"),
		QdrantPVSize:              getWithDefault(cfg, "qdrantPVSize", "5Gi"),
		LBAnnotations:             lbAnnotations,
		ServiceAccountAnnotations: saAnnotations,
		ServiceAccountName:        cfg.Require(("shaideServiceAccountName")),
		Registry: Registry{
			GHCRUser:  getWithDefault(cfg, "ghcrUser", "axem-solutions"),
			GHCRToken: cfg.RequireSecret("ghcrToken"),
		},
		Services: ServiceNames{
			ControlPanel: cfg.Require("controlPanelService"),
			WebApp:       cfg.Require("webappService"),
			Rustfs:       cfg.Require("rustfsService"),
			Qdrant:       cfg.Require("qdrantService"),
		},
		Images: Images{
			ShaideServer: cfg.Require("shaideServerImage"),
			ControlPanel: cfg.Require("controlPanelImage"),
			WebApp:       cfg.Require("webappImage"),
			Rustfs:       cfg.Require("rustfsImage"),
			Qdrant:       cfg.Require("qdrantImage"),
			Busybox:      getWithDefault(cfg, "busyboxImage", "busybox:1.37"),
		},
		Routing: Routing{
			InfraStackRef:    cfg.Get("infraStackRef"),
			GatewayHostname:  cfg.Get("gatewayHostname"),
			GatewayName:      getWithDefault(cfg, "gatewayName", "shared-gateway"),
			GatewayNamespace: getWithDefault(cfg, "gatewayNamespace", "gateway-system"),
		},
		ShaideEnv: AppEnv{
			ShaideServerUiFQDN:       cfg.Require("shaideServerUiFqdn"),
			ShaideServerUiPort:       cfg.Require("shaideServerUiPort"),
			DatabaseURL:              cfg.Require("databaseUrl"),
			S3User:                   cfg.Require("s3User"),
			S3Port:                   cfg.Require("shaideServerS3Port"),
			S3FQDN:                   cfg.Require("shaideServerS3Fqdn"),
			S3UploadProxyRoutePrefix: cfg.Require("s3UploadProxyRoutePrefix"),
			RustFSWebhookARN:         cfg.Require("rustfsWebhookArn"),
			VectorDBUrl:              cfg.Require("vectorDBUrl"),
			MCPNamespace:             cfg.Get("mcpNamespace"),
			RustLibBacktrace:         cfg.Get("rustLibBacktrace"),
			RustSpantrace:            cfg.Get("rustSpantrace"),
			Trial:                    getWithDefault(cfg, "trial", "FALSE"),
		},
		RustEnv: RustFSEnv{
			ConsoleEnabled:        cfg.GetBool("rustfsConsoleEnabled"),
			WebhookEnableShaide:   cfg.Require("rustfsNotifyWebhookEnableShaide"),
			WebhookEndpointShaide: cfg.Require("rustfsNotifyWebhookEndpointShaide"),
			WebhookQueueDirShaide: cfg.Require("rustfsNotifyWebhookQueueDirShaide"),
		},
		Secrets: AppSecrets{
			AdminAuthKey:  cfg.RequireSecret("adminAuthKey"),
			S3Password:    cfg.RequireSecret("s3Password"),
			JWTSecret:     cfg.RequireSecret("jwtSecret"),
			SessionSecret: cfg.RequireSecret("sessionSecret"),
		},
	}
}
