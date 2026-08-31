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
//
// CUDA minor-version compatibility allows an image built with a newer toolkit
// minor (for example CUDA 12.9) to run on a driver whose advertised runtime
// capability has an older minor (for example CUDA 12.8), provided the driver
// meets the minimum for that CUDA major family. Comparing the two CUDA minor
// versions directly therefore rejects compatible nodes.
func (c GPUNodeCapability) Compatible(requiredCUDA Version) bool {
	minimumDriver, known := minimumDriverForCUDAMajor(requiredCUDA.Major)
	if !known {
		// Preserve the conservative version comparison for future or otherwise
		// unknown CUDA major families until their driver floor is defined.
		return c.CUDARuntimeVersion.AtLeast(requiredCUDA)
	}

	return c.DriverVersion.AtLeast(minimumDriver)
}

// minimumDriverForCUDAMajor returns NVIDIA's minimum Linux driver for minor
// version compatibility in the CUDA major family.
//
// Source: https://docs.nvidia.com/deploy/cuda-compatibility/minor-version-compatibility.html
func minimumDriverForCUDAMajor(cudaMajor int) (Version, bool) {
	switch cudaMajor {
	case 11:
		return Version{Major: 450, Minor: 80, Revision: 2}, true
	case 12:
		return Version{Major: 525, Minor: 60, Revision: 13}, true
	case 13:
		return Version{Major: 580, Minor: 65, Revision: 6}, true
	default:
		return Version{}, false
	}
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
