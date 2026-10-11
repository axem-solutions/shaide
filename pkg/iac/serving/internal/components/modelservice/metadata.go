package modelservice

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"sigs.k8s.io/yaml"
)

//go:embed metadata/shaide_metadata.py
var metadataModule string

const metadataMount = "/opt/shaide-metadata"
const metadataMiddleware = "shaide_metadata.RuntimeMetadataMiddleware"

// metadataValues preserves each workload's arrays: Helm replaces arrays rather
// than merging them. Only explicitly opted-in vllmServe containers are changed.
func metadataValues(data []byte, configMapName string) (map[string]any, error) {
	var source struct {
		Shaide struct {
			RuntimeMetadata struct {
				Enabled bool `json:"enabled"`
			} `json:"runtimeMetadata"`
		} `json:"shaide"`
	}
	if err := yaml.Unmarshal(data, &source); err != nil {
		return nil, err
	}
	if !source.Shaide.RuntimeMetadata.Enabled {
		return nil, nil
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	result := map[string]any{}
	for _, role := range []string{"decode", "prefill"} {
		workload, ok := values[role].(map[string]any)
		if !ok || workload["create"] == false {
			continue
		}
		containers, _ := workload["containers"].([]any)
		changed := false
		for _, raw := range containers {
			container, ok := raw.(map[string]any)
			if !ok || container["modelCommand"] != "vllmServe" {
				continue
			}
			args, _ := container["args"].([]any)
			for _, arg := range args {
				if arg == metadataMiddleware {
					return nil, fmt.Errorf("%s: metadata middleware is managed by the platform", role)
				}
			}
			container["args"] = append(args, "--middleware", metadataMiddleware)
			env, _ := container["env"].([]any)
			pathSet := false
			devModeSet := false
			for _, rawEnv := range env {
				entry, ok := rawEnv.(map[string]any)
				if !ok {
					continue
				}
				if entry["name"] == "VLLM_SERVER_DEV_MODE" {
					if entry["value"] != "0" {
						return nil, fmt.Errorf("%s: disable VLLM_SERVER_DEV_MODE before enabling runtime metadata", role)
					}
					devModeSet = true
				}
				if entry["name"] == "PYTHONPATH" {
					value, ok := entry["value"].(string)
					if !ok {
						return nil, fmt.Errorf("%s: PYTHONPATH must be a literal value when runtime metadata is enabled", role)
					}
					entry["value"] = metadataMount + ":" + value
					pathSet = true
				}
			}
			if !pathSet {
				env = append(env, map[string]any{"name": "PYTHONPATH", "value": metadataMount})
			}
			if !devModeSet {
				env = append(env, map[string]any{"name": "VLLM_SERVER_DEV_MODE", "value": "0"})
			}
			container["env"] = env
			mounts, _ := container["volumeMounts"].([]any)
			for _, rawMount := range mounts {
				mount, _ := rawMount.(map[string]any)
				if mount["name"] == "shaide-metadata" || mount["mountPath"] == metadataMount {
					return nil, fmt.Errorf("%s: reserved shaide metadata mount is already in use", role)
				}
			}
			container["volumeMounts"] = append(mounts, map[string]any{"name": "shaide-metadata", "mountPath": metadataMount, "readOnly": true})
			changed = true
		}
		if changed {
			volumes, _ := workload["volumes"].([]any)
			for _, rawVolume := range volumes {
				volume, _ := rawVolume.(map[string]any)
				if volume["name"] == "shaide-metadata" {
					return nil, fmt.Errorf("%s: reserved shaide metadata volume is already in use", role)
				}
			}
			result[role] = map[string]any{"containers": containers, "volumes": append(volumes, map[string]any{"name": "shaide-metadata", "configMap": map[string]any{"name": configMapName}})}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("runtime metadata requires an enabled vllmServe container")
	}
	return result, nil
}

func configureMetadata(ctx *pulumi.Context, valuesPath, release, namespace string, overrides pulumi.Map, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, err
	}
	// A content-addressed name rolls pods when the embedded module changes;
	// importing a module from an updated volume alone would leave old code running.
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(metadataModule)))[:12]
	prefix := release
	if len(prefix) > 40 {
		prefix = prefix[:40]
	}
	name := strings.TrimRight(prefix, "-") + "-metadata-" + hash
	workloads, err := metadataValues(data, name)
	if err != nil || workloads == nil {
		return nil, err
	}
	for role, value := range workloads {
		target := overrides[role].(pulumi.Map)
		for key, item := range value.(map[string]any) {
			target[key] = pulumi.ToOutput(item)
		}
	}
	return corev1.NewConfigMap(ctx, release+"-runtime-metadata", &corev1.ConfigMapArgs{
		Metadata:  &metav1.ObjectMetaArgs{Name: pulumi.String(name), Namespace: pulumi.String(namespace)},
		Immutable: pulumi.Bool(true),
		Data:      pulumi.StringMap{"shaide_metadata.py": pulumi.String(metadataModule)},
	}, opts...)
}
