package appconfig

import (
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
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
	KeyWebappEnabled  stackconfig.Key = "webappEnabled"
	KeyKubeconfig     stackconfig.Key = "kubeconfig"
	KeyContext        stackconfig.Key = "context"
	KeyNamespace      stackconfig.Key = "namespace"
	KeyServiceAccount stackconfig.Key = "shaideServiceAccountName"

	KeyShaideServerImage    stackconfig.Key = "shaideServerImage"
	KeyControlPanelImage    stackconfig.Key = "controlPanelImage"
	KeyWebappImage          stackconfig.Key = "webappImage"
	KeyRustfsImage          stackconfig.Key = "rustfsImage"
	KeyQdrantImage          stackconfig.Key = "qdrantImage"
	KeyBusyboxImage         stackconfig.Key = "busyboxImage"
	KeyKnowledgeCenterImage stackconfig.Key = "knowledgeCenterImage"

	KeyControlPanelService stackconfig.Key = "controlPanelService"
	KeyWebappService       stackconfig.Key = "webappService"
	KeyRustfsService       stackconfig.Key = "rustfsService"
	KeyQdrantService       stackconfig.Key = "qdrantService"

	KeyInfraStackRef    stackconfig.Key = "infraStackRef"
	KeyGatewayHostname  stackconfig.Key = "gatewayHostname"
	KeyGatewayName      stackconfig.Key = "gatewayName"
	KeyGatewayNamespace stackconfig.Key = "gatewayNamespace"

	KeyShaideServerUiFqdn stackconfig.Key = "shaideServerUiFqdn"
	KeyShaideServerUiPort stackconfig.Key = "shaideServerUiPort"
	KeyDatabaseURL        stackconfig.Key = "databaseUrl"
	KeyS3User             stackconfig.Key = "s3User"
	KeyShaideServerS3Fqdn stackconfig.Key = "shaideServerS3Fqdn"
	KeyShaideServerS3Port stackconfig.Key = "shaideServerS3Port"
	KeyRustfsWebhookArn   stackconfig.Key = "rustfsWebhookArn"
	KeyVectorDBURL        stackconfig.Key = "vectorDBUrl"
	KeyMCPNamespace       stackconfig.Key = "mcpNamespace"
	KeyRustLibBacktrace   stackconfig.Key = "rustLibBacktrace"
	KeyRustSpantrace      stackconfig.Key = "rustSpantrace"
	KeyTrial              stackconfig.Key = "trial"

	KeyRustfsConsoleEnabled  stackconfig.Key = "rustfsConsoleEnabled"
	KeyRustfsWebhookEnable   stackconfig.Key = "rustfsNotifyWebhookEnableShaide"
	KeyRustfsWebhookEndpoint stackconfig.Key = "rustfsNotifyWebhookEndpointShaide"
	KeyRustfsWebhookQueueDir stackconfig.Key = "rustfsNotifyWebhookQueueDirShaide"

	KeyAdminAuthKey  stackconfig.Key = "adminAuthKey"
	KeyS3Password    stackconfig.Key = "s3Password"
	KeyJWTSecret     stackconfig.Key = "jwtSecret"
	KeySessionSecret stackconfig.Key = "sessionSecret"

	KeyKCInternalToken     stackconfig.Key = "kcInternalToken"
	KeyKCWebhookToken      stackconfig.Key = "kcWebhookToken"
	KeyKCGatewayToken      stackconfig.Key = "kcGatewayToken"
	KeyKCScopedToken       stackconfig.Key = "kcScopedToken"
	KeyKCContextSigningKey stackconfig.Key = "kcContextSigningKey"
	KeyKCContextPublicKey  stackconfig.Key = "kcContextPublicKey"
	KeyKCEmbeddingUsername stackconfig.Key = "kcEmbeddingUsername"
	KeyKCEmbeddingPassword stackconfig.Key = "kcEmbeddingPassword"

	KeyKnowledgeCenterCPURequest    stackconfig.Key = "knowledgeCenterCpuRequest"
	KeyKnowledgeCenterMemoryRequest stackconfig.Key = "knowledgeCenterMemoryRequest"
	KeyKnowledgeCenterCPULimit      stackconfig.Key = "knowledgeCenterCpuLimit"
	KeyKnowledgeCenterMemoryLimit   stackconfig.Key = "knowledgeCenterMemoryLimit"
	KeyRustfsClientNamespaces       stackconfig.Key = "rustfsClientNamespaces"

	KeyPlacement                stackconfig.Key = "placement"
	KeyNodeSelectorKey          stackconfig.Key = "nodeSelectorKey"
	KeyNodeSelector             stackconfig.Key = "nodeSelector"
	KeyNodeSelectorShaide       stackconfig.Key = "nodeSelectorShaide"
	KeyNodeSelectorControlPanel stackconfig.Key = "nodeSelectorControlPanel"
	KeyNodeSelectorWebapp       stackconfig.Key = "nodeSelectorWebapp"
	KeyNodeSelectorRustfs       stackconfig.Key = "nodeSelectorRustfs"
	KeyNodeSelectorQdrant       stackconfig.Key = "nodeSelectorQdrant"
	KeyNodeSelectorKC           stackconfig.Key = "nodeSelectorKnowledgeCenter"

	KeyStorageClassName       stackconfig.Key = "storageClassName"
	KeyPVNodeHostname         stackconfig.Key = "pvNodeHostname"
	KeyShaidePVSize           stackconfig.Key = "shaidePVSize"
	KeyRustfsPVSize           stackconfig.Key = "rustfsPVSize"
	KeyQdrantPVSize           stackconfig.Key = "qdrantPVSize"
	KeyKnowledgeCenterEnabled stackconfig.Key = "knowledgeCenterEnabled"
	KeySAAnnotations          stackconfig.Key = "serviceAccountAnnotations"
)

