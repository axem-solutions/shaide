package nodes

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/installer/internal/placement"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const aksPool = "kubernetes.azure.com/agentpool"

type scriptedReporter struct {
	prompts []core.ChoicePrompt
	choices [][]string
}

func (r *scriptedReporter) Choose(prompt core.ChoicePrompt) ([]string, error) {
	r.prompts = append(r.prompts, prompt)
	values := r.choices[0]
	r.choices = r.choices[1:]
	return values, nil
}

func (*scriptedReporter) Select(_ string, current string, _ []string) (string, error) {
	return current, nil
}
func (*scriptedReporter) MultiSelect(string, []string) ([]string, error) { return nil, nil }
func (*scriptedReporter) Input(string, string, string) (string, error)   { return "", nil }
func (*scriptedReporter) ProgressModel(core.ModelProgress)               {}

func gptOSS() catalog.Model {
	return catalog.Model{Name: "GPT-OSS-20B", Slug: "gpt-oss-20b", Category: catalog.CategoryGenerative, GPUsPerPod: 1}
}

func bgeM3() catalog.Model {
	return catalog.Model{Name: "BGE-M3", Slug: "bge-m3", Category: catalog.CategoryEmbedder, GPUsPerPod: 1}
}

func node(name string, gpus int64, product string, labels map[string]string, taints ...corev1.Taint) *corev1.Node {
	allLabels := map[string]string{"kubernetes.io/hostname": name}
	for key, value := range labels {
		allLabels[key] = value
	}
	if product != "" {
		allLabels["nvidia.com/gpu.product"] = product
	}

	allocatable := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8"),
		corev1.ResourceMemory: resource.MustParse("32Gi"),
	}
	if gpus > 0 {
		allocatable["nvidia.com/gpu"] = *resource.NewQuantity(gpus, resource.DecimalSI)
	}

	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: allLabels},
		Spec:       corev1.NodeSpec{Taints: taints},
		Status:     corev1.NodeStatus{Allocatable: allocatable},
	}
}

type kubeNode = kube.NodeInfo

// info reads a node back through the inventory, as the stage sees it.
func info(t *testing.T, n *corev1.Node) kube.NodeInfo {
	t.Helper()

	infos, err := kube.ListNodes(context.Background(), fake.NewClientset(n))
	if err != nil {
		t.Fatal(err)
	}
	return infos[0]
}

func aks(poolName string) map[string]string {
	return map[string]string{aksPool: poolName}
}

func newRuntime(reporter core.Reporter, client *fake.Clientset, models ...catalog.Model) *core.Runtime {
	rt := core.NewContext(logger.NewWithWriter(io.Discard), reporter)
	rt.Cluster.Client = client
	rt.Models.Serve = models
	rt.Begin("assign node pools", &state{}, 3)
	return rt
}

func runStage(t *testing.T, rt *core.Runtime) {
	t.Helper()

	for _, step := range Stage().Steps {
		if step.When != nil && !step.When(rt) {
			continue
		}
		if err := step.Run(rt); err != nil {
			t.Fatalf("%s: %v", step.Name, err)
		}
	}
}

func storedAssignment(t *testing.T, client *fake.Clientset) placement.Assignment {
	t.Helper()

	cm, err := client.CoreV1().ConfigMaps(assignmentNamespace).Get(context.Background(), assignmentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("read stored assignment: %v", err)
	}

	var assignment placement.Assignment
	if err := json.Unmarshal([]byte(cm.Data[assignmentDataKey]), &assignment); err != nil {
		t.Fatal(err)
	}
	return assignment
}

