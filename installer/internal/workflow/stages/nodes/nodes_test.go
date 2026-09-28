package nodes

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	"github.com/axem-solutions/ai_platform/installer/internal/kube"
	"github.com/axem-solutions/ai_platform/installer/internal/logger"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type scriptedReporter struct {
	prompts []core.ChoicePrompt
	choices [][]string
	answers []string
}

func (r *scriptedReporter) Choose(prompt core.ChoicePrompt) ([]string, error) {
	r.prompts = append(r.prompts, prompt)
	values := r.choices[0]
	r.choices = r.choices[1:]
	return values, nil
}

func (r *scriptedReporter) Select(_ string, current string, _ []string) (string, error) {
	if len(r.answers) == 0 {
		return current, nil
	}
	answer := r.answers[0]
	r.answers = r.answers[1:]
	return answer, nil
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
	allLabels := map[string]string{}
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

func info(n *corev1.Node) kube.NodeInfo {
	infos, err := kube.ListNodes(context.Background(), fake.NewClientset(n))
	if err != nil {
		panic(err)
	}
	return infos[0]
}

func newRuntime(reporter core.Reporter, client *fake.Clientset, models ...catalog.Model) *core.Runtime {
	rt := core.NewContext(logger.NewWithWriter(io.Discard), reporter)
	rt.Cluster.Client = client
	rt.Bootstrap.Provider = "on-prem"
	rt.Models.Serve = models
	rt.Begin("assign nodes", &state{}, 3)
	return rt
}

func TestListNodesHidesNodesReservedForSomethingElse(t *testing.T) {
	client := fake.NewClientset(
		node("cp", 0, "", nil, corev1.Taint{Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule}),
		node("system", 0, "", nil, corev1.Taint{Key: "CriticalAddonsOnly", Value: "true", Effect: corev1.TaintEffectNoSchedule}),
		node("gpu", 1, "NVIDIA-A10-24Q", nil, corev1.Taint{Key: "nvidia.com/gpu", Value: "present", Effect: corev1.TaintEffectNoSchedule}),
		node("cordoned", 0, "", nil, corev1.Taint{Key: "node.kubernetes.io/unschedulable", Effect: corev1.TaintEffectNoSchedule}),
		node("soft", 0, "", nil, corev1.Taint{Key: "dedicated", Effect: corev1.TaintEffectPreferNoSchedule}),
	)
	rt := newRuntime(&scriptedReporter{}, client)

	if err := listNodes(rt); err != nil {
		t.Fatalf("listNodes: %v", err)
	}

	var names []string
	for _, n := range core.StageData[*state](rt).nodes {
		names = append(names, n.Name)
	}
	if got := strings.Join(names, ","); got != "cordoned,gpu,soft" {
		t.Errorf("listed %s, want the GPU node, the cordoned one and the soft-tainted one", got)
	}
}

func TestInitialOption(t *testing.T) {
	models := []catalog.Model{gptOSS(), bgeM3()}

	tests := []struct {
		name string
		node *corev1.Node
		want string
	}{
		{"model label", node("n", 1, "A10", map[string]string{"axem.dev/model-bge-m3": "true"}), "model:bge-m3"},
		{"cpu label", node("n", 0, "", map[string]string{"axem.dev/workload-cpu": "true"}), OptionCPU},
		{"pool class with one model", node("n", 1, "A10", map[string]string{"axem.dev/workload-embedding": "true"}), "model:bge-m3"},
		{"first run, no GPU", node("n", 0, "", nil), OptionCPU},
		{"first run, GPU", node("n", 1, "A10", nil), OptionUnassigned},
		{"first run, control plane", node("n", 0, "", map[string]string{"node-role.kubernetes.io/control-plane": ""}), OptionUnassigned},
		{"label of a model no longer served", node("n", 1, "A10", map[string]string{"axem.dev/model-gone": "true"}), OptionUnassigned},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := initialOption(info(test.node), models); got != test.want {
				t.Errorf("initialOption = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNodesWithoutGPUCannotJoinAModelPool(t *testing.T) {
	options := nodeOptions(info(node("cpu", 0, "", nil)), []catalog.Model{gptOSS()})

	if got := strings.Join(options, ","); got != "cpu,unassigned" {
		t.Errorf("options = %s, want only cpu and unassigned", got)
	}
}

func TestCheckAssignment(t *testing.T) {
	a10a := info(node("a10-a", 1, "NVIDIA-A10-24Q", nil))
	a10b := info(node("a10-b", 1, "NVIDIA-A10-24Q", nil))
	h100 := info(node("h100", 1, "NVIDIA-H100-80GB-HBM3", nil))
	cpu := info(node("cpu", 0, "", nil))
	nodes := []kube.NodeInfo{a10a, a10b, h100, cpu}
	models := []catalog.Model{gptOSS()}

	check := checkAssignment(nodes, models, []string{"model:gpt-oss-20b", "model:gpt-oss-20b", OptionUnassigned, OptionCPU})
	if check.Blocking != "" {
		t.Fatalf("Blocking = %q, want a valid assignment", check.Blocking)
	}
	if got := check.Groups["model:gpt-oss-20b"]; !got.OK || got.Text != "A10-24Q ×2" {
		t.Errorf("pool status = %+v, want ok with A10-24Q ×2", got)
	}

	mixed := checkAssignment(nodes, models, []string{"model:gpt-oss-20b", OptionUnassigned, "model:gpt-oss-20b", OptionCPU})
	if !strings.Contains(mixed.Blocking, "mixes GPU types") {
		t.Errorf("Blocking = %q, want the mixed GPU types reported", mixed.Blocking)
	}

	empty := checkAssignment(nodes, models, []string{OptionUnassigned, OptionUnassigned, OptionUnassigned, OptionCPU})
	if !strings.Contains(empty.Blocking, "Assign at least one node to GPT-OSS-20B") {
		t.Errorf("Blocking = %q, want the empty pool reported", empty.Blocking)
	}

	noCPU := checkAssignment(nodes, models, []string{"model:gpt-oss-20b", OptionUnassigned, OptionUnassigned, OptionUnassigned})
	if !strings.Contains(noCPU.Blocking, "CPU only") {
		t.Errorf("Blocking = %q, want the missing CPU node reported", noCPU.Blocking)
	}
}

func TestLabelChangesReconcileManagedLabels(t *testing.T) {
	nodes := []kube.NodeInfo{
		info(node("gpu", 1, "A10", map[string]string{
			"axem.dev/workload-generative": "true",
			"axem.dev/model-gone":          "true",
			"nodegroup":                    "generative",
		})),
		info(node("cpu", 0, "", map[string]string{"axem.dev/workload-cpu": "true"})),
		info(node("spare", 0, "", map[string]string{"axem.dev/workload-cpu": "true"})),
	}

	changes := labelChanges(nodes, []catalog.Model{gptOSS()}, []string{"model:gpt-oss-20b", OptionCPU, OptionUnassigned})

	var got []string
	for _, change := range changes {
		got = append(got, change.String())
	}
	want := []string{
		"gpu: +axem.dev/model-gpt-oss-20b, -axem.dev/model-gone",
		"spare: -axem.dev/workload-cpu",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("changes =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAssignAndApplyLabelsTheNodes(t *testing.T) {
	client := fake.NewClientset(
		node("gpu", 1, "NVIDIA-A10-24Q", map[string]string{"axem.dev/model-gone": "true"}),
		node("cpu", 0, "", nil),
	)
	reporter := &scriptedReporter{
		choices: [][]string{{OptionCPU, "model:gpt-oss-20b"}},
		answers: []string{confirmApply},
	}
	rt := newRuntime(reporter, client, gptOSS())

	for _, step := range Stage().Steps {
		if step.When != nil && !step.When(rt) {
			continue
		}
		if err := step.Run(rt); err != nil {
			t.Fatalf("%s: %v", step.Name, err)
		}
	}

	// Nodes are listed by name: cpu first, then gpu.
	rows := reporter.prompts[0].Rows
	if rows[0].Cells[0] != "cpu" || rows[0].Current != OptionCPU {
		t.Errorf("first row = %+v, want cpu starting in CPU only", rows[0])
	}

	gpu, err := client.CoreV1().Nodes().Get(context.Background(), "gpu", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"axem.dev/model-gpt-oss-20b":   "true",
		"axem.dev/workload-generative": "true",
		"nvidia.com/gpu.product":       "NVIDIA-A10-24Q",
	} {
		if gpu.Labels[key] != want {
			t.Errorf("gpu label %s = %q, want %q", key, gpu.Labels[key], want)
		}
	}
	if _, ok := gpu.Labels["axem.dev/model-gone"]; ok {
		t.Error("the stale model label was not removed")
	}

	cpu, _ := client.CoreV1().Nodes().Get(context.Background(), "cpu", metav1.GetOptions{})
	if cpu.Labels["axem.dev/workload-cpu"] != "true" {
		t.Errorf("cpu labels = %v, want axem.dev/workload-cpu", cpu.Labels)
	}
}

// Going back from the confirmation keeps the edits made so far.
func TestBackToAssignmentKeepsTheEdits(t *testing.T) {
	client := fake.NewClientset(node("gpu", 1, "A10", nil), node("cpu", 0, "", nil))
	reporter := &scriptedReporter{
		choices: [][]string{
			{OptionCPU, "model:gpt-oss-20b"},
			{OptionCPU, "model:gpt-oss-20b"},
		},
		answers: []string{confirmBack, confirmApply},
	}
	rt := newRuntime(reporter, client, gptOSS())

	if err := listNodes(rt); err != nil {
		t.Fatal(err)
	}
	if err := assignNodes(rt); err != nil {
		t.Fatal(err)
	}

	if len(reporter.prompts) != 2 {
		t.Fatalf("prompted %d times, want the view shown again after Back", len(reporter.prompts))
	}
	if got := reporter.prompts[1].Rows[1].Current; got != "model:gpt-oss-20b" {
		t.Errorf("second view starts gpu at %q, want the edit kept", got)
	}
}

func TestNothingToApplyWhenLabelsMatch(t *testing.T) {
	client := fake.NewClientset(
		node("gpu", 1, "A10", map[string]string{"axem.dev/model-gpt-oss-20b": "true", "axem.dev/workload-generative": "true"}),
		node("cpu", 0, "", map[string]string{"axem.dev/workload-cpu": "true"}),
	)
	reporter := &scriptedReporter{choices: [][]string{{OptionCPU, "model:gpt-oss-20b"}}}
	rt := newRuntime(reporter, client, gptOSS())

	if err := listNodes(rt); err != nil {
		t.Fatal(err)
	}
	if err := assignNodes(rt); err != nil {
		t.Fatal(err)
	}
	if hasChanges(rt) {
		t.Errorf("changes = %v, want none", core.StageData[*state](rt).changes)
	}
}
