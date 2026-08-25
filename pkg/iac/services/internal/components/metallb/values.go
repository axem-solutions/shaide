package metallb

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

// buildMetalLBValues constructs the Helm values map for the MetalLB chart.
//
// Design notes:
//   - Images pulled from internal Harbor registry (air-gapped).
//   - imagePullPolicy = IfNotPresent: uses images already in Harbor.
//   - controller pinned to controllerNodeHostname via nodeSelector.
//   - speaker runs as DaemonSet on all nodes (no nodeSelector).
//   - frr enabled: required for BGP/L2 advertisement.
func buildMetalLBValues(harborHostname, projectName, controllerNodeHostname string, robotConfigured bool) pulumi.Map {
	base := harborHostname + "/" + projectName

	values := pulumi.Map{
		"controller": pulumi.Map{
			"image": pulumi.Map{
				"repository": pulumi.String(base + "/metallb/controller"),
				"tag":        pulumi.String("v0.15.3"),
				"pullPolicy": pulumi.String("IfNotPresent"),
			},
			"nodeSelector": pulumi.StringMap{
				"kubernetes.io/hostname": pulumi.String(controllerNodeHostname),
			},
		},
		"speaker": pulumi.Map{
			"image": pulumi.Map{
				"repository": pulumi.String(base + "/metallb/speaker"),
				"tag":        pulumi.String("v0.15.3"),
				"pullPolicy": pulumi.String("IfNotPresent"),
			},
			"frr": pulumi.Map{
				"image": pulumi.Map{
					"repository": pulumi.String(base + "/frrouting/frr"),
					"tag":        pulumi.String("10.4.1"),
					"pullPolicy": pulumi.String("IfNotPresent"),
				},
			},
		},
	}

	if robotConfigured {
		values["imagePullSecrets"] = pulumi.Array{
			pulumi.Map{"name": pulumi.String("harbor-pull-secret")},
		}
	}

	return values
}
