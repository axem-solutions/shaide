package gpuoperator

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

// buildGPUOperatorValues constructs the Helm values map for the GPU Operator chart.
//
// Image versions are sourced from the published chart gpu-operator-v25.10.1.tgz.
//
// Design notes:
//   - All images redirected to the internal Harbor registry (air-gapped).
//   - imagePullPolicy = IfNotPresent: uses images already loaded into Harbor.
//   - Operator deployment pinned to gpuNodeHostname via nodeSelector.
//   - DaemonSet workloads are scheduled by the operator itself onto GPU-labelled
//     nodes; no additional nodeSelector is needed here.
//   - driver.enabled = false: NVIDIA drivers are pre-installed by the OS; the
//     operator must not deploy or manage them.
//   - node-feature-discovery sub-chart image also redirected to Harbor.
func buildGPUOperatorValues(harborHostname, projectName, gpuNodeHostname string, robotConfigured bool) pulumi.Map {
	nv := harborHostname + "/" + projectName + "/nvcr.io/nvidia"
	nvK8s := harborHostname + "/" + projectName + "/nvcr.io/nvidia/k8s"
	nvCN := harborHostname + "/" + projectName + "/nvcr.io/nvidia/cloud-native"
	nfd := harborHostname + "/" + projectName + "/registry.k8s.io/nfd/node-feature-discovery"

	values := pulumi.Map{
		// daemonsets.tolerations applies to all NVIDIA daemonsets (toolkit, device-plugin,
		// dcgm, etc.). Required so they can schedule on GPU nodes that carry dedicated taints
		// (dedicated=gpu:NoSchedule on server3, dedicated=gpu-pro:NoSchedule on server4).
		"daemonsets": pulumi.Map{
			"tolerations": pulumi.Array{
				pulumi.Map{
					"key":      pulumi.String("dedicated"),
					"operator": pulumi.String("Exists"),
					"effect":   pulumi.String("NoSchedule"),
				},
			},
		},
		"operator": pulumi.Map{
			"repository":      pulumi.String(nv),
			"image":           pulumi.String("gpu-operator"),
			"version":         pulumi.String("v25.10.1"),
			"imagePullPolicy": pulumi.String("IfNotPresent"),
			"nodeSelector": pulumi.StringMap{
				"kubernetes.io/hostname": pulumi.String(gpuNodeHostname),
			},
			"tolerations": pulumi.Array{
				pulumi.Map{
					"key":      pulumi.String("dedicated"),
					"value":    pulumi.String("gpu"),
					"effect":   pulumi.String("NoSchedule"),
					"operator": pulumi.String("Equal"),
				},
			},
			"initContainer": pulumi.Map{
				"repository": pulumi.String(nv),
				"image":      pulumi.String("cuda"),
				"version":    pulumi.String("13.0.1-base-ubi9"),
			},
		},
		"validator": pulumi.Map{
			"repository": pulumi.String(nv),
			"image":      pulumi.String("gpu-operator"),
			"version":    pulumi.String("v25.10.1"),
		},
		"driver": pulumi.Map{
			"enabled": pulumi.Bool(false),
		},
		// driver.enabled = true: NVIDIA drivers are NOT pre-installed by the OS
		//"driver": pulumi.Map{
		//	"enabled":    pulumi.Bool(true),
		//	"repository": pulumi.String(nv),
		//	"image":      pulumi.String("driver"),
		//	"version":    pulumi.String("580.105.08-ubuntu24.04"),
		//	"manager": pulumi.Map{
		//			"repository": pulumi.String(nvCN),
		//			"image":      pulumi.String("k8s-driver-manager"),
		//			"version":    pulumi.String("v0.9.1"),
		//},
		"toolkit": pulumi.Map{
			"enabled":    pulumi.Bool(true),
			"repository": pulumi.String(nvK8s),
			"image":      pulumi.String("container-toolkit"),
			"version":    pulumi.String("v1.18.1"),
			// RKE2 uses a non-standard containerd socket and config path.
			// Without these, the toolkit fails to signal containerd on install.
			"env": pulumi.Array{
				pulumi.Map{"name": pulumi.String("CONTAINERD_CONFIG"), "value": pulumi.String("/var/lib/rancher/rke2/agent/etc/containerd/config.toml")},
				pulumi.Map{"name": pulumi.String("CONTAINERD_SOCKET"), "value": pulumi.String("/run/k3s/containerd/containerd.sock")},
				pulumi.Map{"name": pulumi.String("CONTAINERD_RUNTIME_CLASS"), "value": pulumi.String("nvidia")},
				pulumi.Map{"name": pulumi.String("CONTAINERD_SET_AS_DEFAULT"), "value": pulumi.String("true")},
			},
		},
		"devicePlugin": pulumi.Map{
			"enabled":    pulumi.Bool(true),
			"repository": pulumi.String(nv),
			"image":      pulumi.String("k8s-device-plugin"),
			"version":    pulumi.String("v0.18.1"),
		},
		"gfd": pulumi.Map{
			"enabled":    pulumi.Bool(true),
			"repository": pulumi.String(nv),
			"image":      pulumi.String("k8s-device-plugin"),
			"version":    pulumi.String("v0.18.1"),
		},
		"dcgmExporter": pulumi.Map{
			"enabled":    pulumi.Bool(true),
			"repository": pulumi.String(nvK8s),
			"image":      pulumi.String("dcgm-exporter"),
			"version":    pulumi.String("4.4.2-4.7.0-distroless"),
		},
		"dcgm": pulumi.Map{
			"enabled":    pulumi.Bool(true),
			"repository": pulumi.String(nvCN),
			"image":      pulumi.String("dcgm"),
			"version":    pulumi.String("4.4.2-1-ubuntu22.04"),
		},
		"migManager": pulumi.Map{
			"enabled":    pulumi.Bool(true),
			"repository": pulumi.String(nvCN),
			"image":      pulumi.String("k8s-mig-manager"),
			"version":    pulumi.String("v0.13.1"),
		},
		"node-feature-discovery": pulumi.Map{
			"image": pulumi.Map{
				"repository": pulumi.String(nfd),
				"tag":        pulumi.String("v0.18.2"),
				"pullPolicy": pulumi.String("IfNotPresent"),
			},
			"worker": pulumi.Map{
				"tolerations": pulumi.Array{
					pulumi.Map{
						"key":      pulumi.String("dedicated"),
						"operator": pulumi.String("Exists"),
						"effect":   pulumi.String("NoSchedule"),
					},
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
