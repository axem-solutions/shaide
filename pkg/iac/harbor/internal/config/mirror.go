package config

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type SyncMode string

const (
	SyncModeAll        SyncMode = "all"
	SyncModeMinVersion SyncMode = "min-version"
	SyncModePinned     SyncMode = "pinned"
)

type Mirror struct {
	Enabled bool

	PublicImages string

	GHCR GHCR
}

type GHCR struct {
	Org   string
	User  string
	Token pulumi.StringOutput

	SyncMode     SyncMode
	MinVersions  string
	PinnedImages string
}

func (m SyncMode) Validate() error {
	switch m {
	case SyncModeAll, SyncModeMinVersion, SyncModePinned:
		return nil
	default:
		return fmt.Errorf(
			"invalid harbor:ghcrSyncMode %q: expected %q, %q, or %q",
			m,
			SyncModeAll,
			SyncModeMinVersion,
			SyncModePinned,
		)
	}
}

func (m Mirror) Validate(robotConfigured bool) error {
	if err := m.GHCR.SyncMode.Validate(); err != nil {
		return err
	}

	if m.Enabled && !robotConfigured {
		return fmt.Errorf("harbor:robotPassword is required when harbor:mirrorEnabled=true")
	}

	return nil
}
