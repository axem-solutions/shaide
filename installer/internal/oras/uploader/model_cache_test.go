package uploader

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/axem-solutions/ai_platform/installer/internal/config/catalog"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
)

// cacheArtifact stores a model artifact with the given layers under ref, as
// the uploader's build does.
func cacheArtifact(t *testing.T, store *oci.Store, ref string, layers ...string) {
	t.Helper()
	ctx := context.Background()

	var descs []ocispec.Descriptor
	for _, layer := range layers {
		data := []byte(layer)
		desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageLayerGzip, data)
		if exists, _ := store.Exists(ctx, desc); !exists {
			if err := store.Push(ctx, desc, bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
		}
		descs = append(descs, desc)
	}

	manifest, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, modelArtifactType, oras.PackManifestOptions{Layers: descs})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Tag(ctx, manifest, ref); err != nil {
		t.Fatal(err)
	}
}

func cachedTags(t *testing.T, store *oci.Store) []string {
	t.Helper()

	var all []string
	if err := store.Tags(context.Background(), "", func(tags []string) error {
		all = append(all, tags...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(all)
	return all
}

func blobExists(t *testing.T, root, layer string) bool {
	t.Helper()

	desc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageLayerGzip, []byte(layer))
	_, err := os.Stat(filepath.Join(root, "blobs", desc.Digest.Algorithm().String(), desc.Digest.Encoded()))
	return err == nil
}

// A superseded tag of the model is removed with the blobs only it used; other
// models, and blobs they share, stay.
func TestPruneSupersededArtifacts(t *testing.T) {
	root := t.TempDir()
	store, err := oci.New(root)
	if err != nil {
		t.Fatal(err)
	}

	cacheArtifact(t, store, "ai-models/gpt-oss-20b:1.0.0", "old gpt-oss weights", "shared tokenizer")
	cacheArtifact(t, store, "ai-models/gpt-oss-20b-large:6cee5e81ee83", "large weights")
	cacheArtifact(t, store, "ai-models/bge-m3:5617a9f61b02", "bge weights", "shared tokenizer")

	var logged []string
	u := &Uploader{logf: func(format string, args ...any) { logged = append(logged, format) }}
	model := catalog.Model{HarborProject: "ai-models", HarborName: "gpt-oss-20b", HarborTag: "6cee5e81ee83"}

	if err := u.pruneSupersededArtifacts(context.Background(), store, model, "ai-models/gpt-oss-20b:6cee5e81ee83"); err != nil {
		t.Fatalf("pruneSupersededArtifacts: %v", err)
	}

	want := []string{"ai-models/bge-m3:5617a9f61b02", "ai-models/gpt-oss-20b-large:6cee5e81ee83"}
	if got := cachedTags(t, store); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tags = %v, want %v", got, want)
	}

	for layer, want := range map[string]bool{
		"old gpt-oss weights": false,
		"shared tokenizer":    true,
		"large weights":       true,
		"bge weights":         true,
	} {
		if got := blobExists(t, root, layer); got != want {
			t.Errorf("blob %q exists = %v, want %v", layer, got, want)
		}
	}

	if len(logged) != 1 {
		t.Errorf("logged %d messages, want the removal logged once", len(logged))
	}
}

// With only the current tag cached there is nothing to prune.
func TestPruneSupersededArtifactsKeepsTheCurrentTag(t *testing.T) {
	root := t.TempDir()
	store, err := oci.New(root)
	if err != nil {
		t.Fatal(err)
	}
	cacheArtifact(t, store, "ai-models/gpt-oss-20b:6cee5e81ee83", "weights")

	u := &Uploader{logf: func(string, ...any) {}}
	model := catalog.Model{HarborProject: "ai-models", HarborName: "gpt-oss-20b", HarborTag: "6cee5e81ee83"}

	if err := u.pruneSupersededArtifacts(context.Background(), store, model, "ai-models/gpt-oss-20b:6cee5e81ee83"); err != nil {
		t.Fatal(err)
	}
	if got := cachedTags(t, store); len(got) != 1 || !blobExists(t, root, "weights") {
		t.Errorf("tags = %v, weights kept = %v, want the current artifact untouched", got, blobExists(t, root, "weights"))
	}
}
