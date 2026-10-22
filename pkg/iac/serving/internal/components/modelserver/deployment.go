// Package modelserver deploys a model's server pods. It replaces the retired
// llm-d-modelservice chart with a plain Deployment shaped like the llm-d v1.0
// single-host decode recipe (guides/recipes/modelserver/base/single-host).
package modelserver

import (
	"fmt"
	"path"
	"strings"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// Port is where the model server serves the OpenAI-compatible API, and
	// the port the InferencePool targets.
	Port     = 8000
	portName = "modelserver"

	containerName = "modelserver"

	// The weights volume is mounted here. The ORAS pull Job lays the
	// Hugging Face cache out under hub/ in the same volume.
	modelVolumeName = "model-storage"
	modelMountPath  = "/model-cache"

	// Labels the router selects on. llm-d.ai/role: decode marks an
	// aggregated server; the router's decode filter also accepts it.
	ModelLabel = "llm-d.ai/model"
	RoleLabel  = "llm-d.ai/role"
	roleDecode = "decode"

	// Graceful shutdown, from the llm-d v1.0 graceful-shutdown component:
	// the pod leaves the pool during the preStop sleep, then vLLM finishes
	// in-flight requests for up to shutdownTimeout before it is killed.
	preStopSleepSeconds           = 15
	shutdownTimeoutSeconds        = 45
	terminationGracePeriodSeconds = 75

	// startupSeconds bounds loading weights and compiling kernels on a
	// fresh node.
	startupProbePeriodSeconds = 10
	startupProbeFailures      = 180

	// timeoutAnnotation lets Pulumi wait as long as the startup probe does
	// before it reports the Deployment as failed.
	timeoutAnnotation = "pulumi.com/timeoutSeconds"
	timeoutSeconds    = "1800"
)

// platformFlags are set by the platform; a model definition must not repeat
// them, as vLLM would take whichever comes last.
var platformFlags = []string{"--port", "--served-model-name", "--shutdown-timeout"}

// Input is everything the Deployment derives from.
type Input struct {
	Model    appConfig.Model
	Category string

	// Image is the stack's vLLM image, used unless the model names its own.
	Image string

	// SimulatorImage is the stack's simulator image, used for engine sim
	// unless the model names its own.
	SimulatorImage string

	// Toleration is the stack's GPU toleration, or nil.
	Toleration *appConfig.Toleration

	// PullSecret names the image pull secret, or is empty.
	PullSecret string

	// MetadataConfigMap holds the runtime metadata module. Required when
	// the model enables runtime metadata.
	MetadataConfigMap string
}

// PodSelector is the label set the Deployment, the InferencePool and the
// embedding Service select the model's pods by.
func PodSelector(model appConfig.Model) map[string]string {
	return map[string]string{
		ModelLabel: model.ModelServerName(),
		RoleLabel:  roleDecode,
	}
}

// Deployment builds the model server Deployment.
func Deployment(in Input) (*appsv1.Deployment, error) {
	def := in.Model.Definition

	container, err := buildContainer(in)
	if err != nil {
		return nil, err
	}

	volumes := append([]corev1.Volume(nil), def.Server.Volumes...)
	if volume, ok := modelVolume(in.Model); ok {
		volumes = append(volumes, volume)
	}

	pod := corev1.PodSpec{
		Containers: []corev1.Container{container},
		Volumes:    volumes,
		Affinity:   affinity(in.Model),
		// Service links inject VLLM_*-looking variables for every Service in
		// the namespace, which vLLM reads as its own settings.
		EnableServiceLinks:            ptr(false),
		AutomountServiceAccountToken:  ptr(false),
		TerminationGracePeriodSeconds: ptr(int64(terminationGracePeriodSeconds)),
		Tolerations:                   tolerations(in.Toleration, def.Server.Tolerations),
	}

	if in.PullSecret != "" {
		pod.ImagePullSecrets = []corev1.LocalObjectReference{{Name: in.PullSecret}}
	}

	if def.Server.RuntimeMetadata {
		if in.MetadataConfigMap == "" {
			return nil, fmt.Errorf("runtime metadata needs its ConfigMap")
		}
		if err := addRuntimeMetadata(&pod, in.MetadataConfigMap); err != nil {
			return nil, err
		}
	}

	selector := PodSelector(in.Model)
	podLabels := map[string]string{}
	for key, value := range metaLabels(in.Model, in.Category) {
		podLabels[key] = value
	}
	for key, value := range selector {
		podLabels[key] = value
	}

	replicas := def.ServerReplicas()

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        in.Model.ModelServerName(),
			Namespace:   in.Model.Namespace,
			Labels:      metaLabels(in.Model, in.Category),
			Annotations: map[string]string{timeoutAnnotation: timeoutSeconds},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Strategy: strategy(replicas),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec:       pod,
			},
		},
	}, nil
}

