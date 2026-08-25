package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	kubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	apiext "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helm_v4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	kustomize "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// generateSelfSignedTLS returns a throwaway self-signed cert/key PEM pair used
// only to satisfy AGC's ResolvedRefs check until cert-manager issues the real
// one (see bootstrapTlsSecret). Its CN/validity are irrelevant — no client
// ever verifies this cert; it exists only long enough for the ACME HTTP-01
// challenge to complete and overwrite it.
func generateSelfSignedTLS() (certPEM string, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generating placeholder key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("generating placeholder serial: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "gateway-tls-bootstrap-placeholder"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("creating placeholder certificate: %w", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	return certPEM, keyPEM, nil
}

// resolveKustomizePath turns a possibly-relative local path into an absolute
// one. Pulumi-Kubernetes v4's kustomize Directory invoke parses the path
// as a URL first and chokes on plain relative paths like "./crds/..." with
// "invalid URL scheme". URLs (https://, git+ssh://) and absolute paths are
// passed through unchanged.
func resolveKustomizePath(configured, fallback string) string {
	path := configured
	if path == "" {
		path = fallback
	}
	if u, err := url.Parse(path); err == nil && u.Scheme != "" {
		return path
	}
	if strings.HasPrefix(path, "git+") {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	return filepath.Join(wd, path)
}

type GatewayProviderConfig struct {
	cloudProvider         string
	provider              string
	namespace             string
	kubeconfig            string // optional — path to kubeconfig; empty = KUBECONFIG env / ~/.kube/config
	istioHub              string
	istioTag              string
	installGatewayApiCrds bool
	gatewayApiCrdsSrc     string
	gieCrdsSrc            string
	gatewayClassName      string
	albName               string
	tlsCertAnnotation     string // optional — only needed when infraStackRef is set (cloud)
	// cert-manager issuer name — when set, gateway-provider creates a Certificate
	// resource that triggers cert-manager (deployed by 03_cluster) to issue a cert
	// into tlsSecretName. The Gateway gets both HTTP (port 80, for the ACME HTTP-01
	// challenge) and HTTPS listeners.
	certManagerIssuer string
	tlsSecretName     string
	// bootstrapTlsSecret seeds tlsSecretName with a throwaway self-signed cert on
	// Azure/AGC stacks. AGC's ResolvedRefs check on the https listener requires the
	// Secret to exist before it will program ANY listener (including the http one
	// cert-manager's HTTP-01 solver needs) — without a placeholder, a brand-new
	// stack deadlocks: no secret -> no listeners programmed -> port 80 unreachable
	// -> ACME challenge can never complete -> secret never created. Only meant for
	// first bootstrap of a stack; safe to leave on afterwards since IgnoreChanges
	// keeps Pulumi from ever touching the Secret's data again once cert-manager
	// takes over.
	bootstrapTlsSecret bool
}

func getConfig(ctx *pulumi.Context) GatewayProviderConfig {
	cfg := config.New(ctx, "")

	provider := cfg.Get("provider")
	if provider == "" {
		provider = "istio"
	}

	namespace := cfg.Get("namespace")
	if namespace == "" {
		namespace = "istio-system"
	}

	istioHub := cfg.Get("istioHub")
	if istioHub == "" {
		istioHub = "docker.io/istio"
	}

	istioTag := cfg.Get("istioTag")
	if istioTag == "" {
		istioTag = "1.28.1"
	}

	gatewayApiCrdsSrc := resolveKustomizePath(cfg.Get("gatewayApiCrdsPath"),
		"https://github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=v1.5.1")
	gieCrdsSrc := resolveKustomizePath(cfg.Get("gieCrdsPath"),
		"https://github.com/kubernetes-sigs/gateway-api-inference-extension/config/crd?ref=v1.4.0")

	cloudProvider := cfg.Require("cloudProvider")
	gatewayClassName := cfg.Require("gatewayClassName")
	albName := cfg.Get("albName")

	// Azure Application Gateway for Containers provisions the gateway-api CRDs
	// itself, so we skip installing them there. Any platform can override explicitly.
	installGatewayApiCrds := cloudProvider != "azure"
	if raw := cfg.Get("installGatewayApiCrds"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			panic("installGatewayApiCrds must be a boolean")
		}
		installGatewayApiCrds = parsed
	}

	// tlsCertAnnotation is only needed when infraStackRef is set (cloud stacks).
	// On-prem stacks omit it entirely.
	tlsCertAnnotation := cfg.Get("tlsCertAnnotation")

	// cert-manager issues the cert into tlsSecretName; the Gateway references that
	// Secret via certificateRefs. Default the Secret name when an issuer is set.
	certManagerIssuer := cfg.Get("certManagerIssuer")
	tlsSecretName := cfg.Get("tlsSecretName")
	if certManagerIssuer != "" && tlsSecretName == "" {
		tlsSecretName = "gateway-tls"
	}

	bootstrapTlsSecret := false
	if raw := cfg.Get("bootstrapTlsSecret"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			panic("bootstrapTlsSecret must be a boolean")
		}
		bootstrapTlsSecret = parsed
	}

	return GatewayProviderConfig{
		cloudProvider:         cloudProvider,
		provider:              provider,
		namespace:             namespace,
		kubeconfig:            cfg.Get("kubeconfig"),
		istioHub:              istioHub,
		istioTag:              istioTag,
		installGatewayApiCrds: installGatewayApiCrds,
		gatewayApiCrdsSrc:     gatewayApiCrdsSrc,
		gieCrdsSrc:            gieCrdsSrc,
		gatewayClassName:      gatewayClassName,
		albName:               albName,
		tlsCertAnnotation:     tlsCertAnnotation,
		certManagerIssuer:     certManagerIssuer,
		tlsSecretName:         tlsSecretName,
		bootstrapTlsSecret:    bootstrapTlsSecret,
	}
}