// Defaults shared by every deployment. They describe how the shaide components
// address each other inside the cluster, so they are the same wherever the
// platform is installed.
const (
	DefaultNamespace      = "app-shaide"
	DefaultServiceAccount = "shaide-server"
	DefaultGatewayName    = "shared-gateway"

	// DefaultMCPNamespace is where shaide-server discovers MCP servers, and
	// where KC-MCP runs. It is a program default rather than a written one,
	// so a stack that pinned the former mcp-gateway namespace keeps it.
	DefaultMCPNamespace = knowledgecenter.MCPNamespace

	// DefaultRustfsClientNamespace holds Loki, which keeps its chunks in
	// RustFS. It is admitted when KC restricts RustFS ingress.
	DefaultRustfsClientNamespace = "monitoring"
	DefaultGatewayNamespace      = "gateway-system"

	DefaultControlPanelService = "control-panel"
	DefaultWebappService       = "webapp"
	DefaultRustfsService       = "rustfs"
	DefaultQdrantService       = "qdrant"

	// WebAppPort is where the webapp listens and its Service is exposed.
	WebAppPort = 8787

	DefaultShaideServerUiFqdn = "control-panel"
	DefaultShaideServerUiPort = "3000"
	DefaultShaideServerS3Fqdn = "rustfs"
	DefaultShaideServerS3Port = "9000"
	DefaultS3User             = "rustfsuser"
	DefaultVectorDBURL        = "http://qdrant:6334"
	DefaultDatabaseURL        = "sqlite:///root/.config/axem/shaide/db/shaide.sqlite"
	DefaultRustfsWebhookArn   = "arn:rustfs:sqs:eu-central-1:shaide:webhook"

	DefaultRustfsWebhookEnable   = "on"
	DefaultRustfsWebhookEndpoint = "http://shaide-server/v1/object-storage/event"
	DefaultRustfsWebhookQueueDir = "/data/deploy/logs/notify"

	DefaultRustLibBacktrace = "1"
	DefaultRustSpantrace    = "0"
	DefaultTrial            = "FALSE"
)

// Upstream names of the images app-shaide deploys, as the image manifest lists
// them. They key Sources.Images.
const (
	ShaideServerImageName = "axem-solutions/shaide_server"
	ControlPanelImageName = "axem-solutions/shaide_control_panel"
	WebappImageName       = "axem-solutions/shaide-webapp"
	RustfsImageName       = "rustfs/rustfs"
	QdrantImageName       = "qdrant/qdrant"
	BusyboxImageName      = "busybox"

	KnowledgeCenterImageName = knowledgecenter.ImageName
)

