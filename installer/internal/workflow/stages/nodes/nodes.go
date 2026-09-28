package nodes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/placement"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	"github.com/axem-solutions/ai_platform/pkg/kube/cluster"
)

// Options a node can be assigned to, besides one per model to serve.
const (
	OptionCPU        = "cpu"
	OptionUnassigned = "unassigned"

	modelOptionPrefix = "model:"
)

const (
	confirmApply = "Apply"
	confirmBack  = "Back to assignment"
)

type state struct {
	nodes   []kube.NodeInfo
	changes []labelChange
}

// labelChange is the label patch that brings one node in line with its
// assignment.
type labelChange struct {
	Node   string
	Set    []string
	Remove []string
}

func Stage() core.Stage {
	return core.Stage{
		Name:     "assign nodes",
		NewState: func() any { return &state{} },
		Steps: []core.Step{
			{
				Name: "list nodes",
				Run:  listNodes,
			},
			{
				Name: "assign nodes",
				Run:  assignNodes,
			},
			{
				Name: "apply node labels",
				When: hasChanges,
				Run:  applyLabels,
			},
		},
	}
}

func modelOption(model catalog.Model) string {
	return modelOptionPrefix + model.Slug
}

// listNodes lists the nodes shaide's workloads can run on. A node reserved by
// a taint they do not tolerate, such as a control plane or a system pool, is
// left out: assigning it would put a workload where it cannot be scheduled.
func listNodes(rt *core.Runtime) error {
	st := core.StageData[*state](rt)

	all, err := kube.ListNodes(context.Background(), rt.Cluster.Client)
	if err != nil {
		return err
	}

	st.nodes = nil
	for _, node := range all {
		if taint, reserved := node.ExclusiveTaint(placement.GPUTaintKey); reserved {
			rt.Detailf("not listing node %s: reserved by taint %s=%s:%s", node.Name, taint.Key, taint.Value, taint.Effect)
			continue
		}
		st.nodes = append(st.nodes, node)
	}

	if len(st.nodes) == 0 {
		return fmt.Errorf("the cluster has no node shaide's workloads can be scheduled on")
	}

	return nil
}

func assignNodes(rt *core.Runtime) error {
	st := core.StageData[*state](rt)
	models := rt.Models.Serve

	if cluster.Provider(rt.Bootstrap.Provider) != cluster.OnPrem {
		rt.Detailf("node labels set here are lost when a node pool replaces a node; set them on the node pool as well")
	}

	prompt := assignmentPrompt(st.nodes, models)

	for {
		values, err := rt.Reporter.Choose(prompt)
		if err != nil {
			return err
		}
		if len(values) != len(st.nodes) {
			return fmt.Errorf("node assignment returned %d choices for %d nodes", len(values), len(st.nodes))
		}

		changes := labelChanges(st.nodes, models, values)
		if len(changes) == 0 {
			rt.Detailf("node labels are up to date")
			st.changes = nil
			return nil
		}

		for _, change := range changes {
			rt.Detailf("%s", change)
		}

		answer, err := rt.Reporter.Select(
			fmt.Sprintf("Apply the label changes to %d %s? The log lists them.", len(changes), plural(len(changes), "node", "nodes")),
			confirmApply,
			[]string{confirmApply, confirmBack},
		)
		if err != nil {
			return err
		}
		if answer == confirmApply {
			st.changes = changes
			return nil
		}

		// Back to the view with the edits kept.
		for i := range prompt.Rows {
			prompt.Rows[i].Current = values[i]
		}
	}
}

func hasChanges(rt *core.Runtime) bool {
	return len(core.StageData[*state](rt).changes) > 0
}

func applyLabels(rt *core.Runtime) error {
	st := core.StageData[*state](rt)

	for _, change := range st.changes {
		set := make(map[string]string, len(change.Set))
		for _, key := range change.Set {
			set[key] = placement.Value
		}

		if err := kube.PatchNodeLabels(context.Background(), rt.Cluster.Client, change.Node, set, change.Remove); err != nil {
			return err
		}

		rt.Detailf("labelled %s", change)
	}

	return nil
}

func (c labelChange) String() string {
	parts := make([]string, 0, len(c.Set)+len(c.Remove))
	for _, key := range c.Set {
		parts = append(parts, "+"+key)
	}
	for _, key := range c.Remove {
		parts = append(parts, "-"+key)
	}

	return fmt.Sprintf("%s: %s", c.Node, strings.Join(parts, ", "))
}

