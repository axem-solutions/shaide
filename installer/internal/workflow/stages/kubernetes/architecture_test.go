package kubernetes

import (
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
)

// arm64 is detected correctly but not installable yet, so the run must stop
// with a message that says so rather than mirroring images that cannot run.
func TestIsSupportedArchitecture(t *testing.T) {
	tests := []struct {
		architecture platform.Architecture
		want         bool
	}{
		{platform.Architecture{OS: "linux", Arch: "amd64"}, true},
		{platform.Architecture{OS: "linux", Arch: "arm64"}, false},
		{platform.Architecture{OS: "linux", Arch: "s390x"}, false},
		{platform.Architecture{OS: "windows", Arch: "amd64"}, false},
		{platform.Architecture{}, false},
	}

	for _, test := range tests {
		t.Run(test.architecture.String(), func(t *testing.T) {
			if got := isSupportedArchitecture(test.architecture); got != test.want {
				t.Errorf("isSupportedArchitecture(%s) = %v, want %v", test.architecture, got, test.want)
			}
		})
	}
}

func TestJoinSupportedArchitectures(t *testing.T) {
	if got := joinSupportedArchitectures(); !strings.Contains(got, "linux/amd64") {
		t.Errorf("joinSupportedArchitectures() = %q, want it to name linux/amd64", got)
	}
}
