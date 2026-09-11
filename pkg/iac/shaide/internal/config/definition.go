package appconfig

import (
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	Namespace = "app-shaide"
	StackName = "shaide"
)

const (
	KeyCloudProvider  stackconfig.Key = "cloudProvider"
	KeyKubeconfig     stackconfig.Key = "kubeconfig"
	KeyNamespace      stackconfig.Key = "namespace"
	KeyServiceAccount stackconfig.Key = "shaideServiceAccountName"
	KeyHarborHostname stackconfig.Key = "harborHostname"
	KeyGHCRUser       stackconfig.Key = "ghcrUser"
	KeyGHCRToken      stackconfig.Key = "ghcrToken"

	KeyShaideServerImage stackconfig.Key = "shaideServerImage"
	KeyControlPanelImage stackconfig.Key = "controlPanelImage"
	KeyWebappImage       stackconfig.Key = "webappImage"
	KeyRustfsImage       stackconfig.Key = "rustfsImage"
	KeyQdrantImage       stackconfig.Key = "qdrantImage"
	KeyBusyboxImage      stackconfig.Key = "busyboxImage"

	KeyControlPanelService stackconfig.Key = "controlPanelService"
	KeyWebappService       stackconfig.Key = "webappService"
	KeyRustfsService       stackconfig.Key = "rustfsService"
	KeyQdrantService       stackconfig.Key = "qdrantService"

	KeyInfraStackRef    stackconfig.Key = "infraStackRef"
	KeyGatewayHostname  stackconfig.Key = "gatewayHostname"
	KeyGatewayName      stackconfig.Key = "gatewayName"
	KeyGatewayNamespace stackconfig.Key = "gatewayNamespace"

	KeyShaideServerUiFqdn       stackconfig.Key = "shaideServerUiFqdn"
	KeyShaideServerUiPort       stackconfig.Key = "shaideServerUiPort"
	KeyDatabaseURL              stackconfig.Key = "databaseUrl"
	KeyS3User                   stackconfig.Key = "s3User"
	KeyShaideServerS3Fqdn       stackconfig.Key = "shaideServerS3Fqdn"
	KeyShaideServerS3Port       stackconfig.Key = "shaideServerS3Port"
	KeyS3UploadProxyRoutePrefix stackconfig.Key = "s3UploadProxyRoutePrefix"
	KeyRustfsWebhookArn         stackconfig.Key = "rustfsWebhookArn"
	KeyVectorDBURL              stackconfig.Key = "vectorDBUrl"
	KeyMCPNamespace             stackconfig.Key = "mcpNamespace"
	KeyRustLibBacktrace         stackconfig.Key = "rustLibBacktrace"
	KeyRustSpantrace            stackconfig.Key = "rustSpantrace"
	KeyTrial                    stackconfig.Key = "trial"

	KeyRustfsConsoleEnabled  stackconfig.Key = "rustfsConsoleEnabled"
	KeyRustfsWebhookEnable   stackconfig.Key = "rustfsNotifyWebhookEnableShaide"
	KeyRustfsWebhookEndpoint stackconfig.Key = "rustfsNotifyWebhookEndpointShaide"
	KeyRustfsWebhookQueueDir stackconfig.Key = "rustfsNotifyWebhookQueueDirShaide"

	KeyAdminAuthKey  stackconfig.Key = "adminAuthKey"
	KeyS3Password    stackconfig.Key = "s3Password"
	KeyJWTSecret     stackconfig.Key = "jwtSecret"
	KeySessionSecret stackconfig.Key = "sessionSecret"

	KeyNodeSelectorKey          stackconfig.Key = "nodeSelectorKey"
	KeyNodeSelector             stackconfig.Key = "nodeSelector"
	KeyNodeSelectorShaide       stackconfig.Key = "nodeSelectorShaide"
	KeyNodeSelectorControlPanel stackconfig.Key = "nodeSelectorControlPanel"
	KeyNodeSelectorWebapp       stackconfig.Key = "nodeSelectorWebapp"
	KeyNodeSelectorRustfs       stackconfig.Key = "nodeSelectorRustfs"
	KeyNodeSelectorQdrant       stackconfig.Key = "nodeSelectorQdrant"

	KeyStorageClassName       stackconfig.Key = "storageClassName"
	KeyPVNodeHostname         stackconfig.Key = "pvNodeHostname"
	KeyShaidePVSize           stackconfig.Key = "shaidePVSize"
	KeyRustfsPVSize           stackconfig.Key = "rustfsPVSize"
	KeyQdrantPVSize           stackconfig.Key = "qdrantPVSize"
	KeyKnowledgeCenterEnabled stackconfig.Key = "knowledgeCenterEnabled"
	KeyLBAnnotations          stackconfig.Key = "lbAnnotations"
	KeySAAnnotations          stackconfig.Key = "serviceAccountAnnotations"
)

