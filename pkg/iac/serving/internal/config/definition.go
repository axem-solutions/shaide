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
	HarborHostname    string
	HarborUser        string
	HarborToken       string
	ModelStorageClass string
}

type Config struct {
	stack.Config
	definition stackconfig.Config[stackInput]
	logf       Logf
}

func New(projectDir string, opts stack.Options, sources Sources, logf Logf) Config {
	definition := newDefinition(opts, sources)

	if logf == nil {
		logf = func(string, ...any) {}
	}

	return Config{
		Config:     stack.NewConfig(Namespace, StackName, projectDir, definition),
		definition: definition,
		logf:       logf,
	}
}

func newDefinition(opts stack.Options, sources Sources) stackconfig.Config[stackInput] {
	return stackconfig.Config[stackInput]{
		Namespace: Namespace,
		Entries: []stackconfig.Entry[stackInput]{
			{
				Key: KeyPlatform,
				Source: stackconfig.Source{
					Value: string(opts.Platform),
				},
				Policy: stackconfig.Policy{Required: true},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					cfg.Platform = loadPlatform(root)
				},
			},
			{
				Key: KeyKubeconfig,
				Source: stackconfig.Source{
					Value: opts.Kubeconfig,
				},
				Policy: stackconfig.Policy{Required: true},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					cfg.Kubernetes.KubeconfigPath = root.Get(KeyKubeconfig.String())
				},
			},
			{
				Key: KeyContext,
				Source: stackconfig.Source{
					Value: opts.Context,
				},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					cfg.Kubernetes.Context = root.Get(KeyContext.String())
				},
			},
			{
				// Models are structured, deployment-specific configuration. The
				// installer leaves this key untouched and the Pulumi program checks
				// that it is present when loading the stack.
				Key: KeyModels,
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					root.RequireObject(KeyModels.String(), &cfg.Models)
				},
			},
			{
				Key: KeyLLMdChart,
				Source: stackconfig.Source{
					Default: DefaultLLMdChartPath,
				},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					cfg.LLMdChartPath = root.Get(KeyLLMdChart.String())
				},
			},
			{
				Key: KeyHarborHostname,
				Source: stackconfig.Source{
					Value: optionalSource(sources.HarborHostname),
				},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					cfg.HarborHostname = root.Get(KeyHarborHostname.String())
				},
			},
			{
				Key: KeyHarborUser,
				Source: stackconfig.Source{
					Value: optionalSource(sources.HarborUser),
				},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					cfg.HarborUser = root.Get(KeyHarborUser.String())
				},
			},
			{
				Key: KeyHarborToken,
				Source: stackconfig.Source{
					Value: optionalSource(sources.HarborToken),
				},
				Policy: stackconfig.Policy{Secret: true},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					if token, err := root.TrySecret(KeyHarborToken.String()); err == nil {
						cfg.HarborToken = token
						cfg.HarborTokenSet = true
					}
				},
			},
			{
				Key: KeyGPUToleration,
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
					// TryObject keeps an omitted toleration nil. GetObject would
					// create an empty toleration that Kubernetes rejects.
					var toleration Toleration
					if err := root.TryObject(KeyGPUToleration.String(), &toleration); err == nil {
						cfg.GPUToleration = &toleration
					}
				},
			},
			{
				Key: KeyModelStorageClass,
				Source: stackconfig.Source{
					Value: optionalSource(sources.ModelStorageClass),
				},
				Setter: func(cfg *stackInput, root *pulumiconfig.Config) {
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
