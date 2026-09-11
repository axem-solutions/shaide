package config

import (
	"strings"

	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const (
	Namespace = "app-serving"
	StackName = "serving"
)

const (
	KeyPlatform          stackconfig.Key = "platform"
	KeyKubeconfig        stackconfig.Key = "kubeconfig"
	KeyContext           stackconfig.Key = "context"
	KeyModels            stackconfig.Key = "models"
	KeyLLMdChart         stackconfig.Key = "llmdChart"
	KeyHarborHostname    stackconfig.Key = "harborHostname"
	KeyHarborUser        stackconfig.Key = "harborUser"
	KeyHarborToken       stackconfig.Key = "harborToken"
	KeyGPUToleration     stackconfig.Key = "gpuToleration"
	KeyModelStorageClass stackconfig.Key = "modelStorageClass"
)

const legacyKeyCloudProvider = "cloudProvider"

// Sources are serving-specific values already known by the installer. Nil
// sources are deliberately omitted so hand-managed Pulumi stack values remain
// intact when the installer updates the stack.
type Sources struct {
	// Models is the selection to serve. Empty leaves the key unwritten so a
	// hand-managed stack value survives, which is how the models arrived
	// before the installer could supply them.
	Models ModelsInput

	HarborHostname    string
	HarborUser        string
	HarborToken       string
	ModelStorageClass string
}

type Config struct {
	stack.Config
	definition stackconfig.Config[Values]
}

func New(projectDir string, opts stack.Options, sources Sources) Config {
	definition := newDefinition(opts, sources)

	return Config{
		Config:     stack.NewConfig(Namespace, StackName, projectDir, definition),
		definition: definition,
	}
}

func newDefinition(opts stack.Options, sources Sources) stackconfig.Config[Values] {
	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries: []stackconfig.Entry[Values]{
			{
				Key: KeyPlatform,
				Source: stackconfig.Source{
					Value: string(opts.Platform),
				},
				Policy: stackconfig.Policy{Required: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Platform = loadPlatform(root)
				},
			},
			{
				Key: KeyKubeconfig,
				Source: stackconfig.Source{
					Value: opts.Kubeconfig,
				},
				Policy: stackconfig.Policy{Required: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Kubernetes.KubeconfigPath = root.Get(KeyKubeconfig.String())
				},
			},
			{
				Key: KeyContext,
				Source: stackconfig.Source{
					Value: opts.Context,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Kubernetes.Context = root.Get(KeyContext.String())
				},
			},
			{
				// Models are structured, deployment-specific configuration. The
				// installer supplies the selection it was given; an empty one is
				// not written, so a hand-managed stack value stays intact.
				Key:    KeyModels,
				Source: stackconfig.Source{Value: optionalModels(sources.Models)},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					root.RequireObject(KeyModels.String(), &cfg.Models)
				},
			},
			{
				Key: KeyLLMdChart,
				Source: stackconfig.Source{
					Default: DefaultLLMdChartPath,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.LLMd.ChartPath = root.Get(KeyLLMdChart.String())
				},
			},
			{
				Key: KeyHarborHostname,
				Source: stackconfig.Source{
					Value: optionalSource(sources.HarborHostname),
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Harbor.Hostname = root.Get(KeyHarborHostname.String())
				},
			},
			{
				Key: KeyHarborUser,
				Source: stackconfig.Source{
					Value: optionalSource(sources.HarborUser),
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Harbor.User = root.Get(KeyHarborUser.String())
				},
			},
			{
				Key: KeyHarborToken,
				Source: stackconfig.Source{
					Value: optionalSource(sources.HarborToken),
				},
				Policy: stackconfig.Policy{Secret: true},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					if token, err := root.TrySecret(KeyHarborToken.String()); err == nil {
						cfg.Harbor.Token = token
						cfg.Harbor.TokenSet = true
					}
				},
			},
			{
				Key: KeyGPUToleration,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					// TryObject keeps an omitted toleration nil. GetObject would
					// create an empty toleration that Kubernetes rejects.
					var toleration Toleration
					if err := root.TryObject(KeyGPUToleration.String(), &toleration); err == nil {
						cfg.Toleration = &toleration
					}
				},
			},
			{
				Key: KeyModelStorageClass,
				Source: stackconfig.Source{
					Value: optionalSource(sources.ModelStorageClass),
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.ModelStorageClass = root.Get(KeyModelStorageClass.String())
				},
			},
		},
	}
}

func optionalSource(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// loadPlatform keeps direct Pulumi CLI deployments compatible with the old
// cloudProvider key while installer-managed stacks use the shared four-value
// platform field. App-serving only distinguishes on-prem from cloud, so the
// legacy "cloud" value can safely map to any cloud platform.
func loadPlatform(root *pulumiconfig.Config) platform.Platform {
	if value := root.Get(KeyPlatform.String()); value != "" {
		return platform.Platform(value)
	}

	switch root.Get(legacyKeyCloudProvider) {
	case "on-prem":
		return platform.OnPrem
	case "cloud":
		return platform.GCP
	default:
		return ""
	}
}

// optionalModels omits an empty selection rather than writing an empty object,
// which RequireObject would accept and the program would then reject as
// "models must be non-empty" with no indication of where it came from.
func optionalModels(models ModelsInput) any {
	if len(models.Generative) == 0 && len(models.Embedder) == 0 {
		return nil
	}

	return models
}
