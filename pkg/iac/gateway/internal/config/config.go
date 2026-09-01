package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	kubernetes "github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	DefaultIstioNamespace = "istio-system"
	DefaultIstioHub       = "docker.io/istio"
	DefaultIstioTag       = "1.28.1"

	DefaultGatewayNamespace = "gateway-system"
	DefaultTLSSecretName    = "gateway-tls"

	DefaultGatewayAPICRDsPath = "https://github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=v1.5.1"
	DefaultGIECRDsPath        = "https://github.com/kubernetes-sigs/gateway-api-inference-extension/config/crd?ref=v1.4.0"
)

type Config struct {
	Platform   platform.Platform
	Kubernetes kubernetes.Connection

	Gateway struct {
		Hostname      string
		ClassName     string
		Namespace     string
		InfraStackRef string

		ALB struct {
			Name     string
			SubnetID string
		}

		StaticIP struct {
			Name string
			IP   string
		}
	}

	Istio struct {
		Enabled   bool
		Namespace string
		Hub       string
		Tag       string
	}

	CRDs struct {
		// installConfigured distinguishes an explicit false from an omitted
		// boolean so the platform-specific default can still be applied.
		installConfigured bool
		InstallGatewayAPI bool
		GatewayAPIPath    string
		GIEPath           string
	}

	TLS struct {
		CertName string
		// CertAnnotation is only needed when a cloud-managed certificate is
		// supplied by direct config or an infrastructure StackReference.
		CertAnnotation string

		// CertManagerIssuer makes cert-manager's gateway-shim issue a
		// certificate into SecretName. The Gateway exposes both HTTPS and the
		// HTTP listener needed by the ACME HTTP-01 challenge.
		CertManagerIssuer string
		SecretName        string

		// BootstrapSecret seeds SecretName with a throwaway self-signed
		// certificate on Azure AGC. AGC requires the Secret before it programs
		// any listener, including the HTTP listener needed for ACME, so a new
		// stack otherwise deadlocks. IgnoreChanges lets cert-manager own the
		// Secret data after bootstrap.
		BootstrapSecret bool
	}
}

func Load(ctx *pulumi.Context, projectDir string) (Config, error) {
	conf := pulumiconfig.New(ctx, "gateway-provider")

	var cfg Config

	loadPlatform(conf, &cfg)
	loadKubernetes(conf, &cfg)
	loadGateway(conf, &cfg)
	loadIstio(conf, &cfg)
	loadCRDs(conf, &cfg)
	loadTLS(conf, &cfg)

	if err := applyDefaults(&cfg); err != nil {
		return Config{}, fmt.Errorf("apply defaults: %w", err)
	}

	resolvePaths(&cfg, projectDir)

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate gateway-provider config: %w", err)
	}

	return cfg, nil
}

func loadPlatform(conf *pulumiconfig.Config, cfg *Config) {
	// Keep the established Pulumi key used by existing stack files and the
	// installer. Platform is the typed representation of cloudProvider.
	cfg.Platform = platform.Platform(conf.Get("cloudProvider"))
}

func loadKubernetes(conf *pulumiconfig.Config, cfg *Config) {
	cfg.Kubernetes.KubeconfigPath = conf.Get("kubeconfig")
	cfg.Kubernetes.Context = conf.Get("context")
}

func loadGateway(conf *pulumiconfig.Config, cfg *Config) {
	cfg.Gateway.Hostname = conf.Get("gatewayHostname")
	cfg.Gateway.ClassName = conf.Get("gatewayClassName")
	cfg.Gateway.Namespace = conf.Get("gatewayNamespace")
	cfg.Gateway.InfraStackRef = conf.Get("infraStackRef")

	cfg.Gateway.ALB.Name = conf.Get("albName")
	cfg.Gateway.ALB.SubnetID = conf.Get("albSubnetId")

	cfg.Gateway.StaticIP.Name = conf.Get("gatewayStaticIPName")
	cfg.Gateway.StaticIP.IP = conf.Get("gatewayStaticIP")
}

func loadCRDs(conf *pulumiconfig.Config, cfg *Config) {
	raw := conf.Get("installGatewayApiCrds")
	if raw != "" {
		cfg.CRDs.InstallGatewayAPI = conf.GetBool("installGatewayApiCrds")
		cfg.CRDs.installConfigured = true
	}

	cfg.CRDs.GatewayAPIPath = conf.Get("gatewayApiCrdsPath")
	cfg.CRDs.GIEPath = conf.Get("gieCrdsPath")
}

