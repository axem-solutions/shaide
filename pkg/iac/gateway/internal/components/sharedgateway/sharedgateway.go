package sharedgateway

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/axem-solutions/ai_platform/pkg/iac/gateway/internal/config"
	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const gatewayName = "shared-gateway"

type infrastructure struct {
	Hostname     pulumi.StringOutput
	CertName     pulumi.StringOutput
	StaticIPName pulumi.StringOutput
	StaticIP     pulumi.StringOutput
	ALBSubnetID  pulumi.StringOutput
}

// Deploy creates the namespace and shared Gateway used for external ingress.
// On Azure AGC it also creates the ApplicationLoadBalancer resource and can
// seed cert-manager's target Secret to break the first-boot ACME deadlock.
func Deploy(
	ctx *pulumi.Context,
	cfg config.Config,
	provider *kubernetes.Provider,
	crdDeps []pulumi.Resource,
) error {
	if !isConfigured(cfg) {
		return nil
	}

	// Istio creates its GatewayClass as part of this update, so that class may
	// legitimately be absent while the Pulumi program is being evaluated.
	classCreatedByThisUpdate := cfg.Istio.Enabled && cfg.Gateway.ClassName == "istio"
	if err := validateGatewayClass(
		ctx.Context(),
		cfg.Kubernetes,
		cfg.Gateway.ClassName,
		classCreatedByThisUpdate,
	); err != nil {
		return fmt.Errorf("validate GatewayClass: %w", err)
	}

	infra, err := resolveInfrastructure(ctx, cfg)
	if err != nil {
		return fmt.Errorf("resolve infrastructure: %w", err)
	}

	// Keep the shared ingress resources isolated from application namespaces.
	namespace, err := iackube.CreateNamespace(
		ctx,
		cfg.Gateway.Namespace,
		pulumi.Provider(provider),
	)
	if err != nil {
		return fmt.Errorf("create namespace: %w", err)
	}

	deps := append([]pulumi.Resource{}, crdDeps...)
	deps = append(deps, namespace)

	deps, err = addBootstrapTLS(ctx, cfg, namespace, provider, deps)
	if err != nil {
		return err
	}

	deps, err = addApplicationLoadBalancer(ctx, cfg, infra, namespace, provider, deps)
	if err != nil {
		return err
	}

	if err := deployGateway(ctx, cfg, infra, namespace, provider, deps); err != nil {
		return fmt.Errorf("deploy gateway: %w", err)
	}

	exportOutputs(ctx, cfg, infra)

	return nil
}

func isConfigured(cfg config.Config) bool {
	// Neither an infrastructure stack nor a direct hostname was supplied, so
	// preserve the documented behavior of skipping shared-Gateway creation.
	return cfg.Gateway.InfraStackRef != "" || cfg.Gateway.Hostname != ""
}

func exportOutputs(ctx *pulumi.Context, cfg config.Config, infra infrastructure) {
	ctx.Export("gatewayName", pulumi.String(gatewayName))
	ctx.Export("gatewayNamespace", pulumi.String(cfg.Gateway.Namespace))
	ctx.Export("gatewayHostname", infra.Hostname)
	ctx.Export("cloudProvider", pulumi.String(cfg.Platform))
}

func resolveInfrastructure(
	ctx *pulumi.Context,
	cfg config.Config,
) (infrastructure, error) {
	if cfg.Gateway.InfraStackRef != "" {
		return infrastructureFromStack(ctx, cfg.Gateway.InfraStackRef)
	}

	return infrastructureFromConfig(cfg), nil
}

func infrastructureFromStack(
	ctx *pulumi.Context,
	stackRef string,
) (infrastructure, error) {
	ref, err := pulumi.NewStackReference(
		ctx,
		"infra-stack",
		&pulumi.StackReferenceArgs{Name: pulumi.String(stackRef)},
	)
	if err != nil {
		return infrastructure{}, err
	}

	return infrastructure{
		Hostname:     ref.GetStringOutput(pulumi.String("gatewayHostname")),
		CertName:     ref.GetStringOutput(pulumi.String("gatewayCertName")),
		StaticIPName: ref.GetStringOutput(pulumi.String("gatewayStaticIPName")),
		StaticIP:     ref.GetStringOutput(pulumi.String("gatewayStaticIP")),
		ALBSubnetID:  ref.GetStringOutput(pulumi.String("albSubnetId")),
	}, nil
}

