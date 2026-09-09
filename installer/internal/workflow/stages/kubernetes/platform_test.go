package kubernetes

import (
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
)

// arm64 is detected correctly but not installable yet, so the run must stop
// with a message that says so rather than mirroring images that cannot run.
func TestIsSupportedPlatform(t *testing.T) {
	tests := []struct {
		platform cluster.Platform
		want     bool
	}{
		{cluster.Platform{OS: "linux", Arch: "amd64"}, true},
		{cluster.Platform{OS: "linux", Arch: "arm64"}, false},
		{cluster.Platform{OS: "linux", Arch: "s390x"}, false},
		{cluster.Platform{OS: "windows", Arch: "amd64"}, false},
		{cluster.Platform{}, false},
	}

	for _, test := range tests {
		t.Run(test.platform.String(), func(t *testing.T) {
			if got := isSupportedPlatform(test.platform); got != test.want {
				t.Errorf("isSupportedPlatform(%s) = %v, want %v", test.platform, got, test.want)
			}
		})
	}
}

func TestJoinSupportedPlatforms(t *testing.T) {
	if got := joinSupportedPlatforms(); !strings.Contains(got, "linux/amd64") {
		t.Errorf("joinSupportedPlatforms() = %q, want it to name linux/amd64", got)
	}
}

// checkDriverCompatibility inspects the CUDA image for the platform the GPU
// nodes run. Reaching it before detection would silently compare the wrong
// variant, so it refuses rather than inspecting a zero platform.
func TestCheckDriverCompatibilityRequiresDetection(t *testing.T) {
	rt := &core.Runtime{
		GlobalState:      core.NewGlobalState(),
		ActiveStageState: core.NewActiveStageState(),
	}

	err := checkDriverCompatibility(rt)
	if err == nil {
		t.Fatal("checkDriverCompatibility() succeeded without a detected platform")
	}

	for _, want := range []string{"not detected", "detect platform"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
