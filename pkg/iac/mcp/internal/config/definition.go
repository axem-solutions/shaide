package appconfig

import (
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	Namespace = "app-mcp"
	StackName = "mcp"
)

const (
	KeyKubeconfig       stackconfig.Key = "kubeconfig"
	KeyNamespace        stackconfig.Key = "namespace"
	KeyShaideNamespace  stackconfig.Key = "shaideNamespace"
	KeyShaideSAName     stackconfig.Key = "shaideServiceAccountName"
	KeyDatasources      stackconfig.Key = "datasources"
	KeyImagePullSecrets stackconfig.Key = "imagePullSecrets"

	KeyCompanyCACert        stackconfig.Key = "companyCACert"
	KeyCompanyCATrustEnvVar stackconfig.Key = "companyCATrustEnvVar"
	KeyNodeSelectorKey      stackconfig.Key = "nodeSelectorKey"
	KeyNodeSelector         stackconfig.Key = "nodeSelector"

	KeyAtlassianOAuthClientSecret stackconfig.Key = "mcpAtlassianOAuthClientSecret"
)

const (
	DefaultNamespace       = "mcp-gateway"
	DefaultShaideNamespace = "app-shaide"
	DefaultShaideSAName    = "shaide-server"
)

// Sources are app-mcp values the installer already knows.
//
// Datasources is the list of MCP servers to run. It is a source rather than a
// prompt because the framework can only ask for scalars, and because the
// intended flow is for an operator to publish an MCP server image into Harbor
// and have the installer offer it — neither of which is a question the stack
// can pose on its own. Until that exists the list arrives from whatever the
// installer is given, and an empty list leaves the stack with nothing to run.
type Sources struct {
	Datasources      []Datasource
	ImagePullSecrets []string
	ShaideNamespace  string
	ShaideSAName     string
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

// newDefinition declares the stack's external configuration.
//
// The deployment tuning keys (probe timings, resource requests, image pull
// policy) are deliberately absent: they are per-datasource refinements with
// working defaults in applyDefaults, and writing them would turn every default
// into a value someone has to maintain.
func newDefinition(opts stack.Options, sources Sources) stackconfig.Config[Values] {
	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries: []stackconfig.Entry[Values]{
			{
				Key:    KeyKubeconfig,
				Source: stackconfig.Source{Value: optionalSource(opts.Kubeconfig)},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Kubeconfig = root.Get(KeyKubeconfig.String())
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
				// The MCP RBAC Role is bound to this ServiceAccount, so a
				// mismatch with the app-shaide stack surfaces as a 403 on the
				// Kubernetes watch rather than a deployment failure.
				Key: KeyShaideNamespace,
				Source: stackconfig.Source{
					Value:   optionalSource(sources.ShaideNamespace),
					Default: DefaultShaideNamespace,
				},
				Policy: stackconfig.Policy{Required: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.ShaideNamespace = root.Get(KeyShaideNamespace.String())
				},
			},
			{
				Key: KeyShaideSAName,
				Source: stackconfig.Source{
					Value:   optionalSource(sources.ShaideSAName),
					Default: DefaultShaideSAName,
				},
				Policy: stackconfig.Policy{Required: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.ShaideServiceAccountName = root.Get(KeyShaideSAName.String())
				},
			},
			{
				Key:    KeyDatasources,
				Source: stackconfig.Source{Value: optionalDatasources(sources.Datasources)},
				Policy: stackconfig.Policy{Required: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					root.RequireObject(KeyDatasources.String(), &cfg.Datasources)
				},
			},
			{
				Key:    KeyImagePullSecrets,
				Source: stackconfig.Source{Value: optionalStrings(sources.ImagePullSecrets)},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					root.GetObject(KeyImagePullSecrets.String(), &cfg.ImagePullSecrets)
				},
			},
			{
				Key: KeyCompanyCACert,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.CompanyCACert = root.Get(KeyCompanyCACert.String())
				},
			},
			{
				Key: KeyCompanyCATrustEnvVar,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.CompanyCATrustEnvVar = root.Get(KeyCompanyCATrustEnvVar.String())
				},
			},
			{
				Key: KeyNodeSelectorKey,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.NodeSelectorKey = root.Get(KeyNodeSelectorKey.String())
				},
			},
			{
				Key: KeyNodeSelector,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.NodeSelector = root.Get(KeyNodeSelector.String())
				},
			},
			{
				Key:    KeyAtlassianOAuthClientSecret,
				Policy: stackconfig.Policy{Secret: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					if root.Get(KeyAtlassianOAuthClientSecret.String()) == "" {
						return
					}

					cfg.Secrets.AtlassianOAuthClientSecret = root.RequireSecret(KeyAtlassianOAuthClientSecret.String())
					cfg.Secrets.HasAtlassianOAuthClientSecret = true

					// The datasource gains the env var that reads this secret,
					// so it is wired only when the secret actually exists.
					addAtlassianSecretEnv(cfg.Datasources)
				},
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

func optionalStrings(values []string) any {
	if len(values) == 0 {
		return nil
	}

	return values
}

func optionalDatasources(values []Datasource) any {
	if len(values) == 0 {
		return nil
	}

	return values
}