// Defaults shared by every deployment. They describe how the shaide components
// address each other inside the cluster, so they are the same wherever the
// platform is installed.
const (
	DefaultNamespace        = "app-shaide"
	DefaultServiceAccount   = "shaide-server"
	DefaultMCPNamespace     = "mcp-gateway"
	DefaultGatewayName      = "shared-gateway"
	DefaultGatewayNamespace = "gateway-system"

	DefaultControlPanelService = "control-panel"
	DefaultWebappService       = "webapp"
	DefaultRustfsService       = "rustfs"
	DefaultQdrantService       = "qdrant"

	DefaultShaideServerUiFqdn       = "control-panel"
	DefaultShaideServerUiPort       = "3000"
	DefaultShaideServerS3Fqdn       = "rustfs"
	DefaultShaideServerS3Port       = "9000"
	DefaultS3User                   = "rustfsuser"
	DefaultS3UploadProxyRoutePrefix = "/s3"
	DefaultVectorDBURL              = "http://qdrant:6334"
	DefaultDatabaseURL              = "sqlite:///root/.config/axem/shaide/db/shaide.sqlite"
	DefaultRustfsWebhookArn         = "arn:rustfs:sqs:eu-central-1:shaide:webhook"

	DefaultRustfsWebhookEnable   = "on"
	DefaultRustfsWebhookEndpoint = "http://shaide-server/v1/object-storage/event"
	DefaultRustfsWebhookQueueDir = "/data/deploy/logs/notify"

	DefaultRustLibBacktrace = "1"
	DefaultRustSpantrace    = "0"
	DefaultTrial            = "FALSE"

	DefaultGHCRUser = "axem-solutions"
)

// Sources are app-shaide values the installer already knows. An empty value is
// not written, so a Pulumi stack value set by hand is left intact.
//
// Images are resolved from the image manifest rather than asked for: the
// installer mirrored them and therefore knows where they now live.
type Sources struct {
	HarborHostname  string
	RegistryUser    string
	RegistryToken   string
	GatewayHostname string

	ShaideServerImage string
	ControlPanelImage string
	WebappImage       string
	RustfsImage       string
	QdrantImage       string
	BusyboxImage      string
}

type Config struct {
	stack.Config
	definition stackconfig.Config[Values]
}

func New(projectDir string, opts stack.Options, sources Sources) Config {
	definition := newDefinition(opts, sources)

	return Config{
		Config:     stack.NewConfig(Namespace, StackName, projectDir, definition),
		definition: definition,
	}
}

// newDefinition declares the stack's entire external configuration.
//
// Only the four secrets are asked for. Everything else is either supplied from
// installer runtime state, or shipped as a default describing how the
// components address one another, which does not vary by deployment. Entries
// with neither remain configurable by hand: Resolve writes only the keys it
// produces, so a value set in Pulumi.<stack>.yaml survives and is read back by
// its setter.
func newDefinition(opts stack.Options, sources Sources) stackconfig.Config[Values] {
	entries := runtimeEntries(opts, sources)
	entries = append(entries, imageEntries(sources)...)
	entries = append(entries, serviceEntries()...)
	entries = append(entries, secretEntries()...)
	entries = append(entries, optionalEntries()...)

	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries:   entries,
	}
}

