package modelservice

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const metadataFixture = `
shaide:
  runtimeMetadata:
    enabled: true
decode:
  create: true
  containers:
    - name: vllm
      modelCommand: vllmServe
      args: [--max-model-len, "100000", --middleware, existing.Extension]
      env:
        - {name: HF_HUB_OFFLINE, value: "1"}
        - {name: PYTHONPATH, value: /existing}
      volumeMounts:
        - {name: weights, mountPath: /weights}
      resources:
        limits: {nvidia.com/gpu: "1"}
    - name: metrics
      image: unrelated
  volumes:
    - {name: weights, emptyDir: {}}
prefill:
  create: false
  containers:
    - {name: vllm, modelCommand: vllmServe}
`

func TestMetadataPreservesWorkloadConfiguration(t *testing.T) {
	result, err := metadataValues([]byte(metadataFixture), "metadata-hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result["prefill"]; ok {
		t.Fatal("modified disabled prefill")
	}
	decode := result["decode"].(map[string]any)
	containers := decode["containers"].([]any)
	if len(containers) != 2 || !reflect.DeepEqual(containers[1], map[string]any{"name": "metrics", "image": "unrelated"}) {
		t.Fatal("unrelated container modified")
	}
	vllm := containers[0].(map[string]any)
	want := []any{"--max-model-len", "100000", "--middleware", "existing.Extension", "--middleware", metadataMiddleware}
	if !reflect.DeepEqual(vllm["args"], want) {
		t.Fatalf("args: %#v", vllm["args"])
	}
	env := vllm["env"].([]any)
	if len(env) != 3 || env[0].(map[string]any)["value"] != "1" || env[1].(map[string]any)["value"] != metadataMount+":/existing" || env[2].(map[string]any)["value"] != "0" {
		t.Fatalf("env: %#v", env)
	}
	if vllm["resources"] == nil || len(vllm["volumeMounts"].([]any)) != 2 || len(decode["volumes"].([]any)) != 2 {
		t.Fatal("lost resources or mounts")
	}
	mount := vllm["volumeMounts"].([]any)[1].(map[string]any)
	if mount["readOnly"] != true {
		t.Fatal("metadata mount must be read-only")
	}
}

func TestMetadataIsOptInAndRejectsIncompatibleSettings(t *testing.T) {
	result, err := metadataValues([]byte(strings.Replace(metadataFixture, "enabled: true", "enabled: false", 1)), "unused")
	if err != nil || result != nil {
		t.Fatalf("disabled: %v %v", result, err)
	}
	for _, data := range []string{
		strings.Replace(metadataFixture, "vllmServe", "simulator", 1),
		strings.Replace(metadataFixture, "name: PYTHONPATH, value: /existing", "name: VLLM_SERVER_DEV_MODE, value: \"1\"", 1),
		strings.Replace(metadataFixture, "name: PYTHONPATH, value: /existing", "name: PYTHONPATH, valueFrom: {secretKeyRef: {name: secret, key: path}}", 1),
		strings.Replace(metadataFixture, "name: weights, mountPath: /weights", "name: shaide-metadata, mountPath: /weights", 1),
	} {
		if _, err := metadataValues([]byte(data), "metadata"); err == nil {
			t.Fatal("accepted incompatible configuration")
		}
	}
}

func TestGPTOSSValuesEnableMetadata(t *testing.T) {
	data, err := os.ReadFile("../../../../../../app_serving/deployments/models/generative/GPT-OSS-20B/ms-gpt-oss-20b/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	result, err := metadataValues(data, "gpt-metadata")
	if err != nil || result["decode"] == nil {
		t.Fatalf("GPT-OSS metadata: %v %v", result, err)
	}
}
