package config

import "github.com/axem-solutions/ai_platform/pkg/kube/platform"

type platformDefaults struct {
	gatewayClassName  string
	tlsCertAnnotation string
}

func defaultsForPlatform(provider platform.Platform) platformDefaults {
	switch provider {
	case platform.GCP:
		return platformDefaults{
			gatewayClassName:  "gke-l7-regional-external-managed",
			tlsCertAnnotation: "networking.gke.io/cert-manager-certs",
		}
	case platform.AWS:
		return platformDefaults{
			gatewayClassName:  "alb",
			tlsCertAnnotation: "alb.ingress.kubernetes.io/certificate-arn",
		}
	case platform.Azure:
		return platformDefaults{
			gatewayClassName: "azure-alb-external",
		}
	case platform.OnPrem:
		return platformDefaults{
			gatewayClassName: "istio",
		}
	default:
		return platformDefaults{}
	}
}
