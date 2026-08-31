package setup

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/chart"
	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/config"
	pkgkube "github.com/axem-solutions/ai_platform/pkg/kube"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	harborprovider "github.com/pulumiverse/pulumi-harbor/sdk/v3/go/harbor"
)

const (
	robotAccountName = "k8s-harbor-sa"
	robotUsername    = "robot$" + robotAccountName
)

type Result struct {
	Username string
	Password pulumi.StringOutput
}

func Ensure(ctx *pulumi.Context, release *helmv3.Release, cfg config.Config) (Result, error) {
	robotPassword, err := ensureResources(
		ctx,
		release,
		cfg,
	)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Username: robotUsername,
		Password: robotPassword,
	}, nil
}

func ensureResources(ctx *pulumi.Context, release *helmv3.Release, cfg config.Config) (pulumi.StringOutput, error) {
	// ResourceNames is a genuinely computed Helm output, so this callback
	// runs only once the Helm release has actually been applied.
	//
	// EnsureDurableForward creates a detached port-forward because Harbor
	// provider operations may continue after the Pulumi program has finished
	// registering resources, including during destroy.
	forwardedURL := pulumi.All(release.ResourceNames, cfg.Harbor.AdminPassword).ApplyT(
		func(args []interface{}) (string, error) {
			return pkgkube.EnsureDurableForward(cfg.Kubernetes.KubeconfigPath, cfg.Kubernetes.Context, cfg.Harbor.Namespace, chart.ServiceName)
		},
	).(pulumi.StringOutput)

	harborAPI, err := harborprovider.NewProvider(ctx, "harbor-api", &harborprovider.ProviderArgs{
		Url:      forwardedURL.ToStringPtrOutput(),
		Username: pulumi.String("admin"),
		Password: cfg.Harbor.AdminPassword.ToStringPtrOutput(),
	}, pulumi.DependsOn([]pulumi.Resource{release}))
	if err != nil {
		return pulumi.StringOutput{}, fmt.Errorf("harbor provider: %w", err)
	}

	projects := make([]pulumi.Resource, 0, len(cfg.Harbor.Projects))

	for _, name := range cfg.Harbor.Projects {
		project, err := harborprovider.NewProject(
			ctx,
			"harbor-project-"+name,
			&harborprovider.ProjectArgs{
				Name:   pulumi.String(name),
				Public: pulumi.Bool(true),

				// Projects contain mirrored images, so deleting a project
				// should also remove its repositories.
				ForceDestroy: pulumi.Bool(true),
			},
			pulumi.Provider(harborAPI),
		)
		if err != nil {
			return pulumi.StringOutput{}, fmt.Errorf(
				"harbor project %q: %w",
				name,
				err,
			)
		}

		projects = append(projects, project)
	}

	robot, err := harborprovider.NewRobotAccount(
		ctx,
		"harbor-robot-"+robotAccountName,
		&harborprovider.RobotAccountArgs{
			Name:   pulumi.String(robotAccountName),
			Level:  pulumi.String("system"),
			Secret: cfg.Harbor.Robot.Password.ToStringPtrOutput(),

			Permissions: harborprovider.RobotAccountPermissionArray{
				&harborprovider.RobotAccountPermissionArgs{
					Kind:      pulumi.String("project"),
					Namespace: pulumi.String("*"),

					Accesses: harborprovider.RobotAccountPermissionAccessArray{
						&harborprovider.RobotAccountPermissionAccessArgs{
							Action:   pulumi.String("pull"),
							Resource: pulumi.String("repository"),
						},
						&harborprovider.RobotAccountPermissionAccessArgs{
							Action:   pulumi.String("push"),
							Resource: pulumi.String("repository"),
						},
						&harborprovider.RobotAccountPermissionAccessArgs{
							Action:   pulumi.String("delete"),
							Resource: pulumi.String("repository"),
						},
						&harborprovider.RobotAccountPermissionAccessArgs{
							Action:   pulumi.String("list"),
							Resource: pulumi.String("repository"),
						},
						&harborprovider.RobotAccountPermissionAccessArgs{
							Action:   pulumi.String("read"),
							Resource: pulumi.String("artifact"),
						},
						&harborprovider.RobotAccountPermissionAccessArgs{
							Action:   pulumi.String("list"),
							Resource: pulumi.String("artifact"),
						},
					},
				},
			},
		},
		pulumi.Provider(harborAPI),
		pulumi.DependsOn(projects),
	)
	if err != nil {
		return pulumi.StringOutput{}, fmt.Errorf(
			"harbor robot account: %w",
			err,
		)
	}

	return robot.Secret, nil
}
