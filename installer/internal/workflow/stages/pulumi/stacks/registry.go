package stacks

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

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

// mirroredImages maps each upstream image name in the manifest to the
// reference the cluster pulls it from. Stacks pick the images they deploy, so
// the installer does not need to know which those are.
func mirroredImages(images []catalog.Image, registry string) map[string]string {
	refs := make(map[string]string, len(images))
	for _, image := range images {
		refs[image.Name] = fmt.Sprintf("%s/%s/%s:%s", registry, image.Project, image.Name, image.Tag)
	}

	return refs
}
