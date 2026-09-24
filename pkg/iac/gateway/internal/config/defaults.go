package config

import "github.com/axem-solutions/ai_platform/pkg/kube/cluster"

// GatewayClasses the stack treats specially. The class decides the Gateway
// implementation, and with it which infrastructure the Gateway needs.
const (
	// AGCGatewayClassName is Azure Application Gateway for Containers. Only
	// this class needs an ApplicationLoadBalancer and its subnet.
	AGCGatewayClassName = "azure-alb-external"

	// IstioGatewayClassName is created by the Istio this stack installs, so it
	// is available on every cluster once the stack has run.
	IstioGatewayClassName = "istio"
)

// SharedGatewayName names the Gateway every platform route attaches to.
const SharedGatewayName = "shared-gateway"

// DefaultALBName names the ApplicationLoadBalancer the stack creates for AGC.
const DefaultALBName = "shared-alb"

// DefaultGatewayClassName is the class a platform most commonly uses.
func DefaultGatewayClassName(provider cluster.Provider) string {
	return defaultsForProvider(provider).gatewayClassName
}

type providerDefaults struct {
	gatewayClassName  string
	tlsCertAnnotation string
}

func defaultsForProvider(provider cluster.Provider) providerDefaults {
	switch provider {
	case cluster.GCP:
		return providerDefaults{
			gatewayClassName:  "gke-l7-regional-external-managed",
			tlsCertAnnotation: "networking.gke.io/cert-manager-certs",
		}
	case cluster.AWS:
		return providerDefaults{
			gatewayClassName:  "alb",
			tlsCertAnnotation: "alb.ingress.kubernetes.io/certificate-arn",
		}
	case cluster.Azure:
		return providerDefaults{
			gatewayClassName: AGCGatewayClassName,
		}
	case cluster.OnPrem:
		return providerDefaults{
			gatewayClassName: IstioGatewayClassName,
		}
	default:
		return providerDefaults{}
	}
}
