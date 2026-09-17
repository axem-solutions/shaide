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
