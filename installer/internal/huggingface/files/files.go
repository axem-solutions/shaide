// Package files picks the files of a Hugging Face model repository that the
// model server loads, so the installer downloads, packs and mirrors those
// alone.
//
// The rules follow vLLM v0.31 with its default "auto" load and config formats:
//
//   - vLLM reads weights from the repository root only. When
//     consolidated*.safetensors exist it loads them in Mistral format, else it
//     takes *.safetensors, or *.bin when there are none. The other root
//     safetensors and .bin files are never read.
//   - Root .pt files are kept: models load named ones as extra heads, such as
//     BGE-M3's sparse_linear.pt and colbert_linear.pt.
//   - original/ is vLLM's default ignore pattern. metal/, onnx/, openvino/ and
//     coreml/ hold the same weights for other runtimes, as do GGUF, ONNX,
//     Flax, TensorFlow and TFLite files anywhere in the repository.
//   - Sentence-transformers module directories such as 2_Dense/ are loaded by
//     vLLM's pooling runner, which prefers model.safetensors over
//     pytorch_model.bin. They are kept, without a .bin next to a safetensors.
//
// Everything else, including configs, tokenizers and remote code, is kept.
package files

import (
	"path"
	"strings"
)

// skippedDirs are top-level directories that hold weights for runtimes other
// than vLLM.
var skippedDirs = map[string]bool{
	"original": true,
	"metal":    true,
	"onnx":     true,
	"openvino": true,
	"coreml":   true,
}

// skippedExts are weight formats vLLM does not load, wherever they are.
var skippedExts = map[string]bool{
	".gguf":      true,
	".onnx":      true,
	".onnx_data": true,
	".msgpack":   true,
	".h5":        true,
	".tflite":    true,
	".ot":        true,
	".mlmodel":   true,
}

const (
	safetensorsExt = ".safetensors"
	consolidated   = "consolidated"
)

// weightExts are the weight formats a repository can carry twice, in the
// order vLLM prefers them.
var weightExts = []string{safetensorsExt, ".bin"}

// Select returns the paths vLLM loads, in their input order. Paths are
// repository-relative and slash-separated, as the Hugging Face API lists them.
func Select(paths []string) []string {
	root := rootWeights(paths)
	withSafetensors := dirsWithSafetensors(paths)

	selected := make([]string, 0, len(paths))
	for _, p := range paths {
		if keep(p, root, withSafetensors) {
			selected = append(selected, p)
		}
	}

	return selected
}

// rootWeightKind is the root weight set vLLM loads.
type rootWeightKind struct {
	ext          string
	consolidated bool
}

func rootWeights(paths []string) rootWeightKind {
	for _, p := range paths {
		if isRoot(p) && isConsolidated(p) {
			return rootWeightKind{ext: safetensorsExt, consolidated: true}
		}
	}

	for _, ext := range weightExts {
		for _, p := range paths {
			if isRoot(p) && path.Ext(p) == ext {
				return rootWeightKind{ext: ext}
			}
		}
	}

	return rootWeightKind{}
}

func dirsWithSafetensors(paths []string) map[string]bool {
	dirs := map[string]bool{}
	for _, p := range paths {
		if path.Ext(p) == safetensorsExt {
			dirs[path.Dir(p)] = true
		}
	}
	return dirs
}

func keep(p string, root rootWeightKind, withSafetensors map[string]bool) bool {
	if first, _, nested := strings.Cut(p, "/"); nested && skippedDirs[first] {
		return false
	}

	ext := path.Ext(p)
	if skippedExts[ext] {
		return false
	}
	if !isWeight(ext) {
		return true
	}

	if isRoot(p) {
		if ext != root.ext {
			return false
		}
		return !root.consolidated || isConsolidated(p)
	}

	// A module directory: vLLM reads its safetensors when there is one.
	return ext == safetensorsExt || !withSafetensors[path.Dir(p)]
}

func isWeight(ext string) bool {
	for _, weightExt := range weightExts {
		if ext == weightExt {
			return true
		}
	}
	return false
}

func isRoot(p string) bool {
	return !strings.Contains(p, "/")
}

func isConsolidated(p string) bool {
	return strings.HasPrefix(p, consolidated) && path.Ext(p) == safetensorsExt
}
