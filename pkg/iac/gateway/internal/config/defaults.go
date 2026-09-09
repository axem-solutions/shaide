package config

import "github.com/axem-solutions/ai_platform/pkg/kube/cluster"

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
			gatewayClassName: "azure-alb-external",
		}
	case cluster.OnPrem:
		return providerDefaults{
			gatewayClassName: "istio",
		}
	default:
		return providerDefaults{}
	}
}
