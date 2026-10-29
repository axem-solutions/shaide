package files

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The fixtures in testdata/ list the files of each catalog model's repository
// at its pinned revision, from the Hugging Face tree API.
func TestSelectCatalogModels(t *testing.T) {
	onnx := func(names ...string) []string {
		out := make([]string, 0, len(names))
		for _, name := range names {
			out = append(out, "onnx/"+name)
		}
		return out
	}

	tests := map[string][]string{
		"BAAI_bge-m3": onnx(
			"Constant_7_attr__value",
			"config.json",
			"model.onnx",
			"model.onnx_data",
			"sentencepiece.bpe.model",
			"special_tokens_map.json",
			"tokenizer.json",
			"tokenizer_config.json",
		),
		"Qwen_Qwen3-Coder-30B-A3B-Instruct":           nil,
		"Qwen_Qwen3.5-27B-FP8":                        nil,
		"deepseek-ai_DeepSeek-Coder-V2-Lite-Instruct": nil,
		// The Dense modules' safetensors are loaded by the pooling runner.
		"google_embeddinggemma-300m": nil,
		"google_gemma-4-31B-it":      nil,
		"jinaai_jina-embeddings-v5-text-small-retrieval": append(onnx("model.onnx", "model.onnx_data"),
			"v5-small-retrieval-F16.gguf",
			"v5-small-retrieval-IQ1_M.gguf",
			"v5-small-retrieval-IQ1_S.gguf",
			"v5-small-retrieval-IQ2_M.gguf",
			"v5-small-retrieval-IQ2_XXS.gguf",
			"v5-small-retrieval-IQ4_NL.gguf",
			"v5-small-retrieval-IQ4_XS.gguf",
			"v5-small-retrieval-Q2_K.gguf",
			"v5-small-retrieval-Q3_K_M.gguf",
			"v5-small-retrieval-Q4_K_M.gguf",
			"v5-small-retrieval-Q5_K_M.gguf",
			"v5-small-retrieval-Q5_K_S.gguf",
			"v5-small-retrieval-Q6_K.gguf",
			"v5-small-retrieval-Q8_0.gguf",
		),
		// vLLM loads the consolidated files in Mistral format.
		"mistralai_Devstral-Small-2-24B-Instruct-2512": {
			"model-00001-of-00006.safetensors",
			"model-00002-of-00006.safetensors",
			"model-00003-of-00006.safetensors",
			"model-00004-of-00006.safetensors",
			"model-00005-of-00006.safetensors",
			"model-00006-of-00006.safetensors",
		},
		"nomic-ai_nomic-embed-text-v1.5": onnx(
			"model.onnx",
			"model_bnb4.onnx",
			"model_fp16.onnx",
			"model_int8.onnx",
			"model_q4.onnx",
			"model_q4f16.onnx",
			"model_quantized.onnx",
			"model_uint8.onnx",
		),
		"openai_gpt-oss-20b": {
			"metal/model.bin",
			"original/config.json",
			"original/dtypes.json",
			"original/model.safetensors",
		},
		"zai-org_GLM-4.7-Flash": nil,
	}

	for name, wantSkipped := range tests {
		t.Run(name, func(t *testing.T) {
			paths := readFixture(t, name)

			skipped := skippedBy(paths, Select(paths))

			slices.Sort(wantSkipped)
			if !slices.Equal(skipped, wantSkipped) {
				t.Errorf("skipped:\n  %s\nwant:\n  %s",
					strings.Join(skipped, "\n  "), strings.Join(wantSkipped, "\n  "))
			}
		})
	}
}

func TestSelectFixturesAreCovered(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 11 {
		t.Errorf("found %d fixtures; add new ones to TestSelectCatalogModels", len(fixtures))
	}
}

func TestSelectWeightFormats(t *testing.T) {
	tests := []struct {
		name        string
		paths       []string
		wantSkipped []string
	}{
		{
			name:        "bin weights without safetensors are kept",
			paths:       []string{"config.json", "pytorch_model.bin", "colbert_linear.pt"},
			wantSkipped: nil,
		},
		{
			name:        "bin weights next to safetensors are skipped",
			paths:       []string{"model.safetensors", "pytorch_model.bin", "training_args.bin"},
			wantSkipped: []string{"pytorch_model.bin", "training_args.bin"},
		},
		{
			name: "a module directory keeps its bin without a safetensors",
			paths: []string{
				"model.safetensors",
				"2_Dense/config.json", "2_Dense/pytorch_model.bin",
				"3_Dense/model.safetensors", "3_Dense/pytorch_model.bin",
			},
			wantSkipped: []string{"3_Dense/pytorch_model.bin"},
		},
		{
			name:        "Flax and TensorFlow weights are skipped",
			paths:       []string{"model.safetensors", "flax_model.msgpack", "tf_model.h5", "rust_model.ot"},
			wantSkipped: []string{"flax_model.msgpack", "rust_model.ot", "tf_model.h5"},
		},
		{
			name:        "a file named like a skipped directory is kept",
			paths:       []string{"model.safetensors", "original"},
			wantSkipped: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skipped := skippedBy(tt.paths, Select(tt.paths))
			if !slices.Equal(skipped, tt.wantSkipped) {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}
		})
	}
}

func readFixture(t *testing.T, name string) []string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("testdata", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(content))
}

// skippedBy returns the sorted paths that selected left out.
func skippedBy(paths, selected []string) []string {
	var skipped []string
	for _, p := range paths {
		if !slices.Contains(selected, p) {
			skipped = append(skipped, p)
		}
	}
	slices.Sort(skipped)
	return skipped
}
