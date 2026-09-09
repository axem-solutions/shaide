package config

import (
	kubernetes "github.com/axem-solutions/ai_platform/pkg/kube/connection"
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Toleration struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Effect   string `json:"effect"`
}

type ModelSourceInput struct {
	HarborRef    string `json:"harborRef"`    // Harbor OCI ref, e.g. "harbor.../ai-models/nomic:1.5.0"
	ModelUri     string `json:"modelUri"`     // path within the PVC, e.g. "hub/org/model-name"
	StorageSize  string `json:"storageSize"`  // e.g. "5Gi"
	StorageClass string `json:"storageClass"` // optional: overrides cluster default StorageClass (e.g. "hyperdisk-balanced" for g4 nodes)
	HostpathNode string `json:"hostpathNode"` // on-prem only: node hostname for the hostpath PV
	HostpathDir  string `json:"hostpathDir"`  // on-prem only: absolute path on the node; defaults to /var/lib/hostpath/models/<slug>
}

type ModelInput struct {
	Name         string            `json:"name"`
	NodeSelector map[string]string `json:"nodeSelector"`
	Enabled      bool              `json:"enabled"`
	RelaseName   string            `json:"releaseName"`
	NameSpace    string            `json:"nameSpace"`
	ModelSource  *ModelSourceInput `json:"modelSource"`
}

type ModelsInput struct {
	Generative []ModelInput `json:"generative"`
	Embedder   []ModelInput `json:"embedder"`
}

type stackInput struct {
	Models ModelsInput

	LLMdChartPath string

	HarborHostname string
	HarborUser     string
	HarborToken    pulumi.StringOutput
	HarborTokenSet bool

	Platform   platform.Platform
	Kubernetes kubernetes.Connection

	GPUToleration *Toleration // optional; injected into model pods via extraConfig

	// ModelStorageClass is the default StorageClass applied to a model's PVC
	// when the model's own modelSource.storageClass is empty. Useful for
	// clusters where the cluster-default StorageClass is incompatible with
	// the target GPU node's machine type (e.g. GKE g4-standard-48 requires
	// hyperdisk-balanced, not the default pd-ssd). Optional; ignored when
	// Platform is on-prem (which always uses hostpath).
	ModelStorageClass string
}