// runtimeEntries carry what the installer discovered about the target cluster.
func runtimeEntries(opts stack.Options, sources Sources) []stackconfig.Entry[Values] {
	return []stackconfig.Entry[Values]{
		{
			Key:    KeyCloudProvider,
			Source: stackconfig.Source{Value: string(opts.Platform)},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.CloudProvider = root.Get(KeyCloudProvider.String())
			},
		},
		{
			Key:    KeyKubeconfig,
			Source: stackconfig.Source{Value: optionalSource(opts.Kubeconfig)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Kubeconfig = root.Get(KeyKubeconfig.String())
			},
		},
		{
			// Set when the images were mirrored into Harbor. It selects the
			// registry the pull secret authenticates against; empty leaves the
			// secret pointing at ghcr.io.
			Key:    KeyHarborHostname,
			Source: stackconfig.Source{Value: optionalSource(sources.HarborHostname)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.HarborHostname = root.Get(KeyHarborHostname.String())
			},
		},
		{
			Key: KeyGHCRUser,
			Source: stackconfig.Source{
				Value:   optionalSource(sources.RegistryUser),
				Default: DefaultGHCRUser,
			},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Registry.GHCRUser = root.Get(KeyGHCRUser.String())
			},
		},
		{
			// The credential must match the registry the pull secret targets:
			// the Harbor robot password when harborHostname is set, a GHCR
			// token otherwise. The installer decides which and passes it here.
			Key:    KeyGHCRToken,
			Source: stackconfig.Source{Value: optionalSource(sources.RegistryToken)},
			Policy: stackconfig.Policy{Required: true, Secret: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Registry.GHCRToken = root.RequireSecret(KeyGHCRToken.String())
			},
		},
		{
			Key:    KeyGatewayHostname,
			Source: stackconfig.Source{Value: optionalSource(sources.GatewayHostname)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Routing.GatewayHostname = root.Get(KeyGatewayHostname.String())
			},
		},
		{
			Key:    KeyNamespace,
			Source: stackconfig.Source{Default: DefaultNamespace},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Namespace = root.Get(KeyNamespace.String())
			},
		},
		{
			Key:    KeyServiceAccount,
			Source: stackconfig.Source{Default: DefaultServiceAccount},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.ServiceAccountName = root.Get(KeyServiceAccount.String())
			},
		},
	}
}

// imageEntries come from the image manifest the installer mirrored, so the
// deployment pulls what was actually published rather than a hand-copied tag.
func imageEntries(sources Sources) []stackconfig.Entry[Values] {
	return []stackconfig.Entry[Values]{
		{
			Key:    KeyShaideServerImage,
			Source: stackconfig.Source{Value: optionalSource(sources.ShaideServerImage)},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.ShaideServer = root.Get(KeyShaideServerImage.String())
			},
		},
		{
			Key:    KeyControlPanelImage,
			Source: stackconfig.Source{Value: optionalSource(sources.ControlPanelImage)},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.ControlPanel = root.Get(KeyControlPanelImage.String())
			},
		},
		{
			Key:    KeyWebappImage,
			Source: stackconfig.Source{Value: optionalSource(sources.WebappImage)},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.WebApp = root.Get(KeyWebappImage.String())
			},
		},
		{
			Key:    KeyRustfsImage,
			Source: stackconfig.Source{Value: optionalSource(sources.RustfsImage)},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.Rustfs = root.Get(KeyRustfsImage.String())
			},
		},
		{
			Key:    KeyQdrantImage,
			Source: stackconfig.Source{Value: optionalSource(sources.QdrantImage)},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.Qdrant = root.Get(KeyQdrantImage.String())
			},
		},
		{
			Key:    KeyBusyboxImage,
			Source: stackconfig.Source{Value: optionalSource(sources.BusyboxImage)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.Busybox = root.Get(KeyBusyboxImage.String())
			},
		},
	}
}

func optionalSource(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return value
}