func infrastructureFromConfig(cfg config.Config) infrastructure {
	// Direct config is used by on-prem stacks and installer bundles that do not
	// have a separate infrastructure stack. Optional certificate, IP, and ALB
	// subnet values intentionally remain empty when they do not apply.
	return infrastructure{
		Hostname:     pulumi.String(cfg.Gateway.Hostname).ToStringOutput(),
		CertName:     pulumi.String(cfg.TLS.CertName).ToStringOutput(),
		StaticIPName: pulumi.String(cfg.Gateway.StaticIP.Name).ToStringOutput(),
		StaticIP:     pulumi.String(cfg.Gateway.StaticIP.IP).ToStringOutput(),
		ALBSubnetID:  pulumi.String(cfg.Gateway.ALB.SubnetID).ToStringOutput(),
	}
}

func addBootstrapTLS(
	ctx *pulumi.Context,
	cfg config.Config,
	namespace *corev1.Namespace,
	provider *kubernetes.Provider,
	deps []pulumi.Resource,
) ([]pulumi.Resource, error) {
	if !needsBootstrapTLS(cfg) {
		return deps, nil
	}

	secret, err := createBootstrapTLSSecret(ctx, cfg, namespace, provider)
	if err != nil {
		return nil, fmt.Errorf("create bootstrap TLS secret: %w", err)
	}

	return append(deps, secret), nil
}

func needsBootstrapTLS(cfg config.Config) bool {
	return usesAzureAGC(cfg) &&
		cfg.TLS.CertManagerIssuer != "" &&
		cfg.TLS.BootstrapSecret
}

func createBootstrapTLSSecret(
	ctx *pulumi.Context,
	cfg config.Config,
	namespace *corev1.Namespace,
	provider *kubernetes.Provider,
) (*corev1.Secret, error) {
	certPEM, keyPEM, err := generateSelfSignedTLS()
	if err != nil {
		return nil, err
	}

	return corev1.NewSecret(
		ctx,
		"gateway-tls-bootstrap",
		&corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(cfg.TLS.SecretName),
				Namespace: namespace.Metadata.Name(),
			},
			Type: pulumi.String("kubernetes.io/tls"),
			StringData: pulumi.StringMap{
				"tls.crt": pulumi.String(certPEM),
				"tls.key": pulumi.String(keyPEM),
			},
		},
		pulumi.Provider(provider),
		pulumi.DependsOn([]pulumi.Resource{namespace}),
		// cert-manager replaces this throwaway data after ACME succeeds. Never
		// let a later Pulumi update overwrite the real certificate and key.
		pulumi.IgnoreChanges([]string{"stringData", "data"}),
	)
}

func addApplicationLoadBalancer(
	ctx *pulumi.Context,
	cfg config.Config,
	infra infrastructure,
	namespace *corev1.Namespace,
	provider *kubernetes.Provider,
	deps []pulumi.Resource,
) ([]pulumi.Resource, error) {
	if !usesAzureAGC(cfg) {
		return deps, nil
	}

	alb, err := createApplicationLoadBalancer(ctx, cfg, infra, namespace, provider)
	if err != nil {
		return nil, fmt.Errorf("create application load balancer: %w", err)
	}

	return append(deps, alb), nil
}

func createApplicationLoadBalancer(
	ctx *pulumi.Context,
	cfg config.Config,
	infra infrastructure,
	namespace *corev1.Namespace,
	provider *kubernetes.Provider,
) (*apiext.CustomResource, error) {
	return apiext.NewCustomResource(
		ctx,
		"shared-alb",
		&apiext.CustomResourceArgs{
			ApiVersion: pulumi.String("alb.networking.azure.io/v1"),
			Kind:       pulumi.String("ApplicationLoadBalancer"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(cfg.Gateway.ALB.Name),
				Namespace: namespace.Metadata.Name(),
			},
			OtherFields: kubernetes.UntypedArgs{
				"spec": pulumi.Map{
					"associations": pulumi.StringArray{infra.ALBSubnetID},
				},
			},
		},
		pulumi.Provider(provider),
		pulumi.DependsOn([]pulumi.Resource{namespace}),
	)
}

