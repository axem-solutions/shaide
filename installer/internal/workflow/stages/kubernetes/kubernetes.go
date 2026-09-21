package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/config/driver"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/oras/inspect"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
	"github.com/axem-solutions/ai_platform/pkg/kube/connection"
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
				Name: "detect provider",
				Run:  detectProvider,
			},
			{
				Name: "detect platform",
				Run:  detectPlatform,
			},
			// Kept disabled, but note the ordering it depends on: it reads
			// rt.Cluster.Platform, so it must stay after "detect platform".
			// {
			// 	Name: "check Nvidia driver compatibility",
			// 	Run:  checkDriverCompatibility,
			// },
		},
	}
}

// detectProvider resolves the target provider from the cluster itself so
// that later stages can branch on it. Doing this here — right after the client
// is built — means discovery already knows whether it is talking to a cloud or
// an on-prem cluster, which is what lets it pick the Harbor bootstrap path.
//
// The value is a default: the gateway-provider template still
// offers its platform selection, pre-filled with what we found here.
func detectProvider(rt *core.Runtime) error {
	detectedProvider, err := cluster.DetectProvider(context.Background(), rt.Cluster.Client)
	if err != nil {
		return err
	}

	rt.Bootstrap.Provider = string(detectedProvider)
	rt.Detailf("detected provider: %q", detectedProvider)

	return nil
}

// supportedPlatforms lists what shaide can currently be installed onto.
var supportedPlatforms = []cluster.Platform{
	{OS: "linux", Arch: "amd64"},
}

// detectPlatform resolves the platform the target cluster runs, then refuses
// anything shaide cannot install onto.
//
// Later stages mirror images for this platform alone.
func detectPlatform(rt *core.Runtime) error {
	detected, err := cluster.DetectPlatform(context.Background(), rt.Cluster.Client)
	if err != nil {
		return err
	}

	if !isSupportedPlatform(detected) {
		return fmt.Errorf(
			"cluster platform %s is not supported yet; shaide currently supports %s",
			detected,
			joinSupportedPlatforms(),
		)
	}

	rt.Cluster.Platform = detected
	rt.Detailf("detected cluster platform: %q", detected)

	return nil
}

func isSupportedPlatform(detected cluster.Platform) bool {
	for _, supported := range supportedPlatforms {
		if detected == supported {
			return true
		}
	}

	return false
}

func joinSupportedPlatforms() string {
	names := make([]string, 0, len(supportedPlatforms))
	for _, supported := range supportedPlatforms {
		names = append(names, supported.String())
	}

	return strings.Join(names, ", ")
}

func checkDriverCompatibility(rt *core.Runtime) error {
	// Without this the zero platform reaches inspect.Platform.Equal, which
	// matches no manifest, and the run fails with "image does not contain
	// platform /" rather than naming the missing step.
	if !rt.Cluster.Platform.IsValid() {
		return fmt.Errorf("cluster platform is not detected yet; %q must run after %q", "check Nvidia driver compatibility", "detect platform")
	}

	cudaImage, err := findImageByName(rt.Bootstrap.Catalog.ServiceImages, cudaImageName)
	if err != nil {
		return fmt.Errorf("find CUDA image: %w", err)
	}

	nodes, err := kube.GetNodeLabels(context.Background(), rt.Cluster.Client, driver.GPUNodeSelector(), driver.RequiredNodeLabels()...)
	if err != nil {
		return fmt.Errorf("read NVIDIA GPU node labels: %w", err)
	}

	// The image is inspected for the platform the GPU nodes run, not the one
	// the installer was built for. Those differ whenever the installer runs
	// somewhere other than the cluster it is installing onto, and reading an
	// amd64 image's CUDA requirements off an arm64 workstation would compare
	// the wrong variant against the nodes.
	envVars, err := inspect.InspectImage(
		context.Background(),
		cudaImage,
		inspect.Platform{
			OS:           rt.Cluster.Platform.OS,
			Architecture: rt.Cluster.Platform.Arch,
		},
		driver.ImageEnvironmentVariables()...,
	)
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
	client, cfg, err := connection.NewK8sClient(
		connection.Connection{
			KubeconfigPath: rt.Cluster.ConfigPath,
			Context:        rt.Cluster.SelectedContext,
		},
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
