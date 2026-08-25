package platform

import (
	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"

	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CreateShaideRBAC grants shaide-server the Kubernetes API access it needs at runtime.
func CreateShaideRBAC(
	ctx *pulumi.Context,
	cfg appconfig.Config,
	providerOpt pulumi.ResourceOption,
) ([]pulumi.Resource, error) {
	roleName := "shaide-server-pod-reader"
	role, err := rbacv1.NewClusterRole(ctx, "shaide-server-pod-reader", &rbacv1.ClusterRoleArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(roleName),
		},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{
					pulumi.String("pods"),
				},
				Verbs: pulumi.StringArray{
					pulumi.String("get"),
					pulumi.String("list"),
					pulumi.String("watch"),
				},
			},
		},
	}, providerOpt)
	if err != nil {
		return nil, err
	}

	binding, err := rbacv1.NewClusterRoleBinding(ctx, "shaide-server-pod-reader-binding", &rbacv1.ClusterRoleBindingArgs{
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String(roleName),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String(cfg.ServiceAccountName),
				Namespace: pulumi.String(cfg.Namespace),
			},
		},
	}, providerOpt, pulumi.DependsOn([]pulumi.Resource{role}))
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{role, binding}, nil
}
