package config

import "fmt"

type Platform string

const (
	PlatformOnprem Platform = "onprem"
	PlatformGCP    Platform = "gcp"
	PlatformAWS    Platform = "aws"
	PlatformAzure  Platform = "azure"
)

func (p Platform) Validate() error {
	switch p {
	case PlatformOnprem, PlatformGCP, PlatformAWS, PlatformAzure:
		return nil
	default:
		return fmt.Errorf(
			"invalid harbor:platform %q: expected %q, %q, %q, or %q",
			p,
			PlatformOnprem,
			PlatformGCP,
			PlatformAWS,
			PlatformAzure,
		)
	}
}

func (p Platform) DefaultStorageMode() (StorageMode, error) {
	switch p {
	case PlatformOnprem:
		return StorageModeHostPath, nil

	case PlatformGCP, PlatformAWS, PlatformAzure:
		return StorageModeDynamic, nil

	default:
		return "", fmt.Errorf("unsupported Harbor platform %q", p)
	}
}
