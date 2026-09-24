package config

import (
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
)

func TestDefaultsForProvider(t *testing.T) {
	tests := []struct {
		name          string
		platform      cluster.Provider
		gatewayClass  string
		tlsAnnotation string
	}{
		{"GCP", cluster.GCP, "gke-l7-regional-external-managed", "networking.gke.io/cert-manager-certs"},
		{"AWS", cluster.AWS, "alb", "alb.ingress.kubernetes.io/certificate-arn"},
		{"Azure", cluster.Azure, "azure-alb-external", ""},
		{"on-prem", cluster.OnPrem, "istio", ""},
		{"unknown", cluster.Provider("unknown"), "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultsForProvider(tt.platform)
			if got.gatewayClassName != tt.gatewayClass {
				t.Errorf("gatewayClassName = %q, want %q", got.gatewayClassName, tt.gatewayClass)
			}
			if got.tlsCertAnnotation != tt.tlsAnnotation {
				t.Errorf("tlsCertAnnotation = %q, want %q", got.tlsCertAnnotation, tt.tlsAnnotation)
			}
		})
	}
}

func TestApplyDefaultsUsesProviderGatewayDefaults(t *testing.T) {
	var cfg Values
	cfg.Platform = cluster.GCP

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
	var cfg Values
	cfg.Platform = cluster.GCP
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
	validConfig := func() Values {
		var cfg Values
		cfg.Platform = cluster.Azure
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

// The class decides AGC. An albName left in the stack file from an earlier
// choice must not turn an Istio Gateway into an AGC one.
func TestUsesAGCFollowsTheClass(t *testing.T) {
	tests := []struct {
		name     string
		platform cluster.Provider
		class    string
		albName  string
		want     bool
	}{
		{"azure agc", cluster.Azure, AGCGatewayClassName, "", true},
		{"azure istio with stale alb name", cluster.Azure, IstioGatewayClassName, "shared-alb", false},
		{"agc class off azure", cluster.GCP, AGCGatewayClassName, "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg Values
			cfg.Platform = test.platform
			cfg.Gateway.ClassName = test.class
			cfg.Gateway.ALB.Name = test.albName

			if got := cfg.UsesAGC(); got != test.want {
				t.Errorf("UsesAGC() = %t, want %t", got, test.want)
			}
		})
	}
}

// A direct pulumi up with the AGC class still names its load balancer.
func TestApplyDefaultsNamesTheALBForAGC(t *testing.T) {
	var cfg Values
	cfg.Platform = cluster.Azure
	cfg.Gateway.ClassName = AGCGatewayClassName

	if err := applyDefaults(&cfg); err != nil {
		t.Fatalf("applyDefaults() error = %v", err)
	}
	if cfg.Gateway.ALB.Name != DefaultALBName {
		t.Errorf("ALB.Name = %q, want %q", cfg.Gateway.ALB.Name, DefaultALBName)
	}
}