func deployGateway(
	ctx *pulumi.Context,
	cfg config.Config,
	infra infrastructure,
	namespace *corev1.Namespace,
	provider *kubernetes.Provider,
	deps []pulumi.Resource,
) error {
	values := pulumi.All(
		infra.Hostname,
		infra.CertName,
		infra.StaticIPName,
		infra.StaticIP,
	)

	// Listener, annotation, and address values depend on asynchronous outputs
	// from the infrastructure StackReference when one is configured.
	listeners := buildListeners(cfg, values)
	annotations := buildAnnotations(cfg, values)
	addresses := buildAddresses(cfg, values)
	spec := buildGatewaySpec(cfg, listeners, addresses)

	_, err := apiext.NewCustomResource(
		ctx,
		gatewayName,
		&apiext.CustomResourceArgs{
			ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
			Kind:       pulumi.String("Gateway"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:        pulumi.String(gatewayName),
				Namespace:   namespace.Metadata.Name(),
				Annotations: annotations,
			},
			OtherFields: kubernetes.UntypedArgs{"spec": spec},
		},
		pulumi.Provider(provider),
		pulumi.DependsOn(deps),
	)

	// cert-manager's gateway-shim creates the Certificate from the
	// cert-manager.io/cluster-issuer annotation. No explicit Certificate
	// resource is needed here.
	return err
}

func buildListeners(cfg config.Config, values pulumi.ArrayOutput) pulumi.Output {
	return values.ApplyT(func(args []interface{}) []map[string]interface{} {
		hostname := args[0].(string)
		certName := args[1].(string)

		if certName != "" {
			// Cloud-managed TLS, such as GKE Certificate Manager, binds the
			// certificate through a provider-specific annotation.
			return []map[string]interface{}{
				httpsListenerWithCertificate(
					hostname,
					cfg.TLS.CertAnnotation,
					certName,
				),
			}
		}

		if cfg.TLS.SecretName != "" {
			// cert-manager writes the issued certificate to the Secret referenced
			// by this listener.
			listeners := []map[string]interface{}{
				httpsListenerWithSecret(hostname, cfg.TLS.SecretName),
			}

			if cfg.TLS.CertManagerIssuer != "" {
				// The ACME HTTP-01 challenge requires port 80 while HTTPS is
				// already configured on the Gateway.
				listeners = append(listeners, httpListener(hostname))
			}

			return listeners
		}

		// With no certificate configuration, expose plain HTTP only.
		return []map[string]interface{}{httpListener(hostname)}
	})
}

func httpsListenerWithCertificate(
	hostname string,
	annotation string,
	certName string,
) map[string]interface{} {
	return map[string]interface{}{
		"name":     "https",
		"protocol": "HTTPS",
		"port":     443,
		"hostname": hostname,
		"tls": map[string]interface{}{
			"mode": "Terminate",
			// Gateway API validation requires either certificateRefs or options
			// for Terminate mode. GKE reads the certificate from Gateway metadata,
			// but this non-empty option is still required by the validator.
			"options": map[string]interface{}{
				annotation: certName,
			},
		},
		"allowedRoutes": allowRoutesFromAllNamespaces(),
	}
}

func httpsListenerWithSecret(hostname, secretName string) map[string]interface{} {
	return map[string]interface{}{
		"name":     "https",
		"protocol": "HTTPS",
		"port":     443,
		"hostname": hostname,
		"tls": map[string]interface{}{
			"mode": "Terminate",
			"certificateRefs": []map[string]interface{}{
				{"kind": "Secret", "name": secretName},
			},
		},
		"allowedRoutes": allowRoutesFromAllNamespaces(),
	}
}

func httpListener(hostname string) map[string]interface{} {
	return map[string]interface{}{
		"name":          "http",
		"protocol":      "HTTP",
		"port":          80,
		"hostname":      hostname,
		"allowedRoutes": allowRoutesFromAllNamespaces(),
	}
}

func allowRoutesFromAllNamespaces() map[string]interface{} {
	return map[string]interface{}{
		"namespaces": map[string]interface{}{"from": "All"},
	}
}

