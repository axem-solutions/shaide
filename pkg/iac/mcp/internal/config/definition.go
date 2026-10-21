package appconfig

import (
	"errors"
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	Namespace = "app-mcp"
	StackName = "mcp"
)

const (
	KeyKubeconfig       stackconfig.Key = "kubeconfig"
	KeyContext          stackconfig.Key = "context"
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

	KeyKnowledgeCenterEnabled  stackconfig.Key = "knowledgeCenterEnabled"
	KeyKnowledgeCenterMCPImage stackconfig.Key = "knowledgeCenterMcpImage"
	KeyKCGatewayToken          stackconfig.Key = "kcGatewayToken"
	KeyKCScopedToken           stackconfig.Key = "kcScopedToken"
	KeyKCContextPublicKey      stackconfig.Key = "kcContextPublicKey"
	KeyPlacement               stackconfig.Key = "placement"

	KeyKCMCPCPURequest    stackconfig.Key = "knowledgeCenterMcpCpuRequest"
	KeyKCMCPMemoryRequest stackconfig.Key = "knowledgeCenterMcpMemoryRequest"
	KeyKCMCPCPULimit      stackconfig.Key = "knowledgeCenterMcpCpuLimit"
	KeyKCMCPMemoryLimit   stackconfig.Key = "knowledgeCenterMcpMemoryLimit"
)

const (
	// DefaultNamespace is where MCP servers run and shaide-server discovers
	// them. It is a program default, not a written one, so a stack that
	// pinned the former mcp-gateway namespace is never moved.
	DefaultNamespace       = knowledgecenter.MCPNamespace
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

	// KnowledgeCenterEnabled is the operator's single Knowledge Center
	// answer, the same one app-shaide receives. With it, KC-MCP runs here and
	// generic datasources become optional. Nil leaves it out.
	KnowledgeCenterEnabled *bool

	// KnowledgeCenter are the installer's credentials; app-mcp reads only
	// GatewayToken, ScopedToken and ContextPublicKey.
	KnowledgeCenter knowledgecenter.Credentials

	// Images maps an upstream image name to the reference the cluster pulls
	// it from; only KC-MCP's is read.
	Images map[string]string

	// Placement names the nodes KC-MCP prefers, e.g. the CPU pools.
	Placement []PlacementTerm
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
	entries := coreEntries(opts, sources)
	entries = append(entries, knowledgeCenterEntries(sources)...)

	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries:   entries,
		Checks:    []stackconfig.Check{requireDatasources, checkKnowledgeCenterCredentials},
	}
}

func coreEntries(opts stack.Options, sources Sources) []stackconfig.Entry[Values] {
	return []stackconfig.Entry[Values]{
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
			// Not written: applyDefaults supplies app-mcp, and a stack
			// that pinned another namespace keeps it.
			Key: KeyNamespace,
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
			// Declared before the datasources and KC-MCP values, whose
			// requirement depends on it.
			Key:    KeyKnowledgeCenterEnabled,
			Source: stackconfig.Source{Value: optionalBool(sources.KnowledgeCenterEnabled), Default: false},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.KnowledgeCenterEnabled = root.GetBool(KeyKnowledgeCenterEnabled.String())
			},
		},
		{
			// Always active, so datasources run alongside KC-MCP. Without
			// KC-MCP there is nothing else to run, so requireDatasources
			// insists on at least one; with it, none is needed. (A When
			// condition would drop the datasources whenever KC is enabled.)
			Key:    KeyDatasources,
			Source: stackconfig.Source{Value: optionalDatasources(sources.Datasources)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				if err := root.TryObject(KeyDatasources.String(), &cfg.Datasources); err != nil && !errors.Is(err, pulumiconfig.ErrMissingVar) {
					cfg.datasourceLoadError = err
				}
			},
		},
		{
			Key:    KeyPlacement,
			Source: stackconfig.Source{Value: optionalPlacement(sources.Placement)},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				_ = root.TryObject(KeyPlacement.String(), &cfg.Placement)
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
		{
			Key: KeyKCMCPCPURequest,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.KnowledgeCenterMCP.Resources.CPURequest = root.Get(KeyKCMCPCPURequest.String())
			},
		},
		{
			Key: KeyKCMCPMemoryRequest,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.KnowledgeCenterMCP.Resources.MemoryRequest = root.Get(KeyKCMCPMemoryRequest.String())
			},
		},
		{
			Key: KeyKCMCPCPULimit,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.KnowledgeCenterMCP.Resources.CPULimit = root.Get(KeyKCMCPCPULimit.String())
			},
		},
		{
			Key: KeyKCMCPMemoryLimit,
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.KnowledgeCenterMCP.Resources.MemoryLimit = root.Get(KeyKCMCPMemoryLimit.String())
			},
		},
	}
}

