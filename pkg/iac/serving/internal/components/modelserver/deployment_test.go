package modelserver

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func vllmModel(t *testing.T, definition string) appConfig.Model {
	t.Helper()

	def, err := modeldef.Parse([]byte(definition))
	if err != nil {
		t.Fatal(err)
	}

	return appConfig.Model{
		Name:         "Example",
		Namespace:    "llm-d-example",
		ReleaseName:  "infra-example",
		NodeSelector: map[string]string{"nodegroup": "generative"},
		ModelPaths:   appConfig.ModelPaths{Slug: def.Slug},
		Definition:   def,
		ModelSource:  &appConfig.ModelSource{ModelUri: "hub/org/example"},
	}
}

const exampleDefinition = `
slug: example
model:
  name: org/example
  size: 10Gi
server:
  args: ["--max-model-len", "4096"]
  env:
    - {name: VLLM_LOGGING_LEVEL, value: INFO}
    - {name: HF_HUB_OFFLINE, value: "0"}
  resources:
    limits: {nvidia.com/gpu: "1"}
  volumes:
    - {name: shm, emptyDir: {medium: Memory}}
  volumeMounts:
    - {name: shm, mountPath: /dev/shm}
`

func build(t *testing.T, model appConfig.Model) *appsv1.Deployment {
	t.Helper()

	deployment, err := Deployment(Input{
		Model:          model,
		Category:       appConfig.CategoryGenerative,
		Image:          "harbor/services/vllm/vllm-openai:v0.31.0",
		SimulatorImage: "harbor/services/llm-d/llm-d-inference-sim:v1.0.0",
		Toleration:     &appConfig.DefaultGPUToleration,
		PullSecret:     "harbor-creds",
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

// vLLM serves the weights the ORAS Job pulled, offline, on the port the
// InferencePool targets, under the name clients request.
func TestDeploymentServesThePulledWeights(t *testing.T) {
	deployment := build(t, vllmModel(t, exampleDefinition))
	container := deployment.Spec.Template.Spec.Containers[0]

	if container.Image != "harbor/services/vllm/vllm-openai:v0.31.0" {
		t.Errorf("image = %q, want the stack's vLLM image", container.Image)
	}
	wantArgs := []string{"org/example", "--port", "8000", "--served-model-name", "org/example", "--shutdown-timeout=45", "--max-model-len", "4096"}
	if !reflect.DeepEqual(container.Command, []string{"vllm", "serve"}) || !reflect.DeepEqual(container.Args, wantArgs) {
		t.Errorf("command = %q %q", container.Command, container.Args)
	}

	// The model's own HF_HUB_OFFLINE replaces the platform's.
	wantEnv := []corev1.EnvVar{
		{Name: "HF_HUB_CACHE", Value: "/model-cache/hub"},
		{Name: "VLLM_LOGGING_LEVEL", Value: "INFO"},
		{Name: "HF_HUB_OFFLINE", Value: "0"},
	}
	if !reflect.DeepEqual(container.Env, wantEnv) {
		t.Errorf("env = %+v", container.Env)
	}

	mounts := container.VolumeMounts
	if len(mounts) != 2 || mounts[1].Name != modelVolumeName || mounts[1].MountPath != modelMountPath || !mounts[1].ReadOnly {
		t.Errorf("mounts = %+v, want the definition's and the read-only weights", mounts)
	}
	volumes := deployment.Spec.Template.Spec.Volumes
	if len(volumes) != 2 || volumes[1].PersistentVolumeClaim.ClaimName != "example-model" {
		t.Errorf("volumes = %+v, want the model's PVC", volumes)
	}
	if container.Ports[0].ContainerPort != Port || container.ReadinessProbe.HTTPGet.Path != "/v1/models" {
		t.Errorf("ports/probes = %+v %+v", container.Ports, container.ReadinessProbe)
	}
}

func TestDeploymentIsSelectedByTheRouter(t *testing.T) {
	model := vllmModel(t, exampleDefinition)
	deployment := build(t, model)

	selector := deployment.Spec.Selector.MatchLabels
	if !reflect.DeepEqual(selector, PodSelector(model)) || selector[ModelLabel] != "model-example" || selector[RoleLabel] != "decode" {
		t.Errorf("selector = %v", selector)
	}
	labels := deployment.Spec.Template.Labels
	for key, value := range selector {
		if labels[key] != value {
			t.Errorf("pod label %s = %q, want %q", key, labels[key], value)
		}
	}
	if labels["axem.dev/model-slug"] != "example" {
		t.Errorf("pod labels = %v, want the model's meta labels", labels)
	}
}

func TestDeploymentIsPlacedAndDrains(t *testing.T) {
	deployment := build(t, vllmModel(t, exampleDefinition))
	pod := deployment.Spec.Template.Spec

	if pod.Affinity == nil || pod.Affinity.NodeAffinity == nil || pod.Affinity.PodAffinity != nil {
		t.Errorf("affinity = %+v, want only the node placement for one replica", pod.Affinity)
	}
	if len(pod.Tolerations) != 1 || pod.Tolerations[0].Key != "nvidia.com/gpu" {
		t.Errorf("tolerations = %+v", pod.Tolerations)
	}
	if pod.ImagePullSecrets[0].Name != "harbor-creds" {
		t.Errorf("pull secrets = %+v", pod.ImagePullSecrets)
	}
	if *pod.TerminationGracePeriodSeconds != 75 || pod.Containers[0].Lifecycle.PreStop.Sleep.Seconds != 15 {
		t.Error("graceful shutdown not configured")
	}
	if *pod.EnableServiceLinks {
		t.Error("service links would leak VLLM_* variables into vLLM")
	}
	if deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Errorf("strategy = %v, want Recreate for one replica", deployment.Spec.Strategy.Type)
	}
}

// Several replicas roll one at a time without spare GPUs, and share a node
// because the weights volume is ReadWriteOnce.
func TestDeploymentRollsReplicasOnOneNode(t *testing.T) {
	model := vllmModel(t, exampleDefinition+"  replicas: 2\n")
	deployment := build(t, model)

	if *deployment.Spec.Replicas != 2 {
		t.Errorf("replicas = %d", *deployment.Spec.Replicas)
	}
	rolling := deployment.Spec.Strategy.RollingUpdate
	if deployment.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType || rolling.MaxSurge.IntValue() != 0 || rolling.MaxUnavailable.IntValue() != 1 {
		t.Errorf("strategy = %+v", deployment.Spec.Strategy)
	}

	podAffinity := deployment.Spec.Template.Spec.Affinity.PodAffinity
	if podAffinity == nil || podAffinity.RequiredDuringSchedulingIgnoredDuringExecution[0].TopologyKey != "kubernetes.io/hostname" {
		t.Errorf("pod affinity = %+v, want replicas on one node", podAffinity)
	}
}

func TestDeploymentRejectsPlatformFlags(t *testing.T) {
	for _, flag := range []string{`"--port", "9000"`, `"--served-model-name=x"`, `"--shutdown-timeout", "5"`} {
		definition := strings.Replace(exampleDefinition, `args: ["--max-model-len", "4096"]`, "args: ["+flag+"]", 1)
		_, err := Deployment(Input{Model: vllmModel(t, definition), Image: "vllm"})
		if err == nil {
			t.Errorf("args %s accepted, want an error", flag)
		}
	}
}

// Without pulled weights vLLM downloads the model into an emptyDir.
func TestDeploymentDownloadsWithoutAModelSource(t *testing.T) {
	model := vllmModel(t, exampleDefinition)
	model.ModelSource = nil
	pod := build(t, model).Spec.Template.Spec

	container := pod.Containers[0]
	if container.Args[0] != "org/example" || container.Env[0] != (corev1.EnvVar{Name: "HF_HOME", Value: modelMountPath}) {
		t.Errorf("args/env = %q %+v", container.Args, container.Env)
	}
	if pod.Volumes[1].EmptyDir == nil || pod.Volumes[1].EmptyDir.SizeLimit.String() != "10Gi" {
		t.Errorf("volumes = %+v, want a sized emptyDir", pod.Volumes)
	}
}

func TestDeploymentRunsTheSimulator(t *testing.T) {
	model := vllmModel(t, `
slug: sim
model: {name: random/model-1, size: 5Mi}
server:
  engine: sim
  image: ghcr.io/llm-d/llm-d-inference-sim:v1.0.0
  replicas: 2
`)
	pod := build(t, model).Spec.Template.Spec
	container := pod.Containers[0]

	if container.Image != "ghcr.io/llm-d/llm-d-inference-sim:v1.0.0" || container.Command != nil {
		t.Errorf("image/command = %q %q", container.Image, container.Command)
	}
	if !reflect.DeepEqual(container.Args, []string{"--model", "random/model-1", "--port", "8000"}) {
		t.Errorf("args = %q", container.Args)
	}
	if len(pod.Volumes) != 0 || len(container.Env) != 0 {
		t.Errorf("volumes/env = %+v %+v, want none for the simulator", pod.Volumes, container.Env)
	}
	if pod.Affinity.PodAffinity != nil {
		t.Error("simulator replicas share no volume, so they need no shared node")
	}
}

// A simulator naming no image runs the stack's, which the installer mirrors.
func TestDeploymentRunsTheStackSimulatorImage(t *testing.T) {
	model := vllmModel(t, "slug: sim\nmodel: {name: shaide/simulator}\nserver: {engine: sim}\n")
	container := build(t, model).Spec.Template.Spec.Containers[0]

	if container.Image != "harbor/services/llm-d/llm-d-inference-sim:v1.0.0" {
		t.Errorf("image = %q, want the stack's simulator image", container.Image)
	}
}

func TestSplitModelURI(t *testing.T) {
	for uri, want := range map[string][2]string{
		"hub/openai/gpt-oss-20b": {"hub", "openai/gpt-oss-20b"},
		"cache/hub/org/name":     {"cache/hub", "org/name"},
		"hub/model":              {"hub", "model"},
	} {
		cache, repository := splitModelURI(uri)
		if cache != want[0] || repository != want[1] {
			t.Errorf("splitModelURI(%q) = %q, %q, want %q", uri, cache, repository, want)
		}
	}
}

// Every packaged model must build: a definition the stack cannot deploy
// should fail here, not on a customer's cluster.
func TestPackagedModelsBuild(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "..", "app_serving", "deployments", "models")
	paths, err := filepath.Glob(filepath.Join(root, "*", "*", modeldef.FileName))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no packaged models found under %s: %v", root, err)
	}

	for _, path := range paths {
		def, err := modeldef.Load(path)
		if err != nil {
			t.Error(err)
			continue
		}

		model := appConfig.Model{
			Name:        filepath.Base(filepath.Dir(path)),
			Namespace:   "llm-d-" + def.Slug,
			ReleaseName: "infra-" + def.Slug,
			ModelPaths:  appConfig.ModelPaths{Slug: def.Slug},
			Definition:  def,
			ModelSource: &appConfig.ModelSource{ModelUri: "hub/" + def.Model.Name},
		}
		in := Input{Model: model, Category: filepath.Base(filepath.Dir(filepath.Dir(path))), Image: "vllm", SimulatorImage: "sim"}
		if def.Server.RuntimeMetadata {
			in.MetadataConfigMap = "metadata"
		}
		if _, err := Deployment(in); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
