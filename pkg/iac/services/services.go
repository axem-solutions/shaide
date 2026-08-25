package services

import (
	gpuoperator "github.com/axem-solutions/ai_platform/pkg/iac/services/internal/components/gpu-operator"
	"github.com/axem-solutions/ai_platform/pkg/iac/services/internal/components/harbor"
	"github.com/axem-solutions/ai_platform/pkg/iac/services/internal/components/metallb"
	"github.com/axem-solutions/ai_platform/pkg/iac/services/internal/config"

	k8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func DeployServices(ctx *pulumi.Context) error {
	cfg := config.NewStackConfig(ctx)

	provider, err := k8s.NewProvider(ctx, "rke2", &k8s.ProviderArgs{
		Kubeconfig: pulumi.String(cfg.Kubeconfig()),
	})
	if err != nil {
		return err
	}

	enabled := cfg.Components()

	// ── Harbor ────────────────────────────────────────────────────────────
	// Deploy first — all other components pull images from Harbor.
	// After this completes:
	//   1. ansible-playbook harbor_setup.yml    (projects + robot + /etc/hosts)
	//   2. pulumi config set --secret harbor:robotPassword $(cat artifacts/harbor-robot-secret)
	//   3. ansible-playbook harbor_upload.yml   (push images)
	//   4. Enable remaining components and re-run pulumi up
	// ─────────────────────────────────────────────────────────────────────
	var harborDeployment *harbor.Deployment

	if enabled["harbor"] {
		robotSecret, robotConfigured := cfg.HarborRobotPassword()
		harborDeployment, err = harbor.Deploy(ctx, harbor.Options{
			AdminPassword:         cfg.HarborAdminPassword(),
			RobotSecret:           robotSecret,
			RobotSecretConfigured: robotConfigured,
			Hostname:              cfg.HarborHostname(),
			NodeHostname:          cfg.HarborNodeHostname(),
			ChartPath:             cfg.HarborChartPath(),
			HostpathBase:          "/var/lib/hostpath/harbor",
			Provider:              provider,
		})
		if err != nil {
			return err
		}

		ctx.Export("harborReleaseName", harborDeployment.Release.Name)
	}

	// ── MetalLB ───────────────────────────────────────────────────────────
	// Deploy after Harbor so images can be pulled from the internal registry.
	// ─────────────────────────────────────────────────────────────────────
	if enabled["metallb"] {
		var harborDeps []pulumi.Resource
		if harborDeployment != nil {
			harborDeps = []pulumi.Resource{harborDeployment.Release}
		}

		robotSecret, robotConfigured := cfg.HarborRobotPassword()
		_, err = metallb.Deploy(ctx, metallb.Options{
			HarborHostname:         cfg.HarborHostname(),
			HarborProjectName:      cfg.HarborProjectName(),
			RobotSecret:            robotSecret,
			RobotSecretConfigured:  robotConfigured,
			ControllerNodeHostname: cfg.MetalLBControllerNodeHostname(),
			L2Interface:            cfg.MetalLBL2Interface(),
			IPPool:                 cfg.MetalLBIPPool(),
			ChartPath:              cfg.MetalLBChartPath(),
			Provider:               provider,
			DependsOn:              harborDeps,
		})
		if err != nil {
			return err
		}
	}

	// ── GPU Operator ──────────────────────────────────────────────────────
	// Deploy after Harbor so images can be pulled from the internal registry.
	// ─────────────────────────────────────────────────────────────────────
	if enabled["gpu-operator"] {
		var harborDeps []pulumi.Resource
		if harborDeployment != nil {
			harborDeps = []pulumi.Resource{harborDeployment.Release}
		}

		robotSecret, robotConfigured := cfg.HarborRobotPassword()
		_, err = gpuoperator.Deploy(ctx, gpuoperator.Options{
			HarborHostname:        cfg.HarborHostname(),
			HarborProjectName:     cfg.HarborProjectName(),
			RobotSecret:           robotSecret,
			RobotSecretConfigured: robotConfigured,
			GPUNodeHostname:       cfg.GPUOperatorGPUNodeHostname(),
			ChartPath:             cfg.GPUOperatorChartPath(),
			Provider:              provider,
			DependsOn:             harborDeps,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
