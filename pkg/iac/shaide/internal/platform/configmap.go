// App Shaide configuration: ConfigMap (env vars) and Secret (credentials) for
// shaide-server, which also feed the control panel.
//
// All values are read from Pulumi config so the same code works across stacks.
// All config keys must be set per stack; secrets via:
//
//	pulumi config set --secret adminAuthKey <value>
//	pulumi config set --secret s3Password <value>      (RustFS; see storage.go)
//	pulumi config set --secret jwtSecret <value>       (random string, at least 32 characters)
//	pulumi config set --secret sessionSecret <value>   (random string, at least 32 characters)
//
// shaide-server uses no object storage of its own; the RustFS credentials
// live in the storage-owned Secret that RustFS and the Knowledge Center read.
package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateAppShaideConfig creates the shared ConfigMap and Secret for the app-shaide stack.
func CreateAppShaideConfig(ctx *pulumi.Context, cfg appconfig.Values, providerOpt, nsOpt pulumi.ResourceOption) (*corev1.ConfigMap, *corev1.Secret, error) {
	shaideConfig, err := corev1.NewConfigMap(ctx, "shaide-config", &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-config"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Data: pulumi.ToStringMap(appConfigData(cfg)),
	}, providerOpt, nsOpt)
	if err != nil {
		return nil, nil, err
	}

	// --- Secret: shaide-server's own credentials ---
	// These must be set per stack: pulumi config set --secret <key> <value>
	secretData := appSecretData(cfg)

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

// appConfigData is the service discovery and application settings ConfigMap.
// Keys are injected as env vars into shaide-server via envFrom.
func appConfigData(cfg appconfig.Values) map[string]string {
	configData := map[string]string{
		"SHAIDE_SERVER_UI_FQDN": cfg.ShaideEnv.ShaideServerUiFQDN,
		"SHAIDE_SERVER_UI_PORT": cfg.ShaideEnv.ShaideServerUiPort,
		"DATABASE_URL":          cfg.ShaideEnv.DatabaseURL,
		"VECTOR_DB_URL":         cfg.ShaideEnv.VectorDBUrl,
		"RUST_LIB_BACKTRACE":    cfg.ShaideEnv.RustLibBacktrace,
		"RUST_SPANTRACE":        cfg.ShaideEnv.RustSpantrace,
		"CONTROL_PANEL_SERVICE": cfg.Services.ControlPanel,
		"WEBAPP_SERVICE":        cfg.Services.WebApp,
		"QDRANT_SERVICE":        cfg.Services.Qdrant,
		"TRIAL":                 cfg.ShaideEnv.Trial,
	}
	// MCP deployment is optional — only wire the namespace through when configured.
	if cfg.ShaideEnv.MCPNamespace != "" {
		configData["MCP_NAMESPACE"] = cfg.ShaideEnv.MCPNamespace
	}
	// shaide-server proxies /app to the webapp only when WEBAPP_URL is set.
	if cfg.WebAppEnabled {
		configData["WEBAPP_URL"] = cfg.WebAppURL()
	}
	// shaide-server serves the KC storage and lifecycle APIs only when
	// KNOWLEDGE_CENTER_URL is set, and signs KC-MCP requests only for the
	// listed target. Both are plain HTTP inside the cluster, which the
	// server accepts only with the explicit opt-ins. MCP_SERVERS stays unset:
	// it would replace Kubernetes discovery with a static list.
	if cfg.KnowledgeCenterEnabled {
		configData["KNOWLEDGE_CENTER_URL"] = knowledgecenter.LocalURL()
		configData["KNOWLEDGE_CENTER_ALLOW_LOCAL_HTTP"] = "true"
		configData["MCP_COLLECTION_CONTEXT_TARGETS"] = knowledgecenter.ContextTargetsJSON()
		configData["MCP_COLLECTION_CONTEXT_ALLOW_LOCAL_HTTP"] = "true"
	}

	return configData
}

// ConfigChecksum identifies the shaide-config data. shaide-server reads the
// ConfigMap as env vars only at start, so its pods carry the checksum and
// roll when the data changes.
func ConfigChecksum(cfg appconfig.Values) string {
	data := appConfigData(cfg)

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	hash := sha256.New()
	for _, key := range keys {
		fmt.Fprintf(hash, "%s=%s\n", key, data[key])
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func appSecretData(cfg appconfig.Values) pulumi.StringMap {
	data := pulumi.StringMap{
		"ADMIN_AUTH_KEY": cfg.Secrets.AdminAuthKey,
		// Current shaide-server versions read ADMIN_PASSWORD. Keep the legacy
		// ADMIN_AUTH_KEY alias so older images remain deployable from the same
		// installer-managed Secret.
		"ADMIN_PASSWORD": cfg.Secrets.AdminAuthKey,
		"JWT_SECRET":     cfg.Secrets.JWTSecret,
		"SESSION_SECRET": cfg.Secrets.SessionSecret,
	}
	// shaide-server authenticates to KC with the internal token and checks
	// RustFS notifications against the webhook token. The KC-MCP signing
	// material is mounted as files instead; see knowledgecenter.go.
	if cfg.KnowledgeCenterEnabled {
		data["KNOWLEDGE_CENTER_INTERNAL_TOKEN"] = cfg.Secrets.KCInternalToken
		data["S3_WEBHOOK_TOKEN"] = cfg.Secrets.KCWebhookToken
	}

	return data
}
