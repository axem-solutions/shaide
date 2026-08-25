package cloudprovider

import (
	"fmt"

	appconfig "github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide/internal/runtime"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// OnPremProvider deploys on-premises-specific resources for the shaide stack.
//
// Storage: the hostpath StorageClass (kubernetes.io/no-provisioner) requires
// static PersistentVolumes to be pre-created. ProvisionStorage creates one PV
// per stateful component, each pinned to pvNodeHostname via nodeAffinity and
// pre-bound via claimRef for deterministic PVC assignment.
//
// Load balancing: handled by MetalLB via the metallb.universe.tf/address-pool
// annotation in lbAnnotations. No additional resources are needed post-deploy.
type OnPremProvider struct{}

// pvDef describes a single PV to create.
type pvDef struct {
	// resourceName is the Pulumi resource logical name.
	resourceName string
	// pvName is the Kubernetes PersistentVolume object name.
	pvName string
	// claimName is the PVC name the PV is pre-bound to.
	// StatefulSet PVC name pattern: <template-name>-<sts-name>-<ordinal>
	claimName string
	// hostPath is the directory on the node backing this PV.
	hostPath string
	// size is the storage capacity (must be >= the PVC request).
	size string
}

// hostPathStorageClass is the name of the no-provisioner StorageClass created by
// the k8s-onprem-airgap-infra stack. Static PVs are only required for this class;
// dynamic provisioners (Longhorn, NFS, Ceph, etc.) create PVs automatically.
const hostPathStorageClass = "hostpath"

// ProvisionStorage creates one hostPath PersistentVolume per stateful component,
// but only when cfg.StorageClassName == "hostpath" (kubernetes.io/no-provisioner).
// For any other StorageClass the provisioner handles PV creation dynamically.
// PVs are cluster-scoped, pinned to cfg.PVNodeHostname via nodeAffinity, and
// pre-bound via claimRef for deterministic PVC assignment.
// The returned resources are wired into StorageDeps so StatefulSets wait for them.
func (p *OnPremProvider) ProvisionStorage(ctx *pulumi.Context, deps *runtime.DeploymentContext, cfg appconfig.Config) ([]pulumi.Resource, error) {
	if cfg.StorageClassName != hostPathStorageClass {
		return nil, nil
	}
	if cfg.PVNodeHostname == "" {
		return nil, fmt.Errorf("pvNodeHostname must be set when storageClassName is %q", hostPathStorageClass)
	}
	pvs := []pvDef{
		{
			resourceName: "shaide-server-pv",
			pvName:       "shaide-server-data",
			claimName:    "shaide-server-data-shaide-server-0",
			hostPath:     "/var/lib/app-shaide/shaide-server-data",
			size:         cfg.ShaidePVSize,
		},
		{
			resourceName: "rustfs-pv",
			pvName:       "rustfs-data",
			claimName:    "rustfs-data-rustfs-0",
			hostPath:     "/var/lib/app-shaide/rustfs-data",
			size:         cfg.RustfsPVSize,
		},
		{
			resourceName: "qdrant-pv",
			pvName:       "qdrant-data",
			claimName:    "qdrant-data-qdrant-0",
			hostPath:     "/var/lib/app-shaide/qdrant-data",
			size:         cfg.QdrantPVSize,
		},
	}

	var resources []pulumi.Resource
	for _, pv := range pvs {
		r, err := corev1.NewPersistentVolume(ctx, pv.resourceName, &corev1.PersistentVolumeArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(pv.pvName),
			},
			Spec: &corev1.PersistentVolumeSpecArgs{
				StorageClassName: pulumi.String(cfg.StorageClassName),
				AccessModes:      pulumi.StringArray{pulumi.String("ReadWriteOnce")},
				Capacity: pulumi.StringMap{
					"storage": pulumi.String(pv.size),
				},
				// Pre-bind to a specific PVC for deterministic assignment.
				// Without claimRef the controller may bind the wrong 5Gi PV
				// (rustfs vs qdrant) since both request the same capacity.
				ClaimRef: &corev1.ObjectReferenceArgs{
					Namespace: pulumi.String(cfg.Namespace),
					Name:      pulumi.String(pv.claimName),
				},
				HostPath: &corev1.HostPathVolumeSourceArgs{
					Path: pulumi.String(pv.hostPath),
					Type: pulumi.StringPtr("DirectoryOrCreate"),
				},
				// nodeAffinity is required for WaitForFirstConsumer binding:
				// the scheduler defers PVC binding until a pod is placed, then
				// selects the PV whose nodeAffinity matches the chosen node.
				NodeAffinity: &corev1.VolumeNodeAffinityArgs{
					Required: &corev1.NodeSelectorArgs{
						NodeSelectorTerms: corev1.NodeSelectorTermArray{
							&corev1.NodeSelectorTermArgs{
								MatchExpressions: corev1.NodeSelectorRequirementArray{
									&corev1.NodeSelectorRequirementArgs{
										Key:      pulumi.String("kubernetes.io/hostname"),
										Operator: pulumi.String("In"),
										Values:   pulumi.StringArray{pulumi.String(cfg.PVNodeHostname)},
									},
								},
							},
						},
					},
				},
			},
		}, deps.ProviderOpt)
		if err != nil {
			return nil, err
		}
		resources = append(resources, r)
	}
	return resources, nil
}

// PostDeployService is a no-op for on-prem.
// MetalLB handles load balancing via the service annotation — no additional
// Kubernetes resources are required after the Service is created.
func (p *OnPremProvider) PostDeployService(_ *pulumi.Context, _ *runtime.DeploymentContext, _ string, _ pulumi.Resource) error {
	return nil
}