func assignmentPrompt(nodes []kube.NodeInfo, models []catalog.Model) core.ChoicePrompt {
	groups := make([]core.ChoiceGroup, 0, len(models)+2)
	for _, model := range models {
		groups = append(groups, core.ChoiceGroup{Option: modelOption(model), Title: modelTitle(model)})
	}
	groups = append(groups,
		core.ChoiceGroup{Option: OptionCPU, Title: "CPU only"},
		core.ChoiceGroup{Option: OptionUnassigned, Title: "Unassigned"},
	)

	rows := make([]core.ChoiceRow, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, core.ChoiceRow{
			Cells:   []string{node.Name, gpuCell(node), computeCell(node)},
			Detail:  nodeDetail(node),
			Options: nodeOptions(node, models),
			Current: initialOption(node, models),
		})
	}

	return core.ChoicePrompt{
		Title:       "Assign nodes to workloads",
		Columns:     []string{"Node", "GPU", "CPU/Mem"},
		Rows:        rows,
		Groups:      groups,
		ClearOption: OptionUnassigned,
		Check: func(values []string) core.ChoiceCheck {
			return checkAssignment(nodes, models, values)
		},
	}
}

// fits reports whether one pod of the model fits on the node's GPUs.
func fits(node kube.NodeInfo, model catalog.Model) bool {
	return node.GPUs >= int64(model.GPUsPerPod)
}

// nodeOptions is what a node can be assigned to: the models whose pod fits
// on its GPUs, CPU work, or nothing.
func nodeOptions(node kube.NodeInfo, models []catalog.Model) []string {
	var options []string
	for _, model := range models {
		if fits(node, model) {
			options = append(options, modelOption(model))
		}
	}

	return append(options, OptionCPU, OptionUnassigned)
}

// initialOption is what the node's labels already say, so a rerun that
// changes nothing is a single confirm.
func initialOption(node kube.NodeInfo, models []catalog.Model) string {
	for _, model := range models {
		if node.Labels[placement.ModelLabel(model.Slug)] == placement.Value && fits(node, model) {
			return modelOption(model)
		}
	}

	if node.Labels[placement.CPULabel] == placement.Value {
		return OptionCPU
	}

	// A node pool labelled with a class serves that class; when exactly one
	// model of the class is served, the node is meant for it.
	var candidates []catalog.Model
	for _, model := range models {
		if node.Labels[placement.ClassLabel(model.Category)] == placement.Value && fits(node, model) {
			candidates = append(candidates, model)
		}
	}
	if len(candidates) == 1 {
		return modelOption(candidates[0])
	}

	// First assignment: nodes without a GPU run the platform itself.
	if !hasManagedLabels(node) && node.GPUs == 0 && !node.ControlPlane {
		return OptionCPU
	}

	return OptionUnassigned
}

func hasManagedLabels(node kube.NodeInfo) bool {
	for key := range node.Labels {
		if placement.Managed(key) {
			return true
		}
	}

	return false
}

// checkAssignment validates each pool and reports the header status of every
// group. The first problem blocks the assignment from being submitted.
func checkAssignment(nodes []kube.NodeInfo, models []catalog.Model, values []string) core.ChoiceCheck {
	members := map[string][]kube.NodeInfo{}
	for i, value := range values {
		members[value] = append(members[value], nodes[i])
	}

	check := core.ChoiceCheck{Groups: map[string]core.GroupStatus{}}
	block := func(format string, args ...any) {
		if check.Blocking == "" {
			check.Blocking = fmt.Sprintf(format, args...)
		}
	}

	for _, model := range models {
		option := modelOption(model)
		pool := members[option]

		switch products := gpuProducts(pool); {
		case len(pool) == 0:
			check.Groups[option] = core.GroupStatus{Text: "no nodes"}
			block("Assign at least one node to %s.", model.Name)
		case len(products) > 1:
			check.Groups[option] = core.GroupStatus{Text: "mixed GPUs"}
			block("%s mixes GPU types (%s); a pool can have one GPU type.", model.Name, strings.Join(products, ", "))
		default:
			short := ""
			for _, node := range pool {
				if !fits(node, model) {
					short = node.Name
					break
				}
			}
			if short != "" {
				check.Groups[option] = core.GroupStatus{Text: "too few GPUs"}
				block("%s has too few GPUs for %s, which needs %d per pod.", short, model.Name, model.GPUsPerPod)
				continue
			}

			check.Groups[option] = core.GroupStatus{
				OK:   true,
				Text: fmt.Sprintf("%s ×%d", shortProduct(firstOr(products, "no GPU")), len(pool)),
			}
		}
	}

	if cpu := members[OptionCPU]; len(cpu) == 0 {
		check.Groups[OptionCPU] = core.GroupStatus{Text: "no nodes"}
		block("Assign at least one node to CPU only; the platform itself runs there.")
	} else {
		check.Groups[OptionCPU] = core.GroupStatus{
			OK:   true,
			Text: fmt.Sprintf("%d %s", len(cpu), plural(len(cpu), "node", "nodes")),
		}
	}

	return check
}