// Sources are app-shaide values the installer already knows. An empty value is
// not written, so a Pulumi stack value set by hand is left intact.
type Sources struct {
	GatewayHostname string

	// WebAppEnabled is the operator's answer to whether the webapp is
	// installed. Nil leaves the stack value, or the default of leaving it out.
	WebAppEnabled *bool

	// Placement names the nodes the components prefer to run on. The
	// installer sets it to the node pools assigned to CPU work.
	Placement []PlacementTerm

	// Images maps an upstream image name to the reference the cluster pulls
	// it from. It may hold images other stacks deploy; only the ones named
	// above are read.
	Images map[string]string

	// KnowledgeCenterEnabled is the operator's single answer to whether the
	// Knowledge Center (KC and KC-MCP) is installed. app-mcp receives the
	// same answer. Nil leaves it out.
	KnowledgeCenterEnabled *bool

	// KnowledgeCenter are the credentials the installer generated once and
	// keeps. They are read only when the Knowledge Center is installed.
	KnowledgeCenter knowledgecenter.Credentials
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
	entries = append(entries, knowledgeCenterEntries(sources.KnowledgeCenter)...)
	entries = append(entries, optionalEntries()...)

	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries:   entries,
		Checks:    []stackconfig.Check{checkKnowledgeCenterCredentials},
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
			// Without it the provider falls back to the kubeconfig's
			// current-context, which need not be the cluster the operator
			// selected.
			Key:    KeyContext,
			Source: stackconfig.Source{Value: optionalSource(opts.Context)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Context = root.Get(KeyContext.String())
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
			// Written only when the installer assigned CPU pools, so a
			// hand-set node selector in Pulumi.<stack>.yaml stays in effect
			// otherwise.
			Key:    KeyPlacement,
			Source: stackconfig.Source{Value: optionalPlacement(sources.Placement)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				_ = root.TryObject(KeyPlacement.String(), &cfg.Placement)
			},
		},
		{
			// Declared before the images, since the webapp image is required
			// only when the webapp is installed.
			Key:    KeyWebappEnabled,
			Source: stackconfig.Source{Value: optionalBool(sources.WebAppEnabled), Default: false},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.WebAppEnabled = root.GetBool(KeyWebappEnabled.String())
			},
		},
		{
			// Declared before the images and credentials, which are required
			// only when the Knowledge Center is installed. The control panel
			// shows the answer through KNOWLEDGE_CENTER_ENABLED.
			Key:    KeyKnowledgeCenterEnabled,
			Source: stackconfig.Source{Value: optionalBool(sources.KnowledgeCenterEnabled), Default: false},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.KnowledgeCenterEnabled = root.GetBool(KeyKnowledgeCenterEnabled.String())
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
// A required image missing from the manifest fails at resolve time. The images
// are pulled anonymously, so no pull secret goes with them.
func imageEntries(sources Sources) []stackconfig.Entry[Values] {
	return []stackconfig.Entry[Values]{
		{
			Key:    KeyShaideServerImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[ShaideServerImageName])},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.ShaideServer = root.Get(KeyShaideServerImage.String())
			},
		},
		{
			Key:    KeyControlPanelImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[ControlPanelImageName])},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.ControlPanel = root.Get(KeyControlPanelImage.String())
			},
		},
		{
			Key:    KeyWebappImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[WebappImageName])},
			Policy: stackconfig.Policy{Required: true, When: stackconfig.WhenEquals(KeyWebappEnabled, true)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.WebApp = root.Get(KeyWebappImage.String())
			},
		},
		{
			Key:    KeyRustfsImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[RustfsImageName])},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.Rustfs = root.Get(KeyRustfsImage.String())
			},
		},
		{
			Key:    KeyQdrantImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[QdrantImageName])},
			Policy: stackconfig.Policy{Required: true},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.Qdrant = root.Get(KeyQdrantImage.String())
			},
		},
		{
			Key:    KeyBusyboxImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[BusyboxImageName])},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.Busybox = root.Get(KeyBusyboxImage.String())
			},
		},
		{
			Key:    KeyKnowledgeCenterImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[KnowledgeCenterImageName])},
			Policy: stackconfig.Policy{Required: true, When: stackconfig.WhenEquals(KeyKnowledgeCenterEnabled, true)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.KnowledgeCenter = root.Get(KeyKnowledgeCenterImage.String())
			},
		},
	}
}

func optionalPlacement(terms []PlacementTerm) any {
	if len(terms) == 0 {
		return nil
	}

	return terms
}

