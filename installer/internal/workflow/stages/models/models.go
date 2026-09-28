package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Actions offered per model. A row starts at the "no change" action of its
// status, so an update run that changes nothing is a single confirm.
const (
	ActionNone      = "-"
	ActionInstall   = "install"
	ActionKeep      = "keep"
	ActionUninstall = "uninstall"
)

// modelSlugLabels are the pod labels app-serving puts on a model's workloads.
// axem.ai is the domain clusters deployed before the move to axem.dev carry.
var modelSlugLabels = []string{"axem.dev/model-slug", "axem.ai/model-slug"}

func Stage() core.Stage {
	return core.Stage{
		Name: "select models",
		Steps: []core.Step{
			{
				Name: "report skipped models",
				When: hasSkippedModels,
				Run:  reportSkippedModels,
			},
			{
				Name: "detect installed models",
				Run:  detectInstalledModels,
			},
			{
				Name: "select models",
				Run:  selectModels,
			},
		},
	}
}

func hasSkippedModels(rt *core.Runtime) bool {
	return len(rt.Bootstrap.Catalog.SkippedModels) > 0
}

// reportSkippedModels names the packaged model directories left out of the
// selection, so a model missing from the list is explained in the log.
func reportSkippedModels(rt *core.Runtime) error {
	for _, skipped := range rt.Bootstrap.Catalog.SkippedModels {
		rt.Detailf("not offering %s: %s", skipped.Dir, skipped.Reason)
	}

	return nil
}

// detectInstalledModels finds which catalog models already run on the cluster,
// from the slug label app-serving puts on each model's pods.
func detectInstalledModels(rt *core.Runtime) error {
	slugs, err := servedSlugs(context.Background(), rt.Cluster.Client)
	if err != nil {
		return err
	}

	rt.Models.Installed = map[string]bool{}
	for _, model := range rt.Bootstrap.Catalog.Models {
		if slugs[model.Slug] {
			rt.Models.Installed[model.Slug] = true
			rt.Detailf("installed: %s", model.Name)
		}
		delete(slugs, model.Slug)
	}

	for slug := range slugs {
		rt.Detailf("model %q runs on the cluster but is not a supported model of this installer; it is left out of the selection", slug)
	}

	return nil
}

func servedSlugs(ctx context.Context, client kubernetes.Interface) (map[string]bool, error) {
	slugs := map[string]bool{}

	for _, label := range modelSlugLabels {
		pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			LabelSelector: label,
		})
		if err != nil {
			return nil, fmt.Errorf("list model pods: %w", err)
		}

		for _, pod := range pods.Items {
			if slug := pod.Labels[label]; slug != "" {
				slugs[slug] = true
			}
		}
	}

	return slugs, nil
}

func selectModels(rt *core.Runtime) error {
	models := rt.Bootstrap.Catalog.Models
	if len(models) == 0 {
		rt.Detailf("the installer offers no supported models")
		return nil
	}

	actions, err := rt.Reporter.Choose(choicePrompt(models, rt.Models.Installed))
	if err != nil {
		return err
	}

	if len(actions) != len(models) {
		return fmt.Errorf("model selection returned %d actions for %d models", len(actions), len(models))
	}

	rt.Models.Serve, rt.Models.Uninstall = applyActions(models, actions)

	for _, model := range rt.Models.Serve {
		rt.Detailf("serve: %s", model.Name)
	}
	for _, model := range rt.Models.Uninstall {
		rt.Detailf("uninstall: %s", model.Name)
	}
	if len(rt.Models.Serve) == 0 {
		rt.Detailf("no models will be served")
	}

	return nil
}

func choicePrompt(models []catalog.Model, installed map[string]bool) core.ChoicePrompt {
	rows := make([]core.ChoiceRow, 0, len(models))

	for _, model := range models {
		status := "Available"
		options := []string{ActionNone, ActionInstall}
		if installed[model.Slug] {
			status = "Installed"
			options = []string{ActionKeep, ActionUninstall}
		}

		rows = append(rows, core.ChoiceRow{
			Cells: []string{
				model.Name,
				categoryLabel(model.Category),
				fmt.Sprint(model.GPUsPerPod),
				model.StorageSize,
				status,
			},
			Detail:  modelDetail(model),
			Options: options,
			Current: options[0],
		})
	}

	return core.ChoicePrompt{
		Title:   "Select models",
		Columns: []string{"Model", "Type", "GPU", "Size", "Status"},
		Rows:    rows,
	}
}

// applyActions splits the catalog by the action chosen for each model.
func applyActions(models []catalog.Model, actions []string) (serve, uninstall []catalog.Model) {
	for i, model := range models {
		switch actions[i] {
		case ActionInstall, ActionKeep:
			serve = append(serve, model)
		case ActionUninstall:
			uninstall = append(uninstall, model)
		}
	}

	return serve, uninstall
}

func categoryLabel(category string) string {
	switch category {
	case catalog.CategoryGenerative:
		return "gen"
	case catalog.CategoryEmbedder:
		return "emb"
	default:
		return category
	}
}

func modelDetail(model catalog.Model) string {
	replicas := model.Replicas
	if replicas == 0 {
		replicas = 1
	}

	lines := []string{
		fmt.Sprintf("%s @ %s", model.ID, model.HarborTag),
		fmt.Sprintf("%d GPU per pod, %d %s", model.GPUsPerPod, replicas, plural(replicas, "replica", "replicas")),
	}

	return strings.Join(lines, "\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}