// serviceEntries describe how the components address each other inside the
// cluster. They are the same in every deployment, so they ship as defaults and
// are never asked for.
func serviceEntries() []stackconfig.Entry[Values] {
	simple := []struct {
		key      stackconfig.Key
		value    string
		required bool
		set      func(*Values, string)
	}{
		{KeyControlPanelService, DefaultControlPanelService, true, func(c *Values, v string) { c.Services.ControlPanel = v }},
		{KeyWebappService, DefaultWebappService, true, func(c *Values, v string) { c.Services.WebApp = v }},
		{KeyRustfsService, DefaultRustfsService, true, func(c *Values, v string) { c.Services.Rustfs = v }},
		{KeyQdrantService, DefaultQdrantService, true, func(c *Values, v string) { c.Services.Qdrant = v }},

		{KeyGatewayName, DefaultGatewayName, false, func(c *Values, v string) { c.Routing.GatewayName = v }},
		{KeyGatewayNamespace, DefaultGatewayNamespace, false, func(c *Values, v string) { c.Routing.GatewayNamespace = v }},

		{KeyShaideServerUiFqdn, DefaultShaideServerUiFqdn, true, func(c *Values, v string) { c.ShaideEnv.ShaideServerUiFQDN = v }},
		{KeyShaideServerUiPort, DefaultShaideServerUiPort, true, func(c *Values, v string) { c.ShaideEnv.ShaideServerUiPort = v }},
		{KeyShaideServerS3Fqdn, DefaultShaideServerS3Fqdn, true, func(c *Values, v string) { c.ShaideEnv.S3FQDN = v }},
		{KeyShaideServerS3Port, DefaultShaideServerS3Port, true, func(c *Values, v string) { c.ShaideEnv.S3Port = v }},
		{KeyS3User, DefaultS3User, true, func(c *Values, v string) { c.ShaideEnv.S3User = v }},
		{KeyS3UploadProxyRoutePrefix, DefaultS3UploadProxyRoutePrefix, true, func(c *Values, v string) { c.ShaideEnv.S3UploadProxyRoutePrefix = v }},
		{KeyVectorDBURL, DefaultVectorDBURL, true, func(c *Values, v string) { c.ShaideEnv.VectorDBUrl = v }},
		{KeyDatabaseURL, DefaultDatabaseURL, true, func(c *Values, v string) { c.ShaideEnv.DatabaseURL = v }},
		{KeyRustfsWebhookArn, DefaultRustfsWebhookArn, true, func(c *Values, v string) { c.ShaideEnv.RustFSWebhookARN = v }},
		{KeyMCPNamespace, DefaultMCPNamespace, false, func(c *Values, v string) { c.ShaideEnv.MCPNamespace = v }},
		{KeyRustLibBacktrace, DefaultRustLibBacktrace, false, func(c *Values, v string) { c.ShaideEnv.RustLibBacktrace = v }},
		{KeyRustSpantrace, DefaultRustSpantrace, false, func(c *Values, v string) { c.ShaideEnv.RustSpantrace = v }},
		{KeyTrial, DefaultTrial, false, func(c *Values, v string) { c.ShaideEnv.Trial = v }},

		{KeyRustfsWebhookEnable, DefaultRustfsWebhookEnable, true, func(c *Values, v string) { c.RustEnv.WebhookEnableShaide = v }},
		{KeyRustfsWebhookEndpoint, DefaultRustfsWebhookEndpoint, true, func(c *Values, v string) { c.RustEnv.WebhookEndpointShaide = v }},
		{KeyRustfsWebhookQueueDir, DefaultRustfsWebhookQueueDir, true, func(c *Values, v string) { c.RustEnv.WebhookQueueDirShaide = v }},
	}

	entries := make([]stackconfig.Entry[Values], 0, len(simple))
	for _, item := range simple {
		entries = append(entries, stackconfig.Entry[Values]{
			Key:    item.key,
			Source: stackconfig.Source{Default: item.value},
			Policy: stackconfig.Policy{Required: item.required},
			Setter: setterFor(item.key, item.set),
		})
	}

	return entries
}

// setterFor adapts a plain string assignment to the framework's setter shape.
func setterFor(key stackconfig.Key, set func(*Values, string)) stackconfig.Setter[Values] {
	return func(cfg *Values, root *pulumiconfig.Config) {
		set(cfg, root.Get(key.String()))
	}
}

