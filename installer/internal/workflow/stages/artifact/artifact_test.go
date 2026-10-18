package artifact

import (
	"io"
	"slices"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
)

// A declined webapp is not deployed, so its image is not uploaded either.
func TestImagesToUploadFollowWebApp(t *testing.T) {
	const server = "axem-solutions/shaide_server"

	for _, tc := range []struct {
		name    string
		enabled bool
		want    []string
	}{
		{"declined", false, []string{server}},
		{"accepted", true, []string{server, shaide.WebAppImage}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRuntime(&fakeReporter{})
			rt.Logger = logger.NewWithWriter(io.Discard)
			rt.Bootstrap.Catalog.ServiceImages = []catalog.Image{{Name: server}, {Name: shaide.WebAppImage}}
			rt.WebApp.Enabled = tc.enabled

			var got []string
			for _, image := range imagesToUpload(rt) {
				got = append(got, image.Name)
			}

			if !slices.Equal(got, tc.want) {
				t.Errorf("imagesToUpload() = %v, want %v", got, tc.want)
			}
		})
	}
}
