// Package mcp deploys the MCP server namespace and its data source runtimes.
package mcp

import (
	"fmt"

	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/components/caconfigmap"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/components/mcpdeployment"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/components/mcpsecret"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/components/networkpolicy"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/components/rbac"
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/mcp/internal/config"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// DeployAppMCP deploys the MCP stack using the resolved Pulumi stack
// configuration. It is the entry point for running the program directly with
// the Pulumi CLI; the installer goes through Stack.Deploy instead.
func DeployAppMCP(ctx *pulumi.Context, projectDir string) error {
	return deployAppMCP(ctx, appconfig.New(projectDir, stackpkg.Options{}, appconfig.Sources{}))
}

func deployAppMCP(ctx *pulumi.Context, stackConfig appconfig.Config) error {
	appConfig, err := stackConfig.Load(ctx)
	if err != nil {
		return fmt.Errorf("load app-mcp config: %w", err)
	}

	// --- K8s Provider ---
	k8sProviderArgs := &kubernetes.ProviderArgs{}
	if appConfig.Kubeconfig != "" {
		k8sProviderArgs.Kubeconfig = pulumi.StringPtr(appConfig.Kubeconfig)
	}
	k8sProvider, err := kubernetes.NewProvider(ctx, "app-mcp-k8s", k8sProviderArgs)
	if err != nil {
		return err
	}
	providerOpt := pulumi.Provider(k8sProvider)

	// --- Namespace ---
	ns, err := iackube.CreateNamespace(ctx, appConfig.Namespace, providerOpt)
	if err != nil {
		return err
	}
	nsOpt := pulumi.DependsOn([]pulumi.Resource{ns})

	// --- RBAC ---
	if err := rbac.Deploy(ctx, appConfig, providerOpt, nsOpt); err != nil {
		return err
	}

	// --- NetworkPolicy: ingress (namespace-level, applied once) ---
	if err := networkpolicy.DeployIngress(ctx, appConfig, providerOpt, nsOpt); err != nil {
		return err
	}

	// --- Shared CA ConfigMap (optional; created once, used by datasources without their own cert) ---
	dsOpts := []pulumi.ResourceOption{providerOpt, nsOpt}
	sharedCM, err := caconfigmap.Deploy(ctx, appConfig, providerOpt, nsOpt)
	if err != nil {
		return err
	}
	if sharedCM != nil {
		dsOpts = append(dsOpts, pulumi.DependsOn([]pulumi.Resource{sharedCM}))
	}

	// --- MCP runtime Secret (optional; created once when secret config is provided) ---
	mcpSecret, err := mcpsecret.Deploy(ctx, appConfig, providerOpt, nsOpt)
	if err != nil {
		return err
	}
	if mcpSecret != nil {
		dsOpts = append(dsOpts, pulumi.DependsOn([]pulumi.Resource{mcpSecret}))
	}

	// --- MCP Server Deployments (one per datasource) ---
	for _, ds := range appConfig.Datasources {
		if err := mcpdeployment.Deploy(ctx, ds, appConfig, dsOpts...); err != nil {
			return err
		}
		if err := networkpolicy.DeployEgress(ctx, ds, appConfig, dsOpts...); err != nil {
			return err
		}
	}

	return nil

}
