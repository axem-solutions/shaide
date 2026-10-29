package shaide

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/axem-solutions/ai_platform/pkg/iac/internal/pulumitest"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"strings"
	"testing"
)

func directConfig() auto.ConfigMap {
	c := knowledgecentertest.Credentials()
	values := map[string]string{
		"knowledgeCenterEnabled": "true", "kcGatewayToken": c.GatewayToken,
		"kcScopedToken": c.ScopedToken, "kcContextPublicKey": c.ContextPublicKey,
		"namespace": "app-shaide", "controlPanelService": "shaide-control-panel", "webappService": "shaide-app", "rustfsService": "rustfs", "qdrantService": "qdrant",
		"shaideServerImage": "harbor/shaide/server:v1", "controlPanelImage": "harbor/shaide/control-panel:v1", "rustfsImage": "harbor/shaide/rustfs:v1", "qdrantImage": "harbor/shaide/qdrant:v1",
		"shaideServiceAccountName": "shaide-server", "cloudProvider": "generic",
		"knowledgeCenterImage": kcHarborImage, "kcInternalToken": c.InternalToken,
		"kcWebhookToken": c.WebhookToken, "kcContextSigningKey": c.ContextSigningKey,
		"kcEmbeddingUsername": c.EmbeddingUsername, "kcEmbeddingPassword": c.EmbeddingPassword,
		"sessionSecret": "retained-session-secret", "s3Password": "retained-storage-password", "s3User": "retained-storage-user", "jwtSecret": "core-jwt-not-a-kc-credential", "adminAuthKey": "core-admin-not-a-kc-credential",
	}
	out := auto.ConfigMap{}
	for k, v := range values {
		out["app-shaide:"+k] = auto.ConfigValue{Value: v, Secret: strings.HasPrefix(k, "kc") && k != "kcEmbeddingUsername" || k == "jwtSecret" || k == "adminAuthKey"}
	}
	return out
}

func deployDirect(t *testing.T, cfg auto.ConfigMap) (pulumitest.Result, error) {
	t.Helper()
	return pulumitest.Run(t, "app-shaide", cfg, func(ctx *pulumi.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("deployment panicked: %v", r)
			}
		}()
		return DeployAppShaide(ctx, "/projects/app-shaide")
	})
}

func assertDirectRejected(t *testing.T, cfg auto.ConfigMap, field string) {
	t.Helper()
	result, err := deployDirect(t, cfg)
	if err == nil {
		t.Error("direct deployment accepted invalid configuration")
	} else {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error does not identify %s: %v", field, err)
		}
		for key, v := range cfg {
			if v.Secret && len(v.Value) > 0 && strings.Contains(err.Error(), v.Value) {
				t.Errorf("error leaked secret field %s", key)
			}
		}
	}
	for _, r := range result.Resources {
		if r.Type != "pulumi:pulumi:Stack" {
			t.Errorf("registered %s %s before rejecting config", r.Type, r.Name)
		}
	}
}

func TestDirectLoadCredentials(t *testing.T) {
	fields := []string{"kcGatewayToken", "kcScopedToken", "kcContextPublicKey", "kcInternalToken", "kcWebhookToken", "kcContextSigningKey", "kcEmbeddingUsername", "kcEmbeddingPassword"}
	for _, field := range fields {
		for _, kind := range []string{"missing", "malformed"} {
			t.Run(field+"/"+kind, func(t *testing.T) {
				cfg := directConfig()
				key := "app-shaide:" + field
				if kind == "missing" {
					delete(cfg, key)
				} else {
					v := cfg[key]
					v.Value = "invalid test credential"
					if field == "kcEmbeddingPassword" {
						v.Value = ""
					}
					cfg[key] = v
				}
				assertDirectRejected(t, cfg, field)
			})
		}
	}
	t.Run("reused-hop", func(t *testing.T) {
		cfg := directConfig()
		cfg["app-shaide:kcScopedToken"] = cfg["app-shaide:kcGatewayToken"]
		assertDirectRejected(t, cfg, "must differ")
	})
	t.Run("mismatched-key", func(t *testing.T) {
		cfg := directConfig()
		key, _ := rsa.GenerateKey(rand.Reader, 2048)
		der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
		cfg["app-shaide:kcContextPublicKey"] = auto.ConfigValue{Value: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), Secret: true}
		assertDirectRejected(t, cfg, "does not match")
	})
	for _, core := range []string{"jwtSecret", "adminAuthKey"} {
		for _, field := range []string{"kcInternalToken", "kcWebhookToken", "kcGatewayToken", "kcScopedToken", "kcEmbeddingPassword"} {
			t.Run(field+"/reuses-"+core, func(t *testing.T) {
				cfg := directConfig()
				cfg["app-shaide:"+core] = cfg["app-shaide:"+field]
				assertDirectRejected(t, cfg, "must differ")
			})
		}
	}
	t.Run("reused-password", func(t *testing.T) {
		cfg := directConfig()
		cfg["app-shaide:kcEmbeddingPassword"] = cfg["app-shaide:kcInternalToken"]
		assertDirectRejected(t, cfg, "must differ")
	})
}

