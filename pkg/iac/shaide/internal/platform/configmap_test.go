package platform

import (
	"reflect"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestAppSecretDataProvidesCurrentAndLegacyAdminPasswordNames(t *testing.T) {
	adminPassword := pulumi.String("admin-password").ToStringOutput()
	data := appSecretData(appconfig.Values{
		Secrets: appconfig.AppSecrets{AdminAuthKey: adminPassword},
	})

	current, currentOK := data["ADMIN_PASSWORD"]
	legacy, legacyOK := data["ADMIN_AUTH_KEY"]
	if !currentOK || !legacyOK {
		t.Fatalf("admin secret keys present: ADMIN_PASSWORD=%t ADMIN_AUTH_KEY=%t", currentOK, legacyOK)
	}
	if !reflect.DeepEqual(current, legacy) {
		t.Fatal("ADMIN_PASSWORD and ADMIN_AUTH_KEY do not use the same configured value")
	}
}

// shaide-server serves the webapp under /app only when WEBAPP_URL is set, so it
// has to follow the operator's answer.
func TestWebAppURLFollowsWebAppEnabled(t *testing.T) {
	services := appconfig.ServiceNames{WebApp: "webapp"}

	enabled := appConfigData(appconfig.Values{WebAppEnabled: true, Services: services})
	if got, want := enabled["WEBAPP_URL"], "http://webapp:8787"; got != want {
		t.Errorf("WEBAPP_URL = %v, want %v", got, want)
	}

	disabled := appConfigData(appconfig.Values{WebAppEnabled: false, Services: services})
	if _, ok := disabled["WEBAPP_URL"]; ok {
		t.Error("WEBAPP_URL was set for a declined webapp")
	}
}

// shaide-server does not use object storage itself: KC owns RustFS. None of
// the former S3 settings reach the public server, enabled or not.
func TestServerReceivesNoStorageSettings(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := appconfig.Values{
			KnowledgeCenterEnabled: enabled,
			ShaideEnv: appconfig.AppEnv{
				S3User: "rustfsuser", S3FQDN: "rustfs", S3Port: "9000",
				RustFSWebhookARN: "arn:rustfs:sqs:eu-central-1:shaide:webhook",
				MCPNamespace:     "app-mcp",
			},
			Services: appconfig.ServiceNames{Rustfs: "rustfs"},
		}
		data := appConfigData(cfg)
		for _, key := range []string{
			"S3_USER", "SHAIDE_SERVER_S3_FQDN", "SHAIDE_SERVER_S3_PORT",
			"S3_UPLOAD_PROXY_ROUTE_PREFIX", "RUSTFS_WEBHOOK_ARN", "RUSTFS_SERVICE",
		} {
			if _, ok := data[key]; ok {
				t.Errorf("enabled=%t: shaide-config carries %s", enabled, key)
			}
		}
		secrets := appSecretData(cfg)
		if _, ok := secrets["S3_PASSWORD"]; ok {
			t.Errorf("enabled=%t: shaide-secrets carries S3_PASSWORD", enabled)
		}
	}
}

// With KC installed, shaide-server forwards storage and lifecycle calls to it
// and signs KC-MCP requests. Kubernetes discovery stays on: MCP_SERVERS
// would replace it with a static list.
func TestKnowledgeCenterServerSettings(t *testing.T) {
	cfg := appconfig.Values{
		KnowledgeCenterEnabled: true,
		ShaideEnv:              appconfig.AppEnv{MCPNamespace: "app-mcp"},
	}
	data := appConfigData(cfg)
	for key, want := range map[string]string{
		"KNOWLEDGE_CENTER_URL":                    "http://knowledge-center:8080",
		"KNOWLEDGE_CENTER_ALLOW_LOCAL_HTTP":       "true",
		"MCP_COLLECTION_CONTEXT_TARGETS":          knowledgecenter.ContextTargetsJSON(),
		"MCP_COLLECTION_CONTEXT_ALLOW_LOCAL_HTTP": "true",
		"MCP_NAMESPACE":                           "app-mcp",
	} {
		if data[key] != want {
			t.Errorf("%s = %q, want %q", key, data[key], want)
		}
	}
	if _, ok := data["MCP_SERVERS"]; ok {
		t.Error("MCP_SERVERS is set; it disables Kubernetes MCP discovery")
	}

	secrets := appSecretData(cfg)
	for _, key := range []string{"KNOWLEDGE_CENTER_INTERNAL_TOKEN", "S3_WEBHOOK_TOKEN"} {
		if _, ok := secrets[key]; !ok {
			t.Errorf("shaide-secrets lacks %s", key)
		}
	}
}

// Without KC the server must not see a KC URL or token: it would try to
// reach a service that does not exist, and token-without-URL is a startup
// error in shaide-server.
func TestDeclinedKnowledgeCenterLeavesServerUnchanged(t *testing.T) {
	cfg := appconfig.Values{ShaideEnv: appconfig.AppEnv{MCPNamespace: "app-mcp"}}
	data := appConfigData(cfg)
	for _, key := range []string{
		"KNOWLEDGE_CENTER_URL", "KNOWLEDGE_CENTER_ALLOW_LOCAL_HTTP",
		"MCP_COLLECTION_CONTEXT_TARGETS", "MCP_COLLECTION_CONTEXT_ALLOW_LOCAL_HTTP", "MCP_SERVERS",
	} {
		if _, ok := data[key]; ok {
			t.Errorf("shaide-config carries %s without KC", key)
		}
	}
	if data["MCP_NAMESPACE"] != "app-mcp" {
		t.Errorf("MCP_NAMESPACE = %q, want generic MCP discovery kept", data["MCP_NAMESPACE"])
	}
	secrets := appSecretData(cfg)
	for _, key := range []string{"KNOWLEDGE_CENTER_INTERNAL_TOKEN", "S3_WEBHOOK_TOKEN"} {
		if _, ok := secrets[key]; ok {
			t.Errorf("shaide-secrets carries %s without KC", key)
		}
	}
}

// A changed shaide-config has to change the checksum, or shaide-server keeps
// running with the env it started with.
func TestConfigChecksumFollowsConfigData(t *testing.T) {
	services := appconfig.ServiceNames{WebApp: "webapp"}
	enabled := appconfig.Values{WebAppEnabled: true, Services: services}
	disabled := appconfig.Values{WebAppEnabled: false, Services: services}

	if ConfigChecksum(enabled) != ConfigChecksum(enabled) {
		t.Error("ConfigChecksum is not stable for the same config")
	}
	if ConfigChecksum(enabled) == ConfigChecksum(disabled) {
		t.Error("ConfigChecksum did not change with WEBAPP_URL")
	}
}