func DeployGatewayProvider(ctx *pulumi.Context) error {
	config := getConfig(ctx)

	// Create an explicit k8s provider targeting the correct cluster.
	// When kubeconfig is empty the provider falls back to KUBECONFIG env / ~/.kube/config.
	k8sProviderArgs := &kubernetes.ProviderArgs{}
	if config.kubeconfig != "" {
		k8sProviderArgs.Kubeconfig = pulumi.StringPtr(config.kubeconfig)
	}
	k8sProvider, err := kubernetes.NewProvider(ctx, "gateway-provider-k8s", k8sProviderArgs)
	if err != nil {
		return err
	}
	providerOpt := pulumi.Provider(k8sProvider)

	// patchForceCRDs forces SSA to take ownership of CRD fields already claimed
	// by another field manager (e.g. GKE kube-addon-manager owns bundle-version
	// annotations and spec.versions). Equivalent to kubectl apply --force-conflicts.
	patchForceCRDs := pulumi.Transforms([]pulumi.ResourceTransform{
		func(_ context.Context, args *pulumi.ResourceTransformArgs) *pulumi.ResourceTransformResult {
			switch args.Type {
			case "kubernetes:apiextensions.k8s.io/v1:CustomResourceDefinition",
				"kubernetes:admissionregistration.k8s.io/v1:ValidatingAdmissionPolicy",
				"kubernetes:admissionregistration.k8s.io/v1:ValidatingAdmissionPolicyBinding":
				// fall through to patchForce below
			default:
				return &pulumi.ResourceTransformResult{Props: args.Props, Opts: args.Opts}
			}
			props := args.Props
			if metaRaw, ok := props["metadata"]; ok {
				if meta, ok := metaRaw.(pulumi.Map); ok {
					annotations := pulumi.Map{}
					if annRaw, ok := meta["annotations"]; ok {
						if ann, ok := annRaw.(pulumi.Map); ok {
							for k, v := range ann {
								annotations[k] = v
							}
						}
					}
					annotations["pulumi.com/patchForce"] = pulumi.String("true")
					meta["annotations"] = annotations
				}
			}
			return &pulumi.ResourceTransformResult{Props: props, Opts: args.Opts}
		},
	})

	// Azure AGC ships the gateway-api CRDs; installGatewayApiCrds is false there.
	crdsDeps := []pulumi.Resource{}
	if config.installGatewayApiCrds {
		gatewayCRDs, err := kustomize.NewDirectory(ctx, "gateway-api-crds",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(config.gatewayApiCrdsSrc),
			},
			providerOpt,
			patchForceCRDs,
		)
		if err != nil {
			return err
		}
		crdsDeps = append(crdsDeps, gatewayCRDs)
	}

	gieCRDs, err := kustomize.NewDirectory(ctx, "gie-crds",
		kustomize.DirectoryArgs{
			Directory: pulumi.String(config.gieCrdsSrc),
		},
		providerOpt,
		patchForceCRDs,
	)
	if err != nil {
		return err
	}
	crdsDeps = append(crdsDeps, gieCRDs)

	if config.provider != "istio" {
		return nil
	}

	istioNamespace, err := corev1.NewNamespace(ctx, config.namespace, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(config.namespace),
		},
	}, providerOpt)
	if err != nil {
		return err
	}

	// Create a new provider that explicitly depends on the namespace.
	nsReadyProvider, err := kubernetes.NewProvider(ctx, "ns-ready-provider", &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(false),
		Kubeconfig:            k8sProviderArgs.Kubeconfig,
	}, pulumi.DependsOn([]pulumi.Resource{istioNamespace}))
	if err != nil {
		return err
	}

	istioBase, err := helm_v4.NewChart(ctx, "istio-base", &helm_v4.ChartArgs{
		Chart: pulumi.String("base"),
		RepositoryOpts: &helm_v4.RepositoryOptsArgs{
			Repo: pulumi.String("https://istio-release.storage.googleapis.com/charts"),
		},
		Version:   pulumi.String(config.istioTag),
		Namespace: istioNamespace.Metadata.Name(),
		Values: pulumi.Map{
			"defaultRevision": pulumi.String("default"),
		},
		SkipCrds: pulumi.Bool(false),
	},
		pulumi.Provider(nsReadyProvider),
		pulumi.Transforms([]pulumi.ResourceTransform{
			func(_ context.Context, args *pulumi.ResourceTransformArgs) *pulumi.ResourceTransformResult {
				// Ensure every child resource waits for the namespace to exist.
				args.Opts.DependsOn = append(args.Opts.DependsOn, istioNamespace)
				if args.Type == "kubernetes:admissionregistration.k8s.io/v1:ValidatingWebhookConfiguration" {
					args.Opts.IgnoreChanges = append(args.Opts.IgnoreChanges,
						"webhooks[*].failurePolicy",
						"webhooks[*].clientConfig",
						"webhooks[*].admissionReviewVersions",
					)
				}
				return &pulumi.ResourceTransformResult{Props: args.Props, Opts: args.Opts}
			},
		}),
	)
	if err != nil {
		return err
	}

	_, err = helm_v4.NewChart(ctx, "istiod", &helm_v4.ChartArgs{
		Chart: pulumi.String("istiod"),
		RepositoryOpts: &helm_v4.RepositoryOptsArgs{
			Repo: pulumi.String("https://istio-release.storage.googleapis.com/charts"),
		},
		Version:   pulumi.String(config.istioTag),
		Namespace: istioNamespace.Metadata.Name(),
		Values: pulumi.Map{
			"meshConfig": pulumi.Map{
				"defaultConfig": pulumi.Map{
					"proxyMetadata": pulumi.Map{
						"ENABLE_GATEWAY_API_INFERENCE_EXTENSION": pulumi.String("true"),
					},
				},
			},
			"pilot": pulumi.Map{
				"env": pulumi.Map{
					"ENABLE_GATEWAY_API_INFERENCE_EXTENSION": pulumi.String("true"),
				},
				"resources": pulumi.Map{
					"requests": pulumi.Map{
						"cpu":    pulumi.String("250m"),
						"memory": pulumi.String("1Gi"),
					},
				},
			},
			"tag": pulumi.String(config.istioTag),
			"hub": pulumi.String(config.istioHub),
			"global": pulumi.Map{
				"hub": pulumi.String(config.istioHub),
				"tag": pulumi.String(config.istioTag),
			},
		},
		SkipAwait: pulumi.Bool(false),
	},
		pulumi.DependsOn([]pulumi.Resource{istioBase}),
		pulumi.Provider(nsReadyProvider),
		pulumi.Transforms([]pulumi.ResourceTransform{
			func(_ context.Context, args *pulumi.ResourceTransformArgs) *pulumi.ResourceTransformResult {
				// Ensure every child resource waits for the namespace to exist.
				args.Opts.DependsOn = append(args.Opts.DependsOn, istioNamespace)
				if args.Type == "kubernetes:admissionregistration.k8s.io/v1:ValidatingWebhookConfiguration" {
					args.Opts.IgnoreChanges = append(args.Opts.IgnoreChanges,
						"webhooks[*].failurePolicy",
						"webhooks[*].clientConfig",
						"webhooks[*].admissionReviewVersions",
					)
				}
				return &pulumi.ResourceTransformResult{Props: args.Props, Opts: args.Opts}
			},
		}),
	)
	if err != nil {
		return err
	}

	// Shared Gateway using the gatewayClassName from stack config.
	if err := createSharedGateway(ctx, &config, crdsDeps, providerOpt); err != nil {
		return err
	}

	return nil
}