func TestDirectLoadValidCredentials(t *testing.T) {
	cfg := directConfig()
	cfg["app-shaide:kcEmbeddingPassword"] = auto.ConfigValue{Value: "  dedicated ordinary password  ", Secret: true}
	signing := cfg["app-shaide:kcContextSigningKey"]
	signing.Value += " \t\r\n"
	cfg["app-shaide:kcContextSigningKey"] = signing
	result, err := deployDirect(t, cfg)
	if err != nil {
		t.Fatalf("valid direct deployment: %v", err)
	}
	secret := result.Must(t, typeSecret, "knowledge-center")
	if len(secret.Secrets) == 0 {
		t.Error("resource lost secret protection")
	}
	if stringMap(result.Must(t, typeSecret, "kc-context-signer").Inputs["stringData"])["private.pem"] != signing.Value {
		t.Error("signing PEM bytes changed")
	}
	if stringMap(secret.Inputs["stringData"])["KC_SHAIDE_PASSWORD"] != "  dedicated ordinary password  " {
		t.Error("password bytes changed")
	}
}

func TestDirectLoadGatewayFileBoundary(t *testing.T) {
	for _, n := range []int{3071, 3072, 3073} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			cfg := directConfig()
			token := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("g", n)))
			cfg["app-shaide:kcGatewayToken"] = auto.ConfigValue{Value: token, Secret: true}
			if len(token) > 4096 {
				assertDirectRejected(t, cfg, "kcGatewayToken")
			} else {
				result, err := deployDirect(t, cfg)
				if err != nil {
					t.Fatalf("valid gateway boundary: %v", err)
				}
				files := stringMap(result.Must(t, typeSecret, "kc-context-signer").Inputs["stringData"])
				if files["gateway_token"] != token {
					t.Error("gateway token bytes changed")
				}
			}
		})
	}
}

func TestDirectLoadSigningEnvelope(t *testing.T) {
	for _, surround := range []string{"leading", "trailing"} {
		t.Run(surround, func(t *testing.T) {
			cfg := directConfig()
			v := cfg["app-shaide:kcContextSigningKey"]
			if surround == "leading" {
				v.Value = " " + v.Value
			} else {
				v.Value += "extra"
			}
			cfg["app-shaide:kcContextSigningKey"] = v
			assertDirectRejected(t, cfg, "kcContextSigningKey")
		})
	}
}

