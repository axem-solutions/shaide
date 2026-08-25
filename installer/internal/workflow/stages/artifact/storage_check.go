package artifact

import (
	"context"
	"fmt"

	"github.com/axem-solutions/ai_platform/installer/internal/config/storage"
	"github.com/axem-solutions/ai_platform/installer/internal/huggingface"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

const modelStorageMultiplier = 3

func reportModelStorage(rt *core.Runtime) error {
	models := rt.Artifact.SelectedModels

	var totalRequiredBytes int64
	for _, model := range models {
		estimate, err := huggingface.EstimateStorage(
			context.Background(),
			rt.Bootstrap.Config.HuggingFace.Token,
			[]huggingface.Model{
				huggingFaceModel(model),
			})
		if err != nil {
			return fmt.Errorf("estimate model storage: %w", err)
		}
		modelBytes := estimate.TotalBytes
		requiredBytes := modelBytes * modelStorageMultiplier
		totalRequiredBytes += requiredBytes

		rt.Detailf(
			"model storage estimate: model=%s size=%s factor=%d required=%s",
			model.HarborName,
			storage.FormatBytes(modelBytes),
			modelStorageMultiplier,
			storage.FormatBytes(requiredBytes),
		)
	}

	if len(models) == 0 {
		rt.Detailf("skipping model storage check: no models selected")
		return nil
	}

	mountPath := rt.Bootstrap.Config.Paths.StorageRoot

	stats, err := storage.GetStats(mountPath)
	if err != nil {
		return fmt.Errorf("check available storage at %q: %w", mountPath, err)
	}

	rt.Detailf(
		"model storage report: selected=%d required=%s capacity=%s used=%s available=%s",
		len(models),
		storage.FormatBytes(totalRequiredBytes),
		storage.FormatBytes(stats.Capacity),
		storage.FormatBytes(stats.Used),
		storage.FormatBytes(stats.Available),
	)

	return nil
}
