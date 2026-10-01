package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/placement"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

// Options a pool can be assigned to, besides one per model to serve.
const (
	OptionCPU        = "cpu"
	OptionUnassigned = "unassigned"

	modelOptionPrefix = "model:"
)

// The assignment is kept in the cluster, so a rerun from any provisioning
// host starts from it. kube-system always exists and outlives the platform's
// own namespaces, which a recreate deletes.
const (
	assignmentNamespace = "kube-system"
	assignmentName      = "shaide-placement"
	assignmentDataKey   = "assignment.json"
)

var assignmentLabels = map[string]string{"app.kubernetes.io/part-of": "shaide"}

type state struct {
	pools []pool
	// stored is the assignment found in the cluster; found is false on a
	// first run.
	stored placement.Assignment
	found  bool
}

// pool is a node pool and the nodes of it the workloads can run on.
type pool struct {
	ID    placement.Pool
	Nodes []kube.NodeInfo
}

func Stage() core.Stage {
	return core.Stage{
		Name:     "assign node pools",
		NewState: func() any { return &state{} },
		Steps: []core.Step{
			{
				Name: "list node pools",
				Run:  listPools,
			},
			{
				Name: "assign node pools",
				Run:  assignPools,
			},
			{
				Name: "save node pool assignment",
				Run:  saveAssignment,
			},
		},
	}
}

func modelOption(model catalog.Model) string {
	return modelOptionPrefix + model.Slug
}

// listPools groups the nodes shaide's workloads can run on into their pools,
// and reads the assignment a previous run stored. A node reserved by a taint
// the workloads do not tolerate, such as a control plane or a system pool,
// is left out.
func listPools(rt *core.Runtime) error {
	st := core.StageData[*state](rt)
	ctx := context.Background()

	nodes, err := kube.ListNodes(ctx, rt.Cluster.Client)
	if err != nil {
		return err
	}

	byID := map[placement.Pool]*pool{}
	for _, node := range nodes {
		if taint, reserved := node.ExclusiveTaint(placement.GPUTaintKey); reserved {
			rt.Detailf("not offering node %s: reserved by taint %s=%s:%s", node.Name, taint.Key, taint.Value, taint.Effect)
			continue
		}

		id := placement.PoolOf(node.Name, node.Labels)
		if byID[id] == nil {
			byID[id] = &pool{ID: id}
		}
		byID[id].Nodes = append(byID[id].Nodes, node)
	}

	st.pools = make([]pool, 0, len(byID))
	for _, p := range byID {
		st.pools = append(st.pools, *p)
	}
	sort.Slice(st.pools, func(i, j int) bool {
		if st.pools[i].ID.Name != st.pools[j].ID.Name {
			return st.pools[i].ID.Name < st.pools[j].ID.Name
		}
		return st.pools[i].ID.Key < st.pools[j].ID.Key
	})

	if len(st.pools) == 0 {
		return fmt.Errorf("the cluster has no node shaide's workloads can be scheduled on")
	}

	for _, p := range st.pools {
		rt.Detailf("node pool %s (%s): %d %s", p.ID.Name, p.ID.Kind(), len(p.Nodes), plural(len(p.Nodes), "node", "nodes"))
	}

	st.stored, st.found, err = readAssignment(ctx, rt)
	return err
}

func readAssignment(ctx context.Context, rt *core.Runtime) (placement.Assignment, bool, error) {
	data, found, err := kube.ReadConfigMapData(ctx, rt.Cluster.Client, assignmentNamespace, assignmentName)
	if err != nil || !found {
		return placement.Assignment{}, false, err
	}

	var assignment placement.Assignment
	if err := json.Unmarshal([]byte(data[assignmentDataKey]), &assignment); err != nil {
		rt.Detailf("ignoring the stored node pool assignment, which cannot be read: %v", err)
		return placement.Assignment{}, false, nil
	}

	return assignment, true, nil
}