func buildContainer(in Input) (corev1.Container, error) {
	def := in.Model.Definition

	for _, arg := range def.Server.Args {
		for _, flag := range platformFlags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return corev1.Container{}, fmt.Errorf("server.args: %s is set by the platform", flag)
			}
		}
	}

	image := def.Server.Image
	if image == "" {
		image = in.Image
		if def.Simulated() {
			image = in.SimulatorImage
		}
	}
	if image == "" {
		return corev1.Container{}, fmt.Errorf("no image for engine %q", def.Server.Engine)
	}

	container := corev1.Container{
		Name:  containerName,
		Image: image,
		Ports: []corev1.ContainerPort{{
			Name:          portName,
			ContainerPort: Port,
			Protocol:      corev1.ProtocolTCP,
		}},
		Resources:    def.Server.Resources,
		VolumeMounts: append([]corev1.VolumeMount(nil), def.Server.VolumeMounts...),
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Sleep: &corev1.SleepAction{Seconds: preStopSleepSeconds},
			},
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler:     httpGet("/health"),
			PeriodSeconds:    10,
			TimeoutSeconds:   5,
			FailureThreshold: 3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler:     httpGet("/v1/models"),
			PeriodSeconds:    5,
			TimeoutSeconds:   2,
			FailureThreshold: 3,
		},
		StartupProbe: &corev1.Probe{
			ProbeHandler:     httpGet("/v1/models"),
			PeriodSeconds:    startupProbePeriodSeconds,
			TimeoutSeconds:   5,
			FailureThreshold: startupProbeFailures,
		},
	}

	if def.Server.StartupProbe != nil {
		container.StartupProbe = def.Server.StartupProbe.DeepCopy()
	}

	var platformEnv []corev1.EnvVar

	switch def.Server.Engine {
	case modeldef.EngineSim:
		// The simulator's own entrypoint takes vLLM's flags.
		container.Args = append([]string{
			"--model", def.Model.Name,
			"--port", fmt.Sprint(Port),
		}, def.Server.Args...)

	default:
		modelArg, env := weightsLocation(in.Model)
		platformEnv = env

		container.Command = []string{"vllm", "serve"}
		container.Args = append([]string{
			modelArg,
			"--port", fmt.Sprint(Port),
			"--served-model-name", def.Model.Name,
			fmt.Sprintf("--shutdown-timeout=%d", shutdownTimeoutSeconds),
		}, def.Server.Args...)

		if _, ok := modelVolume(in.Model); ok {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      modelVolumeName,
				MountPath: modelMountPath,
				ReadOnly:  in.Model.ModelSource != nil,
			})
		}
	}

	container.Env = mergeEnv(platformEnv, def.Server.Env)

	return container, nil
}

// weightsLocation is the model argument vLLM loads and the environment that
// points it at the weights. Weights pulled into the model's volume are read
// offline from its Hugging Face cache; otherwise vLLM downloads them into an
// emptyDir.
func weightsLocation(model appConfig.Model) (string, []corev1.EnvVar) {
	source := model.ModelSource
	if source == nil || source.ModelUri == "" {
		return model.Definition.Model.Name, []corev1.EnvVar{
			{Name: "HF_HOME", Value: modelMountPath},
		}
	}

	// ModelUri is the repository inside the volume's cache, hub/<org>/<name>:
	// the cache is hub/, the model argument <org>/<name>.
	cache, repository := splitModelURI(source.ModelUri)

	return repository, []corev1.EnvVar{
		{Name: "HF_HUB_CACHE", Value: path.Join(modelMountPath, cache)},
		{Name: "HF_HUB_OFFLINE", Value: "1"},
	}
}

