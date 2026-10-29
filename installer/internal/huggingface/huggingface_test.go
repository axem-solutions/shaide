package huggingface

import (
	"slices"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/huggingface/api"
)

// The storage estimate and the progress count only what is downloaded.
func TestPlanDownloadCountsTheSelectedFiles(t *testing.T) {
	plan := planDownload(api.Metadata{
		Siblings: []api.Sibling{
			{Filename: "config.json", Size: 1},
			{Filename: "model-00000-of-00001.safetensors", Size: 100},
			{Filename: "metal/model.bin", Size: 1000},
			{Filename: "original/model.safetensors", Size: 10000},
		},
	})

	if want := []string{"config.json", "model-00000-of-00001.safetensors"}; !slices.Equal(plan.Files, want) {
		t.Errorf("files = %v, want %v", plan.Files, want)
	}
	if plan.Bytes != 101 {
		t.Errorf("bytes = %d, want 101", plan.Bytes)
	}
}