func assignPools(rt *core.Runtime) error {
	st := core.StageData[*state](rt)
	models := rt.Models.Serve

	values, err := rt.Reporter.Choose(assignmentPrompt(st, models))
	if err != nil {
		return err
	}
	if len(values) != len(st.pools) {
		return fmt.Errorf("node pool assignment returned %d choices for %d pools", len(values), len(st.pools))
	}

	rt.Placement = assignmentOf(st.pools, models, values)

	for _, model := range models {
		rt.Detailf("%s runs on %s", model.Name, poolNames(rt.Placement.Models[model.Slug]))
	}
	rt.Detailf("CPU work runs on %s", poolNames(rt.Placement.CPU))

	return nil
}

// saveAssignment stores the assignment in the cluster, and removes the node
// labels earlier installers placed workloads with.
func saveAssignment(rt *core.Runtime) error {
	st := core.StageData[*state](rt)
	ctx := context.Background()

	if !st.found || !sameAssignment(st.stored, rt.Placement) {
		data, err := json.Marshal(rt.Placement)
		if err != nil {
			return fmt.Errorf("encode node pool assignment: %w", err)
		}

		if err := kube.WriteConfigMapData(ctx, rt.Cluster.Client, assignmentNamespace, assignmentName,
			assignmentLabels, map[string]string{assignmentDataKey: string(data)}); err != nil {
			return err
		}
		rt.Detailf("stored the node pool assignment in ConfigMap %s/%s", assignmentNamespace, assignmentName)
	}

	for _, p := range st.pools {
		for _, node := range p.Nodes {
			var legacy []string
			for key := range node.Labels {
				if placement.IsLegacyLabel(key) {
					legacy = append(legacy, key)
				}
			}
			if len(legacy) == 0 {
				continue
			}
			sort.Strings(legacy)

			if err := kube.PatchNodeLabels(ctx, rt.Cluster.Client, node.Name, nil, legacy); err != nil {
				return err
			}
			rt.Detailf("removed labels no longer used for placement from node %s: %s", node.Name, strings.Join(legacy, ", "))
		}
	}

	return nil
}

func sameAssignment(a, b placement.Assignment) bool {
	normalize := func(x placement.Assignment) placement.Assignment {
		out := placement.Assignment{Models: map[string][]placement.Pool{}, CPU: sortedPools(x.CPU)}
		for slug, pools := range x.Models {
			if len(pools) > 0 {
				out.Models[slug] = sortedPools(pools)
			}
		}
		return out
	}

	return reflect.DeepEqual(normalize(a), normalize(b))
}

func sortedPools(pools []placement.Pool) []placement.Pool {
	out := append([]placement.Pool(nil), pools...)
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	if len(out) == 0 {
		return nil
	}
	return out
}

func assignmentOf(pools []pool, models []catalog.Model, values []string) placement.Assignment {
	assignment := placement.Assignment{Models: map[string][]placement.Pool{}}

	bySlug := map[string]catalog.Model{}
	for _, model := range models {
		bySlug[modelOption(model)] = model
	}

	for i, value := range values {
		switch {
		case value == OptionCPU:
			assignment.CPU = append(assignment.CPU, pools[i].ID)
		case strings.HasPrefix(value, modelOptionPrefix):
			if model, ok := bySlug[value]; ok {
				assignment.Models[model.Slug] = append(assignment.Models[model.Slug], pools[i].ID)
			}
		}
	}

	return assignment
}

func assignmentPrompt(st *state, models []catalog.Model) core.ChoicePrompt {
	groups := make([]core.ChoiceGroup, 0, len(models)+2)
	for _, model := range models {
		groups = append(groups, core.ChoiceGroup{Option: modelOption(model), Title: modelTitle(model)})
	}
	groups = append(groups,
		core.ChoiceGroup{Option: OptionCPU, Title: "CPU only"},
		core.ChoiceGroup{Option: OptionUnassigned, Title: "Unassigned"},
	)

	rows := make([]core.ChoiceRow, 0, len(st.pools))
	for _, p := range st.pools {
		rows = append(rows, core.ChoiceRow{
			Cells:   []string{p.ID.Name, nodeCount(p), gpuCell(p)},
			Detail:  poolDetail(p),
			Options: poolOptions(p, models),
			Current: initialOption(p, models, st.stored, st.found),
		})
	}

	pools := st.pools
	return core.ChoicePrompt{
		Title:       "Assign node pools to workloads",
		Columns:     []string{"Pool", "Nodes", "GPU"},
		Rows:        rows,
		Groups:      groups,
		ClearOption: OptionUnassigned,
		Check: func(values []string) core.ChoiceCheck {
			return checkAssignment(pools, models, values)
		},
	}
}

