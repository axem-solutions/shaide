package config

import (
	"github.com/axem-solutions/ai_platform/pkg/kube/platform"
	"github.com/axem-solutions/ai_platform/pkg/stack"
	stackconfig "github.com/axem-solutions/ai_platform/pkg/stack/config"
	pulumiconfig "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const Namespace = "monitoring"

const (
	ComponentLoki       = "loki"
	ComponentGrafana    = "grafana"
	ComponentAlloy      = "alloy"
	ComponentDashboards = "dashboards"
	ComponentPrometheus = "prometheus"
)

var (
	componentOptions = []string{
		ComponentLoki,
		ComponentGrafana,
		ComponentAlloy,
		ComponentDashboards,
		ComponentPrometheus,
	}
	supportedComponents = map[string]struct{}{
		ComponentLoki:       {},
		ComponentGrafana:    {},
		ComponentAlloy:      {},
		ComponentDashboards: {},
		ComponentPrometheus: {},
	}
)

const (
	KeyPlatform               stackconfig.Key = "platform"
	KeyKubeconfig             stackconfig.Key = "kubeconfig"
	KeyContext                stackconfig.Key = "context"
	KeyComponents             stackconfig.Key = "components"
	KeyNamespace              stackconfig.Key = "namespace"
	KeyS3Endpoint             stackconfig.Key = "s3Endpoint"
	KeyS3User                 stackconfig.Key = "s3User"
	KeyS3Password             stackconfig.Key = "s3Password"
	KeyS3BucketLoki           stackconfig.Key = "s3BucketLoki"
	KeyS3ClientImage          stackconfig.Key = "s3ClientImage"
	KeyLokiVersion            stackconfig.Key = "lokiVersion"
	KeyLokiChartPath          stackconfig.Key = "lokiChartPath"
	KeyLokiStorageClass       stackconfig.Key = "lokiStorageClass"
	KeyGrafanaVersion         stackconfig.Key = "grafanaVersion"
	KeyGrafanaChartPath       stackconfig.Key = "grafanaChartPath"
	KeyGrafanaAdminPassword   stackconfig.Key = "grafanaAdminPassword"
	KeyAlloyVersion           stackconfig.Key = "alloyVersion"
	KeyAlloyChartPath         stackconfig.Key = "alloyChartPath"
	KeyPrometheusVersion      stackconfig.Key = "prometheusVersion"
	KeyPrometheusChartPath    stackconfig.Key = "prometheusChartPath"
	KeyPrometheusStorageClass stackconfig.Key = "prometheusStorageClass"
)

type Config struct {
	stack.Config
	definition stackconfig.Config[Values]
}

func New(projectDir string, opts stack.Options) Config {
	definition := newDefinition(opts)

	return Config{
		Config:     stack.NewConfig(Namespace, Namespace, projectDir, definition),
		definition: definition,
	}
}

func newDefinition(opts stack.Options) stackconfig.Config[Values] {
	return stackconfig.Config[Values]{
		Namespace: Namespace,
		Entries: []stackconfig.Entry[Values]{
			{
				Key: KeyPlatform,
				Source: stackconfig.Source{
					Value: string(opts.Platform),
				},
				Policy: stackconfig.Policy{
					Required: true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Platform = platform.Platform(root.Get(KeyPlatform.String()))
				},
			},
			{
				Key: KeyKubeconfig,
				Source: stackconfig.Source{
					Value: opts.Kubeconfig,
				},
				Policy: stackconfig.Policy{
					Required: true,
				},
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
				Key: KeyComponents,
				Prompt: &stackconfig.Prompt{
					Kind:    stackconfig.PromptMultiSelect,
					Title:   "Monitoring components",
					Options: componentOptions,
				},
				Policy: stackconfig.Policy{
					Required: true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					var components []string
					root.RequireObject(KeyComponents.String(), &components)

					cfg.Components = make(map[string]bool, len(components))
					for _, component := range components {
						cfg.Components[component] = true
					}
				},
			},
			{
				Key: KeyNamespace,
				Source: stackconfig.Source{
					Default: DefaultNamespace,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Namespace = root.Get(KeyNamespace.String())
				},
			},
			{
				Key: KeyS3Endpoint,
				Source: stackconfig.Source{
					Default: DefaultS3Endpoint,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.S3.Endpoint = root.Get(KeyS3Endpoint.String())
				},
			},
			{
				Key: KeyS3User,
				Source: stackconfig.Source{
					Default: DefaultS3User,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.S3.User = root.Get(KeyS3User.String())
				},
			},
			{
				Key: KeyS3Password,
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "S3 password",
				},
				Policy: stackconfig.Policy{
					Required: true,
					Secret:   true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.S3.Password = root.RequireSecret(KeyS3Password.String())
				},
			},
			{
				Key: KeyS3BucketLoki,
				Source: stackconfig.Source{
					Default: DefaultS3BucketLoki,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Loki.S3Bucket = root.Get(KeyS3BucketLoki.String())
				},
			},
			{
				Key: KeyS3ClientImage,
				Source: stackconfig.Source{
					Default: DefaultS3ClientImage,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Loki.S3ClientImage = root.Get(KeyS3ClientImage.String())
				},
			},
			{
				Key: KeyLokiVersion,
				Source: stackconfig.Source{
					Default: DefaultLokiVersion,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Loki.Version = root.Get(KeyLokiVersion.String())
				},
			},
			{
				Key: KeyLokiChartPath,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Loki.ChartPath = root.Get(KeyLokiChartPath.String())
				},
			},
			{
				Key: KeyLokiStorageClass,
				Source: stackconfig.Source{
					Default: DefaultStorageClass,
				},
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "Loki storage class",
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Loki.StorageClass = root.Get(KeyLokiStorageClass.String())
				},
			},
			{
				Key: KeyGrafanaVersion,
				Source: stackconfig.Source{
					Default: DefaultGrafanaVersion,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Grafana.Version = root.Get(KeyGrafanaVersion.String())
				},
			},
			{
				Key: KeyGrafanaChartPath,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Grafana.ChartPath = root.Get(KeyGrafanaChartPath.String())
				},
			},
			{
				Key: KeyGrafanaAdminPassword,
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "Grafana admin password",
				},
				Policy: stackconfig.Policy{
					Required: true,
					Secret:   true,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Grafana.AdminPassword = root.RequireSecret(KeyGrafanaAdminPassword.String())
				},
			},
			{
				Key: KeyAlloyVersion,
				Source: stackconfig.Source{
					Default: DefaultAlloyVersion,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Alloy.Version = root.Get(KeyAlloyVersion.String())
				},
			},
			{
				Key: KeyAlloyChartPath,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Alloy.ChartPath = root.Get(KeyAlloyChartPath.String())
				},
			},
			{
				Key: KeyPrometheusVersion,
				Source: stackconfig.Source{
					Default: DefaultPrometheusVersion,
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Prometheus.Version = root.Get(KeyPrometheusVersion.String())
				},
			},
			{
				Key: KeyPrometheusChartPath,
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Prometheus.ChartPath = root.Get(KeyPrometheusChartPath.String())
				},
			},
			{
				Key: KeyPrometheusStorageClass,
				Source: stackconfig.Source{
					Default: DefaultStorageClass,
				},
				Prompt: &stackconfig.Prompt{
					Kind:  stackconfig.PromptInput,
					Title: "Prometheus storage class",
				},
				Setter: func(cfg *Values, root *pulumiconfig.Config) {
					cfg.Prometheus.StorageClass = root.Get(KeyPrometheusStorageClass.String())
				},
			},
		},
	}
}
