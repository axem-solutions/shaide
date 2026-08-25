package harbor

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options holds all inputs required to deploy Harbor.
type Options struct {
	AdminPassword pulumi.StringOutput
	// RobotSecret is the Harbor robot account password (robot$k8s-harbor-sa).
	// Set via vault_harbor_robot_password in ansible vault.yml, then register
	// with Pulumi as harbor:robotPassword. When not yet set, the imagePullSecret
	// is skipped and must be created on a subsequent pulumi up.
	RobotSecret          pulumi.StringOutput
	RobotSecretConfigured bool
	// Hostname is the internal Harbor hostname (e.g. harbor.internal.lan).
	// Must match harbor_hostname in ansible/group_vars/all/harbor.yml.
	Hostname string
	// NodeHostname is the Kubernetes node name where Harbor pods must run.
	// Harbor uses hostPath PVs so all pods are pinned to this node.
	NodeHostname string
	// ChartPath is the local path to the Harbor Helm chart tarball.
	ChartPath string
	// HostpathBase is the base directory on NodeHostname for Harbor data,
	// e.g. /var/lib/hostpath/harbor. Sub-directories are created by Ansible.
	HostpathBase string
	Provider     pulumi.ProviderResource
}

// Deployment holds the resources created by Deploy.
type Deployment struct {
	Namespace  *corev1.Namespace
	Release    *helmv3.Release
	PullSecret *corev1.Secret
}

// Deploy creates the Harbor namespace, hostPath PersistentVolumes, the Helm
// release, and optionally an imagePullSecret in the harbor namespace.
//
// Execution order enforced by DependsOn:
//
//	Namespace → PVs → HelmRelease → PullSecret (if RobotSecretConfigured)
func Deploy(ctx *pulumi.Context, opts Options) (*Deployment, error) {
	ns, err := createNamespace(ctx, opts.Provider)
	if err != nil {
		return nil, fmt.Errorf("harbor namespace: %w", err)
	}

	pvs, err := createVolumes(ctx, opts.NodeHostname, opts.HostpathBase, ns, opts.Provider)
	if err != nil {
		return nil, fmt.Errorf("harbor volumes: %w", err)
	}

	pvDeps := make([]pulumi.Resource, len(pvs))
	for i, pv := range pvs {
		pvDeps[i] = pv
	}

	release, err := helmv3.NewRelease(ctx, "harbor", &helmv3.ReleaseArgs{
		Chart:       pulumi.String(opts.ChartPath),
		Namespace:   pulumi.String("harbor"),
		Name:        pulumi.String("harbor"),
		Values:      buildHarborValues(opts.AdminPassword, "http://"+opts.Hostname, opts.NodeHostname),
		WaitForJobs: pulumi.Bool(true),
		Timeout:     pulumi.Int(600),
	}, pulumi.Provider(opts.Provider), pulumi.DependsOn(append(pvDeps, ns)))
	if err != nil {
		return nil, fmt.Errorf("harbor helm release: %w", err)
	}

	var pullSecret *corev1.Secret
	if opts.RobotSecretConfigured {
		pullSecret, err = createPullSecret(
			ctx,
			"harbor",
			opts.Hostname,
			opts.RobotSecret,
			opts.Provider,
			[]pulumi.Resource{ns, release},
		)
		if err != nil {
			return nil, fmt.Errorf("harbor pull secret: %w", err)
		}
	}

	return &Deployment{
		Namespace:  ns,
		Release:    release,
		PullSecret: pullSecret,
	}, nil
}
