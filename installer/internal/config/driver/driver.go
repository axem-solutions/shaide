package driver

import (
	"fmt"
	"strings"
)

const (
	GPUNodeLabel = "nvidia.com/gpu.present"

	CUDADriverMajorLabel = "nvidia.com/cuda.driver.major"
	CUDADriverMinorLabel = "nvidia.com/cuda.driver.minor"
	CUDADriverRevLabel   = "nvidia.com/cuda.driver.rev"

	CUDARuntimeMajorLabel = "nvidia.com/cuda.runtime.major"
	CUDARuntimeMinorLabel = "nvidia.com/cuda.runtime.minor"

	CUDAVersionEnv       = "CUDA_VERSION"
	NVIDIARequireCUDAEnv = "NVIDIA_REQUIRE_CUDA"

	cudaRequirementPrefix = "cuda>="
	cudaImageName         = "llm-d/llm-d-cuda"
)

type GPUNodeCapability struct {
	NodeName           string
	DriverVersion      Version
	CUDARuntimeVersion Version
}

type CudaImageCapability struct {
	RequiredCUDA Version
	CUDAVersion  Version
}

func GPUNodeSelector() string {
	return GPUNodeLabel + "=true"
}

func RequiredNodeLabels() []string {
	return []string{
		CUDADriverMajorLabel,
		CUDADriverMinorLabel,
		CUDADriverRevLabel,
		CUDARuntimeMajorLabel,
		CUDARuntimeMinorLabel,
	}
}

func ImageEnvironmentVariables() []string {
	return []string{
		CUDAVersionEnv,
		NVIDIARequireCUDAEnv,
	}
}

func NewGPUNodeCapability(nodeName string, labels map[string]string) (GPUNodeCapability, error) {
	driverVersion, err := ParseVersionParts(labels[CUDADriverMajorLabel], labels[CUDADriverMinorLabel], labels[CUDADriverRevLabel])
	if err != nil {
		return GPUNodeCapability{}, fmt.Errorf("parse NVIDIA driver version for node %q: %w", nodeName, err)
	}

	cudaRuntimeVersion, err := ParseVersionParts(labels[CUDARuntimeMajorLabel], labels[CUDARuntimeMinorLabel], "")
	if err != nil {
		return GPUNodeCapability{}, fmt.Errorf("parse CUDA runtime version for node %q: %w", nodeName, err)
	}

	return GPUNodeCapability{
		NodeName:           nodeName,
		DriverVersion:      driverVersion,
		CUDARuntimeVersion: cudaRuntimeVersion,
	}, nil
}

// RequiredCUDAVersion returns the minimum CUDA capability required by the
// container image.
//
// NVIDIA_REQUIRE_CUDA is preferred because it describes the host driver
// requirement. CUDA_VERSION is used as a fallback.
func CudaImageCapabilities(envVars map[string]string) (CudaImageCapability, error) {
	var capability CudaImageCapability

	if value, ok := envVars[NVIDIARequireCUDAEnv]; ok {
		version, err := parseNVIDIARequireCUDA(value)
		if err != nil {
			return CudaImageCapability{}, fmt.Errorf("parse %s: %w", NVIDIARequireCUDAEnv, err)
		}
		capability.RequiredCUDA = version
	}

	if value, ok := envVars[CUDAVersionEnv]; ok {
		version, err := ParseVersion(value)
		if err != nil {
			return CudaImageCapability{}, fmt.Errorf("parse %s value %q: %w", CUDAVersionEnv, value, err)
		}

		// GPU Operator runtime labels contain only major and minor.
		// Ignore the CUDA Toolkit patch version for driver compatibility.
		version.Revision = 0

		capability.CUDAVersion = version
	}

	return capability, nil
}

// Compatible reports whether the node's NVIDIA driver supports the CUDA
// capability required by the image.
func (c GPUNodeCapability) Compatible(requiredCUDA Version) bool {
	return c.CUDARuntimeVersion.AtLeast(requiredCUDA)
}

func parseNVIDIARequireCUDA(requirement string) (Version, error) {
	for field := range strings.FieldsSeq(requirement) {
		value, found := strings.CutPrefix(field, cudaRequirementPrefix)
		if !found {
			continue
		}

		value = strings.Trim(value, ",;")

		version, err := ParseVersion(value)
		if err != nil {
			return Version{}, fmt.Errorf("parse CUDA requirement %q: %w", field, err)
		}

		return version, nil
	}

	return Version{}, fmt.Errorf("requirement %q does not contain %s<version>", requirement, cudaRequirementPrefix)
}
