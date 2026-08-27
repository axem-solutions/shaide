package kubernetes

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/config/driver"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/inspect"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	pkgkube "github.com/axem-solutions/ai_platform/pkg/kube"
)

const cudaImageName = "llm-d/llm-d-cuda"

func Stage() core.Stage {
	return core.Stage{
		Name: "initK8s",
		Steps: []core.Step{
			{
				Name: "load kubernetes config",
				Run:  loadK8sConfig,
			},
			{
				Name: "select kubernetes context",
				Run:  selectKubernetesContext,
			},
			{
				Name: "build kubernetes client",
				Run:  buildKubernetesClient,
			},
			{
				Name: "detect cluster platform",
				Run:  detectClusterPlatform,
			},
			{
				Name: "check Nvidia driver compatibility",
				Run:  checkDriverCompatibility,
			},
		},
	}
}

// detectClusterPlatform resolves the target platform from the cluster itself so
// that later stages can branch on it. Doing this here — right after the client
// is built — means discovery already knows whether it is talking to a cloud or
// an on-prem cluster, which is what lets it pick the Harbor bootstrap path.
//
// The value is a default, not a verdict: the gateway-provider stage still
// offers its platform Select, pre-filled with what we found here.
func detectClusterPlatform(rt *core.Runtime) error {
	platform, err := kube.DetectPlatform(context.Background(), rt.Cluster.Client)
	if err != nil {
		return err
	}

	rt.Bootstrap.CloudPlatform = platform
	rt.Detailf("detected cluster platform: %q", platform)

	return nil
}

func checkDriverCompatibility(rt *core.Runtime) error {
	cudaImage, err := findImageByName(rt.Bootstrap.Catalog.ServiceImages, cudaImageName)
	if err != nil {
		return fmt.Errorf("find CUDA image: %w", err)
	}

	nodes, err := kube.GetNodeLabels(context.Background(), rt.Cluster.Client, driver.GPUNodeSelector(), driver.RequiredNodeLabels()...)
	if err != nil {
		return fmt.Errorf("read NVIDIA GPU node labels: %w", err)
	}

	envVars, err := inspect.InspectImage(context.Background(), cudaImage, inspect.Platform{OS: "linux", Architecture: runtime.GOARCH}, driver.ImageEnvironmentVariables()...)
	if err != nil {
		return fmt.Errorf("inspect CUDA image: %w", err)
	}

	imageCapability, err := driver.CudaImageCapabilities(envVars)
	if err != nil {
		return fmt.Errorf("determine required CUDA version: %w", err)
	}

	rt.Detailf("Image requirements:")
	rt.Detailf("NVIDIA_REQUIRE_CUDA: %s", imageCapability.RequiredCUDA)
	rt.Detailf("Cuda Version: %s", imageCapability.CUDAVersion)
	rt.Detailf("-----------")

	var incompatibleNodes []string

	for _, node := range nodes {
		capability, err := driver.NewGPUNodeCapability(node.NodeName, node.Labels)
		if err != nil {
			return err
		}

		rt.Detailf("Node %s:", capability.NodeName)
		rt.Detailf("CudaRuntime Version: %s", capability.CUDARuntimeVersion)
		rt.Detailf("Driver Version: %s", capability.DriverVersion)

		if !capability.Compatible(imageCapability.RequiredCUDA) {
			errmsg := fmt.Sprintf("%q supports CUDA %s", capability.NodeName, capability.CUDARuntimeVersion)
			incompatibleNodes = append(incompatibleNodes, errmsg)
		}
	}
	if len(incompatibleNodes) > 0 {
		return fmt.Errorf("GPU nodes incompatible with required CUDA %s or newer: %s", imageCapability.RequiredCUDA, strings.Join(incompatibleNodes, ", "))
	}

	return nil
}

func loadK8sConfig(rt *core.Runtime) error {
	rt.Cluster.ConfigPath = rt.Bootstrap.Config.Paths.Kubeconfig

	return nil
}

func findImageByName(images []catalog.Image, name string) (catalog.Image, error) {
	var matched catalog.Image
	found := false

	for _, image := range images {
		if image.Name != name {
			continue
		}
		if found {
			return catalog.Image{}, fmt.Errorf("image manifest contains multiple entries for image %q", name)
		}
		matched = image
		found = true
	}

	if !found {
		return catalog.Image{}, fmt.Errorf("image manifest does not contain image %q", name)
	}

	return matched, nil
}

func buildKubernetesClient(rt *core.Runtime) error {
	client, cfg, err := pkgkube.NewK8sClient(
		rt.Cluster.ConfigPath,
		rt.Cluster.SelectedContext,
	)
	if err != nil {
		return err
	}

	rt.Cluster.Client = client
	rt.Cluster.RESTConfig = cfg
	rt.Detailf("initialized K8s client for context %q", rt.Cluster.SelectedContext)
	return nil
}

func selectKubernetesContext(rt *core.Runtime) error {
	options, err := kube.LoadContextOptions(rt.Cluster.ConfigPath)
	if err != nil {
		return err
	}

	selected, err := rt.Reporter.Select(
		"Select Kubernetes Context",
		options.Context,
		options.Available,
	)
	if err != nil {
		return err
	}

	rt.Cluster.SelectedContext = selected
	rt.Detailf("selected context: %q", rt.Cluster.SelectedContext)
	return nil
}
