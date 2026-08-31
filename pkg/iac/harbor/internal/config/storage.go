package config

import (
	"fmt"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
)

const DefaultHostPathBase = "/var/lib/hostpath/harbor"

type StorageMode string

const (
	StorageModeDynamic  StorageMode = "dynamic"
	StorageModeHostPath StorageMode = "hostpath"
)

func (m StorageMode) String() string {
	return string(m)
}

type Storage struct {
	Mode StorageMode

	// Dynamic provisioning.
	StorageClass string

	// HostPath provisioning.
	HostPathBase string
	NodeHostname string
}

func defaultStorageMode(p platform.Platform) (StorageMode, error) {
	switch p {
	case platform.PlatformOnPrem:
		return StorageModeHostPath, nil

	case platform.PlatformGCP, platform.PlatformAWS, platform.PlatformAzure:
		return StorageModeDynamic, nil

	default:
		return "", fmt.Errorf("unsupported Harbor platform %q", p)
	}
}

func (m StorageMode) Validate() error {
	switch m {
	case StorageModeDynamic, StorageModeHostPath:
		return nil
	default:
		return fmt.Errorf("invalid harbor:storageMode %q: expected %q or %q", m, StorageModeDynamic, StorageModeHostPath)
	}
}

func (s Storage) Validate() error {
	if err := s.Mode.Validate(); err != nil {
		return err
	}

	switch s.Mode {
	case StorageModeHostPath:
		if s.NodeHostname == "" {
			return fmt.Errorf("harbor:nodeHostname is required when harbor:storageMode=%q", StorageModeHostPath)
		}

		if s.StorageClass != "" {
			return fmt.Errorf("harbor:storageClass cannot be used when harbor:storageMode=%q", StorageModeHostPath)
		}

	case StorageModeDynamic:
		if s.HostPathBase != "" {
			return fmt.Errorf("harbor:hostPathBase cannot be used when harbor:storageMode=%q", StorageModeDynamic)
		}
	}

	return nil
}
