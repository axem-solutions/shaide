package driver

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major    int
	Minor    int
	Revision int
}

// ParseVersion parses a dotted version string.
//
// Supported examples:
//
//	"580.82.07"
//	"13.0"
//	"13.0.2"
func ParseVersion(value string) (Version, error) {
	value = strings.TrimSpace(value)

	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version %q: expected major.minor or major.minor.revision", value)
	}

	major, err := parseVersionComponent("major", parts[0])
	if err != nil {
		return Version{}, err
	}

	minor, err := parseVersionComponent("minor", parts[1])
	if err != nil {
		return Version{}, err
	}

	revision := 0

	if len(parts) == 3 {
		revision, err = parseVersionComponent("revision", parts[2])
		if err != nil {
			return Version{}, err
		}
	}

	return Version{
		Major:    major,
		Minor:    minor,
		Revision: revision,
	}, nil
}

// ParseVersionParts creates a Version from separate string components.
// An empty revision is interpreted as zero.
func ParseVersionParts(majorValue string, minorValue string, revisionValue string) (Version, error) {
	major, err := parseVersionComponent("major", majorValue)
	if err != nil {
		return Version{}, err
	}

	minor, err := parseVersionComponent("minor", minorValue)
	if err != nil {
		return Version{}, err
	}

	revision := 0

	if revisionValue != "" {
		revision, err = parseVersionComponent("revision", revisionValue)
		if err != nil {
			return Version{}, err
		}
	}

	return Version{
		Major:    major,
		Minor:    minor,
		Revision: revision,
	}, nil
}

// AtLeast reports whether v is greater than or equal to required.
func (v Version) AtLeast(required Version) bool {
	if v.Major != required.Major {
		return v.Major > required.Major
	}

	if v.Minor != required.Minor {
		return v.Minor > required.Minor
	}

	return v.Revision >= required.Revision
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Revision)
}

func parseVersionComponent(name string, value string) (int, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return 0, fmt.Errorf("version %s component is empty", name)
	}

	component, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse version %s component %q: %w", name, value, err)
	}

	if component < 0 {
		return 0, fmt.Errorf("version %s component cannot be negative: %d", name, component)
	}

	return component, nil
}