func TestDirectSigningJWTContract(t *testing.T) {
	for _, mode := range []string{"Load", "deployment"} {
		for _, kind := range []string{"exact", "trimmed", "distinct"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				cfg := directConfig()
				signing := cfg["app-shaide:kcContextSigningKey"].Value + " \t\r\n"
				jwt := signing
				if kind == "trimmed" {
					jwt = " \t\n" + strings.TrimSpace(signing) + "\r\n "
				} else if kind == "distinct" {
					jwt = "  distinct-jwt-signing-secret-0123456789  "
				}
				cfg["app-shaide:kcContextSigningKey"] = auto.ConfigValue{Value: signing, Secret: true}
				cfg["app-shaide:jwtSecret"] = auto.ConfigValue{Value: jwt, Secret: true}
				var result pulumitest.Result
				var err error
				if mode == "Load" {
					result, err = pulumitest.Run(t, "app-shaide", cfg, func(ctx *pulumi.Context) error {
						return assertLoadedSigningCredentials(t, ctx, cfg)
					})
				} else {
					result, err = deployDirect(t, cfg)
				}
				if kind == "distinct" {
					if err != nil {
						t.Fatal("distinct signing and JWT credentials rejected")
					}
					if mode == "deployment" {
						assertSigningSecret(t, result, signing)
						secret := result.Must(t, typeSecret, "shaide-secrets")
						if len(secret.Secrets) == 0 || stringMap(secret.Inputs["stringData"])["JWT_SECRET"] != jwt {
							t.Error("JWT secret lost original bytes or secret protection")
						}
					}
					return
				}
				if err == nil {
					t.Error("direct entry accepted signing PEM reused as JWT secret")
				} else {
					if !strings.Contains(err.Error(), "kcContextSigningKey and jwtSecret must differ") {
						t.Error("direct entry did not return dedicated signing/JWT field-only error")
					}
					for _, value := range []string{signing, jwt, strings.TrimSpace(signing)} {
						if strings.Contains(err.Error(), value) {
							t.Error("direct entry error leaked signing/JWT credential")
						}
					}
				}
				for _, r := range result.Resources {
					if r.Type != "pulumi:pulumi:Stack" {
						t.Errorf("registered %s %s before rejecting config", r.Type, r.Name)
					}
				}
			})
		}
	}
}

func assertSigningSecret(t *testing.T, result pulumitest.Result, signing string) {
	t.Helper()
	secret := result.Must(t, typeSecret, "kc-context-signer")
	if len(secret.Secrets) == 0 {
		t.Error("signing Secret lost secret protection")
	}
	if stringMap(secret.Inputs["stringData"])["private.pem"] != signing {
		t.Error("signing PEM bytes changed")
	}
}

func TestDirectSigningFileByteBoundary(t *testing.T) {
	for _, mode := range []string{"Load", "deployment"} {
		for _, size := range []int{32768, 32769} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				cfg := directConfig()
				signing := cfg["app-shaide:kcContextSigningKey"].Value
				signing += strings.Repeat(" ", size-len(signing))
				cfg["app-shaide:kcContextSigningKey"] = auto.ConfigValue{Value: signing, Secret: true}
				var result pulumitest.Result
				var err error
				if mode == "Load" {
					result, err = pulumitest.Run(t, "app-shaide", cfg, func(ctx *pulumi.Context) error {
						return assertLoadedSigningCredentials(t, ctx, cfg)
					})
				} else {
					result, err = deployDirect(t, cfg)
				}
				if size == 32768 {
					if err != nil {
						t.Fatal("direct entry rejected valid matching 32768-byte signing PEM")
					}
					if mode == "deployment" {
						assertSigningSecret(t, result, signing)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), "kcContextSigningKey: must be at most 32768 bytes") {
					t.Error("direct entry did not reject original 32769-byte signing file with field-only error")
				}
				if err != nil && strings.Contains(err.Error(), strings.TrimSpace(signing)) {
					t.Error("direct entry error leaked signing PEM")
				}
				for _, r := range result.Resources {
					if r.Type != "pulumi:pulumi:Stack" {
						t.Errorf("registered %s %s before rejecting config", r.Type, r.Name)
					}
				}
			})
		}
	}
}

func assertLoadedSigningCredentials(t *testing.T, ctx *pulumi.Context, cfg auto.ConfigMap) error {
	t.Helper()
	loaded, err := appconfig.New("/projects/app-shaide", stack.Options{}, appconfig.Sources{}).Load(ctx)
	if err != nil {
		return err
	}
	if !pulumi.IsSecret(loaded.Secrets.KCContextSigningKey) || !pulumi.IsSecret(loaded.Secrets.JWTSecret) {
		t.Error("Load lost signing/JWT secret protection")
	}
	ctx.Export("credential-preservation", pulumi.All(loaded.Secrets.KCContextSigningKey, loaded.Secrets.JWTSecret).ApplyT(func(values []interface{}) bool {
		if values[0] != cfg["app-shaide:kcContextSigningKey"].Value || values[1] != cfg["app-shaide:jwtSecret"].Value {
			t.Error("Load changed original signing/JWT bytes")
			return false
		}
		return true
	}))
	return nil
}
