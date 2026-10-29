package mcp

import (
	"encoding/base64"
	"fmt"
	"github.com/axem-solutions/ai_platform/pkg/iac/internal/pulumitest"
	"github.com/axem-solutions/ai_platform/pkg/iac/knowledgecenter/knowledgecentertest"
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
		"namespace": "app-mcp", "shaideNamespace": "app-shaide", "shaideServiceAccountName": "shaide-server", "knowledgeCenterMcpImage": kcMCPHarborImage,
	}
	out := auto.ConfigMap{}
	for k, v := range values {
		out["app-mcp:"+k] = auto.ConfigValue{Value: v, Secret: strings.HasPrefix(k, "kc") && k != "kcEmbeddingUsername" || k == "jwtSecret" || k == "adminAuthKey"}
	}
	return out
}

func deployDirect(t *testing.T, cfg auto.ConfigMap) (pulumitest.Result, error) {
	t.Helper()
	return pulumitest.Run(t, "app-mcp", cfg, func(ctx *pulumi.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("deployment panicked: %v", r)
			}
		}()
		return DeployAppMCP(ctx, "/projects/app-mcp")
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
	fields := []string{"kcGatewayToken", "kcScopedToken", "kcContextPublicKey"}
	for _, field := range fields {
		for _, kind := range []string{"missing", "malformed"} {
			t.Run(field+"/"+kind, func(t *testing.T) {
				cfg := directConfig()
				key := "app-mcp:" + field
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
		cfg["app-mcp:kcScopedToken"] = cfg["app-mcp:kcGatewayToken"]
		assertDirectRejected(t, cfg, "must differ")
	})
}

func TestDirectLoadValidCredentials(t *testing.T) {
	cfg := directConfig()
	result, err := deployDirect(t, cfg)
	if err != nil {
		t.Fatalf("valid direct deployment: %v", err)
	}
	secret := result.Must(t, typeSecret, "kc-mcp-context")
	if len(secret.Secrets) == 0 {
		t.Error("resource lost secret protection")
	}
}

func TestDirectLoadDatasources(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, input := range []string{"missing", "[]", "null", "not-json", "{}", `[{"name":"jira","image":"harbor/jira:v1"},{"port":"invalid"}]`} {
			t.Run(fmt.Sprintf("enabled=%t/%s", enabled, input), func(t *testing.T) {
				cfg := directConfig()
				cfg["app-mcp:knowledgeCenterEnabled"] = auto.ConfigValue{Value: fmt.Sprint(enabled)}
				if input != "missing" {
					cfg["app-mcp:datasources"] = auto.ConfigValue{Value: input}
				}
				empty := input == "missing" || input == "[]" || input == "null"
				if enabled && empty {
					if _, err := deployDirect(t, cfg); err != nil {
						t.Fatalf("KC-only direct deploy rejected: %v", err)
					}
				} else {
					assertDirectRejected(t, cfg, "datasources")
				}
			})
		}
	}
	t.Run("generic-without-KC", func(t *testing.T) {
		cfg := directConfig()
		cfg["app-mcp:knowledgeCenterEnabled"] = auto.ConfigValue{Value: "false"}
		cfg["app-mcp:datasources"] = auto.ConfigValue{Value: `[{"name":"jira","image":"harbor/jira:v1"}]`}
		result, err := deployDirect(t, cfg)
		if err != nil {
			t.Fatal(err)
		}
		result.Must(t, typeDeployment, "mcp-jira")
	})
}

func TestDirectLoadGatewayFileBoundary(t *testing.T) {
	for _, n := range []int{3071, 3072, 3073} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			cfg := directConfig()
			token := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("g", n)))
			cfg["app-mcp:kcGatewayToken"] = auto.ConfigValue{Value: token, Secret: true}
			if len(token) > 4096 {
				assertDirectRejected(t, cfg, "kcGatewayToken")
			} else {
				result, err := deployDirect(t, cfg)
				if err != nil {
					t.Fatalf("valid gateway boundary: %v", err)
				}
				files := stringMap(result.Must(t, typeSecret, "kc-mcp-context").Inputs["stringData"])
				if files["gateway_token"] != token {
					t.Error("gateway token bytes changed")
				}
			}
		})
	}
}