// gpuProducts is the distinct GPU products of a pool, sorted.
func gpuProducts(pool []kube.NodeInfo) []string {
	seen := map[string]bool{}
	var products []string
	for _, node := range pool {
		product := node.GPUProduct
		if product == "" && node.GPUs > 0 {
			product = "unknown GPU"
		}
		if product == "" || seen[product] {
			continue
		}
		seen[product] = true
		products = append(products, product)
	}

	sort.Strings(products)
	return products
}

// labelChanges compares each node's managed labels with the ones its
// assignment calls for. Managed labels the assignment does not call for are
// removed, which also clears the pools of models no longer served.
func labelChanges(nodes []kube.NodeInfo, models []catalog.Model, values []string) []labelChange {
	byOption := map[string]catalog.Model{}
	for _, model := range models {
		byOption[modelOption(model)] = model
	}

	var changes []labelChange
	for i, node := range nodes {
		want := wantedLabels(values[i], byOption)

		change := labelChange{Node: node.Name}
		for _, key := range want {
			if node.Labels[key] != placement.Value {
				change.Set = append(change.Set, key)
			}
		}
		for key := range node.Labels {
			if placement.Managed(key) && !contains(want, key) {
				change.Remove = append(change.Remove, key)
			}
		}
		sort.Strings(change.Set)
		sort.Strings(change.Remove)

		if len(change.Set) > 0 || len(change.Remove) > 0 {
			changes = append(changes, change)
		}
	}

	return changes
}

func wantedLabels(option string, models map[string]catalog.Model) []string {
	switch option {
	case OptionCPU:
		return []string{placement.CPULabel}
	case OptionUnassigned:
		return nil
	}

	model, ok := models[option]
	if !ok {
		return nil
	}

	var keys []string
	for key := range placement.ModelSelector(model) {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func modelTitle(model catalog.Model) string {
	kind := "gen"
	if model.Category == catalog.CategoryEmbedder {
		kind = "emb"
	}

	return fmt.Sprintf("%s · %s · %d GPU", model.Name, kind, model.GPUsPerPod)
}

func gpuCell(node kube.NodeInfo) string {
	if node.GPUs == 0 {
		return "-"
	}

	return fmt.Sprintf("%s ×%d", shortProduct(firstOr([]string{node.GPUProduct}, "GPU")), node.GPUs)
}

func computeCell(node kube.NodeInfo) string {
	const gib = 1 << 30
	return fmt.Sprintf("%dc/%dG", (node.CPUMillis+500)/1000, (node.MemoryBytes+gib/2)/gib)
}

func nodeDetail(node kube.NodeInfo) string {
	var hardware []string
	if node.InstanceType != "" {
		hardware = append(hardware, node.InstanceType)
	}
	if node.ControlPlane {
		hardware = append(hardware, "control plane")
	}

	if node.GPUs == 0 {
		hardware = append(hardware, "no GPU")
	} else {
		gpu := fmt.Sprintf("%s ×%d", firstOr([]string{node.GPUProduct}, "unknown GPU"), node.GPUs)
		if node.GPUMemoryMiB > 0 {
			gpu += fmt.Sprintf(", %d GB each", (node.GPUMemoryMiB+512)/1024)
		}
		if node.MIGCapable {
			gpu += ", MIG capable"
		}
		hardware = append(hardware, gpu)
	}

	var managed []string
	for key, value := range node.Labels {
		if placement.Managed(key) && value == placement.Value {
			managed = append(managed, key)
		}
	}
	sort.Strings(managed)

	labels := "labels: none"
	if len(managed) > 0 {
		labels = "labels: " + strings.Join(managed, ", ")
	}

	return strings.Join(hardware, ", ") + "\n" + labels
}

// shortProduct trims the vendor prefix the GPU Operator reports, so
// "NVIDIA-A10-24Q" reads as "A10-24Q".
func shortProduct(product string) string {
	return strings.TrimPrefix(product, "NVIDIA-")
}

func firstOr(values []string, fallback string) string {
	if len(values) == 0 || values[0] == "" {
		return fallback
	}

	return values[0]
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}

	return false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}