// secretEntries are the only values an operator is asked for. Each is a
// distinct credential the deployment cannot invent, and losing one after an
// install invalidates whatever it protects, so they are recorded per stack.
func secretEntries() []stackconfig.Entry[Values] {
	secrets := []struct {
		key   stackconfig.Key
		title string
		set   func(*Values, pulumi.StringOutput)
	}{
		{KeyAdminAuthKey, "Shaide admin password", func(c *Values, v pulumi.StringOutput) { c.Secrets.AdminAuthKey = v }},
		{KeyS3Password, "Object storage (rustfs) password", func(c *Values, v pulumi.StringOutput) { c.Secrets.S3Password = v }},
		{KeyJWTSecret, "JWT signing secret", func(c *Values, v pulumi.StringOutput) { c.Secrets.JWTSecret = v }},
		{KeySessionSecret, "Control panel session secret", func(c *Values, v pulumi.StringOutput) { c.Secrets.SessionSecret = v }},
	}

	entries := make([]stackconfig.Entry[Values], 0, len(secrets))
	for _, item := range secrets {
		key, set := item.key, item.set
		entries = append(entries, stackconfig.Entry[Values]{
			Key: key,
			Prompt: &stackconfig.Prompt{
				Kind:  stackconfig.PromptInput,
				Title: item.title,
			},
			Policy: stackconfig.Policy{Required: true, Secret: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				set(cfg, root.RequireSecret(key.String()))
			},
		})
	}

	return entries
}

// optionalEntries are per-deployment refinements. They are neither asked for
// nor written, so the program's own defaults apply unless someone sets them by
// hand in Pulumi.<stack>.yaml, which Resolve then leaves alone.
func optionalEntries() []stackconfig.Entry[Values] {
	plain := []struct {
		key stackconfig.Key
		set func(*Values, string)
	}{
		{KeyInfraStackRef, func(c *Values, v string) { c.Routing.InfraStackRef = v }},
		{KeyNodeSelectorKey, func(c *Values, v string) { c.NodeSelectorKey = v }},
		{KeyNodeSelector, func(c *Values, v string) { c.NodeSelector = v }},
		{KeyNodeSelectorShaide, func(c *Values, v string) { c.NodeSelectorShaide = v }},
		{KeyNodeSelectorControlPanel, func(c *Values, v string) { c.NodeSelectorControlPanel = v }},
		{KeyNodeSelectorWebapp, func(c *Values, v string) { c.NodeSelectorWebApp = v }},
		{KeyNodeSelectorRustfs, func(c *Values, v string) { c.NodeSelectorRustfs = v }},
		{KeyNodeSelectorQdrant, func(c *Values, v string) { c.NodeSelectorQdrant = v }},
		{KeyStorageClassName, func(c *Values, v string) { c.StorageClassName = v }},
		{KeyPVNodeHostname, func(c *Values, v string) { c.PVNodeHostname = v }},
		{KeyShaidePVSize, func(c *Values, v string) { c.ShaidePVSize = v }},
		{KeyRustfsPVSize, func(c *Values, v string) { c.RustfsPVSize = v }},
		{KeyQdrantPVSize, func(c *Values, v string) { c.QdrantPVSize = v }},
	}

	entries := make([]stackconfig.Entry[Values], 0, len(plain)+4)
	for _, item := range plain {
		entries = append(entries, stackconfig.Entry[Values]{
			Key:    item.key,
			Setter: setterFor(item.key, item.set),
		})
	}

	return append(entries,
		stackconfig.Entry[Values]{
			Key: KeyKnowledgeCenterEnabled,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.KnowledgeCenterEnabled = root.GetBool(KeyKnowledgeCenterEnabled.String())
			},
		},
		stackconfig.Entry[Values]{
			Key: KeyRustfsConsoleEnabled,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.RustEnv.ConsoleEnabled = root.GetBool(KeyRustfsConsoleEnabled.String())
			},
		},
		stackconfig.Entry[Values]{
			Key: KeyLBAnnotations,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				root.GetObject(KeyLBAnnotations.String(), &cfg.LBAnnotations)
			},
		},
		stackconfig.Entry[Values]{
			Key: KeySAAnnotations,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				root.GetObject(KeySAAnnotations.String(), &cfg.ServiceAccountAnnotations)
			},
		},
	)
}

// Defaults the definition does not write, so they apply to a direct pulumi up
// as well as an installer-driven deployment.
const (
	DefaultNodeSelectorKey = "nodegroup"
	DefaultBusyboxImage    = "busybox:1.37"
	DefaultPVSize          = "5Gi"
)
