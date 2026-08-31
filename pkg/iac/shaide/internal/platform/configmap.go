// App Shaide configuration: ConfigMap (env vars) and Secret (credentials).
//
// All values are read from Pulumi config so the same code works across stacks.
// All config keys must be set per stack; secrets via:
//
//	pulumi config set --secret adminAuthKey <value>
//	pulumi config set --secret s3Password <value>
//	pulumi config set --secret jwtSecret <value>
package platform

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateAppShaideConfig creates the shared ConfigMap and Secret for the app-shaide stack.
func CreateAppShaideConfig(ctx *pulumi.Context, cfg appconfig.Config, providerOpt, nsOpt pulumi.ResourceOption) (*corev1.ConfigMap, *corev1.Secret, error) {
	// --- ConfigMap: service discovery and application settings ---
	// Keys are injected as env vars into shaide-server via envFrom.
	configData := pulumi.StringMap{
		"SHAIDE_SERVER_UI_FQDN":        pulumi.String(cfg.ShaideEnv.ShaideServerUiFQDN),
		"SHAIDE_SERVER_UI_PORT":        pulumi.String(cfg.ShaideEnv.ShaideServerUiPort),
		"DATABASE_URL":                 pulumi.String(cfg.ShaideEnv.DatabaseURL),
		"S3_USER":                      pulumi.String(cfg.ShaideEnv.S3User),
		"SHAIDE_SERVER_S3_FQDN":        pulumi.String(cfg.ShaideEnv.S3FQDN),
		"SHAIDE_SERVER_S3_PORT":        pulumi.String(cfg.ShaideEnv.S3Port),
		"S3_UPLOAD_PROXY_ROUTE_PREFIX": pulumi.String(cfg.ShaideEnv.S3UploadProxyRoutePrefix),
		"RUSTFS_WEBHOOK_ARN":           pulumi.String(cfg.ShaideEnv.RustFSWebhookARN),
		"VECTOR_DB_URL":                pulumi.String(cfg.ShaideEnv.VectorDBUrl),
		"RUST_LIB_BACKTRACE":           pulumi.String(cfg.ShaideEnv.RustLibBacktrace),
		"RUST_SPANTRACE":               pulumi.String(cfg.ShaideEnv.RustSpantrace),
		"CONTROL_PANEL_SERVICE":        pulumi.String(cfg.Services.ControlPanel),
		"WEBAPP_SERVICE":               pulumi.String(cfg.Services.WebApp),
		"RUSTFS_SERVICE":               pulumi.String(cfg.Services.Rustfs),
		"QDRANT_SERVICE":               pulumi.String(cfg.Services.Qdrant),
		"TRIAL":                        pulumi.String(cfg.ShaideEnv.Trial),
	}
	// MCP deployment is optional — only wire the namespace through when configured.
	if cfg.ShaideEnv.MCPNamespace != "" {
		configData["MCP_NAMESPACE"] = pulumi.String(cfg.ShaideEnv.MCPNamespace)
	}

	shaideConfig, err := corev1.NewConfigMap(ctx, "shaide-config", &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-config"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Data: configData,
	}, providerOpt, nsOpt)
	if err != nil {
		return nil, nil, err
	}

	// --- Secret: credentials for S3 (rustfs) and admin auth ---
	// These must be set per stack: pulumi config set --secret <key> <value>
	secretData := pulumi.StringMap{
		"ADMIN_AUTH_KEY": cfg.Secrets.AdminAuthKey,
		"S3_PASSWORD":    cfg.Secrets.S3Password,
		"JWT_SECRET":     cfg.Secrets.JWTSecret,
	}

	shaideSecrets, err := corev1.NewSecret(ctx, "shaide-secrets", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-secrets"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		StringData: secretData,
	}, providerOpt, nsOpt)
	if err != nil {
		return nil, nil, err
	}

	return shaideConfig, shaideSecrets, nil
}
