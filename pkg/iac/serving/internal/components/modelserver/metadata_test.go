package modelserver

import (
	"path/filepath"
	"reflect"
	"testing"

	appConfig "github.com/axem-solutions/ai_platform/pkg/iac/serving/internal/config"
	"github.com/axem-solutions/ai_platform/pkg/iac/serving/modeldef"
	corev1 "k8s.io/api/core/v1"
)

func metadataPod() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: containerName,
			Args: []string{"--max-model-len", "100000", "--middleware", "existing.Extension"},
			Env: []corev1.EnvVar{
				{Name: "HF_HUB_OFFLINE", Value: "1"},
				{Name: "PYTHONPATH", Value: "/existing"},
			},
			VolumeMounts: []corev1.VolumeMount{{Name: "weights", MountPath: "/weights"}},
		}},
		Volumes: []corev1.Volume{{Name: "weights"}},
	}
}

func TestMetadataPreservesTheContainer(t *testing.T) {
	pod := metadataPod()
	if err := addRuntimeMetadata(&pod, "metadata-hash"); err != nil {
		t.Fatal(err)
	}

	container := pod.Containers[0]
	wantArgs := []string{"--max-model-len", "100000", "--middleware", "existing.Extension", "--middleware", metadataMiddleware}
	if !reflect.DeepEqual(container.Args, wantArgs) {
		t.Errorf("args = %q", container.Args)
	}

	wantEnv := []corev1.EnvVar{
		{Name: "HF_HUB_OFFLINE", Value: "1"},
		{Name: "PYTHONPATH", Value: metadataMount + ":/existing"},
		{Name: "VLLM_SERVER_DEV_MODE", Value: "0"},
	}
	if !reflect.DeepEqual(container.Env, wantEnv) {
		t.Errorf("env = %+v", container.Env)
	}

	if len(container.VolumeMounts) != 2 || !container.VolumeMounts[1].ReadOnly {
		t.Errorf("mounts = %+v, want the metadata mount added read-only", container.VolumeMounts)
	}
	if len(pod.Volumes) != 2 || pod.Volumes[1].ConfigMap.Name != "metadata-hash" {
		t.Errorf("volumes = %+v", pod.Volumes)
	}
}

func TestMetadataRejectsIncompatibleSettings(t *testing.T) {
	cases := map[string]func(*corev1.PodSpec){
		"dev mode on": func(pod *corev1.PodSpec) {
			pod.Containers[0].Env = append(pod.Containers[0].Env, corev1.EnvVar{Name: "VLLM_SERVER_DEV_MODE", Value: "1"})
		},
		"PYTHONPATH from a secret": func(pod *corev1.PodSpec) {
			pod.Containers[0].Env[1] = corev1.EnvVar{Name: "PYTHONPATH", ValueFrom: &corev1.EnvVarSource{}}
		},
		"reserved mount": func(pod *corev1.PodSpec) {
			pod.Containers[0].VolumeMounts[0].Name = metadataVolume
		},
		"reserved volume": func(pod *corev1.PodSpec) {
			pod.Volumes[0].Name = metadataVolume
		},
		"middleware already set": func(pod *corev1.PodSpec) {
			pod.Containers[0].Args = append(pod.Containers[0].Args, "--middleware", metadataMiddleware)
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			pod := metadataPod()
			mutate(&pod)
			if err := addRuntimeMetadata(&pod, "metadata"); err == nil {
				t.Error("accepted an incompatible configuration")
			}
		})
	}
}

func TestMetadataConfigMapNameFollowsTheModule(t *testing.T) {
	name := MetadataConfigMapName("model-a-very-long-model-name-that-goes-on-and-on")
	if len(name) > 63 {
		t.Errorf("name %q is longer than a Kubernetes name may be", name)
	}
	if MetadataModule()[metadataFile] != metadataModule {
		t.Error("ConfigMap content is not the module")
	}
}

// GPT-OSS-20B is the model that opts in; its packaged definition must deploy
// with the middleware.
func TestGPTOSSEnablesMetadata(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "..", "app_serving", "deployments", "models", "generative", "GPT-OSS-20B", modeldef.FileName)
	def, err := modeldef.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	model := appConfig.Model{
		Name:        "GPT-OSS-20B",
		Namespace:   "llm-d-gpt-oss-20b",
		ReleaseName: "infra-gpt-oss-20b",
		ModelPaths:  appConfig.ModelPaths{Slug: def.Slug},
		Definition:  def,
		ModelSource: &appConfig.ModelSource{ModelUri: "hub/openai/gpt-oss-20b"},
	}

	deployment, err := Deployment(Input{
		Model:             model,
		Category:          appConfig.CategoryGenerative,
		Image:             "vllm/vllm-openai:test",
		MetadataConfigMap: MetadataConfigMapName(model.ModelServerName()),
	})
	if err != nil {
		t.Fatal(err)
	}

	args := deployment.Spec.Template.Spec.Containers[0].Args
	if args[len(args)-1] != metadataMiddleware {
		t.Errorf("args = %q, want the metadata middleware", args)
	}
}