func optionalBool(value *bool) any {
	if value == nil {
		return nil
	}

	return *value
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
		{KeyVectorDBURL, DefaultVectorDBURL, true, func(c *Values, v string) { c.ShaideEnv.VectorDBUrl = v }},
		{KeyDatabaseURL, DefaultDatabaseURL, true, func(c *Values, v string) { c.ShaideEnv.DatabaseURL = v }},
		{KeyRustfsWebhookArn, DefaultRustfsWebhookArn, true, func(c *Values, v string) { c.ShaideEnv.RustFSWebhookARN = v }},
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

// knowledgeCenterEntries carry the credentials the installer generated for
// the Knowledge Center. They are never asked for and never generated here:
// an enabled KC without them fails at resolve time, and a declined KC
// neither needs nor writes them.
func knowledgeCenterEntries(c knowledgecenter.Credentials) []stackconfig.Entry[Values] {
	whenEnabled := stackconfig.WhenEquals(KeyKnowledgeCenterEnabled, true)

	items := []struct {
		key      stackconfig.Key
		value    string
		secret   bool
		validate func(string) error
		set      func(*Values, *pulumiconfig.Config, string)
	}{
		{KeyKCInternalToken, c.InternalToken, true, knowledgecenter.ValidateServiceToken, secretSetter(func(v *Values, s pulumi.StringOutput) { v.Secrets.KCInternalToken = s })},
		{KeyKCWebhookToken, c.WebhookToken, true, knowledgecenter.ValidateServiceToken, secretSetter(func(v *Values, s pulumi.StringOutput) { v.Secrets.KCWebhookToken = s })},
		{KeyKCGatewayToken, c.GatewayToken, true, knowledgecenter.ValidateGatewayToken, secretSetter(func(v *Values, s pulumi.StringOutput) { v.Secrets.KCGatewayToken = s })},
		{KeyKCScopedToken, c.ScopedToken, true, knowledgecenter.ValidateHopToken, secretSetter(func(v *Values, s pulumi.StringOutput) { v.Secrets.KCScopedToken = s })},
		{KeyKCContextSigningKey, c.ContextSigningKey, true, knowledgecenter.ValidateSigningKey, secretSetter(func(v *Values, s pulumi.StringOutput) { v.Secrets.KCContextSigningKey = s })},
		{KeyKCContextPublicKey, c.ContextPublicKey, true, knowledgecenter.ValidatePublicKey, secretSetter(func(v *Values, s pulumi.StringOutput) { v.Secrets.KCContextPublicKey = s })},
		{KeyKCEmbeddingPassword, c.EmbeddingPassword, true, knowledgecenter.ValidatePassword, secretSetter(func(v *Values, s pulumi.StringOutput) { v.Secrets.KCEmbeddingPassword = s })},
		{KeyKCEmbeddingUsername, c.EmbeddingUsername, false, knowledgecenter.ValidateUsername, func(v *Values, root *pulumiconfig.Config, key string) {
			v.KnowledgeCenter.EmbeddingUsername = root.Get(key)
		}},
	}

	entries := make([]stackconfig.Entry[Values], 0, len(items))
	for _, item := range items {
		key, set := item.key, item.set
		entries = append(entries, stackconfig.Entry[Values]{
			Key:    key,
			Source: stackconfig.Source{Value: optionalSecretSource(item.value)},
			Policy: stackconfig.Policy{
				Required: true,
				Secret:   item.secret,
				When:     whenEnabled,
				Validate: item.validate,
			},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				set(cfg, root, key.String())
			},
		})
	}

	return entries
}

// secretSetter reads a secret only when it is present, so a declined KC
// loads without it.
func secretSetter(assign func(*Values, pulumi.StringOutput)) func(*Values, *pulumiconfig.Config, string) {
	return func(v *Values, root *pulumiconfig.Config, key string) {
		if value, err := root.TrySecret(key); err == nil {
			assign(v, value)
		}
	}
}

// optionalSecretSource keeps a secret's exact bytes and only drops an absent
// value; a password may legitimately contain surrounding spaces.
func optionalSecretSource(value string) any {
	if value == "" {
		return nil
	}

	return value
}

// validateLoadedKnowledgeCenter checks the exact decrypted stack config bytes
// synchronously before deployment. Plaintext stays local to validation; the
// entry setters still retain Secret outputs for all resource inputs.
func validateLoadedKnowledgeCenter(ctx *pulumi.Context) error {
	root := pulumiconfig.New(ctx, Namespace)
	if !root.GetBool(KeyKnowledgeCenterEnabled.String()) {
		return nil
	}
	values := stackconfig.Values{KeyKnowledgeCenterEnabled: true}
	for _, entry := range knowledgeCenterEntries(knowledgecenter.Credentials{}) {
		value, ok := ctx.GetConfig(Namespace + ":" + entry.Key.String())
		if !ok || value == "" {
			return fmt.Errorf("%s:%s is required", Namespace, entry.Key)
		}
		if entry.Policy.Validate != nil {
			if err := entry.Policy.Validate(value); err != nil {
				return fmt.Errorf("%s: %w", entry.Key, err)
			}
		}
		values[entry.Key] = value
	}
	for _, key := range []stackconfig.Key{KeyJWTSecret, KeyAdminAuthKey} {
		values[key], _ = ctx.GetConfig(Namespace + ":" + key.String())
	}
	return checkKnowledgeCenterCredentials(values)
}

