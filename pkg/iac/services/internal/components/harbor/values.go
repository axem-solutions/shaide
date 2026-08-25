package harbor

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// buildHarborValues constructs the Helm values map for the Harbor chart.
//
// Design notes:
//   - expose.type = clusterIP:  Harbor is internal-only; no ingress or NodePort.
//   - TLS disabled:             HTTP on port 80 inside the cluster.
//   - Image overrides omitted:  Harbor images are pre-loaded into containerd by
//     Ansible (harbor_preload role). The chart's default image names match what
//     was imported so no registry override is needed for Harbor itself.
//   - imagePullPolicy = IfNotPresent: ensures containerd uses pre-loaded images
//     and does not attempt to pull from the internet.
//   - Trivy disabled:           Vulnerability scanning requires internet access.
//   - nodeSelector:             All Harbor pods pinned to the same node as the PVs.
func buildHarborValues(
	adminPassword pulumi.StringOutput,
	externalURL string,
	nodeHostname string,
) pulumi.Map {
	nodeSelector := pulumi.StringMap{
		"kubernetes.io/hostname": pulumi.String(nodeHostname),
	}

	ifNotPresent := pulumi.Map{"pullPolicy": pulumi.String("IfNotPresent")}

	return pulumi.Map{
		"expose": pulumi.Map{
			"type": pulumi.String("clusterIP"),
			"clusterIP": pulumi.Map{
				"name": pulumi.String("harbor"),
				"ports": pulumi.Map{
					"httpPort":  pulumi.Int(80),
					"httpsPort": pulumi.Int(443),
				},
			},
			"tls": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
		},

		"externalURL":         pulumi.String(externalURL),
		"harborAdminPassword": adminPassword,
		"nodeSelector":        nodeSelector,
		"updateStrategy":      pulumi.Map{"type": pulumi.String("Recreate")},

		// ── Persistence ────────────────────────────────────────────────────────
		// storageClass "hostpath" → Harbor chart creates PVCs that bind to the
		// hostPath PVs created by volumes.go (StorageClass deployed by infra stack).
		"persistence": pulumi.Map{
			"enabled":        pulumi.Bool(true),
			"resourcePolicy": pulumi.String("keep"),
			"persistentVolumeClaim": pulumi.Map{
				"registry": pulumi.Map{
					"storageClass": pulumi.String("hostpath"),
					"size":         pulumi.String("150Gi"),
					"accessMode":   pulumi.String("ReadWriteOnce"),
				},
				"jobservice": pulumi.Map{
					"jobLog": pulumi.Map{
						"storageClass": pulumi.String("hostpath"),
						"size":         pulumi.String("1Gi"),
						"accessMode":   pulumi.String("ReadWriteOnce"),
					},
				},
				"database": pulumi.Map{
					"storageClass": pulumi.String("hostpath"),
					"size":         pulumi.String("2Gi"),
					"accessMode":   pulumi.String("ReadWriteOnce"),
				},
				"redis": pulumi.Map{
					"storageClass": pulumi.String("hostpath"),
					"size":         pulumi.String("1Gi"),
					"accessMode":   pulumi.String("ReadWriteOnce"),
				},
			},
		},

		// ── Per-component node selector + image pull policy ───────────────────
		"core":    pulumi.Map{"nodeSelector": nodeSelector, "image": ifNotPresent},
		"portal":  pulumi.Map{"nodeSelector": nodeSelector, "image": ifNotPresent},
		"jobservice": pulumi.Map{
			"nodeSelector": nodeSelector,
			"image":        ifNotPresent,
		},
		"nginx": pulumi.Map{"nodeSelector": nodeSelector, "image": ifNotPresent},
		"registry": pulumi.Map{
			"nodeSelector": nodeSelector,
			"controller":   pulumi.Map{"image": ifNotPresent},
		},
		"database": pulumi.Map{
			"nodeSelector": nodeSelector,
			"internal":     pulumi.Map{"image": ifNotPresent},
		},
		"redis": pulumi.Map{
			"nodeSelector": nodeSelector,
			"internal":     pulumi.Map{"image": ifNotPresent},
		},

		// ── Disabled components ────────────────────────────────────────────────
		"trivy": pulumi.Map{"enabled": pulumi.Bool(false)},
	}
}