// splitModelURI splits "hub/org/name" into "hub" and "org/name". A URI with
// fewer segments is a repository without an owner, directly in the cache.
func splitModelURI(uri string) (cache, repository string) {
	segments := strings.Split(strings.Trim(uri, "/"), "/")
	if len(segments) < 3 {
		return segments[0], strings.Join(segments[1:], "/")
	}

	cut := len(segments) - 2
	return strings.Join(segments[:cut], "/"), strings.Join(segments[cut:], "/")
}

// modelVolume is the weights volume: the model's PVC, or an emptyDir vLLM
// downloads into. The simulator needs neither.
func modelVolume(model appConfig.Model) (corev1.Volume, bool) {
	if model.Definition.Server.Engine == modeldef.EngineSim {
		return corev1.Volume{}, false
	}

	if model.ModelSource != nil {
		return corev1.Volume{
			Name: modelVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: model.ClaimName(),
					ReadOnly:  true,
				},
			},
		}, true
	}

	volume := corev1.Volume{
		Name:         modelVolumeName,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
	if size, err := resource.ParseQuantity(model.Definition.Model.Size); err == nil {
		volume.EmptyDir.SizeLimit = &size
	}
	return volume, true
}

// affinity places the pods on the model's nodes. Replicas sharing the
// weights volume must also share a node: the volume is ReadWriteOnce, so a
// replica on another node could never attach it.
func affinity(model appConfig.Model) *corev1.Affinity {
	result := model.NodeAffinity()

	if model.ModelSource == nil || model.Definition.ServerReplicas() <= 1 || model.Definition.Server.Engine == modeldef.EngineSim {
		return result
	}

	if result == nil {
		result = &corev1.Affinity{}
	}
	result.PodAffinity = &corev1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{MatchLabels: PodSelector(model)},
			TopologyKey:   "kubernetes.io/hostname",
		}},
	}
	return result
}

// strategy replaces pods without needing spare GPUs: a single replica is
// recreated, as a rolling update would wait for a GPU the old pod holds;
// several replicas roll one at a time, so the model keeps serving.
func strategy(replicas int32) appsv1.DeploymentStrategy {
	if replicas <= 1 {
		return appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	}

	maxUnavailable := intstr.FromInt32(1)
	maxSurge := intstr.FromInt32(0)
	return appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &maxUnavailable,
			MaxSurge:       &maxSurge,
		},
	}
}

func tolerations(stack *appConfig.Toleration, model []corev1.Toleration) []corev1.Toleration {
	var result []corev1.Toleration
	if stack != nil {
		result = append(result, corev1.Toleration{
			Key:      stack.Key,
			Operator: corev1.TolerationOperator(stack.Operator),
			Value:    stack.Value,
			Effect:   corev1.TaintEffect(stack.Effect),
		})
	}
	return append(result, model...)
}

// mergeEnv puts the platform's variables first; a model's own variable of the
// same name replaces the platform's.
func mergeEnv(platform, model []corev1.EnvVar) []corev1.EnvVar {
	overridden := map[string]bool{}
	for _, env := range model {
		overridden[env.Name] = true
	}

	var result []corev1.EnvVar
	for _, env := range platform {
		if !overridden[env.Name] {
			result = append(result, env)
		}
	}
	return append(result, model...)
}

func httpGet(path string) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromString(portName)},
	}
}

func metaLabels(model appConfig.Model, category string) map[string]string {
	labels := map[string]string{}
	for key, value := range model.MetaLabelValues(category) {
		labels[key] = value
	}
	labels["app.kubernetes.io/name"] = model.ModelServerName()
	return labels
}

func ptr[T any](value T) *T {
	return &value
}