// minGPUs is the GPU count of the pool's smallest node: what one pod placed
// anywhere in the pool can rely on.
func minGPUs(p pool) int64 {
	smallest := int64(-1)
	for _, node := range p.Nodes {
		if smallest < 0 || node.GPUs < smallest {
			smallest = node.GPUs
		}
	}
	if smallest < 0 {
		return 0
	}
	return smallest
}

// fits reports whether one pod of the model fits on every node of the pool.
func fits(p pool, model catalog.Model) bool {
	return minGPUs(p) >= int64(model.GPUsPerPod)
}

// poolOptions is what a pool can be assigned to: the models whose pod fits on
// every one of its nodes, CPU work, or nothing.
func poolOptions(p pool, models []catalog.Model) []string {
	var options []string
	for _, model := range models {
		if fits(p, model) {
			options = append(options, modelOption(model))
		}
	}

	return append(options, OptionCPU, OptionUnassigned)
}

// initialOption is where the pool already is, so a rerun that changes nothing
// is a single confirm: the stored assignment, else the labels an earlier
// installer put on its nodes, else a first-run guess.
func initialOption(p pool, models []catalog.Model, stored placement.Assignment, found bool) string {
	options := poolOptions(p, models)
	offered := func(option string) bool {
		for _, candidate := range options {
			if candidate == option {
				return true
			}
		}
		return false
	}

	if found {
		for _, model := range models {
			if containsPool(stored.Models[model.Slug], p.ID) && offered(modelOption(model)) {
				return modelOption(model)
			}
		}
		if containsPool(stored.CPU, p.ID) {
			return OptionCPU
		}
		return OptionUnassigned
	}

	for _, model := range models {
		if allNodesLabelled(p, placement.LegacyModelLabel(model.Slug)) && offered(modelOption(model)) {
			return modelOption(model)
		}
	}
	if allNodesLabelled(p, placement.LegacyCPULabel) {
		return OptionCPU
	}

	// First run: pools without a GPU run the platform itself.
	if !hasGPU(p) && !allControlPlane(p) {
		return OptionCPU
	}

	return OptionUnassigned
}

func containsPool(pools []placement.Pool, id placement.Pool) bool {
	for _, pool := range pools {
		if pool == id {
			return true
		}
	}
	return false
}

func allNodesLabelled(p pool, label string) bool {
	for _, node := range p.Nodes {
		if node.Labels[label] != "true" {
			return false
		}
	}
	return len(p.Nodes) > 0
}

func hasGPU(p pool) bool {
	for _, node := range p.Nodes {
		if node.GPUs > 0 {
			return true
		}
	}
	return false
}

func allControlPlane(p pool) bool {
	for _, node := range p.Nodes {
		if !node.ControlPlane {
			return false
		}
	}
	return true
}

