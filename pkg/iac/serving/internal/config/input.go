package config

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

// PlacementTerm admits the nodes whose Key label is one of Values, e.g. the
// node pools a model may run on. A model's terms are alternatives: a node
// matching any of them qualifies.
type PlacementTerm struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

type ModelInput struct {
	Name         string            `json:"name"`
	NodeSelector map[string]string `json:"nodeSelector"`
	// Placement, when set, replaces NodeSelector.
	Placement   []PlacementTerm   `json:"placement,omitempty"`
	Enabled     bool              `json:"enabled"`
	RelaseName  string            `json:"releaseName"`
	NameSpace   string            `json:"nameSpace"`
	ModelSource *ModelSourceInput `json:"modelSource"`
}

type ModelsInput struct {
	Generative []ModelInput `json:"generative"`
	Embedder   []ModelInput `json:"embedder"`
}
