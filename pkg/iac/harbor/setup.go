package harbor

import (
	"fmt"
	"strings"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	harborprovider "github.com/pulumiverse/pulumi-harbor/sdk/v3/go/harbor"

	pkgkube "github.com/axem-solutions/ai_platform/pkg/kube"
)

// parseProjectNames splits harbor:projects (one name per line, same
// convention as harbor:publicImages/harbor:ghcrMinVersions) into a clean
// list. No default baked in — an unset or empty config value means no
// projects get declared here.
func parseProjectNames(raw string) []string {
	var names []string
	for _, line := range strings.Split(raw, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// ensureHarborSetup configures Harbor once it's actually deployed: declares
// each name in cfg.Projects as a public Harbor project, and declares the
// robot account with its password set to cfg.RobotPassword. Both become real
// Pulumi resources via the pulumiverse/pulumi-harbor provider — the engine
// diffs and reconciles them on every subsequent apply, no hand-rolled
// idempotency logic needed.
//
// Returns a pulumi.StringOutput derived from the robot account's own Secret
// output, not cfg.RobotPassword itself. Callers (createHarborPullSecret,
// deployImageMirror) must consume this returned Output instead of the raw
// config value — that's what makes Pulumi's dependency graph wait for the
// robot account to actually exist in Harbor before creating anything that
// depends on it being able to authenticate.
func ensureHarborSetup(
	ctx *pulumi.Context,
	release *helmv3.Release,
	cfg harborConfig,
) (pulumi.StringOutput, error) {
	// Gated on release.ResourceNames (a genuinely computed output, unlike
	// e.g. release.Name which just echoes back an input) so this callback
	// only actually runs once the Helm release has really been applied —
	// during `pulumi up`'s preview phase the input is unknown, so the
	// callback (and the port-forward it performs) is skipped entirely.
	//
	// Uses a detached daemon (pkg/kube.EnsureDurableForward), not a
	// port-forward tied to this program's own process: the engine can still
	// be issuing provider operations against this URL — including deletes,
	// during `pulumi destroy` — well after this program has registered
	// everything and exited. A forward that dies with this process would
	// already be gone by then.
	forwardedURL := pulumi.All(release.ResourceNames, cfg.AdminPassword).ApplyT(
		func(args []interface{}) (string, error) {
			return pkgkube.EnsureDurableForward(cfg.KubeconfigPath, cfg.KubeContext, cfg.Namespace, harborServiceName)
		},
	).(pulumi.StringOutput)

	harborAPI, err := harborprovider.NewProvider(ctx, "harbor-api", &harborprovider.ProviderArgs{
		Url:      forwardedURL.ToStringPtrOutput(),
		Username: pulumi.String("admin"),
		Password: cfg.AdminPassword.ToStringPtrOutput(),
	}, pulumi.DependsOn([]pulumi.Resource{release}))
	if err != nil {
		return pulumi.StringOutput{}, fmt.Errorf("harbor provider: %w", err)
	}

	projects := make([]pulumi.Resource, 0, len(cfg.Projects))
	for _, name := range cfg.Projects {
		project, err := harborprovider.NewProject(ctx, "harbor-project-"+name, &harborprovider.ProjectArgs{
			Name:   pulumi.String(name),
			Public: pulumi.Bool(true),
			// Without this, deleting a non-empty project (the normal case —
			// these hold real mirrored images) fails outright. Harbor deletes
			// the project's repositories first when this is set.
			ForceDestroy: pulumi.Bool(true),
		}, pulumi.Provider(harborAPI))
		if err != nil {
			return pulumi.StringOutput{}, fmt.Errorf("harbor project %q: %w", name, err)
		}
		projects = append(projects, project)
	}

	// Permissions match what the pull secret and mirror jobs actually need:
	// pull to run images, push to mirror into Harbor, delete/list/read for
	// upkeep — same scope the installer's own robot account already has.
	robot, err := harborprovider.NewRobotAccount(ctx, "harbor-robot-"+harborRobotAccountName, &harborprovider.RobotAccountArgs{
		Name:   pulumi.String(harborRobotAccountName),
		Level:  pulumi.String("system"),
		Secret: cfg.RobotPassword.ToStringPtrOutput(),
		Permissions: harborprovider.RobotAccountPermissionArray{
			&harborprovider.RobotAccountPermissionArgs{
				Kind:      pulumi.String("project"),
				Namespace: pulumi.String("*"),
				Accesses: harborprovider.RobotAccountPermissionAccessArray{
					&harborprovider.RobotAccountPermissionAccessArgs{Action: pulumi.String("pull"), Resource: pulumi.String("repository")},
					&harborprovider.RobotAccountPermissionAccessArgs{Action: pulumi.String("push"), Resource: pulumi.String("repository")},
					&harborprovider.RobotAccountPermissionAccessArgs{Action: pulumi.String("delete"), Resource: pulumi.String("repository")},
					&harborprovider.RobotAccountPermissionAccessArgs{Action: pulumi.String("list"), Resource: pulumi.String("repository")},
					&harborprovider.RobotAccountPermissionAccessArgs{Action: pulumi.String("read"), Resource: pulumi.String("artifact")},
					&harborprovider.RobotAccountPermissionAccessArgs{Action: pulumi.String("list"), Resource: pulumi.String("artifact")},
				},
			},
		},
	}, pulumi.Provider(harborAPI), pulumi.DependsOn(projects))
	if err != nil {
		return pulumi.StringOutput{}, fmt.Errorf("harbor robot account: %w", err)
	}

	return robot.Secret, nil
}
