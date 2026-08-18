package platform

import (
	"fmt"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"

	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const hostpathBase = "/var/lib/hostpath/models"

const orasVersion = "v1.3.1"

// CreateModelStorage creates a PVC and a one-time ORAS pull Job that populates
// it from Harbor. The Job:
//   - runs with workingDir /model-cache/hub so ORAS extracts layers into
//     /model-cache/hub/models--org--name/... matching HF_HUB_CACHE=/model-cache/hub
//     set by the pvc+hf:// URI (e.g. pvc+hf://<pvc>/hub/org/model)
//   - writes /model-cache/.pulled on success (idempotent: skips if marker exists)
//   - mounts harbor-creds secret as /root/.docker/config.json for ORAS authentication
//   - backoffLimit: 3, ttlSecondsAfterFinished: 86400 (auto-cleaned after 24h)
//
// The caller must DependsOn the returned Job before starting the ModelService pod
// to prevent the pod from starting with an empty PVC under HF_HUB_OFFLINE=1.
//
// On partial failure (no marker written): delete the Job and PVC manually, then
// re-run pulumi up to start fresh.
func CreateModelStorage(ctx *pulumi.Context, model appConfig.Model, llmdNamespace *corev1.Namespace, opts ...pulumi.ResourceOption) (*corev1.PersistentVolumeClaim, pulumi.Resource, error) {
	src := model.ModelSource
	pvcName := model.Slug + "-model"
	pvcOpts := append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{llmdNamespace})}, opts...)

	// On on-prem with hostpath storage, create a PV bound to the target node before the PVC.
	// The PV uses a local path under hostpathBase/<slug> on the node specified by HostpathNode.
	// The directory must exist on the node before pulumi up (managed via ansible hostpath_dirs role).
	if model.CloudProvider == "on-prem" && model.ModelSource != nil && model.ModelSource.HostpathNode != "" {
		pvName := pvcName + "-pv"
		hostpathDir := model.ModelSource.HostpathDir
		if hostpathDir == "" {
			hostpathDir = hostpathBase + "/" + model.Slug
		}
		_, err := corev1.NewPersistentVolume(ctx, pvName, &corev1.PersistentVolumeArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(pvName),
			},
			Spec: &corev1.PersistentVolumeSpecArgs{
				Capacity:                      pulumi.StringMap{"storage": pulumi.String(src.StorageSize)},
				AccessModes:                   pulumi.StringArray{pulumi.String("ReadWriteOnce")},
				PersistentVolumeReclaimPolicy: pulumi.String("Retain"),
				StorageClassName:              pulumi.String("hostpath"),
				Local: &corev1.LocalVolumeSourceArgs{
					Path: pulumi.String(hostpathDir),
				},
				NodeAffinity: &corev1.VolumeNodeAffinityArgs{
					Required: &corev1.NodeSelectorArgs{
						NodeSelectorTerms: corev1.NodeSelectorTermArray{
							&corev1.NodeSelectorTermArgs{
								MatchExpressions: corev1.NodeSelectorRequirementArray{
									&corev1.NodeSelectorRequirementArgs{
										Key:      pulumi.String("kubernetes.io/hostname"),
										Operator: pulumi.String("In"),
										Values:   pulumi.StringArray{pulumi.String(model.ModelSource.HostpathNode)},
									},
								},
							},
						},
					},
				},
			},
		}, opts...)
		if err != nil {
			return nil, nil, fmt.Errorf("create hostpath PV %q: %w", pvName, err)
		}
	}

	pvcSpec := &corev1.PersistentVolumeClaimSpecArgs{
		AccessModes: pulumi.StringArray{pulumi.String("ReadWriteOnce")},
		Resources: &corev1.VolumeResourceRequirementsArgs{
			Requests: pulumi.StringMap{"storage": pulumi.String(src.StorageSize)},
		},
	}
	// On-prem RKE2 clusters use hostpath (kubernetes.io/no-provisioner) — static PVs only.
	// A matching PV must be pre-created before pulumi up (see infra/on-prem/ansible/inventory-dev/host_vars/).
	// On cloud (GKE) no class is specified, using the cluster default (standard-rwo or equivalent).
	if model.CloudProvider == "on-prem" {
		pvcSpec.StorageClassName = pulumi.StringPtr("hostpath")
	} else if model.ModelSource.StorageClass != "" {
		pvcSpec.StorageClassName = pulumi.StringPtr(model.ModelSource.StorageClass)
	}

	pvc, err := corev1.NewPersistentVolumeClaim(ctx, pvcName, &corev1.PersistentVolumeClaimArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(pvcName),
			Namespace: pulumi.String(model.Namespace),
		},
		Spec: pvcSpec,
	}, pvcOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create model PVC %q: %w", pvcName, err)
	}

	// On air-gapped on-prem the ORAS image is pre-loaded into the "services" Harbor project.
	// On cloud (GKE has internet) it is pulled directly from ghcr.io.
	orasImage := "ghcr.io/oras-project/oras:" + orasVersion
	if model.CloudProvider == "on-prem" {
		orasImage = model.HarborHostname + "/images-infra/oras-project/oras:" + orasVersion
	}

	// --plain-http is required: Harbor is deployed with TLS disabled (ClusterIP HTTP).
	// ORAS v1.x defaults to HTTPS; without this flag the pull fails with a TLS error.
	//
	// ORAS extracts artifact layers relative to its working directory. The pull Job
	// uses WorkingDir /model-cache/hub so layers land at:
	//   /model-cache/hub/models--org--name/{blobs,snapshots,refs}/...
	// This matches HF_HUB_CACHE=/model-cache/hub set by the pvc+hf:// URI scheme
	// (e.g. pvc+hf://<pvc>/hub/org/model). The idempotency marker is kept at
	// /model-cache/.pulled (outside hub/) so it is written exactly once and is not
	// affected by what ORAS extracts inside hub/.
	// Harbor artifacts must include refs/main (created by model-upload.sh before push).
	pullCmd := fmt.Sprintf(
		`[ -f /model-cache/.pulled ] && echo "already pulled, skipping" && exit 0; `+
			`mkdir -p /model-cache/hub && `+
			`oras pull --plain-http %s && `+
			`touch /model-cache/.pulled`,
		src.HarborRef,
	)

	backoffLimit := 3
	ttlSeconds := 86400

	jobName := model.Slug + "-model-pull"
	jobOpts := append([]pulumi.ResourceOption{pulumi.DependsOn([]pulumi.Resource{pvc})}, opts...)
	job, err := batchv1.NewJob(ctx, jobName, &batchv1.JobArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(jobName),
			Namespace: pulumi.String(model.Namespace),
		},
		Spec: &batchv1.JobSpecArgs{
			BackoffLimit:            pulumi.IntPtr(backoffLimit),
			TtlSecondsAfterFinished: pulumi.IntPtr(ttlSeconds),
			Template: &corev1.PodTemplateSpecArgs{
				Spec: &corev1.PodSpecArgs{
					RestartPolicy: pulumi.String("OnFailure"),
					// Run the pull job on the same node class as the model service so
					// the PVC is provisioned in the correct zone. Zonal PDs are zone-locked:
					// if the pull job runs in a different zone than the GPU node, the model
					// service pod cannot be scheduled because it cannot access the PV.
					Affinity: model.NodeAffinityArgs(),
					// The pull job must land on the same node as the model service pod so the
					// hostpath PV is accessible. If the GPU node carries a taint, tolerate it.
					Tolerations: buildTolerations(model),
					ImagePullSecrets: corev1.LocalObjectReferenceArray{
						&corev1.LocalObjectReferenceArgs{
							Name: pulumi.String("harbor-creds"),
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("harbor-creds"),
							Secret: &corev1.SecretVolumeSourceArgs{
								SecretName: pulumi.String("harbor-creds"),
								Items: corev1.KeyToPathArray{
									&corev1.KeyToPathArgs{
										Key:  pulumi.String(".dockerconfigjson"),
										Path: pulumi.String("config.json"),
									},
								},
							},
						},
						&corev1.VolumeArgs{
							Name: pulumi.String("model-storage"),
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSourceArgs{
								ClaimName: pulumi.String(pvcName),
							},
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:       pulumi.String("oras-pull"),
							Image:      pulumi.String(orasImage),
							WorkingDir: pulumi.String("/model-cache/hub"),
							Command: pulumi.StringArray{
								pulumi.String("sh"),
								pulumi.String("-c"),
								pulumi.String(pullCmd),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("500m"),
									"memory": pulumi.String("4Gi"),
								},
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("1"),
									"memory": pulumi.String("16Gi"),
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("harbor-creds"),
									MountPath: pulumi.String("/root/.docker"),
									ReadOnly:  pulumi.Bool(true),
								},
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("model-storage"),
									MountPath: pulumi.String("/model-cache"),
								},
							},
						},
					},
				},
			},
		},
	}, jobOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create ORAS pull Job %q: %w", jobName, err)
	}

	return pvc, job, nil
}

func buildTolerations(model appConfig.Model) corev1.TolerationArray {
	t := model.GPUToleration
	if t == nil || *t == (appConfig.Toleration{}) {
		return nil
	}
	return corev1.TolerationArray{
		&corev1.TolerationArgs{
			Key:      pulumi.String(t.Key),
			Operator: pulumi.String(t.Operator),
			Value:    pulumi.String(t.Value),
			Effect:   pulumi.String(t.Effect),
		},
	}
}
