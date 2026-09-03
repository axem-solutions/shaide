package config

import (
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
)

func TestDefaultsForPlatform(t *testing.T) {
	tests := []struct {
		name          string
		platform      platform.Platform
		gatewayClass  string
		tlsAnnotation string
	}{
		{"GCP", platform.GCP, "gke-l7-regional-external-managed", "networking.gke.io/cert-manager-certs"},
		{"AWS", platform.AWS, "alb", "alb.ingress.kubernetes.io/certificate-arn"},
		{"Azure", platform.Azure, "azure-alb-external", ""},
		{"on-prem", platform.OnPrem, "istio", ""},
		{"unknown", platform.Platform("unknown"), "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultsForPlatform(tt.platform)
			if got.gatewayClassName != tt.gatewayClass {
				t.Errorf("gatewayClassName = %q, want %q", got.gatewayClassName, tt.gatewayClass)
			}
			if got.tlsCertAnnotation != tt.tlsAnnotation {
				t.Errorf("tlsCertAnnotation = %q, want %q", got.tlsCertAnnotation, tt.tlsAnnotation)
			}
		})
	}
}

func TestApplyDefaultsUsesPlatformGatewayDefaults(t *testing.T) {
	var cfg Config
	cfg.Platform = platform.GCP

	if err := applyDefaults(&cfg); err != nil {
		t.Fatalf("applyDefaults() error = %v", err)
	}

	if cfg.Gateway.ClassName != "gke-l7-regional-external-managed" {
		t.Errorf("Gateway.ClassName = %q", cfg.Gateway.ClassName)
	}
	if cfg.TLS.CertAnnotation != "networking.gke.io/cert-manager-certs" {
		t.Errorf("TLS.CertAnnotation = %q", cfg.TLS.CertAnnotation)
	}
}

func TestApplyDefaultsPreservesConfiguredGatewayValues(t *testing.T) {
	var cfg Config
	cfg.Platform = platform.GCP
	cfg.Gateway.ClassName = "custom-class"
	cfg.TLS.CertAnnotation = "custom.example/certificate"

	if err := applyDefaults(&cfg); err != nil {
		t.Fatalf("applyDefaults() error = %v", err)
	}

	if cfg.Gateway.ClassName != "custom-class" {
		t.Errorf("Gateway.ClassName = %q, want custom-class", cfg.Gateway.ClassName)
	}
	if cfg.TLS.CertAnnotation != "custom.example/certificate" {
		t.Errorf("TLS.CertAnnotation = %q, want custom annotation", cfg.TLS.CertAnnotation)
	}
}

func TestValidateRequiresOneGatewaySource(t *testing.T) {
	validConfig := func() Config {
		var cfg Config
		cfg.Platform = platform.Azure
		cfg.Gateway.ClassName = "azure-alb-external"
		cfg.Gateway.Namespace = DefaultGatewayNamespace
		cfg.CRDs.GIEPath = DefaultGIECRDsPath
		return cfg
	}

	t.Run("direct hostname", func(t *testing.T) {
		cfg := validConfig()
		cfg.Gateway.Hostname = "shaide.example.com"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("infrastructure stack", func(t *testing.T) {
		cfg := validConfig()
		cfg.Gateway.InfraStackRef = "organization/azure-cluster/axem-dev-westeurope"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("missing source", func(t *testing.T) {
		cfg := validConfig()
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want a gateway source error")
		}
	})

	t.Run("conflicting sources", func(t *testing.T) {
		cfg := validConfig()
		cfg.Gateway.Hostname = "shaide.example.com"
		cfg.Gateway.InfraStackRef = "organization/azure-cluster/axem-dev-westeurope"
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want a mutually-exclusive source error")
		}
	})
}