func loadTLS(conf *pulumiconfig.Config, cfg *Config) {
	cfg.TLS.CertName = conf.Get("gatewayCertName")
	cfg.TLS.CertAnnotation = conf.Get("tlsCertAnnotation")
	cfg.TLS.CertManagerIssuer = conf.Get("certManagerIssuer")
	cfg.TLS.SecretName = conf.Get("tlsSecretName")
	cfg.TLS.BootstrapSecret = conf.GetBool("bootstrapTlsSecret")
}

func loadIstio(conf *pulumiconfig.Config, cfg *Config) {
	// Istio was historically the default Gateway implementation. An explicit
	// non-Istio provider skips only the Istio control-plane installation.
	provider := conf.Get("provider")
	cfg.Istio.Enabled = provider == "" || provider == "istio"

	if cfg.Istio.Enabled {
		cfg.Istio.Namespace = conf.Get("namespace")
		cfg.Istio.Hub = conf.Get("istioHub")
		cfg.Istio.Tag = conf.Get("istioTag")
	}
}

func applyDefaults(cfg *Config) error {
	if cfg.Istio.Namespace == "" {
		cfg.Istio.Namespace = DefaultIstioNamespace
	}

	if cfg.Istio.Hub == "" {
		cfg.Istio.Hub = DefaultIstioHub
	}

	if cfg.Istio.Tag == "" {
		cfg.Istio.Tag = DefaultIstioTag
	}

	if cfg.Gateway.Namespace == "" {
		cfg.Gateway.Namespace = DefaultGatewayNamespace
	}

	// Azure Application Gateway for Containers provisions the Gateway API
	// CRDs itself. Any platform can still override this explicitly.
	if !cfg.CRDs.installConfigured {
		cfg.CRDs.InstallGatewayAPI = defaultInstallGatewayAPICRDs(cfg.Platform)
	}

	if cfg.CRDs.GatewayAPIPath == "" {
		cfg.CRDs.GatewayAPIPath = DefaultGatewayAPICRDsPath
	}

	if cfg.CRDs.GIEPath == "" {
		cfg.CRDs.GIEPath = DefaultGIECRDsPath
	}

	// cert-manager issues into the Secret referenced by the HTTPS listener.
	// Preserve the historical default whenever an issuer is configured.
	if cfg.TLS.CertManagerIssuer != "" &&
		cfg.TLS.SecretName == "" {
		cfg.TLS.SecretName = DefaultTLSSecretName
	}

	return nil
}

func defaultInstallGatewayAPICRDs(p platform.Platform) bool {
	return p != platform.Azure
}

func resolvePaths(cfg *Config, projectDir string) {
	cfg.CRDs.GatewayAPIPath = resolveProjectPath(
		projectDir,
		cfg.CRDs.GatewayAPIPath,
	)

	cfg.CRDs.GIEPath = resolveProjectPath(
		projectDir,
		cfg.CRDs.GIEPath,
	)
}

func resolveProjectPath(projectDir, path string) string {
	if path == "" {
		return ""
	}

	// Keep remote kustomize sources untouched.
	if u, err := url.Parse(path); err == nil && u.Scheme != "" {
		return path
	}

	if strings.HasPrefix(path, "git+") {
		return path
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	if projectDir != "" {
		path = filepath.Join(projectDir, path)
	}

	// Pulumi-Kubernetes v4 parses a Kustomize directory as a URL first and
	// rejects plain relative paths with "invalid URL scheme". Always hand it
	// an absolute path while leaving remote sources untouched above.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}

	return filepath.Clean(absPath)
}

func (cfg Config) Validate() error {
	if err := cfg.Platform.Validate(); err != nil {
		return err
	}

	if cfg.Gateway.ClassName == "" {
		return fmt.Errorf("gateway class name cannot be empty")
	}

	if cfg.Gateway.Namespace == "" {
		return fmt.Errorf("gateway namespace cannot be empty")
	}

	if cfg.CRDs.GIEPath == "" {
		return fmt.Errorf("GIE CRDs path cannot be empty")
	}

	if cfg.CRDs.InstallGatewayAPI &&
		cfg.CRDs.GatewayAPIPath == "" {
		return fmt.Errorf(
			"Gateway API CRDs path cannot be empty when Gateway API CRD installation is enabled",
		)
	}

	if cfg.Istio.Enabled {
		if cfg.Istio.Namespace == "" {
			return fmt.Errorf("Istio namespace cannot be empty")
		}

		if cfg.Istio.Hub == "" {
			return fmt.Errorf("Istio hub cannot be empty")
		}

		if cfg.Istio.Tag == "" {
			return fmt.Errorf("Istio tag cannot be empty")
		}
	}

	if cfg.TLS.BootstrapSecret &&
		cfg.TLS.SecretName == "" {
		return fmt.Errorf(
			"TLS secret name is required when bootstrap TLS secret is enabled",
		)
	}

	return nil
}