func storeAssignment(t *testing.T, client *fake.Clientset, assignment placement.Assignment) {
	t.Helper()

	data, err := json.Marshal(assignment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ConfigMaps(assignmentNamespace).Create(context.Background(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: assignmentName, Namespace: assignmentNamespace},
		Data:       map[string]string{assignmentDataKey: string(data)},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func poolNamesOf(rt *core.Runtime) []string {
	var names []string
	for _, p := range core.StageData[*state](rt).pools {
		names = append(names, p.ID.String())
	}
	return names
}

// Nodes are grouped by the label naming their pool; a node without one is a
// pool of its own, and nodes reserved by a taint are not offered.
func TestListPoolsGroupsNodesByPool(t *testing.T) {
	client := fake.NewClientset(
		node("aks-gpu-1", 1, "NVIDIA-A10-24Q", aks("generative"), corev1.Taint{Key: "nvidia.com/gpu", Value: "present", Effect: corev1.TaintEffectNoSchedule}),
		node("aks-gpu-2", 1, "NVIDIA-A10-24Q", aks("generative")),
		node("aks-sys-1", 0, "", aks("system"), corev1.Taint{Key: "CriticalAddonsOnly", Value: "true", Effect: corev1.TaintEffectNoSchedule}),
		node("server2", 0, "", map[string]string{"nodegroup": "no-gpu"}),
		node("server4", 1, "NVIDIA-H100", nil),
	)
	rt := newRuntime(&scriptedReporter{}, client)

	if err := listPools(rt); err != nil {
		t.Fatalf("listPools: %v", err)
	}

	got := strings.Join(poolNamesOf(rt), ",")
	want := "kubernetes.azure.com/agentpool=generative,nodegroup=no-gpu,kubernetes.io/hostname=server4"
	if got != want {
		t.Errorf("pools = %s, want %s", got, want)
	}
	if n := len(core.StageData[*state](rt).pools[0].Nodes); n != 2 {
		t.Errorf("generative pool has %d nodes, want 2", n)
	}
}

// A stored assignment is where pools start, so a rerun is a single confirm.
func TestStoredAssignmentIsPreselected(t *testing.T) {
	client := fake.NewClientset(
		node("aks-gpu-1", 1, "NVIDIA-A10-24Q", aks("generative")),
		node("aks-cpu-1", 0, "", aks("shaide")),
	)
	storeAssignment(t, client, placement.Assignment{
		Models: map[string][]placement.Pool{"gpt-oss-20b": {{Key: aksPool, Name: "generative"}}},
		CPU:    []placement.Pool{{Key: aksPool, Name: "shaide"}},
	})
	reporter := &scriptedReporter{choices: [][]string{{"model:gpt-oss-20b", OptionCPU}}}
	rt := newRuntime(reporter, client, gptOSS())

	runStage(t, rt)

	rows := reporter.prompts[0].Rows
	if rows[0].Cells[0] != "generative" || rows[0].Current != "model:gpt-oss-20b" {
		t.Errorf("generative row = %+v, want it preselected for GPT-OSS-20B", rows[0])
	}
	if rows[1].Current != OptionCPU {
		t.Errorf("shaide row starts at %q, want cpu", rows[1].Current)
	}
}

// Without a stored assignment, the node labels an earlier installer wrote
// place the pools, so an upgrade starts from the existing placement.
func TestLegacyLabelsArePreselectedAndRemoved(t *testing.T) {
	client := fake.NewClientset(
		node("aks-gpu-1", 1, "NVIDIA-A10-24Q", map[string]string{
			aksPool:                        "generative",
			"axem.dev/model-gpt-oss-20b":   "true",
			"axem.dev/workload-generative": "true",
		}),
		node("aks-cpu-1", 0, "", map[string]string{aksPool: "shaide", "axem.dev/workload-cpu": "true"}),
	)
	reporter := &scriptedReporter{choices: [][]string{{"model:gpt-oss-20b", OptionCPU}}}
	rt := newRuntime(reporter, client, gptOSS())

	runStage(t, rt)

	rows := reporter.prompts[0].Rows
	if rows[0].Current != "model:gpt-oss-20b" || rows[1].Current != OptionCPU {
		t.Errorf("rows start at %q, %q, want the legacy placement", rows[0].Current, rows[1].Current)
	}

	for _, name := range []string{"aks-gpu-1", "aks-cpu-1"} {
		n, err := client.CoreV1().Nodes().Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for key := range n.Labels {
			if placement.IsLegacyLabel(key) {
				t.Errorf("node %s still has %s", name, key)
			}
		}
		if n.Labels[aksPool] == "" {
			t.Errorf("node %s lost its pool label", name)
		}
	}
}

// On a first run, pools without a GPU run the platform and GPU pools wait
// for the operator.
func TestFirstRunGuess(t *testing.T) {
	gpu := pool{ID: placement.Pool{Key: aksPool, Name: "generative"}, Nodes: []kubeNode{info(t, node("g", 1, "A10", nil))}}
	cpu := pool{ID: placement.Pool{Key: aksPool, Name: "shaide"}, Nodes: []kubeNode{info(t, node("c", 0, "", nil))}}
	cp := pool{ID: placement.Pool{Key: "kubernetes.io/hostname", Name: "cp"}, Nodes: []kubeNode{info(t, node("cp", 0, "", map[string]string{"node-role.kubernetes.io/control-plane": ""}))}}

	models := []catalog.Model{gptOSS()}
	for _, test := range []struct {
		pool pool
		want string
	}{
		{gpu, OptionUnassigned},
		{cpu, OptionCPU},
		{cp, OptionUnassigned},
	} {
		if got := initialOption(test.pool, models, placement.Assignment{}, false); got != test.want {
			t.Errorf("%s starts at %q, want %q", test.pool.ID.Name, got, test.want)
		}
	}
}

// A pool without GPUs cannot serve a model, and one whose smallest node has
// too few GPUs cannot serve a model that needs more.
func TestPoolOptions(t *testing.T) {
	cpu := pool{Nodes: []kubeNode{info(t, node("c", 0, "", nil))}}
	if got := strings.Join(poolOptions(cpu, []catalog.Model{gptOSS()}), ","); got != "cpu,unassigned" {
		t.Errorf("cpu pool options = %s, want cpu,unassigned", got)
	}

	big := gptOSS()
	big.GPUsPerPod = 2
	mixed := pool{Nodes: []kubeNode{info(t, node("a", 2, "A10", nil)), info(t, node("b", 1, "A10", nil))}}
	if got := strings.Join(poolOptions(mixed, []catalog.Model{big}), ","); got != "cpu,unassigned" {
		t.Errorf("options = %s, want the 2-GPU model left out", got)
	}
}

func TestCheckAssignment(t *testing.T) {
	a10 := pool{ID: placement.Pool{Key: aksPool, Name: "a10"}, Nodes: []kubeNode{info(t, node("a", 1, "NVIDIA-A10-24Q", nil)), info(t, node("b", 1, "NVIDIA-A10-24Q", nil))}}
	h100 := pool{ID: placement.Pool{Key: aksPool, Name: "h100"}, Nodes: []kubeNode{info(t, node("h", 1, "NVIDIA-H100", nil))}}
	both := pool{ID: placement.Pool{Key: "nodegroup", Name: "both"}, Nodes: []kubeNode{info(t, node("x", 1, "NVIDIA-A10-24Q", nil)), info(t, node("y", 1, "NVIDIA-H100", nil))}}
	cpu := pool{ID: placement.Pool{Key: aksPool, Name: "shaide"}, Nodes: []kubeNode{info(t, node("c", 0, "", nil))}}
	pools := []pool{a10, h100, both, cpu}
	models := []catalog.Model{gptOSS()}

	ok := checkAssignment(pools, models, []string{"model:gpt-oss-20b", OptionUnassigned, OptionUnassigned, OptionCPU})
	if ok.Blocking != "" {
		t.Fatalf("Blocking = %q, want a valid assignment", ok.Blocking)
	}
	if got := ok.Groups["model:gpt-oss-20b"]; !got.OK || got.Text != "A10-24Q, 2 nodes" {
		t.Errorf("model status = %+v", got)
	}

	for name, test := range map[string]struct {
		values []string
		want   string
	}{
		"no pools":            {[]string{OptionUnassigned, OptionUnassigned, OptionUnassigned, OptionCPU}, "Assign at least one node pool to GPT-OSS-20B"},
		"pools differ":        {[]string{"model:gpt-oss-20b", "model:gpt-oss-20b", OptionUnassigned, OptionCPU}, "different GPU types"},
		"pool mixes products": {[]string{OptionUnassigned, OptionUnassigned, "model:gpt-oss-20b", OptionCPU}, "both mixes GPU types"},
		"no cpu pool":         {[]string{"model:gpt-oss-20b", OptionUnassigned, OptionUnassigned, OptionUnassigned}, "CPU only"},
	} {
		got := checkAssignment(pools, models, test.values).Blocking
		if !strings.Contains(got, test.want) {
			t.Errorf("%s: Blocking = %q, want it to mention %q", name, got, test.want)
		}
	}
}

// The assignment is stored in the cluster and set for the deploy stage; an
// unchanged assignment is not written again.
func TestAssignmentIsStoredAndExposed(t *testing.T) {
	client := fake.NewClientset(
		node("aks-gpu-1", 1, "NVIDIA-A10-24Q", aks("generative")),
		node("aks-emb-1", 1, "NVIDIA-A10-24Q", aks("embedding")),
		node("aks-cpu-1", 0, "", aks("shaide")),
	)
	reporter := &scriptedReporter{choices: [][]string{{"model:bge-m3", "model:gpt-oss-20b", OptionCPU}}}
	rt := newRuntime(reporter, client, gptOSS(), bgeM3())

	runStage(t, rt)

	want := placement.Assignment{
		Models: map[string][]placement.Pool{
			"gpt-oss-20b": {{Key: aksPool, Name: "generative"}},
			"bge-m3":      {{Key: aksPool, Name: "embedding"}},
		},
		CPU: []placement.Pool{{Key: aksPool, Name: "shaide"}},
	}
	if !sameAssignment(rt.Placement, want) {
		t.Errorf("rt.Placement = %+v, want %+v", rt.Placement, want)
	}
	if got := storedAssignment(t, client); !sameAssignment(got, want) {
		t.Errorf("stored = %+v, want %+v", got, want)
	}

	// A rerun with the same choices finds it stored and leaves it.
	reporter.choices = [][]string{{"model:bge-m3", "model:gpt-oss-20b", OptionCPU}}
	client.ClearActions()
	rt2 := newRuntime(reporter, client, gptOSS(), bgeM3())
	runStage(t, rt2)

	for _, action := range client.Actions() {
		if action.GetResource().Resource == "configmaps" && action.GetVerb() != "get" {
			t.Errorf("unchanged assignment was written again: %s", action.GetVerb())
		}
	}
}
