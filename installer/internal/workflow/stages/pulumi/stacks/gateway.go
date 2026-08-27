package stacks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/axem-solutions/ai_platform/installer/internal/iac"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/gateway"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

const (
	gatewayHostnameEnv = "GATEWAY_HOSTNAME"
	gatewayCertNameEnv = "GATEWAY_CERT_NAME"

	cloudProviderGCP    = "gcp"
	cloudProviderAWS    = "aws"
	cloudProviderAzure  = "azure"
	cloudProviderOnPrem = "on-prem"
)

// platformDefaults returns sensible defaults for a given cloud provider.
// Users can override any of these at the per-prompt step.
type platformDefaults struct {
	gatewayClassName    string
	tlsCertAnnotation   string
	certNameDescription string
}

func defaultsForPlatform(provider string) platformDefaults {
	switch provider {
	case cloudProviderGCP:
		return platformDefaults{
			gatewayClassName:    "gke-l7-regional-external-managed",
			tlsCertAnnotation:   "networking.gke.io/cert-manager-certs",
			certNameDescription: "GKE Certificate Manager certificate name",
		}
	case cloudProviderAWS:
		return platformDefaults{
			gatewayClassName:    "alb",
			tlsCertAnnotation:   "alb.ingress.kubernetes.io/certificate-arn",
			certNameDescription: "ACM certificate ARN",
		}
	case cloudProviderAzure:
		return platformDefaults{
			// Application Gateway for Containers, installed by the AKS ALB
			// controller. This is the class the Azure clusters in
			// infra/gateway-provider actually expose; the older AGIC class
			// (azure-application-gateway) is not present on any of them.
			gatewayClassName: "azure-alb-external",
			// AGC binds certificates through the Gateway listener's
			// certificateRefs, not through an annotation. The AGIC annotation
			// (appgw.ingress.kubernetes.io/ssl-certificate) does nothing here,
			// so default to none and let anyone still on AGIC type it in.
			tlsCertAnnotation:   "",
			certNameDescription: "Application Gateway certificate name",
		}
	case cloudProviderOnPrem:
		return platformDefaults{
			gatewayClassName:    "istio",
			tlsCertAnnotation:   "", // not used; TLS is self-managed
			certNameDescription: "TLS certificate reference (usually empty on-prem)",
		}
	}
	return platformDefaults{}
}