// knowledgeCenterEntries carry KC-MCP's image and the three credentials it
// uses. They come from the installer only, are required only with KC, and
// are never generated here.
func knowledgeCenterEntries(sources Sources) []stackconfig.Entry[Values] {
	whenEnabled := stackconfig.WhenEquals(KeyKnowledgeCenterEnabled, true)
	c := sources.KnowledgeCenter

	entries := []stackconfig.Entry[Values]{
		{
			Key:    KeyKnowledgeCenterMCPImage,
			Source: stackconfig.Source{Value: optionalSource(sources.Images[knowledgecenter.MCPImageName])},
			Policy: stackconfig.Policy{Required: true, When: whenEnabled},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				cfg.Images.KnowledgeCenterMCP = root.Get(KeyKnowledgeCenterMCPImage.String())
			},
		},
	}

	for _, item := range []struct {
		key      stackconfig.Key
		value    string
		validate func(string) error
		assign   func(*Values, pulumi.StringOutput)
	}{
		{KeyKCGatewayToken, c.GatewayToken, knowledgecenter.ValidateGatewayToken, func(v *Values, s pulumi.StringOutput) { v.Secrets.KCGatewayToken = s }},
		{KeyKCScopedToken, c.ScopedToken, knowledgecenter.ValidateHopToken, func(v *Values, s pulumi.StringOutput) { v.Secrets.KCScopedToken = s }},
		{KeyKCContextPublicKey, c.ContextPublicKey, knowledgecenter.ValidatePublicKey, func(v *Values, s pulumi.StringOutput) { v.Secrets.KCContextPublicKey = s }},
	} {
		key, assign := item.key, item.assign
		entries = append(entries, stackconfig.Entry[Values]{
			Key:    key,
			Source: stackconfig.Source{Value: optionalSecretSource(item.value)},
			Policy: stackconfig.Policy{Required: true, Secret: true, When: whenEnabled, Validate: item.validate},
			Setter: func(cfg *Values, root *pulumiconfig.Config) {
				if value, err := root.TrySecret(key.String()); err == nil {
					assign(cfg, value)
				}
			},
		})
	}

	return entries
}

// requireDatasources fails a stack with nothing to run: no generic
// datasource and no KC-MCP.
func requireDatasources(values stackconfig.Values) error {
	if values[KeyKnowledgeCenterEnabled] == true {
		return nil
	}
	if _, ok := values[KeyDatasources]; !ok {
		return fmt.Errorf("%s:%s is required", Namespace, KeyDatasources)
	}
	return nil
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
	for _, entry := range knowledgeCenterEntries(Sources{}) {
		if entry.Key == KeyKnowledgeCenterEnabled || entry.Key == KeyKnowledgeCenterMCPImage {
			continue
		}
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
	return checkKnowledgeCenterCredentials(values)
}

// checkKnowledgeCenterCredentials repeats the token format checks on the
// exact values written (Policy.Validate sees them trimmed) and rejects a
// gateway token equal to the scoped token, which KC-MCP refuses at start.
// Errors name fields, never values.
func checkKnowledgeCenterCredentials(values stackconfig.Values) error {
	if values[KeyKnowledgeCenterEnabled] != true {
		return nil
	}

	text := func(key stackconfig.Key) string {
		s, _ := values[key].(string)
		return s
	}
	for _, key := range []stackconfig.Key{KeyKCGatewayToken, KeyKCScopedToken} {
		check := knowledgecenter.ValidateHopToken
		if key == KeyKCGatewayToken {
			check = knowledgecenter.ValidateGatewayToken
		}
		if err := check(text(key)); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}

	return knowledgecenter.ValidateDistinct(map[string]string{
		KeyKCGatewayToken.String(): text(KeyKCGatewayToken),
		KeyKCScopedToken.String():  text(KeyKCScopedToken),
	})
}

func optionalBool(value *bool) any {
	if value == nil {
		return nil
	}

	return *value
}

// optionalSecretSource keeps a secret's exact bytes and only drops an absent
// value.
func optionalSecretSource(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func optionalPlacement(terms []PlacementTerm) any {
	if len(terms) == 0 {
		return nil
	}

	return terms
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
