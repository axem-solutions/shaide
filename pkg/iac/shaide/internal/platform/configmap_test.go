package platform

import (
	"reflect"
	"testing"

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
