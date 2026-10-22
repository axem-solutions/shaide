package modelserver

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

//go:embed metadata/shaide_metadata.py
var metadataModule string

const (
	metadataMount      = "/opt/shaide-metadata"
	metadataMiddleware = "shaide_metadata.RuntimeMetadataMiddleware"
	metadataVolume     = "shaide-metadata"
	metadataFile       = "shaide_metadata.py"
)

// MetadataConfigMapName is content addressed: a new module is a new
// ConfigMap, so the pod template changes and the pods roll. Importing a module
// from an updated volume alone would leave the old code running.
func MetadataConfigMapName(model string) string {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(metadataModule)))[:12]
	prefix := model
	if len(prefix) > 40 {
		prefix = prefix[:40]
	}
	return strings.TrimRight(prefix, "-") + "-metadata-" + hash
}

// MetadataModule is the ConfigMap's content.
func MetadataModule() map[string]string {
	return map[string]string{metadataFile: metadataModule}
}

// addRuntimeMetadata loads the platform's vLLM middleware, which serves
// GET /v1/shaide/metadata, into the model server container. Everything the
// model definition sets is kept; settings that would conflict with the
// middleware are rejected rather than overridden.
func addRuntimeMetadata(pod *corev1.PodSpec, configMapName string) error {
	container := &pod.Containers[0]

	for _, arg := range container.Args {
		if arg == metadataMiddleware {
			return fmt.Errorf("server.args: the metadata middleware is managed by the platform")
		}
	}
	container.Args = append(container.Args, "--middleware", metadataMiddleware)

	pathSet := false
	devModeSet := false
	for i := range container.Env {
		env := &container.Env[i]
		switch env.Name {
		case "VLLM_SERVER_DEV_MODE":
			if env.ValueFrom != nil || env.Value != "0" {
				return fmt.Errorf("server.env: disable VLLM_SERVER_DEV_MODE before enabling runtime metadata")
			}
			devModeSet = true
		case "PYTHONPATH":
			if env.ValueFrom != nil {
				return fmt.Errorf("server.env: PYTHONPATH must be a literal value when runtime metadata is enabled")
			}
			env.Value = metadataMount + ":" + env.Value
			pathSet = true
		}
	}
	if !pathSet {
		container.Env = append(container.Env, corev1.EnvVar{Name: "PYTHONPATH", Value: metadataMount})
	}
	if !devModeSet {
		container.Env = append(container.Env, corev1.EnvVar{Name: "VLLM_SERVER_DEV_MODE", Value: "0"})
	}

	for _, mount := range container.VolumeMounts {
		if mount.Name == metadataVolume || mount.MountPath == metadataMount {
			return fmt.Errorf("server.volumeMounts: the shaide metadata mount is reserved")
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      metadataVolume,
		MountPath: metadataMount,
		ReadOnly:  true,
	})

	for _, volume := range pod.Volumes {
		if volume.Name == metadataVolume {
			return fmt.Errorf("server.volumes: the shaide metadata volume is reserved")
		}
	}
	pod.Volumes = append(pod.Volumes, corev1.Volume{
		Name: metadataVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
			},
		},
	})

	return nil
}
