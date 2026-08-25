// RBAC for Shaide Server to watch MCP pods.
//
// Resources created:
//   - Role        mcp-pod-reader        namespace: mcp-gateway  (verbs: watch, list on pods)
//   - RoleBinding shaide-mcp-pod-reader namespace: mcp-gateway  (binds Shaide SA → mcp-pod-reader)
//
// The Shaide Server ServiceAccount already exists in app-shaide — it is created and owned
// by the app_shaide stack. This package grants that existing ServiceAccount watch/list
// access to MCP pods.
//
// The SA name is configurable via shaideServiceAccountName (default: shaide-server) and
// must match the value set in the corresponding app_shaide stack config.
//
// A namespace-scoped Role is sufficient — no ClusterRole needed.
package rbac

import (
	appconfig "app_mcp/internal/config"

	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Deploy(ctx *pulumi.Context, cfg appconfig.Config, providerOpt, nsOpt pulumi.ResourceOption) error {
	readerRole, err := rbacv1.NewRole(ctx, "mcp-pod-reader", &rbacv1.RoleArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("mcp-pod-reader"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{pulumi.String("pods")},
				Verbs:     pulumi.StringArray{pulumi.String("watch"), pulumi.String("list")},
			},
		},
	}, providerOpt, nsOpt)
	if err != nil {
		return err
	}

	_, err = rbacv1.NewRoleBinding(ctx, "shaide-mcp-pod-reader", &rbacv1.RoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("shaide-mcp-pod-reader"),
			Namespace: pulumi.String(cfg.Namespace),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("Role"),
			Name:     pulumi.String("mcp-pod-reader"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String(cfg.ShaideServiceAccountName),
				Namespace: pulumi.String(cfg.ShaideNamespace),
			},
		},
	}, providerOpt, nsOpt, pulumi.DependsOn([]pulumi.Resource{readerRole}))
	return err
}
