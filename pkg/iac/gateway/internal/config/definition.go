package config

import (
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const Namespace = "gateway-provider"

const (
	KeyCloudProvider         stackconfig.Key = "cloudProvider"
	KeyKubeconfig            stackconfig.Key = "kubeconfig"
	KeyContext               stackconfig.Key = "context"
	KeyGatewayHostname       stackconfig.Key = "gatewayHostname"
	KeyGatewayClassName      stackconfig.Key = "gatewayClassName"
	KeyGatewayNamespace      stackconfig.Key = "gatewayNamespace"
	KeyInfraStackRef         stackconfig.Key = "infraStackRef"
	KeyALBName               stackconfig.Key = "albName"
	KeyALBSubnetID           stackconfig.Key = "albSubnetId"
	KeyGatewayStaticIPName   stackconfig.Key = "gatewayStaticIPName"
	KeyGatewayStaticIP       stackconfig.Key = "gatewayStaticIP"
	KeyProvider              stackconfig.Key = "provider"
	KeyIstioNamespace        stackconfig.Key = "namespace"
	KeyIstioHub              stackconfig.Key = "istioHub"
	KeyIstioTag              stackconfig.Key = "istioTag"
	KeyInstallGatewayAPICRDs stackconfig.Key = "installGatewayApiCrds"
	KeyGatewayAPICRDsPath    stackconfig.Key = "gatewayApiCrdsPath"
	KeyGIECRDsPath           stackconfig.Key = "gieCrdsPath"
	KeyGatewayCertName       stackconfig.Key = "gatewayCertName"
	KeyTLSCertAnnotation     stackconfig.Key = "tlsCertAnnotation"
	KeyCertManagerIssuer     stackconfig.Key = "certManagerIssuer"
	KeyTLSSecretName         stackconfig.Key = "tlsSecretName"
	KeyBootstrapTLSSecret    stackconfig.Key = "bootstrapTlsSecret"
)

// Packaged CRD locations. The installer image ships the manifests next to the
// project, so an install does not reach GitHub for them. They are declared here
// rather than as Go defaults because the Go defaults are the upstream URLs,
// which is what a developer running pulumi directly should get.
const (
	PackagedGatewayAPICRDsPath = "crds/gateway-api/standard"
	PackagedGIECRDsPath        = "crds/gie"
)

type Config struct {
	stack.Config
	definition stackconfig.Config[Values]
}

func New(projectDir string, opts stack.Options) Config {
	definition := newDefinition(opts)

	return Config{
		Config:     stack.NewConfig(Namespace, Namespace, projectDir, definition),
		definition: definition,
	}
}

// newDefinition declares the stack's entire external configuration.
//
// Only values that must differ per cluster and cannot be derived are prompted
// for. Everything else is either supplied from installer runtime state, shipped
// as a packaged default, or left unset so applyDefaults can resolve it from the
// platform. An unprompted entry is still configurable: Resolve only writes the
// keys it produces, so a value set by hand in Pulumi.<stack>.yaml survives and
// is read back by its setter.
//
// cloudProvider is declared first because the Azure-only entries below depend
// on it, and Validate requires a condition's key to be declared earlier.
func newDefinition(opts stack.Options) stackconfig.Config[Values] {
	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries: []stackconfig.Entry[Values]{
			{
				Key: KeyCloudProvider,
				Source: stackconfig.Source{
					Value: string(opts.Platform),
				},
				Policy: stackconfig.Policy{
					Required: true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Platform = platform.Platform(root.Get(KeyCloudProvider.String()))
				},
			},
			{
				Key: KeyKubeconfig,
				Source: stackconfig.Source{
					Value: opts.Kubeconfig,
				},
				Policy: stackconfig.Policy{
					Required: true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Kubernetes.KubeconfigPath = root.Get(KeyKubeconfig.String())
				},
			},
			{
				Key: KeyContext,
				Source: stackconfig.Source{
					Value: opts.Context,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Kubernetes.Context = root.Get(KeyContext.String())
				},
			},
			{
				// The hostname the platform is served on. There is no sensible
				// default and every cluster needs its own.
				Key: KeyGatewayHostname,
				Prompt: &stackconfig.Prompt{
					Kind:        stackconfig.PromptInput,
					Title:       "Gateway hostname",
					Placeholder: "shaide.example.com",
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.Hostname = root.Get(KeyGatewayHostname.String())
				},
			},
			{
				// Which Gateway implementation the cluster runs. The platform
				// narrows it but does not decide it: Azure clusters split
				// between Application Gateway for Containers and Istio, so the
				// platform default is offered as the answer rather than
				// applied silently.
				Key: KeyGatewayClassName,
				Source: stackconfig.Source{
					Default: defaultsForPlatform(opts.Platform).gatewayClassName,
				},
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "Gateway class name",
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.ClassName = root.Get(KeyGatewayClassName.String())
				},
			},
			{
				Key: KeyGatewayNamespace,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.Namespace = root.Get(KeyGatewayNamespace.String())
				},
			},
			{
				Key: KeyInfraStackRef,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.InfraStackRef = root.Get(KeyInfraStackRef.String())
				},
			},
			{
				// Application Gateway for Containers exists only on Azure, so
				// the prompt does not appear anywhere else.
				Key: KeyALBName,
				Prompt: &stackconfig.Prompt{
					Kind:        stackconfig.PromptInput,
					Title:       "Azure Application Gateway for Containers name",
					Placeholder: "shared-alb",
				},
				Policy: stackconfig.Policy{
					When: stackconfig.WhenEquals(KeyCloudProvider, string(platform.Azure)),
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.ALB.Name = root.Get(KeyALBName.String())
				},
			},
			{
				Key: KeyALBSubnetID,
				Prompt: &stackconfig.Prompt{
					Kind:        stackconfig.PromptInput,
					Title:       "Azure subnet resource ID for Application Gateway for Containers",
					Placeholder: "/subscriptions/.../subnets/<subnet>",
				},
				Policy: stackconfig.Policy{
					When: stackconfig.WhenEquals(KeyCloudProvider, string(platform.Azure)),
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.ALB.SubnetID = root.Get(KeyALBSubnetID.String())
				},
			},
			{
				// Preallocated addresses are the exception rather than the
				// rule, so they are configurable without being asked for.
				Key: KeyGatewayStaticIPName,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.StaticIP.Name = root.Get(KeyGatewayStaticIPName.String())
				},
			},
			{
				Key: KeyGatewayStaticIP,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Gateway.StaticIP.IP = root.Get(KeyGatewayStaticIP.String())
				},
			},
			{
				Key: KeyProvider,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					provider := root.Get(KeyProvider.String())
					cfg.Istio.Enabled = provider == "" || provider == ProviderIstio
				},
			},
			{
				Key: KeyIstioNamespace,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Istio.Namespace = root.Get(KeyIstioNamespace.String())
				},
			},
			{
				Key: KeyIstioHub,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Istio.Hub = root.Get(KeyIstioHub.String())
				},
			},
			{
				Key: KeyIstioTag,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Istio.Tag = root.Get(KeyIstioTag.String())
				},
			},
			{
				// Empty leaves the platform default: every platform installs
				// the CRDs except Azure, where the managed service owns them.
				Key: KeyInstallGatewayAPICRDs,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					if root.Get(KeyInstallGatewayAPICRDs.String()) == "" {
						return
					}

					cfg.CRDs.InstallGatewayAPI = root.GetBool(KeyInstallGatewayAPICRDs.String())
					cfg.CRDs.installConfigured = true
				},
			},
			{
				Key: KeyGatewayAPICRDsPath,
				Source: stackconfig.Source{
					Default: PackagedGatewayAPICRDsPath,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.CRDs.GatewayAPIPath = root.Get(KeyGatewayAPICRDsPath.String())
				},
			},
			{
				Key: KeyGIECRDsPath,
				Source: stackconfig.Source{
					Default: PackagedGIECRDsPath,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.CRDs.GIEPath = root.Get(KeyGIECRDsPath.String())
				},
			},
			{
				// A cloud-managed certificate, bound through a provider
				// annotation. Mutually exclusive with certManagerIssuer below,
				// and unused on Azure, so it is not asked for.
				Key: KeyGatewayCertName,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.TLS.CertName = root.Get(KeyGatewayCertName.String())
				},
			},
			{
				Key: KeyTLSCertAnnotation,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.TLS.CertAnnotation = root.Get(KeyTLSCertAnnotation.String())
				},
			},
			{
				// Without an issuer the Gateway serves plain HTTP, so this
				// materially changes the deployment and the issuer name is
				// per-cluster.
				Key: KeyCertManagerIssuer,
				Prompt: &stackconfig.Prompt{
					Kind:        stackconfig.PromptInput,
					Title:       "cert-manager ClusterIssuer for Gateway TLS (empty serves HTTP only)",
					Placeholder: "letsencrypt",
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.TLS.CertManagerIssuer = root.Get(KeyCertManagerIssuer.String())
				},
			},
			{
				Key: KeyTLSSecretName,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.TLS.SecretName = root.Get(KeyTLSSecretName.String())
				},
			},
			{
				Key: KeyBootstrapTLSSecret,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.TLS.BootstrapSecret = root.GetBool(KeyBootstrapTLSSecret.String())
				},
			},
		},
	}
}