// checkKnowledgeCenterCredentials applies the rules that span entries. It
// also repeats the format checks on the exact values written, since
// Policy.Validate sees them trimmed. Errors name fields, never values.
func checkKnowledgeCenterCredentials(values stackconfig.Values) error {
	if values[KeyKnowledgeCenterEnabled] != true {
		return nil
	}

	text := func(key stackconfig.Key) string {
		s, _ := values[key].(string)
		return s
	}

	formats := []struct {
		key   stackconfig.Key
		check func(string) error
	}{
		{KeyKCInternalToken, knowledgecenter.ValidateServiceToken},
		{KeyKCWebhookToken, knowledgecenter.ValidateServiceToken},
		{KeyKCGatewayToken, knowledgecenter.ValidateGatewayToken},
		{KeyKCScopedToken, knowledgecenter.ValidateHopToken},
		{KeyKCEmbeddingUsername, knowledgecenter.ValidateUsername},
	}
	for _, format := range formats {
		if err := format.check(text(format.key)); err != nil {
			return fmt.Errorf("%s: %w", format.key, err)
		}
	}

	// shaide-server requires a dedicated context signing key, including when
	// the PEM and JWT secret differ only in surrounding whitespace.
	signing, jwt := text(KeyKCContextSigningKey), text(KeyJWTSecret)
	if signing == jwt || strings.TrimSpace(signing) == strings.TrimSpace(jwt) {
		return fmt.Errorf("%s and %s must differ", KeyKCContextSigningKey, KeyJWTSecret)
	}

	if err := knowledgecenter.ValidateKeyPair(text(KeyKCContextSigningKey), text(KeyKCContextPublicKey)); err != nil {
		return err
	}

	// Every hop has its own credential.
	own := map[string]string{
		KeyKCInternalToken.String():     text(KeyKCInternalToken),
		KeyKCWebhookToken.String():      text(KeyKCWebhookToken),
		KeyKCGatewayToken.String():      text(KeyKCGatewayToken),
		KeyKCScopedToken.String():       text(KeyKCScopedToken),
		KeyKCEmbeddingPassword.String(): text(KeyKCEmbeddingPassword),
	}
	if err := knowledgecenter.ValidateDistinct(own); err != nil {
		return err
	}

	// None doubles as a core secret: shaide-server refuses KC tokens equal to
	// JWT_SECRET, and the embedding account must not share the admin
	// password. Core secrets are not compared with each other here.
	for _, core := range []stackconfig.Key{KeyJWTSecret, KeyAdminAuthKey} {
		value := text(core)
		if value == "" {
			continue
		}
		for name, kc := range own {
			if kc == value {
				return fmt.Errorf("%s and %s must differ", name, core)
			}
		}
	}

	return nil
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
		{KeyNodeSelectorKC, func(c *Values, v string) { c.NodeSelectorKnowledgeCenter = v }},
		{KeyMCPNamespace, func(c *Values, v string) { c.ShaideEnv.MCPNamespace = v }},
		{KeyKnowledgeCenterCPURequest, func(c *Values, v string) { c.KnowledgeCenter.Resources.CPURequest = v }},
		{KeyKnowledgeCenterMemoryRequest, func(c *Values, v string) { c.KnowledgeCenter.Resources.MemoryRequest = v }},
		{KeyKnowledgeCenterCPULimit, func(c *Values, v string) { c.KnowledgeCenter.Resources.CPULimit = v }},
		{KeyKnowledgeCenterMemoryLimit, func(c *Values, v string) { c.KnowledgeCenter.Resources.MemoryLimit = v }},
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
			// Namespaces whose pods may still reach RustFS once the Knowledge
			// Center restricts its ingress. Default: monitoring (Loki).
			Key: KeyRustfsClientNamespaces,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				_ = root.TryObject(KeyRustfsClientNamespaces.String(), &cfg.RustfsClientNamespaces)
			},
		},
		stackconfig.Entry[Values]{
			Key: KeyRustfsConsoleEnabled,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.RustEnv.ConsoleEnabled = root.GetBool(KeyRustfsConsoleEnabled.String())
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
