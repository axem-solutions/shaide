package metallb

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options holds all inputs required to deploy MetalLB.
type Options struct {
	// HarborHostname is the internal Harbor hostname used to build image URLs.
	HarborHostname string
	// HarborProjectName is the Harbor project where infrastructure images are stored.
	HarborProjectName string
	// RobotSecret is the Harbor robot account secret for pulling images.
	// Only required after ansible/harbor_setup.yml has run.
	RobotSecret           pulumi.StringOutput
	RobotSecretConfigured bool
	// ControllerNodeHostname pins the MetalLB controller pod to a specific node.
	// Also used as the nodeSelector in L2Advertisement so only this node's speaker
	// is elected to announce LoadBalancer IPs.
	ControllerNodeHostname string
	// L2Interface restricts L2 announcements to a specific NIC on the speaker node.
	// Leave empty to allow MetalLB to use any interface.
	L2Interface string
	// IPPool is the MetalLB IP address pool range (e.g. "10.99.10.200-10.99.10.220").
	IPPool    string
	ChartPath string
	Provider  pulumi.ProviderResource
	// DependsOn lists resources that must be ready before MetalLB deploys
	// (typically the Harbor Helm release).
	DependsOn []pulumi.Resource
}

// Deployment holds the resources created by Deploy.
type Deployment struct {
	Namespace *corev1.Namespace
	Release   *helmv3.Release
}

// Deploy creates the metallb-system namespace, optional imagePullSecret,
// the MetalLB Helm release, and the IPAddressPool + L2Advertisement CRs.
//
// Execution order enforced by DependsOn:
//
//	Namespace → PullSecret (if configured) → HelmRelease → IPPool + L2Advert
func Deploy(ctx *pulumi.Context, opts Options) (*Deployment, error) {
	ns, err := createNamespace(ctx, opts.Provider)
	if err != nil {
		return nil, fmt.Errorf("metallb namespace: %w", err)
	}

	nsDeps := []pulumi.Resource{ns}

	if opts.RobotSecretConfigured {
		_, err = createPullSecret(ctx, opts.HarborHostname, opts.RobotSecret, opts.Provider, nsDeps)
		if err != nil {
			return nil, fmt.Errorf("metallb pull secret: %w", err)
		}
	}

	releaseDeps := append(opts.DependsOn, ns)

	release, err := helmv3.NewRelease(ctx, "metallb", &helmv3.ReleaseArgs{
		Chart:       pulumi.String(opts.ChartPath),
		Namespace:   pulumi.String("metallb-system"),
		Name:        pulumi.String("metallb"),
		Values:      buildMetalLBValues(opts.HarborHostname, opts.HarborProjectName, opts.ControllerNodeHostname, opts.RobotSecretConfigured),
		WaitForJobs: pulumi.Bool(true),
		Timeout:     pulumi.Int(300),
	}, pulumi.Provider(opts.Provider), pulumi.DependsOn(releaseDeps))
	if err != nil {
		return nil, fmt.Errorf("metallb helm release: %w", err)
	}

	if err = createIPPool(ctx, opts.IPPool, opts.ControllerNodeHostname, opts.L2Interface, opts.Provider, []pulumi.Resource{release}); err != nil {
		return nil, fmt.Errorf("metallb ip pool: %w", err)
	}

	return &Deployment{
		Namespace: ns,
		Release:   release,
	}, nil
}