func DeployGatewayProvider(rt *core.Runtime) error {
	workdir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectGatewayProvider)
	stateDir := rt.Bootstrap.Config.Paths.PulumiState

	// Cloud platform selection drives the per-platform defaults below.
	// Pre-filled with the platform detected from the cluster's node
	// spec.providerID in the initK8s stage, so the common case is a confirm
	// rather than a choice, while an override is still possible.
	detected := rt.Bootstrap.CloudPlatform
	if detected == "" {
		detected = cloudProviderGCP
	}

	platform, err := rt.Reporter.Select(
		"Cloud platform",
		detected,
		[]string{cloudProviderGCP, cloudProviderAWS, cloudProviderAzure, cloudProviderOnPrem},
	)
	if err != nil {
		return err
	}

	pd := defaultsForPlatform(platform)

	// Gateway class can be overridden per cluster.
	gatewayClassName, err := promptGatewayClass(rt, pd.gatewayClassName)
	if err != nil {
		return err
	}

	// TLS cert annotation key — clearing the field omits cert binding entirely,
	// so this uses promptOptional rather than promptWithDefault, which would
	// hand the default back for an empty answer.
	tlsCertAnnotation, err := promptOptional(rt,
		"TLS cert annotation key (empty for none)",
		pd.tlsCertAnnotation,
	)
	if err != nil {
		return err
	}

	// Gateway hostname — required for shared gateway creation. Env var > prompt.
	hostname := strings.TrimSpace(os.Getenv(gatewayHostnameEnv))
	if hostname == "" {
		for {
			value, err := rt.Reporter.Input(
				"Gateway hostname (e.g. shaide.example.com)",
				"", "",
			)
			if err != nil {
				return err
			}
			value = strings.TrimSpace(value)
			if value != "" {
				hostname = value
				break
			}
			rt.Detailf("gateway hostname is required")
		}
	} else {
		rt.Detailf("using gateway hostname from %s env var", gatewayHostnameEnv)
	}
	// Persist so downstream stages (app-shaide) can use the same value
	// without re-prompting or relying on a StackReference.
	rt.Bootstrap.GatewayHostname = hostname
	// Persist the platform pick so app-serving and app-shaide can derive
	// their own cloudProvider values without re-prompting.
	rt.Bootstrap.CloudPlatform = platform

	// Gateway cert name — optional; only meaningful when tlsCertAnnotation is set.
	certName := strings.TrimSpace(os.Getenv(gatewayCertNameEnv))
	if certName == "" && tlsCertAnnotation != "" {
		value, err := rt.Reporter.Input(
			fmt.Sprintf("%s (leave empty to skip TLS binding)", pd.certNameDescription),
			"", "",
		)
		if err != nil {
			return err
		}
		certName = strings.TrimSpace(value)
	} else if certName != "" {
		rt.Detailf("using gateway cert name from %s env var", gatewayCertNameEnv)
	}

	deployConfig := auto.ConfigMap{
		pulumiConfigKey(projectGatewayProvider, "kubeconfig"): {
			Value: rt.Cluster.ConfigPath,
		},
		"gateway-provider:gatewayApiCrdsPath": {
			Value: filepath.Join(workdir, "crds", "gateway-api", "standard"),
		},
		"gateway-provider:gieCrdsPath": {
			Value: filepath.Join(workdir, "crds", "gie"),
		},
		pulumiConfigKey(projectGatewayProvider, "cloudProvider"): {
			Value: platform,
		},
		pulumiConfigKey(projectGatewayProvider, "gatewayClassName"): {
			Value: gatewayClassName,
		},
		pulumiConfigKey(projectGatewayProvider, "gatewayHostname"): {
			Value: hostname,
		},
	}
	if tlsCertAnnotation != "" {
		deployConfig[pulumiConfigKey(projectGatewayProvider, "tlsCertAnnotation")] = auto.ConfigValue{
			Value: tlsCertAnnotation,
		}
	}
	if certName != "" {
		deployConfig[pulumiConfigKey(projectGatewayProvider, "gatewayCertName")] = auto.ConfigValue{
			Value: certName,
		}
	}

	// Resource-ownership policy: take ownership of any existing istio-base /
	// istiod resources so the Catalogd Helm charts can adopt them instead of
	// failing with "already exists". Idempotent; no-op on a fresh cluster.
	const istioNamespace = "istio-system"
	rt.Detailf("ensuring installer ownership of Istio chart resources in %s", istioNamespace)
	if err := ensureChartOwnership(rt, "istio-base", istioNamespace, istioBaseChartResources(istioNamespace)); err != nil {
		return fmt.Errorf("take ownership of istio-base resources: %w", err)
	}
	if err := ensureChartOwnership(rt, "istiod", istioNamespace, istiodChartResources(istioNamespace)); err != nil {
		return fmt.Errorf("take ownership of istiod resources: %w", err)
	}

	deployer, err := iac.NewDeployer(iac.DeployerOptions{
		ProjectName: projectGatewayProvider,
		StackName:   stackGatewayProvider,
		WorkDir:     workdir,
		StateDir:    stateDir,
		Logger:      rt.Logger.Writer(),
		Config:      deployConfig,
		Destroy:     false,
		Passphrase:  rt.Bootstrap.Config.Pulumi.ConfigPassphrase,
	})
	if err != nil {
		return err
	}

	_, err = deployer.Deploy(context.Background(), gateway.DeployGatewayProvider)
	if err != nil {
		return err
	}
	return nil
}

// promptWithDefault asks the user for a value, returning the default if empty.
// promptGatewayClass offers the GatewayClasses the cluster actually accepts,
// rather than asking the operator to type one. A per-platform default that the
// cluster does not expose produces a Gateway that never programs, and the name
// is not guessable — AKS exposes azure-alb-external, GKE
// gke-l7-regional-external-managed, and so on.
//
// preferred is the platform default; it is pre-selected when present. If the
// classes cannot be listed (Gateway API CRDs absent, RBAC), fall back to
// free-text entry so the installer still works on a cluster it cannot
// introspect.
func promptGatewayClass(rt *core.Runtime, preferred string) (string, error) {
	label := fmt.Sprintf("Gateway class name [%s]", preferred)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	classes, err := kube.ListGatewayClasses(ctx, rt.Cluster.RESTConfig)
	if err != nil {
		rt.Detailf("could not list gateway classes (%v); falling back to manual entry", err)
		return promptWithDefault(rt, label, preferred)
	}

	if len(classes) == 0 {
		rt.Detailf("no accepted gateway classes found; falling back to manual entry")
		return promptWithDefault(rt, label, preferred)
	}

	current := preferred
	if !slices.Contains(classes, current) {
		// The platform default is not installed here. Offer it anyway so the
		// operator can still pick it, but select an available class by default.
		rt.Detailf("gateway class %q is not present on this cluster", preferred)
		if preferred != "" {
			classes = append(classes, preferred)
			slices.Sort(classes)
		}
		current = classes[0]
	}

	return rt.Reporter.Select("Gateway class name", current, classes)
}

// promptOptional pre-fills defaultValue and returns exactly what is left in the
// field, empty string included. Use it where "none" is a meaningful answer;
// promptWithDefault cannot express that, since it treats a cleared field as a
// request for the default.
func promptOptional(rt *core.Runtime, label, defaultValue string) (string, error) {
	value, err := rt.Reporter.Input(label, defaultValue, defaultValue)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(value), nil
}

// promptWithDefault pre-fills defaultValue and falls back to it when the field
// is cleared. Use it where a value is always required; see promptOptional when
// empty is a valid answer.
func promptWithDefault(rt *core.Runtime, label, defaultValue string) (string, error) {
	value, err := rt.Reporter.Input(label, defaultValue, defaultValue)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}
