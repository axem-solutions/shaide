package stacks

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	stackpkg "github.com/axem-solutions/ai_platform/pkg/stack"
)

// Names the image manifest uses for the components app-shaide deploys. The
// stack needs a fully qualified reference for each, and the installer is the
// only party that knows where they were mirrored to.
const (
	shaideServerImageName = "axem-solutions/shaide_server"
	controlPanelImageName = "axem-solutions/control_panel"
	webappImageName       = "axem-solutions/shaide-webapp"
	rustfsImageName       = "rustfs/rustfs"
	qdrantImageName       = "qdrant/qdrant"
	busyboxImageName      = "busybox"
)

func DeployAppShaide(rt *core.Runtime) error {
	workDir := filepath.Join(rt.Bootstrap.Config.Paths.ProjectsDir, projectAppShaide)

	options, err := shaideOptions(rt)
	if err != nil {
		return err
	}

	shaideStack := shaide.NewStack(
		workDir,
		stackpkg.Options{
			Platform:   platform.Platform(rt.Bootstrap.CloudPlatform),
			Kubeconfig: rt.Cluster.ConfigPath,
			Context:    rt.Cluster.SelectedContext,
		},
		options,
	)

	deployer, err := newStackDeployer(rt, shaideStack, stackDeploymentOptions{})
	if err != nil {
		return err
	}

	if _, err := deployer.Deploy(context.Background()); err != nil {
		return err
	}

	return nil
}

// shaideOptions resolves the values the installer knows: where the images were
// mirrored, and the credential that can pull them from there.
func shaideOptions(rt *core.Runtime) (shaide.Options, error) {
	registry := harborRegistryHostname(rt)

	images := map[string]string{}
	for _, name := range []string{
		shaideServerImageName,
		controlPanelImageName,
		webappImageName,
		rustfsImageName,
		qdrantImageName,
		busyboxImageName,
	} {
		reference, err := mirroredImageRef(rt.Bootstrap.Catalog.ServiceImages, registry, name)
		if err != nil {
			return shaide.Options{}, err
		}
		images[name] = reference
	}

	// The pull secret authenticates against the registry the references point
	// at, so the Harbor robot credential travels with them.
	return shaide.Options{
		HarborHostname:  registry,
		RegistryUser:    rt.Discovery.Auth.Username,
		RegistryToken:   rt.Discovery.Auth.Password,
		GatewayHostname: rt.Bootstrap.GatewayHostname,

		ShaideServerImage: images[shaideServerImageName],
		ControlPanelImage: images[controlPanelImageName],
		WebappImage:       images[webappImageName],
		RustfsImage:       images[rustfsImageName],
		QdrantImage:       images[qdrantImageName],
		BusyboxImage:      images[busyboxImageName],
	}, nil
}

// harborRegistryHostname is the in-cluster address of the registry the images
// were mirrored into. Pods resolve it through cluster DNS, unlike the
// port-forward the installer itself uses.
func harborRegistryHostname(rt *core.Runtime) string {
	return fmt.Sprintf(
		"%s.%s.svc.cluster.local",
		rt.Bootstrap.Config.Harbor.Service,
		rt.Bootstrap.Config.Harbor.Namespace,
	)
}

// mirroredImageRef builds the reference the cluster pulls, from the manifest
// entry that says where the installer put it.
func mirroredImageRef(images []catalog.Image, registry, name string) (string, error) {
	for _, image := range images {
		if image.Name != name {
			continue
		}

		return fmt.Sprintf("%s/%s/%s:%s", registry, image.Project, image.Name, image.Tag), nil
	}

	return "", fmt.Errorf("image manifest has no entry for %q, which app-shaide needs", name)
}
