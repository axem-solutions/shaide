package storage

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/iac/harbor/internal/config"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Result struct {
	Dependencies []pulumi.Resource
}

func Prepare(ctx *pulumi.Context, provider *kubernetes.Provider, namespace *corev1.Namespace, cfg config.Storage) (Result, error) {
	switch cfg.Mode {
	case config.StorageModeDynamic:
		return prepareDynamic()

	case config.StorageModeHostPath:
		return prepareHostPath(ctx, provider, namespace, cfg)

	default:
		return Result{}, fmt.Errorf("unsupported Harbor storage mode %q", cfg.Mode)
	}
}

func prepareDynamic() (Result, error) {
	// No explicit PersistentVolumes are needed.
	//
	// The Harbor Helm chart creates PVCs and the cluster's StorageClass
	// dynamically provisions the backing storage.
	return Result{}, nil
}

func prepareHostPath(ctx *pulumi.Context, provider *kubernetes.Provider, namespace *corev1.Namespace, cfg config.Storage) (Result, error) {
	pvs, err := createVolumes(ctx, cfg.NodeHostname, cfg.HostPathBase, namespace, provider)

	if err != nil {
		return Result{}, err
	}

	dependencies := make([]pulumi.Resource, 0, len(pvs))

	for _, pv := range pvs {
		dependencies = append(dependencies, pv)
	}

	return Result{
		Dependencies: dependencies,
	}, nil
}
