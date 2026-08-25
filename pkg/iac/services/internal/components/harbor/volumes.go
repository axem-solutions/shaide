package harbor

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// harborVolume describes one Harbor PersistentVolume backed by a hostPath directory.
type harborVolume struct {
	// component is used to name the PV (harbor-pv-<component>).
	component string
	// pvcName is the exact PVC name created by the Harbor chart.
	// Deployments use <release>-<component>; StatefulSets use
	// <volumeClaimTemplate>-<statefulsetName>-<ordinal>.
	pvcName string
	size    string
	// subDir is the sub-directory under hostpathBase on the node.
	subDir string
}

var harborVolumes = []harborVolume{
	{component: "registry",   pvcName: "harbor-registry",              size: "150Gi", subDir: "registry"},
	{component: "jobservice", pvcName: "harbor-jobservice",            size: "1Gi",  subDir: "jobservice"},
	{component: "database",   pvcName: "database-data-harbor-database-0", size: "2Gi",  subDir: "database"},
	{component: "redis",      pvcName: "data-harbor-redis-0",          size: "1Gi",  subDir: "redis"},
}

// createVolumes creates one hostPath PersistentVolume per Harbor component.
//
// Each PV uses claimRef to bind exclusively to its PVC (harbor-<component>),
// preventing accidental cross-binding with other PVs.
// Node affinity pins every PV to nodeHostname so that Harbor pods always
// schedule on the node that owns the hostPath directories.
func createVolumes(
	ctx *pulumi.Context,
	nodeHostname string,
	hostpathBase string,
	ns *corev1.Namespace,
	provider pulumi.ProviderResource,
) ([]*corev1.PersistentVolume, error) {
	pvs := make([]*corev1.PersistentVolume, 0, len(harborVolumes))

	for _, vol := range harborVolumes {
		pvName := fmt.Sprintf("harbor-pv-%s", vol.component)
		hostPath := fmt.Sprintf("%s/%s", hostpathBase, vol.subDir)

		pv, err := corev1.NewPersistentVolume(ctx, pvName, &corev1.PersistentVolumeArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(pvName),
			},
			Spec: &corev1.PersistentVolumeSpecArgs{
				Capacity: pulumi.StringMap{
					"storage": pulumi.String(vol.size),
				},
				AccessModes:                   pulumi.StringArray{pulumi.String("ReadWriteOnce")},
				PersistentVolumeReclaimPolicy: pulumi.String("Retain"),
				StorageClassName:              pulumi.String("hostpath"),
				// claimRef ensures this PV binds only to its matching PVC.
				ClaimRef: &corev1.ObjectReferenceArgs{
					Namespace: pulumi.String("harbor"),
					Name:      pulumi.String(vol.pvcName),
				},
				NodeAffinity: &corev1.VolumeNodeAffinityArgs{
					Required: &corev1.NodeSelectorArgs{
						NodeSelectorTerms: corev1.NodeSelectorTermArray{
							&corev1.NodeSelectorTermArgs{
								MatchExpressions: corev1.NodeSelectorRequirementArray{
									&corev1.NodeSelectorRequirementArgs{
										Key:      pulumi.String("kubernetes.io/hostname"),
										Operator: pulumi.String("In"),
										Values:   pulumi.StringArray{pulumi.String(nodeHostname)},
									},
								},
							},
						},
					},
				},
				HostPath: &corev1.HostPathVolumeSourceArgs{
					Path: pulumi.String(hostPath),
					Type: pulumi.StringPtr("DirectoryOrCreate"),
				},
			},
		}, pulumi.Provider(provider), pulumi.DependsOn([]pulumi.Resource{ns}))
		if err != nil {
			return nil, fmt.Errorf("creating PV %s: %w", pvName, err)
		}

		pvs = append(pvs, pv)
	}

	return pvs, nil
}
