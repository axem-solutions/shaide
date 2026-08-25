package config

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type cloudProvider string

const (
	cloud  cloudProvider = "cloud"
	onPrem cloudProvider = "on-prem"
)

type Toleration struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
	Effect   string `json:"effect"`
}

type modelSourceInput struct {
	HarborRef    string `json:"harborRef"`    // Harbor OCI ref, e.g. "harbor.../ai-models/nomic:1.5.0"
	ModelUri     string `json:"modelUri"`     // path within the PVC, e.g. "hub/org/model-name"
	StorageSize  string `json:"storageSize"`  // e.g. "5Gi"
	StorageClass string `json:"storageClass"` // optional: overrides cluster default StorageClass (e.g. "hyperdisk-balanced" for g4 nodes)
	HostpathNode string `json:"hostpathNode"` // on-prem only: node hostname for the hostpath PV
	HostpathDir  string `json:"hostpathDir"`  // on-prem only: absolute path on the node; defaults to /var/lib/hostpath/models/<slug>
}

type modelInput struct {
	Name         string            `json:"name"`
	NodeSelector map[string]string `json:"nodeSelector"`
	Enabled      bool              `json:"enabled"`
	RelaseName   string            `json:"releaseName"`
	NameSpace    string            `json:"nameSpace"`
	ModelSource  *modelSourceInput `json:"modelSource"`
}

type modelsInput struct {
	Generative []modelInput `json:"generative"`
	Embedder   []modelInput `json:"embedder"`
}

type stackInput struct {
	Models modelsInput

	LLMdChartPath string

	HarborHostname string
	HarborUser     string
	HarborToken    pulumi.StringOutput
	HarborTokenSet bool

	CloudProvider cloudProvider
	Kubeconfig    string
	GPUToleration *Toleration // optional; injected into model pods via extraConfig

	// ModelStorageClass is the default StorageClass applied to a model's PVC
	// when the model's own modelSource.storageClass is empty. Useful for
	// clusters where the cluster-default StorageClass is incompatible with
	// the target GPU node's machine type (e.g. GKE g4-standard-48 requires
	// hyperdisk-balanced, not the default pd-ssd). Optional; ignored when
	// CloudProvider is on-prem (which always uses hostpath).
	ModelStorageClass string
}

func (in stackInput) hasModelSource() bool {
	for _, m := range in.Models.Generative {
		if m.ModelSource != nil {
			return true
		}
	}
	for _, m := range in.Models.Embedder {
		if m.ModelSource != nil {
			return true
		}
	}
	return false
}

func (in stackInput) validateStack() error {
	switch in.CloudProvider {
	case cloud:
		// Harbor credentials are also required on cloud when any model pulls
		// from Harbor (modelSource set). Without them the ORAS pull Job cannot
		// authenticate and fails at runtime rather than at pulumi up time.
		if in.hasModelSource() {
			if in.HarborHostname == "" {
				return fmt.Errorf("harborHostname is required when modelSource is set")
			}
			if in.HarborUser == "" {
				return fmt.Errorf("harborUser is required when modelSource is set")
			}
			if !in.HarborTokenSet {
				return fmt.Errorf("harborToken is required when modelSource is set")
			}
		}
		return nil

	case onPrem:
		if in.Kubeconfig == "" {
			return fmt.Errorf("kubeconfig is required for %q", in.CloudProvider)
		}
		if in.HarborHostname == "" {
			return fmt.Errorf("harborHostname is required for %q", in.CloudProvider)
		}
		if in.HarborUser == "" {
			return fmt.Errorf("harborUser is required for %q", in.CloudProvider)
		}
		if !in.HarborTokenSet {
			return fmt.Errorf("harborToken is required for %q", in.CloudProvider)
		}
		return nil

	default:
		return fmt.Errorf("invalid cloudProvider %q", in.CloudProvider)
	}
}

func loadStack(ctx *pulumi.Context) (stackInput, error) {
	cfg := pulumiconfig.New(ctx, "")

	input := stackInput{
		Kubeconfig:        cfg.Get("kubeconfig"),
		HarborHostname:    cfg.Get("harborHostname"),
		HarborUser:        cfg.Get("harborUser"),
		CloudProvider:     cloudProvider(cfg.Get("cloudProvider")),
		LLMdChartPath:     cfg.Get("llmdChart"),
		ModelStorageClass: cfg.Get("modelStorageClass"),
	}

	// Read harborToken unconditionally — it is required for on-prem and also
	// needed for cloud stacks that use modelSource.harborRef. TrySecret returns
	// an error when the key is absent; in that case we leave HarborTokenSet false
	// and validateStack() enforces the requirement for on-prem.
	if token, err := cfg.TrySecret("harborToken"); err == nil {
		input.HarborToken = token
		input.HarborTokenSet = true
	}

	switch input.CloudProvider {
	case onPrem, cloud:
		// provider-specific validation is handled by validateStack()
	default:
		return stackInput{}, fmt.Errorf("invalid cloudProvider %q", input.CloudProvider)
	}

	if err := cfg.GetObject("models", &input.Models); err != nil {
		return stackInput{}, fmt.Errorf("invalid models config: %w", err)
	}

	// Use TryObject (not GetObject): GetObject returns a nil error when the key
	// is absent, which would set GPUToleration to a pointer to an empty
	// Toleration{} and emit an invalid pod toleration ({key:"", operator:""}).
	// TryObject errors on an absent key, so an unset gpuToleration stays nil
	// ("nil = no toleration injected"), mirroring the harborToken handling above.
	var tol Toleration
	if err := cfg.TryObject("gpuToleration", &tol); err == nil {
		input.GPUToleration = &tol
	}

	if err := input.validateStack(); err != nil {
		return stackInput{}, err
	}

	return input, nil
}
