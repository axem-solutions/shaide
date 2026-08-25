package gpuoperator

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Options holds all inputs required to deploy the NVIDIA GPU Operator.
type Options struct {
	// HarborHostname is the internal Harbor hostname used to build image URLs.
	HarborHostname string
	// HarborProjectName is the Harbor project where infrastructure images are stored.
	HarborProjectName string
	// RobotSecret is the Harbor robot account secret for pulling images.
	// Only required after ansible/harbor_setup.yml has run.
	RobotSecret           pulumi.StringOutput
	RobotSecretConfigured bool
	// GPUNodeHostname pins the GPU Operator controller pod to the GPU worker node.
	GPUNodeHostname string
	ChartPath       string
	Provider        pulumi.ProviderResource
	// DependsOn lists resources that must be ready before the GPU Operator deploys
	// (typically the Harbor Helm release).
	DependsOn []pulumi.Resource
}

// Deployment holds the resources created by Deploy.
type Deployment struct {
	Namespace *corev1.Namespace
	Release   *helmv3.Release
}

// Deploy creates the gpu-operator namespace, optional imagePullSecret,
// and the GPU Operator Helm release.
//
// Execution order enforced by DependsOn:
//
//	Namespace → PullSecret (if configured) → HelmRelease
func Deploy(ctx *pulumi.Context, opts Options) (*Deployment, error) {
	ns, err := createNamespace(ctx, opts.Provider)
	if err != nil {
		return nil, fmt.Errorf("gpu-operator namespace: %w", err)
	}

	nsDeps := []pulumi.Resource{ns}

	if opts.RobotSecretConfigured {
		_, err = createPullSecret(ctx, opts.HarborHostname, opts.RobotSecret, opts.Provider, nsDeps)
		if err != nil {
			return nil, fmt.Errorf("gpu-operator pull secret: %w", err)
		}
	}

	releaseDeps := append(opts.DependsOn, ns)

	release, err := helmv3.NewRelease(ctx, "gpu-operator", &helmv3.ReleaseArgs{
		Chart:       pulumi.String(opts.ChartPath),
		Namespace:   pulumi.String("gpu-operator"),
		Name:        pulumi.String("gpu-operator"),
		Values:      buildGPUOperatorValues(opts.HarborHostname, opts.HarborProjectName, opts.GPUNodeHostname, opts.RobotSecretConfigured),
		WaitForJobs: pulumi.Bool(true),
		Timeout:     pulumi.Int(600),
	}, pulumi.Provider(opts.Provider), pulumi.DependsOn(releaseDeps))
	if err != nil {
		return nil, fmt.Errorf("gpu-operator helm release: %w", err)
	}

	return &Deployment{
		Namespace: ns,
		Release:   release,
	}, nil
}
