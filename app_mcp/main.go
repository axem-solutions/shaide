// App MCP application stack orchestrator.
//
// Creates all MCP namespace resources and dispatches each component to its own file:
//   - internal/components/rbac            (ServiceAccount, Role, RoleBinding)
//   - internal/components/networkpolicy   (allow-shaide-ingress + per-datasource egress)
//   - internal/components/mcpdeployment   (ConfigMap, Deployment, Service per datasource)
package main

import (
	"app_mcp/internal/components/caconfigmap"
	"app_mcp/internal/components/mcpdeployment"
	"app_mcp/internal/components/mcpsecret"
	"app_mcp/internal/components/networkpolicy"
	"app_mcp/internal/components/rbac"
	appconfig "app_mcp/internal/config"
	iackube "github.com/axem-solutions/ai_platform/pkg/iac/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		appConfig := appconfig.Load(ctx)

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
	})
}