// checkAssignment validates each model's pools and reports the header status
// of every group. The first problem blocks the assignment from being
// submitted.
func checkAssignment(pools []pool, models []catalog.Model, values []string) core.ChoiceCheck {
	members := map[string][]pool{}
	for i, value := range values {
		members[value] = append(members[value], pools[i])
	}

	check := core.ChoiceCheck{Groups: map[string]core.GroupStatus{}}
	block := func(format string, args ...any) {
		if check.Blocking == "" {
			check.Blocking = fmt.Sprintf(format, args...)
		}
	}

	for _, model := range models {
		option := modelOption(model)
		assigned := members[option]

		if len(assigned) == 0 {
			check.Groups[option] = core.GroupStatus{Text: "no pools"}
			block("Assign at least one node pool to %s.", model.Name)
			continue
		}

		if mixed := mixedPool(assigned); mixed != "" {
			check.Groups[option] = core.GroupStatus{Text: "mixed GPUs"}
			block("Node pool %s mixes GPU types; %s needs one GPU type.", mixed, model.Name)
			continue
		}

		products := gpuProducts(assigned)
		if len(products) > 1 {
			check.Groups[option] = core.GroupStatus{Text: "mixed GPUs"}
			block("The node pools of %s have different GPU types (%s); a model runs on one GPU type.", model.Name, strings.Join(products, ", "))
			continue
		}

		short := ""
		nodes := 0
		for _, p := range assigned {
			nodes += len(p.Nodes)
			if !fits(p, model) && short == "" {
				short = p.ID.Name
			}
		}
		if short != "" {
			check.Groups[option] = core.GroupStatus{Text: "too few GPUs"}
			block("Node pool %s has nodes with too few GPUs for %s, which needs %d per pod.", short, model.Name, model.GPUsPerPod)
			continue
		}

		check.Groups[option] = core.GroupStatus{
			OK:   true,
			Text: fmt.Sprintf("%s, %d %s", shortProduct(firstOr(products, "no GPU")), nodes, plural(nodes, "node", "nodes")),
		}
	}

	if cpu := members[OptionCPU]; len(cpu) == 0 {
		check.Groups[OptionCPU] = core.GroupStatus{Text: "no pools"}
		block("Assign at least one node pool to CPU only; the platform itself runs there.")
	} else {
		nodes := 0
		for _, p := range cpu {
			nodes += len(p.Nodes)
		}
		check.Groups[OptionCPU] = core.GroupStatus{
			OK:   true,
			Text: fmt.Sprintf("%d %s", nodes, plural(nodes, "node", "nodes")),
		}
	}

	return check
}

// mixedPool names the first pool whose nodes have different GPU products.
func mixedPool(pools []pool) string {
	for _, p := range pools {
		if len(gpuProducts([]pool{p})) > 1 {
			return p.ID.Name
		}
	}
	return ""
}

// gpuProducts is the distinct GPU products of the pools' nodes, sorted.
func gpuProducts(pools []pool) []string {
	seen := map[string]bool{}
	var products []string
	for _, p := range pools {
		for _, node := range p.Nodes {
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
	}

	sort.Strings(products)
	return products
}

func poolNames(pools []placement.Pool) string {
	if len(pools) == 0 {
		return "no pools"
	}

	names := make([]string, 0, len(pools))
	for _, p := range pools {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func modelTitle(model catalog.Model) string {
	kind := "gen"
	if model.Category == catalog.CategoryEmbedder {
		kind = "emb"
	}

	return fmt.Sprintf("%s · %s · %d GPU", model.Name, kind, model.GPUsPerPod)
}

func nodeCount(p pool) string {
	return fmt.Sprintf("%d %s", len(p.Nodes), plural(len(p.Nodes), "node", "nodes"))
}

func gpuCell(p pool) string {
	products := gpuProducts([]pool{p})
	switch {
	case !hasGPU(p):
		return "-"
	case len(products) > 1:
		return "mixed"
	default:
		return fmt.Sprintf("%s ×%d", shortProduct(firstOr(products, "GPU")), minGPUs(p))
	}
}

func poolDetail(p pool) string {
	names := make([]string, 0, len(p.Nodes))
	for _, node := range p.Nodes {
		names = append(names, node.Name)
	}

	hardware := "no GPU"
	if hasGPU(p) {
		hardware = strings.Join(gpuProducts([]pool{p}), ", ")
		if p.Nodes[0].MIGCapable {
			hardware += ", MIG capable"
		}
	}

	return fmt.Sprintf("%s, %s\nnodes: %s", p.ID.Kind(), hardware, strings.Join(names, ", "))
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

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}
