package artifact

import (
	"io"
	"slices"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/pkg/iac/mcp"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving"
	"github.com/axem-solutions/ai_platform/pkg/iac/shaide"
)

// A declined Knowledge Center is not deployed, so neither the KC nor the
// KC-MCP image is uploaded.
func TestImagesToUploadFollowKnowledgeCenter(t *testing.T) {
	const server = "axem-solutions/shaide_server"

	for _, tc := range []struct {
		name    string
		enabled bool
		want    []string
	}{
		{"declined", false, []string{server}},
		{"accepted", true, []string{server, shaide.KnowledgeCenterImage, mcp.KnowledgeCenterMCPImage}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRuntime(&fakeReporter{})
			rt.Logger = logger.NewWithWriter(io.Discard)
			rt.Bootstrap.Catalog.ServiceImages = []catalog.Image{
				{Name: server},
				{Name: shaide.KnowledgeCenterImage},
				{Name: mcp.KnowledgeCenterMCPImage},
			}
			rt.KnowledgeCenter.Enabled = tc.enabled

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

// The simulator image is mirrored only when a simulated model is served,
// whether newly selected or kept.
func TestImagesToUploadFollowSimulator(t *testing.T) {
	const server = "axem-solutions/shaide_server"
	gpt := catalog.Model{Name: "GPT-OSS-20B"}
	sim := catalog.Model{Name: "Simulator", Simulated: true}

	for _, tc := range []struct {
		name  string
		serve []catalog.Model
		want  []string
	}{
		{"not selected", []catalog.Model{gpt}, []string{server}},
		{"nothing served", nil, []string{server}},
		{"selected", []catalog.Model{gpt, sim}, []string{server, serving.SimulatorImageName}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRuntime(&fakeReporter{})
			rt.Logger = logger.NewWithWriter(io.Discard)
			rt.Bootstrap.Catalog.ServiceImages = []catalog.Image{{Name: server}, {Name: serving.SimulatorImageName}}
			rt.Models.Serve = tc.serve

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
