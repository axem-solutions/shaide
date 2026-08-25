// Company-internal CA ConfigMap.
//
// Optional. Created when cfg.CompanyCACert is set. Provides the company's internal root CA
// certificate as a single ConfigMap used by all datasources that do not define their own CACert.
//
// Naming:
//   - CompanyCAName ("company-internal-ca") is exported so mcpdeployment can reference it by name
//     without needing a runtime handle.
//
// Precedence (resolved in mcpdeployment):
//   ds.CACert != ""           → per-datasource ConfigMap mcp-<name>-ca  (takes precedence)
//   cfg.CompanyCACert != ""   → this ConfigMap company-internal-ca       (fallback)
//   neither                   → no CA cert mounted
package caconfigmap

import (
	appconfig "app_mcp/internal/config"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CompanyCAName is the Kubernetes name of the company-internal CA ConfigMap.
// Referenced by mcpdeployment when a datasource has no per-datasource CACert.
const CompanyCAName = "company-internal-ca"

// Deploy creates the company-internal-ca ConfigMap. Skipped when cfg.CompanyCACert is empty.
// Returns the created resource (nil when skipped) so main can add it to DependsOn.
func Deploy(ctx *pulumi.Context, cfg appconfig.Config, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	if cfg.CompanyCACert == "" {
		return nil, nil
	}
	cm, err := corev1.NewConfigMap(ctx, CompanyCAName, &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(CompanyCAName),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Data: pulumi.StringMap{
			"ca.crt": pulumi.String(cfg.CompanyCACert),
		},
	}, opts...)
	return cm, err
}