func buildAnnotations(
	cfg config.Config,
	values pulumi.ArrayOutput,
) pulumi.StringMapOutput {
	return values.ApplyT(func(args []interface{}) map[string]string {
		certName := args[1].(string)
		// patchForce lets Pulumi take ownership of metadata fields claimed by
		// the unique field manager created by an earlier Pulumi update.
		annotations := map[string]string{
			"pulumi.com/patchForce": "true",
		}

		// GKE Certificate Manager requires the binding annotation on Gateway
		// metadata; placing it only in listener tls.options is insufficient.
		if certName != "" {
			annotations[cfg.TLS.CertAnnotation] = certName
		}
		if cfg.TLS.CertManagerIssuer != "" {
			annotations["cert-manager.io/cluster-issuer"] = cfg.TLS.CertManagerIssuer
		}
		if usesAzureAGC(cfg) {
			annotations["alb.networking.azure.io/alb-namespace"] = cfg.Gateway.Namespace
			annotations["alb.networking.azure.io/alb-name"] = cfg.Gateway.ALB.Name
		}

		return annotations
	}).(pulumi.StringMapOutput)
}

func buildAddresses(cfg config.Config, values pulumi.ArrayOutput) pulumi.Output {
	// GKE binds a compute.Address by name. Azure Istio binds the literal public
	// IP, while Azure AGC owns its frontend and must not receive spec.addresses.
	return values.ApplyT(func(args []interface{}) []map[string]interface{} {
		if cfg.Platform == platform.Azure {
			if usesAzureAGC(cfg) {
				return nil
			}

			staticIP := args[3].(string)
			if staticIP == "" {
				return nil
			}

			return []map[string]interface{}{
				{"type": "IPAddress", "value": staticIP},
			}
		}

		staticIPName := args[2].(string)
		if staticIPName == "" {
			return nil
		}

		return []map[string]interface{}{
			{"type": "NamedAddress", "value": staticIPName},
		}
	})
}

func buildGatewaySpec(
	cfg config.Config,
	listeners pulumi.Output,
	addresses pulumi.Output,
) map[string]interface{} {
	spec := map[string]interface{}{
		"gatewayClassName": cfg.Gateway.ClassName,
		"listeners":        listeners,
		"addresses":        addresses,
	}

	if usesAzureIstio(cfg) {
		spec["infrastructure"] = azureIstioInfrastructure()
	}

	return spec
}

func usesAzureAGC(cfg config.Config) bool {
	return cfg.Platform == platform.Azure && cfg.Gateway.ALB.Name != ""
}

func usesAzureIstio(cfg config.Config) bool {
	return cfg.Platform == platform.Azure && cfg.Gateway.ALB.Name == ""
}

func azureIstioInfrastructure() map[string]interface{} {
	// Istio propagates infrastructure.annotations to its generated LoadBalancer
	// Service. Azure LB must probe Istio's status port; HTTP on port 80 returns
	// 404 and would otherwise mark the gateway unhealthy.
	return map[string]interface{}{
		"annotations": map[string]string{
			"service.beta.kubernetes.io/port_80_health-probe_port":                     "15021",
			"service.beta.kubernetes.io/port_443_health-probe_port":                    "15021",
			"service.beta.kubernetes.io/port_443_health-probe_protocol":                "http",
			"service.beta.kubernetes.io/azure-load-balancer-health-probe-request-path": "/healthz/ready",
		},
	}
}

// generateSelfSignedTLS returns a throwaway certificate used only to satisfy
// Azure Application Gateway for Containers' ResolvedRefs check until
// cert-manager issues the real one. Its identity and validity are irrelevant:
// no client verifies it, and it exists only long enough for the ACME HTTP-01
// challenge to complete and overwrite the bootstrap Secret.
func generateSelfSignedTLS() (certPEM string, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate placeholder key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generate placeholder serial: %w", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "gateway-tls-bootstrap-placeholder",
		},
		NotBefore: now,
		NotAfter:  now.Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature |
			x509.KeyUsageKeyEncipherment,
	}

	der, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		&key.PublicKey,
		key,
	)
	if err != nil {
		return "", "", fmt.Errorf("create placeholder certificate: %w", err)
	}

	certPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))

	return certPEM, keyPEM, nil
}