// createSharedGateway creates a namespace and Gateway for external load balancing.
// GCP-specific resources (HealthCheckPolicy) are only created when cloudProvider is "gcp".
// On Azure it also creates the ApplicationLoadBalancer CR (AGC) and, when a
// certManagerIssuer is configured, a cert-manager Certificate for TLS.
func createSharedGateway(ctx *pulumi.Context, gwCfg *GatewayProviderConfig, crdsDeps []pulumi.Resource, providerOpt pulumi.ResourceOption) error {
	cfg := config.New(ctx, "")

	infraStackRef := cfg.Get("infraStackRef")
	gatewayHostname := cfg.Get("gatewayHostname")
	if infraStackRef == "" && gatewayHostname == "" {
		// Neither infra stack reference nor direct hostname — skip shared gateway creation
		return nil
	}

	var hostnameOut, certNameOut, staticIPNameOut, staticIPOut, albSubnetIdOut pulumi.StringOutput
	if infraStackRef != "" {
		ref, err := pulumi.NewStackReference(ctx, "infra-stack", &pulumi.StackReferenceArgs{
			Name: pulumi.String(infraStackRef),
		})
		if err != nil {
			return err
		}
		hostnameOut = ref.GetStringOutput(pulumi.String("gatewayHostname"))
		certNameOut = ref.GetStringOutput(pulumi.String("gatewayCertName"))
		staticIPNameOut = ref.GetStringOutput(pulumi.String("gatewayStaticIPName"))
		staticIPOut = ref.GetStringOutput(pulumi.String("gatewayStaticIP"))
		albSubnetIdOut = ref.GetStringOutput(pulumi.String("albSubnetId"))
	} else {
		// Direct config: hostname (and optionally cert/static IP/ALB subnet) supplied
		// as stack config instead of via a Pulumi StackReference. Used by the on-prem
		// path (cert/staticIP unset, stay empty) and by the installer bundle (cloud
		// deploys without a separate infra stack to reference).
		hostnameOut = pulumi.String(gatewayHostname).ToStringOutput()
		certNameOut = pulumi.String(cfg.Get("gatewayCertName")).ToStringOutput()
		staticIPNameOut = pulumi.String(cfg.Get("gatewayStaticIPName")).ToStringOutput()
		staticIPOut = pulumi.String(cfg.Get("gatewayStaticIP")).ToStringOutput()
		albSubnetIdOut = pulumi.String(cfg.Get("albSubnetId")).ToStringOutput()
	}

	gatewayNamespace := "gateway-system"

	// Namespace for the shared gateway
	gwNs, err := corev1.NewNamespace(ctx, gatewayNamespace, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(gatewayNamespace),
		},
	}, providerOpt)
	if err != nil {
		return err
	}

	// Build listener and gateway annotations using Apply to resolve async outputs.
	// For GKE Certificate Manager, the cert annotation must be on the Gateway metadata —
	// GKE ignores it when placed inside listener tls.options.
	combined := pulumi.All(hostnameOut, certNameOut, staticIPNameOut, staticIPOut)

	listenerOutput := combined.ApplyT(func(args []interface{}) []map[string]interface{} {
		h := args[0].(string)
		c := args[1].(string)
		// Cloud TLS: cert managed by GKE Certificate Manager, referenced via annotation.
		if c != "" {
			return []map[string]interface{}{
				{
					"name":     "https",
					"protocol": "HTTPS",
					"port":     443,
					"hostname": h,
					"tls": map[string]interface{}{
						"mode": "Terminate",
						// options is required by the Gateway API validator when mode is Terminate
						// (certificateRefs or options must be set). GKE reads the cert from
						// metadata.annotations instead, but options must be non-absent.
						"options": map[string]interface{}{
							gwCfg.tlsCertAnnotation: c,
						},
					},
					"allowedRoutes": map[string]interface{}{
						"namespaces": map[string]interface{}{
							"from": "All",
						},
					},
				},
			}
		}
		// TLS via certificateRefs — used by cert-manager (Azure). The referenced
		// Secret is populated by cert-manager from the cluster-issuer annotation
		// on the Gateway.
		if gwCfg.tlsSecretName != "" {
			listeners := []map[string]interface{}{
				{
					"name":     "https",
					"protocol": "HTTPS",
					"port":     443,
					"hostname": h,
					"tls": map[string]interface{}{
						"mode": "Terminate",
						"certificateRefs": []map[string]interface{}{
							{
								"kind": "Secret",
								"name": gwCfg.tlsSecretName,
							},
						},
					},
					"allowedRoutes": map[string]interface{}{
						"namespaces": map[string]interface{}{
							"from": "All",
						},
					},
				},
			}
			// cert-manager HTTP-01 challenge needs an HTTP listener on port 80.
			if gwCfg.certManagerIssuer != "" {
				listeners = append(listeners, map[string]interface{}{
					"name":     "http",
					"protocol": "HTTP",
					"port":     80,
					"hostname": h,
					"allowedRoutes": map[string]interface{}{
						"namespaces": map[string]interface{}{
							"from": "All",
						},
					},
				})
			}
			return listeners
		}
		// No TLS: plain HTTP listener.
		return []map[string]interface{}{
			{
				"name":     "http",
				"protocol": "HTTP",
				"port":     80,
				"hostname": h,
				"allowedRoutes": map[string]interface{}{
					"namespaces": map[string]interface{}{
						"from": "All",
					},
				},
			},
		}
	})

	annotationsOutput := combined.ApplyT(func(args []interface{}) map[string]string {
		c := args[1].(string)
		// patchForce lets Pulumi take ownership of fields claimed by a prior field
		// manager (each pulumi up gets a new unique pulumi-kubernetes-XXXXXXXX ID).
		annotations := map[string]string{
			"pulumi.com/patchForce": "true",
		}
		if c != "" {
			annotations[gwCfg.tlsCertAnnotation] = c
		}
		if gwCfg.certManagerIssuer != "" {
			annotations["cert-manager.io/cluster-issuer"] = gwCfg.certManagerIssuer
		}
		if gwCfg.cloudProvider == "azure" && gwCfg.albName != "" {
			annotations["alb.networking.azure.io/alb-namespace"] = gatewayNamespace
			annotations["alb.networking.azure.io/alb-name"] = gwCfg.albName
		}
		return annotations
	}).(pulumi.StringMapOutput)

	// spec.addresses binds a pre-allocated static IP to the Gateway load balancer.
	// GKE: NamedAddress references a compute.Address resource by name.
	// Azure AGC: manages its own frontend hostname — spec.addresses is not used.
	// Azure Istio: IPAddress with the actual IP value from the pre-allocated Public IP.
	addressesOutput := combined.ApplyT(func(args []interface{}) []map[string]interface{} {
		if gwCfg.cloudProvider == "azure" {
			if gwCfg.albName != "" {
				return nil // AGC manages its own frontend
			}
			ip := args[3].(string)
			if ip == "" {
				return nil
			}
			return []map[string]interface{}{
				{"type": "IPAddress", "value": ip},
			}
		}
		ipName := args[2].(string)
		if ipName == "" {
			return nil
		}
		return []map[string]interface{}{
			{"type": "NamedAddress", "value": ipName},
		}
	})

	gatewayDeps := append(crdsDeps, gwNs)
	if gwCfg.cloudProvider == "azure" && gwCfg.albName != "" && gwCfg.certManagerIssuer != "" && gwCfg.bootstrapTlsSecret {
		certPEM, keyPEM, err := generateSelfSignedTLS()
		if err != nil {
			return err
		}
		bootstrapSecret, err := corev1.NewSecret(ctx, "gateway-tls-bootstrap", &corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(gwCfg.tlsSecretName),
				Namespace: pulumi.String(gatewayNamespace),
			},
			Type: pulumi.String("kubernetes.io/tls"),
			StringData: pulumi.StringMap{
				"tls.crt": pulumi.String(certPEM),
				"tls.key": pulumi.String(keyPEM),
			},
			// Real content is written by cert-manager once the ACME challenge
			// completes. Never let Pulumi re-apply the throwaway placeholder
			// data over that on subsequent runs.
		}, pulumi.DependsOn([]pulumi.Resource{gwNs}), pulumi.IgnoreChanges([]string{"stringData", "data"}), providerOpt)
		if err != nil {
			return err
		}
		gatewayDeps = append(gatewayDeps, bootstrapSecret)
	}
	if gwCfg.cloudProvider == "azure" && gwCfg.albName != "" {
		alb, err := apiext.NewCustomResource(ctx, "shared-alb", &apiext.CustomResourceArgs{
			ApiVersion: pulumi.String("alb.networking.azure.io/v1"),
			Kind:       pulumi.String("ApplicationLoadBalancer"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(gwCfg.albName),
				Namespace: pulumi.String(gatewayNamespace),
			},
			OtherFields: kubernetes.UntypedArgs{
				"spec": pulumi.Map{
					"associations": pulumi.StringArray{albSubnetIdOut},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{gwNs}), providerOpt)
		if err != nil {
			return err
		}
		gatewayDeps = append(gatewayDeps, alb)
	}

	gwSpec := map[string]interface{}{
		"gatewayClassName": gwCfg.gatewayClassName,
		"addresses":        addressesOutput,
		"listeners":        listenerOutput,
	}
	// Azure + Istio: Istio propagates infrastructure.annotations to the generated
	// LoadBalancer Service. The health probe annotation tells the Azure LB to use
	// Istio's status port instead of HTTP on port 80 (which returns 404).
	if gwCfg.cloudProvider == "azure" && gwCfg.albName == "" {
		gwSpec["infrastructure"] = map[string]interface{}{
			"annotations": map[string]string{
				"service.beta.kubernetes.io/port_80_health-probe_port":                     "15021",
				"service.beta.kubernetes.io/port_443_health-probe_port":                    "15021",
				"service.beta.kubernetes.io/port_443_health-probe_protocol":                "http",
				"service.beta.kubernetes.io/azure-load-balancer-health-probe-request-path": "/healthz/ready",
			},
		}
	}

	_, err = apiext.NewCustomResource(ctx, "shared-gateway", &apiext.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("Gateway"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:        pulumi.String("shared-gateway"),
			Namespace:   pulumi.String(gatewayNamespace),
			Annotations: annotationsOutput,
		},
		OtherFields: kubernetes.UntypedArgs{
			"spec": gwSpec,
		},
	}, pulumi.DependsOn(gatewayDeps), providerOpt)
	if err != nil {
		return err
	}

	// The cert-manager Certificate is created automatically by cert-manager's
	// gateway-shim from the "cert-manager.io/cluster-issuer" annotation on the
	// Gateway, so no explicit Certificate resource is declared here.

	ctx.Export("gatewayName", pulumi.String("shared-gateway"))
	ctx.Export("gatewayNamespace", pulumi.String(gatewayNamespace))

	return nil
}
